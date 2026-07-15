package sk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
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
	if c == nil || c.persistencePath == "" {
		return
	}
	data, err := os.ReadFile(c.persistencePath)
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
	if c == nil || c.persistencePath == "" {
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
	err = writeForecastCachePayload(c.persistencePath, payload)
	finishPersist()
	if err == nil {
		c.persistedGeneration = generation
	}
}

func writeForecastCachePayload(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}
