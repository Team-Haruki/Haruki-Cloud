package imagecache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"
)

// GC defaults (addendum A6).
const (
	DefaultGCInterval            = time.Hour
	DefaultGCBatch               = 500
	DefaultGCObjectRetentionDays = 30
	gcSampleKeys                 = 10
)

// GCConfig is the image_cache.gc_* block. Zero Interval, Batch and
// ObjectRetentionDays select the defaults.
type GCConfig struct {
	Enabled bool
	DryRun  bool
	// Enable only after every content writer implements ContentLockSQL.
	ObjectDeleteEnabled bool
	Interval            time.Duration
	Batch               int
	ObjectRetentionDays int
}

func (c GCConfig) normalized() GCConfig {
	if c.Interval <= 0 {
		c.Interval = DefaultGCInterval
	}
	if c.Batch <= 0 {
		c.Batch = DefaultGCBatch
	}
	if c.ObjectRetentionDays <= 0 {
		c.ObjectRetentionDays = DefaultGCObjectRetentionDays
	}
	c.Batch = min(c.Batch, 1000)
	return c
}

// GCReport summarises one GC cycle.
type GCReport struct {
	ExpiredRenderRows    int
	DeletedRenderRows    int
	OrphanEntries        int
	DeletedEntries       int
	DeletedObjects       int
	ObjectLeaks          int
	RetriedObjectDeletes int
	PendingObjectDeletes int
	// SkippedLiveObjectDeletes counts object deletes dropped because a live
	// row records the same cdn_path again (the bytes were re-stored).
	SkippedLiveObjectDeletes int
	DryRun                   bool
}

// GC collects the render index and its garage objects. It deletes index rows
// first and objects second (C6): phase 1 drops expired render_cache_index
// rows, phase 2 drops unreferenced garage image_cache_entries rows older than
// the retention window and then their recorded cdn_path objects.
type GC struct {
	index   *PGStore
	objects storage.Store
	cfg     GCConfig
	log     *logger.Logger
	now     func() time.Time

	mu sync.Mutex
}

// pendingObjectDelete is an object delete owed for a collected row.
type pendingObjectDelete struct {
	Hash string
	Key  storage.Key
}

// NewGC returns nil when GC is disabled or the index is unavailable. A nil
// objects store is treated as Disabled.
func NewGC(index *PGStore, objects storage.Store, cfg GCConfig, log *logger.Logger) *GC {
	if !cfg.Enabled || index == nil {
		return nil
	}
	if objects == nil {
		objects = storage.Disabled()
	}
	if log == nil {
		log = logger.NewLoggerFromGlobal("ImageCacheGC")
	}
	return &GC{index: index, objects: objects, cfg: cfg.normalized(), log: log, now: time.Now}
}

// Start runs a cycle every Interval (the first after one Interval) until ctx
// is cancelled.
func (g *GC) Start(ctx context.Context) {
	if g == nil {
		return
	}
	ticker := time.NewTicker(g.cfg.Interval)
	g.log.InfoContext(ctx, "image cache gc started",
		"interval", g.cfg.Interval.String(), "dry_run", g.cfg.DryRun,
		"object_delete_enabled", g.cfg.ObjectDeleteEnabled,
		"batch", g.cfg.Batch, "object_retention_days", g.cfg.ObjectRetentionDays)
	go func() {
		defer ticker.Stop()
		g.loop(ctx, ticker.C)
	}()
}

func (g *GC) loop(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
			_, _ = g.RunOnce(ctx)
		}
	}
}

