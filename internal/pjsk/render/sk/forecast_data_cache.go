package sk

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
)

const (
	forecastDataEntryTTL        = 7 * 24 * time.Hour
	forecastDataFailureEntryTTL = 15 * time.Minute
	forecastDataMaxEntries      = 512
	forecastDataMaxErrorBytes   = 512
)

type forecastDataCache struct {
	mu       sync.Mutex
	provider ForecastProvider
	entries  map[forecastDataCacheKey]*forecastDataCacheEntry
	inFlight map[forecastDataCacheKey]struct{}

	persistencePath string
	entryTTL        time.Duration
	failureTTL      time.Duration
	retryInterval   time.Duration
	retryLimit      int
	maxEntries      int
	generation      uint64

	persistMu           sync.Mutex
	persistedGeneration uint64
}

type forecastDataCacheKey struct {
	Region        string
	EventID       int
	Scope         ForecastScope
	WlCharacterID int
}

type forecastDataCacheEntry struct {
	data          map[string]ForecastSourceData
	refreshedAt   time.Time
	lastAttemptAt time.Time
	lastError     string
}

const (
	forecastDataRefreshInterval      = 5 * time.Minute
	forecastDataRefreshRetryInterval = 5 * time.Second
	forecastDataRefreshRetryLimit    = 3
)

var errForecastRefreshInProgress = errors.New("forecast refresh already in progress")

func newForecastDataCache(provider ForecastProvider) *forecastDataCache {
	return &forecastDataCache{
		provider:      provider,
		entries:       make(map[forecastDataCacheKey]*forecastDataCacheEntry),
		inFlight:      make(map[forecastDataCacheKey]struct{}),
		entryTTL:      forecastDataEntryTTL,
		failureTTL:    forecastDataFailureEntryTTL,
		retryInterval: forecastDataRefreshRetryInterval,
		retryLimit:    forecastDataRefreshRetryLimit,
		maxEntries:    forecastDataMaxEntries,
	}
}

func newForecastDataCacheWithPath(provider ForecastProvider, cachePath string) *forecastDataCache {
	cache := newForecastDataCache(provider)
	cache.persistencePath = strings.TrimSpace(cachePath)
	cache.loadPersisted()
	return cache
}

func (c *forecastDataCache) SetProvider(provider ForecastProvider) {
	if c == nil || provider == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.provider = provider
	c.entries = make(map[forecastDataCacheKey]*forecastDataCacheEntry)
	c.inFlight = make(map[forecastDataCacheKey]struct{})
	c.generation++
}

func (c *forecastDataCache) CachedBySource(region string, eventID int, ranks []int) (map[string]ForecastSourceData, error) {
	return c.CachedBySourceQuery(ForecastQuery{
		Region:  region,
		EventID: eventID,
		Ranks:   ranks,
		Scope:   ForecastScopeTotal,
	})
}

func (c *forecastDataCache) CachedBySourceQuery(query ForecastQuery) (map[string]ForecastSourceData, error) {
	if c == nil {
		return nil, errors.New("forecast cache is not configured")
	}
	normalizedQuery := normalizeForecastQuery(query)
	key, ok := newForecastDataCacheKey(normalizedQuery)
	if !ok {
		return nil, errors.New("invalid forecast cache params")
	}

	now := time.Now().UTC()
	c.mu.Lock()
	c.pruneLocked(now)
	entry := c.entries[key]
	if entry == nil || lenNonEmptyForecastData(entry.data) == 0 {
		lastErr := ""
		if entry != nil {
			lastErr = entry.lastError
		}
		c.mu.Unlock()
		if lastErr != "" {
			return nil, fmt.Errorf("forecast cache is not ready: %s", lastErr)
		}
		return nil, errors.New("forecast cache is not ready")
	}
	var refreshProvider ForecastProvider
	if shouldRefreshForecastEntry(entry, now, c.retryInterval) {
		if provider, err := c.beginRefreshLocked(key); err == nil {
			refreshProvider = provider
		}
	}
	data := filterForecastSourceDataMap(entry.data, normalizedQuery.Ranks)
	c.mu.Unlock()

	if refreshProvider != nil {
		c.startRefreshWithProvider(refreshProvider, key, normalizedQuery)
	}
	if lenNonEmptyForecastData(data) == 0 {
		return nil, errors.New("预测缓存暂无这些档位的数据")
	}
	return data, nil
}

func (c *forecastDataCache) StartRefresh(region string, eventID int) {
	c.StartRefreshQuery(ForecastQuery{
		Region:  region,
		EventID: eventID,
		Scope:   ForecastScopeTotal,
	})
}

