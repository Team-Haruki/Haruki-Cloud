package mysekai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const housingCompetitionStatsCacheVersion = 1

type housingCompetitionStatsCache struct {
	mu              sync.Mutex
	path            string
	refreshInterval time.Duration
	buckets         map[housingCompetitionStatsCacheKey]*housingCompetitionStatsBucket
}

type housingCompetitionStatsCacheKey struct {
	Region    string `json:"region"`
	HousingID int    `json:"housing_id"`
}

type housingCompetitionStatsBucket struct {
	entries     map[string]HousingCompetitionEntry
	refreshedAt time.Time
	sampledAt   time.Time
}

type persistedHousingCompetitionStatsCache struct {
	Version int                                      `json:"version"`
	Buckets []persistedHousingCompetitionStatsBucket `json:"buckets"`
}

type persistedHousingCompetitionStatsBucket struct {
	Key         housingCompetitionStatsCacheKey    `json:"key"`
	RefreshedAt int64                              `json:"refreshed_at"`
	SampledAt   int64                              `json:"sampled_at"`
	Entries     []persistedHousingCompetitionEntry `json:"entries"`
}

type persistedHousingCompetitionEntry struct {
	CacheKey      string `json:"cache_key"`
	CompetitionID int    `json:"competition_id,omitempty"`
	OwnerUserName string `json:"owner_user_name,omitempty"`
	EntryName     string `json:"entry_name,omitempty"`
	EntryWord     string `json:"entry_word,omitempty"`
	ThumbnailPath string `json:"thumbnail_path,omitempty"`
	SubmittedAt   int64  `json:"submitted_at,omitempty"`
	ReviewCount   int    `json:"review_count"`
	TabType       string `json:"tab_type,omitempty"`
	LastSeenAt    int64  `json:"last_seen_at,omitempty"`
}

func newHousingCompetitionStatsCache(cachePath string, refreshInterval time.Duration) *housingCompetitionStatsCache {
	if refreshInterval <= 0 {
		refreshInterval = DefaultHousingCompetitionRefreshInterval
	}
	cache := &housingCompetitionStatsCache{
		path:            strings.TrimSpace(cachePath),
		refreshInterval: refreshInterval,
		buckets:         make(map[housingCompetitionStatsCacheKey]*housingCompetitionStatsBucket),
	}
	cache.loadPersisted()
	return cache
}

func (c *housingCompetitionStatsCache) RefreshInterval() time.Duration {
	if c == nil || c.refreshInterval <= 0 {
		return DefaultHousingCompetitionRefreshInterval
	}
	return c.refreshInterval
}

func (c *housingCompetitionStatsCache) GetOrRefresh(ctx context.Context, api HousingCompetitionListClient, region string, housingID, sampleCount int) ([]HousingCompetitionEntry, time.Time, int, error) {
	if c == nil {
		return fetchHousingCompetitionSamples(ctx, api, region, housingID, sampleCount, 0)
	}
	key, ok := newHousingCompetitionStatsCacheKey(region, housingID)
	if !ok {
		return nil, time.Time{}, 0, fmt.Errorf("invalid housing competition cache key")
	}
	now := time.Now().UTC()

	c.mu.Lock()
	if bucket := c.buckets[key]; bucket != nil && len(bucket.entries) > 0 && !shouldRefreshHousingCompetitionStats(bucket, now, c.RefreshInterval()) {
		entries, sampledAt := bucket.snapshot()
		c.mu.Unlock()
		return entries, sampledAt, 0, nil
	}
	staleEntries, staleSampledAt := c.snapshotLocked(key)
	c.mu.Unlock()

	entries, sampledAt, refreshedCount, err := c.Refresh(ctx, api, region, housingID, sampleCount)
	if err != nil {
		if len(staleEntries) > 0 {
			return staleEntries, staleSampledAt, 0, nil
		}
		return nil, time.Time{}, 0, err
	}
	return entries, sampledAt, refreshedCount, nil
}

