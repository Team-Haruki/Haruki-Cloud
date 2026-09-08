package drawingcache

import (
	"context"
	"fmt"
	"time"
)

const cacheTouchInterval = time.Second
const maxPendingCacheTouches = 4096

type pendingCacheTouch struct {
	record      CacheRecord
	persistedAt time.Time
}

// Called under lifecycleMu, shared by HTTP metadata operations and service GC.
func (a *API) applyPendingTouch(record *CacheRecord) {
	pending, ok := a.pendingTouches[record.Sha256Key]
	if !ok {
		return
	}
	if pending.record.FilePath != record.FilePath || !pending.record.CreatedAt.Equal(record.CreatedAt) || pending.record.TTLSeconds != record.TTLSeconds {
		delete(a.pendingTouches, record.Sha256Key)
		return
	}
	record.LastUsedAt = pending.record.LastUsedAt
	record.ExpiresAt = pending.record.ExpiresAt
}

func (a *API) touchRecord(record *CacheRecord, now time.Time) error {
	if now.Before(record.LastUsedAt) {
		now = record.LastUsedAt
	}
	persistedAt := record.LastUsedAt
	if pending, ok := a.pendingTouches[record.Sha256Key]; ok {
		persistedAt = pending.persistedAt
	}
	interval := cacheTouchInterval
	if record.TTLSeconds > 0 {
		interval = min(interval, time.Duration(record.TTLSeconds)*time.Second/4)
	}
	if now.Sub(persistedAt) >= interval {
		if err := a.persistTouch(record, now); err != nil {
			return err
		}
		delete(a.pendingTouches, record.Sha256Key)
		return nil
	}
	if len(a.pendingTouches) >= maxPendingCacheTouches {
		if err := a.flushTouchesLocked(); err != nil {
			return err
		}
	}
	if a.pendingTouches == nil {
		a.pendingTouches = make(map[string]pendingCacheTouch)
	}
	updated := *record
	updated.LastUsedAt, updated.ExpiresAt = now, cacheExpiresAt(now, record.TTLSeconds)
	a.pendingTouches[record.Sha256Key] = pendingCacheTouch{record: updated, persistedAt: persistedAt}
	return nil
}

func (a *API) flushTouchesLocked() error {
	if len(a.pendingTouches) == 0 {
		return nil
	}
	tx, err := a.dao.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	finite, err := tx.Prepare(`UPDATE image_cache_index SET last_used_at = ?, expires_at = ? WHERE sha256_key = ? AND file_path = ? AND created_at = ? AND ttl_seconds = ?`)
	if err != nil {
		return err
	}
	defer finite.Close()
	permanent, err := tx.Prepare(`UPDATE image_cache_index SET last_used_at = ? WHERE sha256_key = ? AND file_path = ? AND created_at = ? AND ttl_seconds = ?`)
	if err != nil {
		return err
	}
	defer permanent.Close()
	for _, pending := range a.pendingTouches {
		r := pending.record
		args := []any{r.LastUsedAt.UTC().Format(sqliteTimeLayout)}
		statement := permanent
		if !isInfiniteTTL(r.TTLSeconds) {
			statement = finite
			args = append(args, r.ExpiresAt.UTC().Format(sqliteTimeLayout))
		}
		args = append(args, r.Sha256Key, r.FilePath, r.CreatedAt.UTC().Format(sqliteTimeLayout), r.TTLSeconds)
		if _, err := statement.Exec(args...); err != nil {
			return fmt.Errorf("flush cache touch: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	clear(a.pendingTouches)
	return nil
}

func (a *API) flushTouches() error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	return a.flushTouchesLocked()
}

func (a *API) cleanupExpired(limit int) (int, error) {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	// GC must see the latest successful hits before selecting or deleting rows.
	if err := a.flushTouchesLocked(); err != nil {
		return 0, err
	}
	return cleanupExpiredBatch(a.dao.db, a.storageDir, limit)
}

func (a *API) maintain(ctx context.Context, interval time.Duration) {
	flush := time.NewTicker(cacheTouchInterval)
	defer flush.Stop()
	var gc <-chan time.Time
	if interval > 0 {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		gc = ticker.C
	}
	cleanup := func() {
		for ctx.Err() == nil {
			n, err := a.cleanupExpired(defaultGCBatchSize)
			if err != nil {
				drawingCacheLogger.WarnContext(ctx, "cache GC cleanup failed", "error_type", drawingCacheErrorType(err))
				return
			}
			if n < defaultGCBatchSize {
				return
			}
		}
	}
	if interval > 0 {
		cleanup()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-flush.C:
			if err := a.flushTouches(); err != nil {
				drawingCacheLogger.WarnContext(ctx, "cache touch flush failed", "error_type", drawingCacheErrorType(err))
			}
		case <-gc:
			cleanup()
		}
	}
}

func (a *API) persistTouch(record *CacheRecord, now time.Time) error {
	if !isInfiniteTTL(record.TTLSeconds) {
		return a.dao.TouchRecordOnHit(record.Sha256Key, now, record.TTLSeconds)
	}
	result, err := a.dao.db.Exec(`UPDATE image_cache_index SET last_used_at = ? WHERE sha256_key = ?`, now.UTC().Format(sqliteTimeLayout), record.Sha256Key)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrRecordNotFound
	}
	return nil
}
