package mysekai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

const (
	housingCompetitionStatsCacheVersion         = 1
	housingCompetitionStatsEntryTTL             = 90 * 24 * time.Hour
	housingCompetitionStatsMaxEntries           = 50_000
	housingCompetitionStatsMaxEntriesPerBucket  = 20_000
	housingCompetitionStatsMaxBuckets           = 64
	housingCompetitionStatsSharedRefreshTimeout = 2 * time.Minute
)

type housingCompetitionStatsCache struct {
	mu               sync.Mutex
	path             string
	refreshInterval  time.Duration
	buckets          map[housingCompetitionStatsCacheKey]*housingCompetitionStatsBucket
	entryTTL         time.Duration
	maxEntries       int
	maxBucketEntries int
	maxBuckets       int
	generation       uint64

	refreshes           singleflight.Group
	persistMu           sync.Mutex
	persistedGeneration uint64
}

type housingCompetitionStatsCacheKey struct {
	Region    string `json:"region"`
	HousingID int    `json:"housing_id"`
}

type housingCompetitionStatsBucket struct {
	entries         map[string]HousingCompetitionEntry
	refreshedAt     time.Time
	sampledAt       time.Time
	snapshotEntries []HousingCompetitionEntry
	snapshotDirty   bool
}

type housingCompetitionRefreshResult struct {
	entries        []HousingCompetitionEntry
	sampledAt      time.Time
	refreshedCount int
	err            error
	operations     []commandtrace.Stats
	leader         *housingCompetitionRefreshToken
}

type housingCompetitionRefreshToken byte

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
		path:             strings.TrimSpace(cachePath),
		refreshInterval:  refreshInterval,
		buckets:          make(map[housingCompetitionStatsCacheKey]*housingCompetitionStatsBucket),
		entryTTL:         housingCompetitionStatsEntryTTL,
		maxEntries:       housingCompetitionStatsMaxEntries,
		maxBucketEntries: housingCompetitionStatsMaxEntriesPerBucket,
		maxBuckets:       housingCompetitionStatsMaxBuckets,
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
	c.pruneLocked(now)
	if bucket := c.buckets[key]; bucket != nil && len(bucket.entries) > 0 && !shouldRefreshHousingCompetitionStats(bucket, now, c.RefreshInterval()) {
		finishSnapshot := commandtrace.MeasureOperation(ctx, housingCacheSnapshotStage)
		entries, sampledAt := bucket.snapshot()
		finishSnapshot()
		c.mu.Unlock()
		return entries, sampledAt, 0, nil
	}
	staleAvailable := c.buckets[key] != nil && len(c.buckets[key].entries) > 0
	c.mu.Unlock()

	entries, sampledAt, refreshedCount, err := c.Refresh(ctx, api, region, housingID, sampleCount)
	if err != nil {
		if staleAvailable {
			finishSnapshot := commandtrace.MeasureOperation(ctx, housingCacheSnapshotStage)
			c.mu.Lock()
			staleEntries, staleSampledAt := c.snapshotLocked(key)
			c.mu.Unlock()
			finishSnapshot()
			if len(staleEntries) > 0 {
				return staleEntries, staleSampledAt, 0, nil
			}
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
	if ctx == nil {
		ctx = context.TODO()
	}
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, 0, err
	}
	if api == nil {
		return nil, time.Time{}, 0, fmt.Errorf("sekai api client is not configured")
	}
	sampleCount = normalizeHousingCompetitionSampleCount(sampleCount)
	flightKey := fmt.Sprintf("%s:%d:%d", key.Region, key.HousingID, sampleCount)
	callerToken := new(housingCompetitionRefreshToken)
	finishWait := commandtrace.MeasureOperation(ctx, "housing_cache.refresh_wait")
	resultCh := c.refreshes.DoChan(flightKey, func() (any, error) {
		background := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
		sharedBase, cancel := context.WithTimeout(background, housingCompetitionStatsSharedRefreshTimeout)
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)

		finishFetch := commandtrace.MeasureOperation(sharedCtx, "housing_cache.fetch")
		entries, sampledAt, refreshedCount, err := fetchHousingCompetitionSamples(sharedCtx, api, region, housingID, sampleCount, 0)
		finishFetch()
		if err != nil {
			return housingCompetitionRefreshResult{
				err:        err,
				operations: trace.Snapshot().Operations,
				leader:     callerToken,
			}, nil
		}
		now := time.Now().UTC()

		finishMerge := commandtrace.MeasureOperation(sharedCtx, "housing_cache.merge")
		c.mu.Lock()
		bucket := c.buckets[key]
		if bucket == nil {
			bucket = &housingCompetitionStatsBucket{
				entries:       make(map[string]HousingCompetitionEntry),
				snapshotDirty: true,
			}
			c.buckets[key] = bucket
		}
		mergeHousingCompetitionEntries(bucket.entries, entries)
		bucket.snapshotDirty = true
		bucket.refreshedAt = now
		if !sampledAt.IsZero() {
			bucket.sampledAt = sampledAt.UTC()
		} else {
			bucket.sampledAt = now
		}
		c.pruneLocked(now)
		finishMerge()
		finishSnapshot := commandtrace.MeasureOperation(sharedCtx, housingCacheSnapshotStage)
		merged, mergedSampledAt := bucket.snapshot()
		finishSnapshot()
		c.generation++
		generation := c.generation
		c.mu.Unlock()

		c.persistLatest(sharedCtx, generation)
		return housingCompetitionRefreshResult{
			entries:        merged,
			sampledAt:      mergedSampledAt,
			refreshedCount: refreshedCount,
			operations:     trace.Snapshot().Operations,
			leader:         callerToken,
		}, nil
	})

	select {
	case <-ctx.Done():
		finishWait()
		return nil, time.Time{}, 0, ctx.Err()
	case completed := <-resultCh:
		finishWait()
		result, ok := completed.Val.(housingCompetitionRefreshResult)
		if !ok {
			return nil, time.Time{}, 0, fmt.Errorf("unexpected housing competition refresh result")
		}
		commandtrace.MergeOperations(ctx, result.operations)
		if result.leader != callerToken {
			commandtrace.RecordOperation(ctx, "housing_cache.shared", 0)
		}
		if result.err != nil {
			return nil, time.Time{}, 0, result.err
		}
		return append([]HousingCompetitionEntry(nil), result.entries...), result.sampledAt, result.refreshedCount, nil
	}
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
	return collectHousingCompetitionSamples(ctx, api, region, housingID, sampleCount, sampleIntervalMillis)
}