func (c *housingCompetitionStatsCache) Refresh(ctx context.Context, api HousingCompetitionListClient, region string, housingID, sampleCount int) ([]HousingCompetitionEntry, time.Time, int, error) {
	if c == nil {
		return fetchHousingCompetitionSamples(ctx, api, region, housingID, sampleCount, 0)
	}
	key, ok := newHousingCompetitionStatsCacheKey(region, housingID)
	if !ok {
		return nil, time.Time{}, 0, fmt.Errorf("invalid housing competition cache key")
	}

	entries, sampledAt, refreshedCount, err := fetchHousingCompetitionSamples(ctx, api, region, housingID, sampleCount, 0)
	if err != nil {
		return nil, time.Time{}, 0, err
	}
	now := time.Now().UTC()

	c.mu.Lock()
	bucket := c.buckets[key]
	if bucket == nil {
		bucket = &housingCompetitionStatsBucket{entries: make(map[string]HousingCompetitionEntry)}
		c.buckets[key] = bucket
	}
	mergeHousingCompetitionEntries(bucket.entries, entries)
	bucket.refreshedAt = now
	if !sampledAt.IsZero() {
		bucket.sampledAt = sampledAt.UTC()
	} else {
		bucket.sampledAt = now
	}
	merged, mergedSampledAt := bucket.snapshot()
	persistPath := c.path
	persisted := c.snapshotForPersistenceLocked()
	c.mu.Unlock()

	if persistPath != "" {
		_ = writeHousingCompetitionStatsCacheFile(persistPath, persisted)
	}
	return merged, mergedSampledAt, refreshedCount, nil
}

func fetchHousingCompetitionSamples(ctx context.Context, api HousingCompetitionListClient, region string, housingID, sampleCount, sampleIntervalMillis int) ([]HousingCompetitionEntry, time.Time, int, error) {
	if api == nil {
		return nil, time.Time{}, 0, fmt.Errorf("sekai api client is not configured")
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = renderRegionDefaultString()
	}
	if housingID <= 0 {
		return nil, time.Time{}, 0, fmt.Errorf("invalid housing_id")
	}
	sampleCount = normalizeHousingCompetitionSampleCount(sampleCount)

	merged := make(map[string]HousingCompetitionEntry)
	var sampledAt time.Time
	for i := 0; i < sampleCount; i++ {
		if err := ctx.Err(); err != nil {
			return nil, time.Time{}, i, err
		}
		raw, err := api.GetMySekaiHousingCompetitionList(region, housingID, true)
		if err != nil {
			return nil, time.Time{}, i, err
		}
		entries, lotteryAt, err := parseHousingCompetitionEntries(raw)
		if err != nil {
			return nil, time.Time{}, i, err
		}
		if lotteryAt > 0 {
			current := time.UnixMilli(lotteryAt).UTC()
			if sampledAt.IsZero() || current.After(sampledAt) {
				sampledAt = current
			}
		}
		mergeHousingCompetitionEntries(merged, entries)
		if i+1 < sampleCount && sampleIntervalMillis > 0 {
			if err := waitHousingCompetitionSampleInterval(ctx, time.Duration(sampleIntervalMillis)*time.Millisecond); err != nil {
				return nil, time.Time{}, i + 1, err
			}
		}
	}
	if sampledAt.IsZero() {
		sampledAt = time.Now().UTC()
	}
	return housingCompetitionEntriesFromMap(merged), sampledAt, sampleCount, nil
}

func newHousingCompetitionStatsCacheKey(region string, housingID int) (housingCompetitionStatsCacheKey, bool) {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = renderRegionDefaultString()
	}
	if housingID <= 0 {
		return housingCompetitionStatsCacheKey{}, false
	}
	return housingCompetitionStatsCacheKey{
		Region:    region,
		HousingID: housingID,
	}, true
}

func renderRegionDefaultString() string {
	return "jp"
}

