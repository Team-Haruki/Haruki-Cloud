package sk

import (
	"context"
	"encoding/json"
	"sort"
	"time"

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
	data, err := c.store.Get(context.Background(), c.storeKey)
	if err != nil || len(data) == 0 {
		return
	}
	var persisted persistedForecastDataCache
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	if persisted.Version != forecastCachePersistenceVersion {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
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

func (c *forecastDataCache) persistLatest(ctx context.Context, requestedGeneration uint64) {
	if c == nil || c.store == nil {
		return
	}
	finishWait := commandtrace.MeasureOperation(ctx, "forecast_cache.persist_wait")
	c.persistMu.Lock()
	finishWait()
	defer c.persistMu.Unlock()
	if c.persistedGeneration >= requestedGeneration {
		return
	}

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
		return
	}
	finishPersist := commandtrace.MeasureOperation(ctx, "forecast_cache.persist")
	// Persistence outlives the refresh that triggered it, as the old file
	// write did: a cancelled caller must not drop the snapshot.
	err = c.store.Put(context.WithoutCancel(ctx), c.storeKey, payload, storage.PutOptions{ContentType: "application/json"})
	finishPersist()
	if err == nil {
		c.persistedGeneration = generation
	}
}