func (c *forecastDataCache) StartRefreshQuery(query ForecastQuery) {
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.TODO(), config.SKForecastRefreshTimeout)
		defer cancel()
		_ = c.RefreshNowQuery(ctx, query)
	}()
}

func (c *forecastDataCache) RefreshNow(ctx context.Context, region string, eventID int) error {
	return c.RefreshNowQuery(ctx, ForecastQuery{
		Region:  region,
		EventID: eventID,
		Scope:   ForecastScopeTotal,
	})
}

func (c *forecastDataCache) RefreshNowQuery(ctx context.Context, query ForecastQuery) error {
	if c == nil {
		return errors.New("forecast cache is not configured")
	}
	normalizedQuery := normalizeForecastQuery(query)
	key, ok := newForecastDataCacheKey(normalizedQuery)
	if !ok {
		return errors.New("invalid forecast cache params")
	}
	provider, err := c.beginRefresh(key)
	if err != nil {
		return err
	}
	return c.refreshNowWithProvider(ctx, provider, key, normalizedQuery)
}

func (c *forecastDataCache) refreshNowWithProvider(ctx context.Context, provider ForecastProvider, key forecastDataCacheKey, normalizedQuery ForecastQuery) error {
	finishFetch := commandtrace.MeasureOperation(ctx, "forecast_cache.fetch")
	data, refreshErr := fetchForecastDataWithRetry(ctx, provider, normalizedQuery, c.retryLimit, c.retryInterval)
	finishFetch()
	now := time.Now().UTC()
	if refreshErr != nil {
		c.finishFailure(ctx, key, now, refreshErr)
		return refreshErr
	}
	if lenNonEmptyForecastData(data) == 0 {
		refreshErr = errors.New("forecast source returned empty data")
		c.finishFailure(ctx, key, now, refreshErr)
		return refreshErr
	}
	c.finishSuccess(ctx, key, now, data)
	return nil
}

func (c *forecastDataCache) beginRefresh(key forecastDataCacheKey) (ForecastProvider, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.beginRefreshLocked(key)
}

func (c *forecastDataCache) beginRefreshLocked(key forecastDataCacheKey) (ForecastProvider, error) {
	if c.provider == nil {
		return nil, errors.New("forecast provider is not configured")
	}
	if _, ok := c.inFlight[key]; ok {
		return nil, errForecastRefreshInProgress
	}
	c.inFlight[key] = struct{}{}
	return c.provider, nil
}

func (c *forecastDataCache) startRefreshWithProvider(provider ForecastProvider, key forecastDataCacheKey, query ForecastQuery) {
	go func() {
		ctx, cancel := context.WithTimeout(context.TODO(), config.SKForecastRefreshTimeout)
		defer cancel()
		_ = c.refreshNowWithProvider(ctx, provider, key, query)
	}()
}

func (c *forecastDataCache) finishSuccess(ctx context.Context, key forecastDataCacheKey, now time.Time, data map[string]ForecastSourceData) {
	finishMerge := commandtrace.MeasureOperation(ctx, "forecast_cache.merge")
	c.mu.Lock()

	entry := c.entries[key]
	if entry == nil {
		entry = &forecastDataCacheEntry{}
		c.entries[key] = entry
	}
	entry.data = mergeForecastSourceDataMap(entry.data, data)
	entry.refreshedAt = now
	entry.lastAttemptAt = now
	entry.lastError = ""
	delete(c.inFlight, key)
	c.pruneLocked(now)
	c.generation++
	generation := c.generation
	c.mu.Unlock()
	finishMerge()

	c.persistLatest(ctx, generation)
}

func (c *forecastDataCache) finishFailure(ctx context.Context, key forecastDataCacheKey, now time.Time, err error) {
	finishMerge := commandtrace.MeasureOperation(ctx, "forecast_cache.merge")
	defer finishMerge()
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.entries[key]
	if entry == nil {
		entry = &forecastDataCacheEntry{}
		c.entries[key] = entry
	}
	entry.lastAttemptAt = now
	if err != nil {
		entry.lastError = truncateForecastCacheError(err.Error())
	}
	delete(c.inFlight, key)
	c.pruneLocked(now)
}

func shouldRefreshForecastEntry(entry *forecastDataCacheEntry, now time.Time, retryInterval time.Duration) bool {
	if entry == nil || lenNonEmptyForecastData(entry.data) == 0 {
		return false
	}
	if !entry.refreshedAt.IsZero() && now.Sub(entry.refreshedAt) < forecastDataRefreshInterval {
		return false
	}
	return entry.lastAttemptAt.IsZero() || now.Sub(entry.lastAttemptAt) >= retryInterval
}

type forecastDataCacheEntryAge struct {
	key      forecastDataCacheKey
	activity time.Time
	hasData  bool
}