func collectHousingCompetitionSamples(ctx context.Context, api HousingCompetitionListClient, region string, housingID, sampleCount, sampleIntervalMillis int) ([]HousingCompetitionEntry, time.Time, int, error) {
	merged := make(map[string]HousingCompetitionEntry)
	var sampledAt time.Time
	for i := 0; i < sampleCount; i++ {
		if err := ctx.Err(); err != nil {
			return nil, time.Time{}, i, err
		}
		entries, sampleTime, err := fetchHousingCompetitionSample(api, region, housingID)
		if err != nil {
			return nil, time.Time{}, i, err
		}
		if sampleTime.After(sampledAt) {
			sampledAt = sampleTime
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

func fetchHousingCompetitionSample(api HousingCompetitionListClient, region string, housingID int) ([]HousingCompetitionEntry, time.Time, error) {
	raw, err := api.GetMySekaiHousingCompetitionList(region, housingID, true)
	if err != nil {
		return nil, time.Time{}, err
	}
	entries, lotteryAt, err := parseHousingCompetitionEntries(raw)
	if err != nil {
		return nil, time.Time{}, err
	}
	if lotteryAt <= 0 {
		return entries, time.Time{}, nil
	}
	return entries, time.UnixMilli(lotteryAt).UTC(), nil
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
	entries := b.snapshotView()
	return append([]HousingCompetitionEntry(nil), entries...), b.sampledAt
}

func (b *housingCompetitionStatsBucket) snapshotView() []HousingCompetitionEntry {
	if b == nil {
		return nil
	}
	if b.snapshotDirty || b.snapshotEntries == nil {
		b.snapshotEntries = housingCompetitionEntriesFromMap(b.entries)
		b.snapshotDirty = false
	}
	return b.snapshotEntries
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

func (c *housingCompetitionStatsCache) pruneLocked(now time.Time) {
	if c == nil || len(c.buckets) == 0 {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	limits := c.pruneLimits()
	c.pruneExpiredBuckets(now.Add(-limits.entryTTL).UnixMilli(), limits.maxBucketEntries)
	c.trimBuckets(limits.maxBuckets)
	c.trimEntries(limits.maxEntries)
}

type housingCompetitionPruneLimits struct {
	entryTTL         time.Duration
	maxEntries       int
	maxBucketEntries int
	maxBuckets       int
}

func (c *housingCompetitionStatsCache) pruneLimits() housingCompetitionPruneLimits {
	limits := housingCompetitionPruneLimits{
		entryTTL:         c.entryTTL,
		maxEntries:       c.maxEntries,
		maxBucketEntries: c.maxBucketEntries,
		maxBuckets:       c.maxBuckets,
	}
	if limits.entryTTL <= 0 {
		limits.entryTTL = housingCompetitionStatsEntryTTL
	}
	if limits.maxEntries <= 0 {
		limits.maxEntries = housingCompetitionStatsMaxEntries
	}
	if limits.maxBucketEntries <= 0 {
		limits.maxBucketEntries = housingCompetitionStatsMaxEntriesPerBucket
	}
	if limits.maxBuckets <= 0 {
		limits.maxBuckets = housingCompetitionStatsMaxBuckets
	}
	return limits
}

func (c *housingCompetitionStatsCache) pruneExpiredBuckets(cutoffMillis int64, maxBucketEntries int) {
	for key, bucket := range c.buckets {
		if bucket == nil {
			delete(c.buckets, key)
			continue
		}
		bucketActivity := unixMilliOrZero(bucket.refreshedAt)
		if bucketActivity <= 0 {
			bucketActivity = unixMilliOrZero(bucket.sampledAt)
		}
		if bucketActivity > 0 && bucketActivity < cutoffMillis {
			delete(c.buckets, key)
			continue
		}
		if len(bucket.entries) > maxBucketEntries {
			removeLowestRankedHousingCompetitionEntries(bucket, len(bucket.entries)-maxBucketEntries)
			bucket.snapshotDirty = true
		}
	}
}

type housingCompetitionBucketAge struct {
	key housingCompetitionStatsCacheKey
	at  time.Time
}

func (c *housingCompetitionStatsCache) trimBuckets(maxBuckets int) {
	if len(c.buckets) <= maxBuckets {
		return
	}
	ages := make([]housingCompetitionBucketAge, 0, len(c.buckets))
	for key, bucket := range c.buckets {
		at := bucket.refreshedAt
		if at.IsZero() {
			at = bucket.sampledAt
		}
		ages = append(ages, housingCompetitionBucketAge{key: key, at: at})
	}
	sort.Slice(ages, func(i, j int) bool {
		if !ages[i].at.Equal(ages[j].at) {
			return ages[i].at.Before(ages[j].at)
		}
		if ages[i].key.Region != ages[j].key.Region {
			return ages[i].key.Region < ages[j].key.Region
		}
		return ages[i].key.HousingID < ages[j].key.HousingID
	})
	for _, item := range ages[:len(ages)-maxBuckets] {
		delete(c.buckets, item.key)
	}
}

type housingCompetitionGlobalEntryAge struct {
	bucketKey      housingCompetitionStatsCacheKey
	entryKey       string
	bucketActivity int64
	reviewCount    int
	lastSeen       int64
}

func (c *housingCompetitionStatsCache) trimEntries(maxEntries int) {
	total := c.entryCount()
	if total <= maxEntries {
		return
	}
	ages := c.globalEntryAges(total)
	sortHousingCompetitionGlobalEntryAges(ages)
	c.removeGlobalEntries(ages[:total-maxEntries])
}

func (c *housingCompetitionStatsCache) entryCount() int {
	total := 0
	for _, bucket := range c.buckets {
		total += len(bucket.entries)
	}
	return total
}

func (c *housingCompetitionStatsCache) globalEntryAges(capacity int) []housingCompetitionGlobalEntryAge {
	ages := make([]housingCompetitionGlobalEntryAge, 0, capacity)
	for bucketKey, bucket := range c.buckets {
		bucketActivity := unixMilliOrZero(bucket.refreshedAt)
		if bucketActivity <= 0 {
			bucketActivity = unixMilliOrZero(bucket.sampledAt)
		}
		for entryKey, entry := range bucket.entries {
			ages = append(ages, housingCompetitionGlobalEntryAge{
				bucketKey:      bucketKey,
				entryKey:       entryKey,
				bucketActivity: bucketActivity,
				reviewCount:    entry.ReviewCount,
				lastSeen:       entry.LastSeenAt,
			})
		}
	}
	return ages
}

func sortHousingCompetitionGlobalEntryAges(ages []housingCompetitionGlobalEntryAge) {
	sort.Slice(ages, func(i, j int) bool {
		if ages[i].bucketActivity != ages[j].bucketActivity {
			return ages[i].bucketActivity < ages[j].bucketActivity
		}
		if ages[i].reviewCount != ages[j].reviewCount {
			return ages[i].reviewCount < ages[j].reviewCount
		}
		if ages[i].lastSeen != ages[j].lastSeen {
			return ages[i].lastSeen < ages[j].lastSeen
		}
		if ages[i].bucketKey.Region != ages[j].bucketKey.Region {
			return ages[i].bucketKey.Region < ages[j].bucketKey.Region
		}
		if ages[i].bucketKey.HousingID != ages[j].bucketKey.HousingID {
			return ages[i].bucketKey.HousingID < ages[j].bucketKey.HousingID
		}
		return ages[i].entryKey < ages[j].entryKey
	})
}

func (c *housingCompetitionStatsCache) removeGlobalEntries(ages []housingCompetitionGlobalEntryAge) {
	for _, item := range ages {
		bucket := c.buckets[item.bucketKey]
		if bucket == nil {
			continue
		}
		delete(bucket.entries, item.entryKey)
		bucket.snapshotDirty = true
		if len(bucket.entries) == 0 {
			delete(c.buckets, item.bucketKey)
		}
	}
}

func removeLowestRankedHousingCompetitionEntries(bucket *housingCompetitionStatsBucket, count int) {
	if bucket == nil || count <= 0 {
		return
	}
	type entryAge struct {
		key         string
		reviewCount int
		lastSeen    int64
	}
	ages := make([]entryAge, 0, len(bucket.entries))
	for key, entry := range bucket.entries {
		ages = append(ages, entryAge{key: key, reviewCount: entry.ReviewCount, lastSeen: entry.LastSeenAt})
	}
	sort.Slice(ages, func(i, j int) bool {
		if ages[i].reviewCount != ages[j].reviewCount {
			return ages[i].reviewCount < ages[j].reviewCount
		}
		if ages[i].lastSeen != ages[j].lastSeen {
			return ages[i].lastSeen < ages[j].lastSeen
		}
		return ages[i].key < ages[j].key
	})
	if count > len(ages) {
		count = len(ages)
	}
	for _, item := range ages[:count] {
		delete(bucket.entries, item.key)
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
			entries:       make(map[string]HousingCompetitionEntry, len(item.Entries)),
			refreshedAt:   timeFromUnixMilli(item.RefreshedAt),
			sampledAt:     timeFromUnixMilli(item.SampledAt),
			snapshotDirty: true,
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
	c.pruneLocked(time.Now().UTC())
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
		entries := bucket.snapshotView()
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

func (c *housingCompetitionStatsCache) persistLatest(ctx context.Context, requestedGeneration uint64) {
	if c == nil || c.path == "" {
		return
	}
	finishWait := commandtrace.MeasureOperation(ctx, "housing_cache.persist_wait")
	c.persistMu.Lock()
	finishWait()
	defer c.persistMu.Unlock()
	if c.persistedGeneration >= requestedGeneration {
		return
	}

	finishSnapshot := commandtrace.MeasureOperation(ctx, housingCacheSnapshotStage)
	c.mu.Lock()
	c.pruneLocked(time.Now().UTC())
	persisted := c.snapshotForPersistenceLocked()
	generation := c.generation
	c.mu.Unlock()
	finishSnapshot()

	finishEncode := commandtrace.MeasureOperation(ctx, "housing_cache.encode")
	payload, err := json.Marshal(persisted)
	finishEncode()
	if err != nil {
		return
	}
	finishPersist := commandtrace.MeasureOperation(ctx, "housing_cache.persist")
	err = writeHousingCompetitionStatsCachePayload(c.path, payload)
	finishPersist()
	if err == nil {
		c.persistedGeneration = generation
	}
}

func writeHousingCompetitionStatsCachePayload(path string, payload []byte) error {
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