func shouldRefreshHousingCompetitionStats(bucket *housingCompetitionStatsBucket, now time.Time, interval time.Duration) bool {
	if bucket == nil || len(bucket.entries) == 0 {
		return true
	}
	if interval <= 0 {
		interval = DefaultHousingCompetitionRefreshInterval
	}
	if bucket.refreshedAt.IsZero() {
		return true
	}
	return now.Sub(bucket.refreshedAt) >= interval
}

func (b *housingCompetitionStatsBucket) snapshot() ([]HousingCompetitionEntry, time.Time) {
	if b == nil {
		return nil, time.Time{}
	}
	return housingCompetitionEntriesFromMap(b.entries), b.sampledAt
}

func (c *housingCompetitionStatsCache) snapshotLocked(key housingCompetitionStatsCacheKey) ([]HousingCompetitionEntry, time.Time) {
	if c == nil {
		return nil, time.Time{}
	}
	bucket := c.buckets[key]
	if bucket == nil || len(bucket.entries) == 0 {
		return nil, time.Time{}
	}
	return bucket.snapshot()
}

func mergeHousingCompetitionEntries(dst map[string]HousingCompetitionEntry, entries []HousingCompetitionEntry) {
	for _, entry := range entries {
		key := housingCompetitionEntryCacheKey(entry)
		if key == "" {
			continue
		}
		entry.CacheKey = key
		if entry.LastSeenAt <= 0 {
			entry.LastSeenAt = time.Now().UTC().UnixMilli()
		}
		current, ok := dst[key]
		if !ok {
			dst[key] = entry
			continue
		}
		dst[key] = mergeHousingCompetitionEntry(current, entry)
	}
}

func mergeHousingCompetitionEntry(current, next HousingCompetitionEntry) HousingCompetitionEntry {
	out := current
	if next.ReviewCount >= current.ReviewCount {
		out.ReviewCount = next.ReviewCount
	}
	if next.CompetitionID != 0 {
		out.CompetitionID = next.CompetitionID
	}
	if strings.TrimSpace(next.OwnerUserName) != "" {
		out.OwnerUserName = next.OwnerUserName
	}
	if strings.TrimSpace(next.EntryName) != "" {
		out.EntryName = next.EntryName
	}
	if strings.TrimSpace(next.EntryWord) != "" {
		out.EntryWord = next.EntryWord
	}
	if strings.TrimSpace(next.ThumbnailPath) != "" {
		out.ThumbnailPath = next.ThumbnailPath
	}
	if next.SubmittedAt > 0 {
		out.SubmittedAt = next.SubmittedAt
	}
	if strings.TrimSpace(next.TabType) != "" {
		out.TabType = next.TabType
	}
	if next.LastSeenAt > out.LastSeenAt {
		out.LastSeenAt = next.LastSeenAt
	}
	if next.OwnerUserID != 0 {
		out.OwnerUserID = next.OwnerUserID
	}
	if next.CacheKey != "" {
		out.CacheKey = next.CacheKey
	}
	return out
}

func housingCompetitionEntriesFromMap(entries map[string]HousingCompetitionEntry) []HousingCompetitionEntry {
	out := make([]HousingCompetitionEntry, 0, len(entries))
	for _, entry := range entries {
		entry.OwnerUserID = 0
		out = append(out, entry)
	}
	sortHousingCompetitionEntries(out)
	return out
}