func (c *forecastDataCache) pruneLocked(now time.Time) {
	if c == nil || len(c.entries) == 0 {
		return
	}
	now = normalizedForecastCacheTime(now)
	entryTTL, failureTTL, maxEntries := c.prunePolicy()
	c.pruneExpired(now, entryTTL, failureTTL)
	if len(c.entries) <= maxEntries {
		return
	}
	ages := c.evictableEntryAges()
	sortForecastDataCacheEntryAges(ages)
	removeCount := min(len(c.entries)-maxEntries, len(ages))
	for _, item := range ages[:removeCount] {
		delete(c.entries, item.key)
	}
}

func normalizedForecastCacheTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now
}

func (c *forecastDataCache) prunePolicy() (time.Duration, time.Duration, int) {
	entryTTL := c.entryTTL
	if entryTTL <= 0 {
		entryTTL = forecastDataEntryTTL
	}
	failureTTL := c.failureTTL
	if failureTTL <= 0 {
		failureTTL = forecastDataFailureEntryTTL
	}
	maxEntries := c.maxEntries
	if maxEntries <= 0 {
		maxEntries = forecastDataMaxEntries
	}
	return entryTTL, failureTTL, maxEntries
}

func (c *forecastDataCache) pruneExpired(now time.Time, entryTTL, failureTTL time.Duration) {
	for key, entry := range c.entries {
		if _, refreshing := c.inFlight[key]; refreshing {
			continue
		}
		if entry == nil {
			delete(c.entries, key)
			continue
		}
		activityAt, hasData := forecastDataCacheEntryActivity(entry)
		ttl := forecastCacheEntryTTL(hasData, entryTTL, failureTTL)
		if activityAt.IsZero() || now.Sub(activityAt) >= ttl {
			delete(c.entries, key)
		}
	}
}

func forecastCacheEntryTTL(hasData bool, entryTTL, failureTTL time.Duration) time.Duration {
	if hasData {
		return entryTTL
	}
	return failureTTL
}

func (c *forecastDataCache) evictableEntryAges() []forecastDataCacheEntryAge {
	ages := make([]forecastDataCacheEntryAge, 0, len(c.entries))
	for key, entry := range c.entries {
		if _, refreshing := c.inFlight[key]; refreshing {
			continue
		}
		activityAt, hasData := forecastDataCacheEntryActivity(entry)
		ages = append(ages, forecastDataCacheEntryAge{key: key, activity: activityAt, hasData: hasData})
	}
	return ages
}

func forecastDataCacheEntryActivity(entry *forecastDataCacheEntry) (time.Time, bool) {
	if entry == nil {
		return time.Time{}, false
	}
	hasData := lenNonEmptyForecastData(entry.data) > 0
	if hasData {
		return entry.refreshedAt, true
	}
	return entry.lastAttemptAt, false
}

func sortForecastDataCacheEntryAges(ages []forecastDataCacheEntryAge) {
	sort.Slice(ages, func(i, j int) bool {
		if ages[i].hasData != ages[j].hasData {
			return !ages[i].hasData
		}
		if !ages[i].activity.Equal(ages[j].activity) {
			return ages[i].activity.Before(ages[j].activity)
		}
		if ages[i].key.Region != ages[j].key.Region {
			return ages[i].key.Region < ages[j].key.Region
		}
		if ages[i].key.EventID != ages[j].key.EventID {
			return ages[i].key.EventID < ages[j].key.EventID
		}
		if ages[i].key.Scope != ages[j].key.Scope {
			return ages[i].key.Scope < ages[j].key.Scope
		}
		return ages[i].key.WlCharacterID < ages[j].key.WlCharacterID
	})
}

