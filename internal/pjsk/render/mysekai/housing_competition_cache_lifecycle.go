package mysekai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"haruki-cloud/internal/cachepersist"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"
)

// ConfigureCachePersistence must run before warmup and serving requests. The
// namespace must be stable across restarts and unique among concurrent writers.
func (c *Controller) ConfigureCachePersistence(ctx context.Context, namespace string) error {
	if c == nil || c.housingCompetitionStats == nil {
		return nil
	}
	return c.housingCompetitionStats.configurePersistence(ctx, namespace)
}

func (c *housingCompetitionStatsCache) configurePersistence(ctx context.Context, namespace string) error {
	if c == nil {
		return nil
	}
	if err := c.refreshTasks.Configure(ctx); err != nil {
		return err
	}
	if c.store == nil {
		return nil
	}

	key, err := cachepersist.NamespacedKey(c.storeKey, namespace)
	if err != nil {
		c.persistenceMu.Lock()
		c.persistenceDisabled = true
		c.persistenceMu.Unlock()
		logger.WarnContext(ctx, "Cache persistence disabled: invalid instance namespace", slog.String("cache", "mysekai"))
		// Read-only: the shared snapshot still warms the cache, as it did when
		// the constructor read it.
		c.loadLegacySnapshot(ctx)
		return err
	}
	c.persistenceMu.Lock()
	defer c.persistenceMu.Unlock()
	c.mu.Lock()
	busy := c.generation != 0
	c.mu.Unlock()
	if c.persistence != nil || busy {
		return errors.New("cache persistence must be configured before refresh")
	}
	c.legacyKey = c.storeKey
	c.storeKey = key
	// A readable namespaced object takes precedence; a missing one falls back
	// to the pre-namespace shared object as the migration snapshot, so the
	// common case is a single store read. Any other read error leaves
	// persistence disabled: the namespaced snapshot may exist unread, and a
	// later write must not replace it with older data.
	err = c.readSnapshot(ctx, key, true)
	switch {
	case err == nil:
		c.persistenceDisabled = false
	case errors.Is(err, storage.ErrNotExist):
		c.persistenceDisabled = false
		c.loadLegacySnapshot(ctx)
	default:
		c.persistenceDisabled = true
		logger.WarnContext(ctx, "Cache snapshot namespace read failed; persistence stays disabled", slog.String("cache", "mysekai"), slog.String("error_type", fmt.Sprintf("%T", err)))
	}
	return nil
}

// loadLegacySnapshot reads the pre-namespace shared object once, merging it
// into whatever the cache holds. It is the fallback when the namespaced
// object cannot be read and the warm start of a cache that stays read-only.
func (c *housingCompetitionStatsCache) loadLegacySnapshot(ctx context.Context) {
	if c.legacyLoaded || c.store == nil {
		return
	}
	c.legacyLoaded = true
	key := c.legacyKey
	if key == "" {
		key = c.storeKey
	}
	_ = c.readSnapshot(ctx, key, false)
}

// readSnapshot is one bounded startup read of a persisted snapshot on the
// background I/O budget.
func (c *housingCompetitionStatsCache) readSnapshot(ctx context.Context, key storage.Key, replace bool) error {
	readCtx, cancel := context.WithTimeout(storage.WithBackgroundIO(ctx), 10*time.Second)
	defer cancel()
	return c.loadPersistedContext(readCtx, key, replace)
}

func (c *Controller) FlushCachePersistence(ctx context.Context) error {
	if c == nil {
		return nil
	}
	return c.housingCompetitionStats.flushPersistence(ctx)
}

func (c *Controller) CloseCachePersistence(ctx context.Context) error {
	if c == nil {
		return nil
	}
	return c.housingCompetitionStats.closePersistence(ctx)
}

func (c *housingCompetitionStatsCache) flushPersistence(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.persistenceMu.Lock()
	writer := c.persistence
	c.persistenceMu.Unlock()
	return writer.Flush(ctx)
}

func (c *housingCompetitionStatsCache) closePersistence(ctx context.Context) error {
	if c == nil {
		return nil
	}
	drainErr := c.refreshTasks.Close(ctx)
	c.persistenceMu.Lock()
	writer := c.persistence
	c.persistenceMu.Unlock()
	return errors.Join(drainErr, writer.Close(ctx))
}
