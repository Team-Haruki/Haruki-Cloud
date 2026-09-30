package sk

import (
	"context"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
	"sort"
	"time"

	"haruki-cloud/internal/cachepersist"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
)

const forecastCachePersistenceVersion = 1

type persistedForecastDataCache struct {
	Version int                               `json:"version"`
	Entries []persistedForecastDataCacheEntry `json:"entries"`
}

type persistedForecastDataCacheEntry struct {
	Key         forecastDataCacheKey          `json:"key"`
	Data        map[string]ForecastSourceData `json:"data"`
	RefreshedAt int64                         `json:"refreshed_at"`
}

func (c *forecastDataCache) loadPersisted() {
	if c == nil || c.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(storage.WithBackgroundIO(context.Background()), 10*time.Second)
	defer cancel()
	_ = c.loadPersistedContext(ctx, c.storeKey, false)
}

func (c *forecastDataCache) loadPersistedContext(ctx context.Context, key storage.Key, replace bool) error {
	data, err := c.store.Get(ctx, key)
	if err != nil {
		return err
	}
	var persisted persistedForecastDataCache
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}
	if persisted.Version != forecastCachePersistenceVersion {
		return fmt.Errorf("unsupported cache snapshot version %d", persisted.Version)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if replace {
		c.entries = make(map[forecastDataCacheKey]*forecastDataCacheEntry)
	}
	for _, item := range persisted.Entries {
		key, ok := item.Key.normalized()
		if !ok || lenNonEmptyForecastData(item.Data) == 0 {
			continue
		}
		entry := &forecastDataCacheEntry{
			data:        cloneForecastSourceDataMap(item.Data),
			refreshedAt: time.UnixMilli(item.RefreshedAt).UTC(),
		}
		if item.RefreshedAt <= 0 {
			entry.refreshedAt = time.Time{}
		}
		c.entries[key] = entry
	}
	c.pruneLocked(time.Now().UTC())
	return nil
}

func (c *forecastDataCache) snapshotForPersistenceLocked() persistedForecastDataCache {
	out := persistedForecastDataCache{
		Version: forecastCachePersistenceVersion,
		Entries: make([]persistedForecastDataCacheEntry, 0, len(c.entries)),
	}
	for key, entry := range c.entries {
		if entry == nil || lenNonEmptyForecastData(entry.data) == 0 {
			continue
		}
		refreshedAt := int64(0)
		if !entry.refreshedAt.IsZero() {
			refreshedAt = entry.refreshedAt.UTC().UnixMilli()
		}
		out.Entries = append(out.Entries, persistedForecastDataCacheEntry{
			Key:         key,
			Data:        cloneForecastSourceDataMap(entry.data),
			RefreshedAt: refreshedAt,
		})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		left := out.Entries[i].Key
		right := out.Entries[j].Key
		if left.Region != right.Region {
			return left.Region < right.Region
		}
		if left.EventID != right.EventID {
			return left.EventID < right.EventID
		}
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		return left.WlCharacterID < right.WlCharacterID
	})
	return out
}

func (c *forecastDataCache) persistLatest(_ context.Context, requestedGeneration uint64) {
	if c == nil || c.store == nil {
		return
	}
	c.persistenceMu.Lock()
	if c.persistenceDisabled {
		c.persistenceMu.Unlock()
		return
	}
	if c.persistence == nil {
		c.persistence = cachepersist.New("forecast_cache", c.persistSnapshot, cachepersist.Options{})
	}
	writer := c.persistence
	c.persistenceMu.Unlock()
	writer.Schedule(requestedGeneration)
}

func (c *forecastDataCache) persistSnapshot(ctx context.Context) (uint64, error) {
	c.persistMu.Lock()
	defer c.persistMu.Unlock()

	finishSnapshot := commandtrace.MeasureOperation(ctx, "forecast_cache.snapshot")
	c.mu.Lock()
	c.pruneLocked(time.Now().UTC())
	persisted := c.snapshotForPersistenceLocked()
	generation := c.generation
	c.mu.Unlock()
	finishSnapshot()

	finishEncode := commandtrace.MeasureOperation(ctx, "forecast_cache.encode")
	payload, err := json.Marshal(persisted)
	finishEncode()
	if err != nil {
		return 0, err
	}
	finishPersist := commandtrace.MeasureOperation(ctx, "forecast_cache.persist")
	err = c.store.Put(ctx, c.storeKey, payload, storage.PutOptions{ContentType: "application/json"})
	finishPersist()
	if err == nil {
		c.persistedGeneration = generation
	}
	return generation, err
}
