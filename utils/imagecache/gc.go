package imagecache

import (
	"context"
	"errors"
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
	// maxGCPendingObjectDeletes bounds the in-memory retry list; the oldest
	// entries are dropped first.
	maxGCPendingObjectDeletes = 10000
	gcSampleKeys              = 10
)

// GCConfig is the image_cache.gc_* block. Zero Interval, Batch and
// ObjectRetentionDays select the defaults.
type GCConfig struct {
	Enabled             bool
	DryRun              bool
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
	DryRun               bool
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
	// pending holds object deletes still owed after their row was deleted.
	pending []storage.Key
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
	report := GCReport{DryRun: g.cfg.DryRun}
	now := g.now()
	g.retryPending(ctx, &report)
	err := errors.Join(g.collectRenderRows(ctx, now, &report), g.collectEntries(ctx, now, &report))
	report.PendingObjectDeletes = len(g.pending)
	args := []any{
		"dry_run", report.DryRun,
		"expired_render_rows", report.ExpiredRenderRows, "deleted_render_rows", report.DeletedRenderRows,
		"orphan_entries", report.OrphanEntries, "deleted_entries", report.DeletedEntries,
		"deleted_objects", report.DeletedObjects, "object_leaks", report.ObjectLeaks,
		"retried_object_deletes", report.RetriedObjectDeletes, "pending_object_deletes", report.PendingObjectDeletes,
	}
	if err != nil {
		g.log.ErrorContext(ctx, "image cache gc cycle failed", append(args, "error", err)...)
		return report, err
	}
	g.log.InfoContext(ctx, "image cache gc cycle", args...)
	return report, nil
}

func (g *GC) retryPending(ctx context.Context, report *GCReport) {
	if len(g.pending) == 0 {
		return
	}
	kept := g.pending[:0]
	for _, key := range g.pending {
		if err := g.objects.Delete(ctx, key); err != nil {
			kept = append(kept, key)
			continue
		}
		report.RetriedObjectDeletes++
	}
	clear(g.pending[len(kept):])
	g.pending = kept
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
	deleted, err := g.index.DeleteRender(ctx, keys)
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
	if g.cfg.DryRun {
		sample := make([]string, 0, min(len(entries), gcSampleKeys))
		for _, entry := range entries[:min(len(entries), gcSampleKeys)] {
			sample = append(sample, entry.CDNPath)
		}
		g.log.InfoContext(ctx, "image cache gc dry run: orphan garage entries",
			"count", len(entries), "retention_cutoff", cutoff, "sample", sample)
		return nil
	}
	var errs []error
	for _, entry := range entries {
		if err := g.collectEntry(ctx, entry, report); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// collectEntry deletes the row first, then the recorded cdn_path verbatim. A
// failed object delete after the row is gone is a leak retried next cycle.
func (g *GC) collectEntry(ctx context.Context, entry ImageEntry, report *GCReport) error {
	deleted, err := g.index.DeleteEntries(ctx, []string{entry.Hash})
	if err != nil {
		return err
	}
	if deleted == 0 {
		return nil
	}
	report.DeletedEntries += int(deleted)
	key, err := storage.CleanKey(entry.CDNPath)
	if err != nil {
		report.ObjectLeaks++
		g.log.WarnContext(ctx, "image cache gc leaked an object with an invalid cdn_path",
			"cdn_path", entry.CDNPath, "error", err)
		return nil
	}
	if err := g.objects.Delete(ctx, key); err != nil {
		report.ObjectLeaks++
		g.addPending(ctx, key)
		g.log.WarnContext(ctx, "image cache gc object delete failed; retrying next cycle",
			"cdn_path", entry.CDNPath, "error", err)
		return nil
	}
	report.DeletedObjects++
	return nil
}

func (g *GC) addPending(ctx context.Context, key storage.Key) {
	if len(g.pending) >= maxGCPendingObjectDeletes {
		dropped := g.pending[0]
		g.pending = append(g.pending[:0], g.pending[1:]...)
		g.log.WarnContext(ctx, "image cache gc pending object deletes full; dropping the oldest",
			"cdn_path", string(dropped), "limit", maxGCPendingObjectDeletes)
	}
	g.pending = append(g.pending, key)
}

func sampleStrings(values []string) []string {
	return values[:min(len(values), gcSampleKeys)]
}