func truncateForecastCacheError(value string) string {
	value = strings.ToValidUTF8(value, "")
	if len(value) <= forecastDataMaxErrorBytes {
		return value
	}
	cut := forecastDataMaxErrorBytes
	for cut > 0 && cut < len(value) && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

func fetchForecastDataWithRetry(ctx context.Context, provider ForecastProvider, query ForecastQuery, retryLimit int, retryInterval time.Duration) (map[string]ForecastSourceData, error) {
	if ctx == nil {
		ctx = context.TODO()
	}
	var lastErr error
	for attempt := 1; attempt <= retryLimit; attempt++ {
		data, err := fetchForecastData(ctx, provider, query)
		if err == nil && lenNonEmptyForecastData(data) > 0 {
			return data, nil
		}
		if err == nil {
			err = errors.New("forecast source returned empty data")
		}
		lastErr = err
		if attempt >= retryLimit {
			break
		}
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func fetchForecastData(ctx context.Context, provider ForecastProvider, query ForecastQuery) (map[string]ForecastSourceData, error) {
	if provider == nil {
		return nil, errors.New("forecast provider is not configured")
	}
	if bySourceQuery, ok := provider.(ForecastProviderBySourceQuery); ok {
		return bySourceQuery.FetchBySourceQuery(ctx, query)
	}
	if bySource, ok := provider.(ForecastProviderBySource); ok {
		if query.Scope != ForecastScopeTotal || query.WlCharacterID != nil {
			return nil, errors.New("forecast provider does not support scoped forecast query")
		}
		return bySource.FetchBySource(ctx, query.Region, query.EventID, nil)
	}
	if byQuery, ok := provider.(ForecastProviderQuery); ok {
		merged, err := byQuery.FetchQuery(ctx, query)
		if err != nil {
			return nil, err
		}
		if len(merged) == 0 {
			return nil, nil
		}
		return map[string]ForecastSourceData{
			"forecast": {
				Scores:    cloneForecastScores(merged),
				FetchedAt: time.Now().UTC().UnixMilli(),
			},
		}, nil
	}
	merged, err := provider.Fetch(ctx, query.Region, query.EventID, nil)
	if err != nil {
		return nil, err
	}
	if len(merged) == 0 {
		return nil, nil
	}
	return map[string]ForecastSourceData{
		"forecast": {
			Scores:    cloneForecastScores(merged),
			FetchedAt: time.Now().UTC().UnixMilli(),
		},
	}, nil
}

func newForecastDataCacheKey(query ForecastQuery) (forecastDataCacheKey, bool) {
	normalizedQuery := normalizeForecastQuery(query)
	if normalizedQuery.Region == "" || normalizedQuery.EventID <= 0 {
		return forecastDataCacheKey{}, false
	}
	wlCharacterID := 0
	if normalizedQuery.WlCharacterID != nil && *normalizedQuery.WlCharacterID > 0 {
		wlCharacterID = *normalizedQuery.WlCharacterID
	}
	return forecastDataCacheKey{
		Region:        normalizedQuery.Region,
		EventID:       normalizedQuery.EventID,
		Scope:         normalizedQuery.Scope,
		WlCharacterID: wlCharacterID,
	}.normalized()
}

func (key forecastDataCacheKey) normalized() (forecastDataCacheKey, bool) {
	key.Region = strings.ToLower(strings.TrimSpace(key.Region))
	key.Scope = normalizeForecastScope(key.Scope)
	if key.Scope == ForecastScopeTotal {
		key.WlCharacterID = 0
	}
	if key.Region == "" || key.EventID <= 0 {
		return forecastDataCacheKey{}, false
	}
	return key, true
}

func filterForecastSourceDataMap(in map[string]ForecastSourceData, ranks []int) map[string]ForecastSourceData {
	if len(in) == 0 {
		return nil
	}
	rankFilter := make(map[int]struct{}, len(ranks))
	for _, rank := range ranks {
		if rank > 0 {
			rankFilter[rank] = struct{}{}
		}
	}

	out := make(map[string]ForecastSourceData, len(in))
	for source, data := range in {
		scores := make(map[int]ForecastScore, len(data.Scores))
		for rank, score := range data.Scores {
			if len(rankFilter) > 0 && !forecastRankSelected(rank, rankFilter) {
				continue
			}
			scores[rank] = score
		}
		if len(scores) == 0 {
			continue
		}
		out[source] = ForecastSourceData{
			Scores:    scores,
			FetchedAt: data.FetchedAt,
		}
	}
	return out
}

func mergeForecastSourceDataMap(previous, next map[string]ForecastSourceData) map[string]ForecastSourceData {
	out := cloneForecastSourceDataMap(previous)
	for source, data := range next {
		if len(data.Scores) == 0 {
			continue
		}
		if out == nil {
			out = make(map[string]ForecastSourceData)
		}
		out[source] = ForecastSourceData{
			Scores:    cloneForecastScores(data.Scores),
			FetchedAt: data.FetchedAt,
		}
	}
	return out
}

func cloneForecastSourceDataMap(in map[string]ForecastSourceData) map[string]ForecastSourceData {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]ForecastSourceData, len(in))
	for source, data := range in {
		if len(data.Scores) == 0 {
			continue
		}
		out[source] = ForecastSourceData{
			Scores:    cloneForecastScores(data.Scores),
			FetchedAt: data.FetchedAt,
		}
	}
	return out
}

func cloneForecastScores(in map[int]ForecastScore) map[int]ForecastScore {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int]ForecastScore, len(in))
	for rank, score := range in {
		out[rank] = score
	}
	return out
}

func lenNonEmptyForecastData(data map[string]ForecastSourceData) int {
	count := 0
	for _, sourceData := range data {
		if len(sourceData.Scores) > 0 {
			count++
		}
	}
	return count
}
