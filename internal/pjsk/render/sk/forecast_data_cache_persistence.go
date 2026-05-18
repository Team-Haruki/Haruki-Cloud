package sk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
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
	return out
}

func writeForecastCacheFile(path string, cache persistedForecastDataCache) error {
	if path == "" {
		return nil
	}
	payload, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
