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
	c.storeKey = key
	c.persistenceDisabled = false
	// Constructors loaded the old shared key. A readable namespaced object takes
	// precedence; only a missing object uses that legacy migration snapshot.
	readCtx, cancel := context.WithTimeout(storage.WithBackgroundIO(ctx), 10*time.Second)
	err = c.loadPersistedContext(readCtx, key, true)
	cancel()
	if err != nil && !errors.Is(err, storage.ErrNotExist) {
		logger.WarnContext(ctx, "Cache snapshot namespace read failed", slog.String("cache", "mysekai"), slog.String("error_type", fmt.Sprintf("%T", err)))
	}

	return nil
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