func housingCompetitionEntryCacheKey(entry HousingCompetitionEntry) string {
	if entry.CacheKey != "" {
		return entry.CacheKey
	}
	parts := []string{
		fmt.Sprintf("%d", entry.CompetitionID),
		fmt.Sprintf("%d", entry.OwnerUserID),
		fmt.Sprintf("%d", entry.SubmittedAt),
		strings.TrimSpace(entry.ThumbnailPath),
		strings.TrimSpace(entry.EntryName),
	}
	raw := strings.Join(parts, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func waitHousingCompetitionSampleInterval(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return nil
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *housingCompetitionStatsCache) loadPersisted() {
	if c == nil || c.path == "" {
		return
	}
	data, err := os.ReadFile(c.path)
	if err != nil || len(data) == 0 {
		return
	}
	var persisted persistedHousingCompetitionStatsCache
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	if persisted.Version != housingCompetitionStatsCacheVersion {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, item := range persisted.Buckets {
		key, ok := newHousingCompetitionStatsCacheKey(item.Key.Region, item.Key.HousingID)
		if !ok {
			continue
		}
		bucket := &housingCompetitionStatsBucket{
			entries:     make(map[string]HousingCompetitionEntry, len(item.Entries)),
			refreshedAt: timeFromUnixMilli(item.RefreshedAt),
			sampledAt:   timeFromUnixMilli(item.SampledAt),
		}
		for _, persistedEntry := range item.Entries {
			entry := persistedEntry.toEntry()
			if entry.CacheKey == "" {
				continue
			}
			bucket.entries[entry.CacheKey] = entry
		}
		if len(bucket.entries) == 0 {
			continue
		}
		c.buckets[key] = bucket
	}
}

func (c *housingCompetitionStatsCache) snapshotForPersistenceLocked() persistedHousingCompetitionStatsCache {
	out := persistedHousingCompetitionStatsCache{
		Version: housingCompetitionStatsCacheVersion,
		Buckets: make([]persistedHousingCompetitionStatsBucket, 0, len(c.buckets)),
	}
	for key, bucket := range c.buckets {
		if bucket == nil || len(bucket.entries) == 0 {
			continue
		}
		persistedBucket := persistedHousingCompetitionStatsBucket{
			Key:         key,
			RefreshedAt: unixMilliOrZero(bucket.refreshedAt),
			SampledAt:   unixMilliOrZero(bucket.sampledAt),
			Entries:     make([]persistedHousingCompetitionEntry, 0, len(bucket.entries)),
		}
		entries := housingCompetitionEntriesFromMap(bucket.entries)
		for _, entry := range entries {
			persistedBucket.Entries = append(persistedBucket.Entries, persistedHousingCompetitionEntry{
				CacheKey:      entry.uniqueKey(),
				CompetitionID: entry.CompetitionID,
				OwnerUserName: entry.OwnerUserName,
				EntryName:     entry.EntryName,
				EntryWord:     entry.EntryWord,
				ThumbnailPath: entry.ThumbnailPath,
				SubmittedAt:   entry.SubmittedAt,
				ReviewCount:   entry.ReviewCount,
				TabType:       entry.TabType,
				LastSeenAt:    entry.LastSeenAt,
			})
		}
		out.Buckets = append(out.Buckets, persistedBucket)
	}
	sort.Slice(out.Buckets, func(i, j int) bool {
		if out.Buckets[i].Key.Region != out.Buckets[j].Key.Region {
			return out.Buckets[i].Key.Region < out.Buckets[j].Key.Region
		}
		return out.Buckets[i].Key.HousingID < out.Buckets[j].Key.HousingID
	})
	return out
}

func (p persistedHousingCompetitionEntry) toEntry() HousingCompetitionEntry {
	return HousingCompetitionEntry{
		CacheKey:      strings.TrimSpace(p.CacheKey),
		CompetitionID: p.CompetitionID,
		OwnerUserName: p.OwnerUserName,
		EntryName:     p.EntryName,
		EntryWord:     p.EntryWord,
		ThumbnailPath: p.ThumbnailPath,
		SubmittedAt:   p.SubmittedAt,
		ReviewCount:   p.ReviewCount,
		TabType:       p.TabType,
		LastSeenAt:    p.LastSeenAt,
	}
}

func writeHousingCompetitionStatsCacheFile(path string, cache persistedHousingCompetitionStatsCache) error {
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

func timeFromUnixMilli(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

func unixMilliOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMilli()
}