// RunOnce performs one cycle and logs one summary line. Dry-run performs the
// SELECTs only. Errors from each phase are joined; a failing phase does not
// stop the next one.
func (g *GC) RunOnce(ctx context.Context) (GCReport, error) {
	if g == nil {
		return GCReport{}, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	ctx = storage.WithBackgroundIO(ctx)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	report := GCReport{DryRun: g.cfg.DryRun}
	now := g.now()
	err := errors.Join(
		g.retryPending(ctx, &report),
		g.collectRenderRows(ctx, now, &report),
		g.collectEntries(ctx, now, &report),
	)
	if g.cfg.ObjectDeleteEnabled && !g.cfg.DryRun {
		var pending int
		countErr := g.index.executor().QueryRowContext(ctx, countObjectDeletesSQL).Scan(&pending)
		err = errors.Join(err, countErr)
		report.PendingObjectDeletes = pending
	}
	args := []any{
		"dry_run", report.DryRun,
		"expired_render_rows", report.ExpiredRenderRows, "deleted_render_rows", report.DeletedRenderRows,
		"orphan_entries", report.OrphanEntries, "deleted_entries", report.DeletedEntries,
		"deleted_objects", report.DeletedObjects, "object_leaks", report.ObjectLeaks,
		"retried_object_deletes", report.RetriedObjectDeletes, "pending_object_deletes", report.PendingObjectDeletes,
		"skipped_live_object_deletes", report.SkippedLiveObjectDeletes,
	}
	if err != nil {
		g.log.ErrorContext(ctx, "image cache gc cycle failed", append(args, "error", err)...)
		return report, err
	}
	g.log.InfoContext(ctx, "image cache gc cycle", args...)
	return report, nil
}

// retryPending reads a bounded durable batch. Failed work survives restarts;
// the next cycle does not re-scan the entire outstanding deletion queue.
func (g *GC) retryPending(ctx context.Context, report *GCReport) error {
	if g.cfg.DryRun || !g.cfg.ObjectDeleteEnabled {
		return nil
	}
	if g.objects == storage.Disabled() {
		return storage.ErrNotConfigured
	}
	if !g.index.lifecycle.Load() {
		return errors.New("imagecache: object deletion requires initialized content lock protocol")
	}
	pending, err := g.index.pendingObjects(ctx, g.now(), g.cfg.Batch)
	if err != nil {
		return err
	}
	var errs []error
	for _, owed := range pending {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := g.deletePending(ctx, owed, report, true); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deletePending runs only after the retirement transaction committed. A crash
// after Delete leaves an idempotent outbox retry, never a resurrected index row.
func (g *GC) deletePending(ctx context.Context, owed pendingObjectDelete, report *GCReport, retry bool) error {
	return g.index.WithContentLock(ctx, owed.Hash, func(ctx context.Context, index *PGStore) error {
		var one int
		err := index.executor().QueryRowContext(ctx, pendingObjectSQL, owed.Hash, string(owed.Key)).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		live, err := index.LiveEntryPaths(ctx, []string{owed.Hash})
		if err != nil {
			return err
		}
		if _, exists := live[liveEntryKey(owed.Hash, string(owed.Key))]; exists {
			_, err := index.executor().ExecContext(ctx, finishObjectDeleteSQL, owed.Hash, string(owed.Key))
			if err == nil {
				report.SkippedLiveObjectDeletes++
			}
			return err
		}
		key, err := storage.CleanKey(string(owed.Key))
		if err == nil {
			err = g.objects.Delete(ctx, key)
		}
		if err != nil && (errors.Is(err, storage.ErrNotConfigured) || !errors.Is(err, storage.ErrNotExist)) {
			report.ObjectLeaks++
			g.log.WarnContext(ctx, "image cache gc object delete failed; retained in durable queue", "error", err)
			_, err = index.executor().ExecContext(ctx, retryObjectDeleteSQL, owed.Hash, string(owed.Key))
			return err
		}
		if _, err := index.executor().ExecContext(ctx, finishObjectDeleteSQL, owed.Hash, string(owed.Key)); err != nil {
			return err
		}
		if retry {
			report.RetriedObjectDeletes++
		} else {
			report.DeletedObjects++
		}
		return nil
	})
}

func (g *GC) collectRenderRows(ctx context.Context, now time.Time, report *GCReport) error {
	keys, err := g.index.ExpiredRenderKeys(ctx, now, g.cfg.Batch)
	if err != nil {
		return err
	}
	report.ExpiredRenderRows = len(keys)
	if g.cfg.DryRun {
		g.log.InfoContext(ctx, "image cache gc dry run: expired render rows",
			"count", len(keys), "sample", sampleStrings(keys))
		return nil
	}
	deleted, err := g.index.DeleteExpiredRender(ctx, keys, now)
	report.DeletedRenderRows = int(deleted)
	return err
}

func (g *GC) collectEntries(ctx context.Context, now time.Time, report *GCReport) error {
	cutoff := now.Add(-time.Duration(g.cfg.ObjectRetentionDays) * 24 * time.Hour)
	entries, err := g.index.OrphanGarageEntries(ctx, cutoff, g.cfg.Batch)
	if err != nil {
		return err
	}
	report.OrphanEntries = len(entries)
	if g.cfg.DryRun || !g.cfg.ObjectDeleteEnabled {
		sample := make([]string, 0, min(len(entries), gcSampleKeys))
		for _, entry := range entries[:min(len(entries), gcSampleKeys)] {
			sample = append(sample, entry.CDNPath)
		}
		g.log.InfoContext(ctx, "image cache gc dry run: orphan garage entries",
			"count", len(entries), "retention_cutoff", cutoff, "sample", sample, "object_delete_enabled", g.cfg.ObjectDeleteEnabled)
		return nil
	}
	if g.objects == storage.Disabled() {
		return storage.ErrNotConfigured
	}
	if !g.index.lifecycle.Load() {
		return errors.New("imagecache: object deletion requires initialized content lock protocol")
	}
	var errs []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := g.collectEntry(ctx, entry, cutoff, report); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// collectEntry durably retires ownership first. A writer may acquire the same
// hash lock between these transactions; deletePending then keeps its object.
func (g *GC) collectEntry(ctx context.Context, entry ImageEntry, cutoff time.Time, report *GCReport) error {
	if g.objects == storage.Disabled() {
		return storage.ErrNotConfigured
	}
	if !g.cfg.ObjectDeleteEnabled || !g.index.lifecycle.Load() {
		return errors.New("imagecache: object deletion protocol not enabled")
	}
	var deleted int64
	err := g.index.WithContentLock(ctx, entry.Hash, func(ctx context.Context, index *PGStore) error {
		var err error
		deleted, err = index.retireObject(ctx, entry, cutoff)
		return err
	})
	if err != nil || deleted == 0 {
		return err
	}
	report.DeletedEntries += int(deleted)
	owed := pendingObjectDelete{Hash: entry.Hash, Key: storage.Key(entry.CDNPath)}
	if err := g.deletePending(ctx, owed, report, false); err != nil {
		return fmt.Errorf("imagecache: durable object delete: %w", err)
	}
	return nil
}

func sampleStrings(values []string) []string {
	return values[:min(len(values), gcSampleKeys)]
}
