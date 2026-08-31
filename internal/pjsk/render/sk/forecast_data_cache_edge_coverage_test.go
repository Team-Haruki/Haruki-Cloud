package sk

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type forecastBaseStub struct {
	data map[int]ForecastScore
	err  error
}

func (s forecastBaseStub) Fetch(context.Context, string, int, []int) (map[int]ForecastScore, error) {
	return cloneForecastScores(s.data), s.err
}

type forecastQueryStub struct {
	forecastBaseStub
	queryData map[int]ForecastScore
	queryErr  error
}

func (s forecastQueryStub) FetchQuery(context.Context, ForecastQuery) (map[int]ForecastScore, error) {
	return cloneForecastScores(s.queryData), s.queryErr
}

type forecastBySourceStub struct {
	forecastBaseStub
	bySourceData map[string]ForecastSourceData
	bySourceErr  error
}

func (s forecastBySourceStub) FetchBySource(context.Context, string, int, []int) (map[string]ForecastSourceData, error) {
	return cloneForecastSourceDataMap(s.bySourceData), s.bySourceErr
}

type forecastBySourceQueryStub struct {
	forecastBaseStub
	bySourceQueryData map[string]ForecastSourceData
	bySourceQueryErr  error
}

func (s forecastBySourceQueryStub) FetchBySourceQuery(context.Context, ForecastQuery) (map[string]ForecastSourceData, error) {
	return cloneForecastSourceDataMap(s.bySourceQueryData), s.bySourceQueryErr
}

func TestForecastDataCacheConfigurationEdges(t *testing.T) {
	var nilCache *forecastDataCache
	if _, err := nilCache.CachedBySourceQuery(ForecastQuery{}); err == nil {
		t.Fatal("nil cache read unexpectedly succeeded")
	}
	if err := nilCache.RefreshNowQuery(context.Background(), ForecastQuery{}); err == nil {
		t.Fatal("nil cache refresh unexpectedly succeeded")
	}
	nilCache.StartRefreshQuery(ForecastQuery{})

	cache := newForecastDataCache(nil)
	cache.SetProvider(nil)
	if _, err := cache.CachedBySourceQuery(ForecastQuery{}); err == nil {
		t.Fatal("invalid cache query unexpectedly succeeded")
	}
	if err := cache.RefreshNowQuery(context.Background(), ForecastQuery{Region: "jp"}); err == nil {
		t.Fatal("invalid refresh query unexpectedly succeeded")
	}
	key := forecastDataCacheKey{Region: "jp", EventID: 1, Scope: ForecastScopeTotal}
	if _, err := cache.beginRefresh(key); err == nil {
		t.Fatal("missing provider unexpectedly began refresh")
	}
}

func TestForecastDataCacheNotReadyAndResetEdges(t *testing.T) {
	cache := newForecastDataCache(forecastBaseStub{})
	key := forecastDataCacheKey{Region: "jp", EventID: 1, Scope: ForecastScopeTotal}
	cache.entries[key] = &forecastDataCacheEntry{lastAttemptAt: time.Now().UTC(), lastError: "upstream unavailable"}
	if _, err := cache.CachedBySource("jp", 1, nil); err == nil || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("cached failure error = %v", err)
	}
	cache.entries[key] = &forecastDataCacheEntry{data: forecastSourceFixture(100), refreshedAt: time.Now().UTC()}
	if _, err := cache.CachedBySource("jp", 1, []int{999}); err == nil {
		t.Fatal("missing cached rank unexpectedly succeeded")
	}
	if _, err := cache.beginRefresh(key); err != nil {
		t.Fatalf("begin refresh: %v", err)
	}
	if _, err := cache.beginRefresh(key); !errors.Is(err, errForecastRefreshInProgress) {
		t.Fatalf("duplicate refresh error = %v", err)
	}
	cache.SetProvider(forecastBaseStub{data: map[int]ForecastScore{100: {Score: 2}}})
	if len(cache.entries) != 0 || len(cache.inFlight) != 0 || cache.generation != 1 {
		t.Fatalf("provider reset left cache state: %+v", cache)
	}
}

func TestForecastDataCacheEmptyRefreshAndFailureEdges(t *testing.T) {
	cache := newForecastDataCache(forecastBaseStub{})
	cache.retryLimit = 1
	if err := cache.RefreshNow(context.Background(), "jp", 2); err == nil {
		t.Fatal("empty refresh unexpectedly succeeded")
	}
	key := forecastDataCacheKey{Region: "jp", EventID: 2, Scope: ForecastScopeTotal}
	if cache.entries[key] == nil || cache.entries[key].lastError == "" {
		t.Fatalf("empty refresh failure was not recorded: %+v", cache.entries[key])
	}
	cache.inFlight[key] = struct{}{}
	cache.finishFailure(context.Background(), key, time.Now().UTC(), nil)
	if _, ok := cache.inFlight[key]; ok {
		t.Fatal("finishFailure did not clear in-flight marker")
	}
}

func TestFetchForecastDataProviderVariants(t *testing.T) {
	query := ForecastQuery{Region: "jp", EventID: 1, Scope: ForecastScopeTotal}
	if _, err := fetchForecastData(context.Background(), nil, query); err == nil {
		t.Fatal("nil provider unexpectedly succeeded")
	}
	byQuery := forecastBySourceQueryStub{bySourceQueryData: forecastSourceFixture(101)}
	if got, err := fetchForecastData(context.Background(), byQuery, query); err != nil || got["source"].Scores[100].Score != 101 {
		t.Fatalf("by-source-query fetch = %+v, %v", got, err)
	}
	bySource := forecastBySourceStub{bySourceData: forecastSourceFixture(102)}
	if got, err := fetchForecastData(context.Background(), bySource, query); err != nil || got["source"].Scores[100].Score != 102 {
		t.Fatalf("by-source fetch = %+v, %v", got, err)
	}
	chapter := ForecastScopeChapter
	query.Scope = chapter
	if _, err := fetchForecastData(context.Background(), bySource, query); err == nil {
		t.Fatal("scoped legacy provider unexpectedly succeeded")
	}
}

func TestFetchForecastDataMergedProviderVariants(t *testing.T) {
	query := ForecastQuery{Region: "jp", EventID: 1, Scope: ForecastScopeTotal}
	queryProvider := forecastQueryStub{queryData: map[int]ForecastScore{100: {Score: 103}}}
	if got, err := fetchForecastData(context.Background(), queryProvider, query); err != nil || got["forecast"].Scores[100].Score != 103 {
		t.Fatalf("query provider fetch = %+v, %v", got, err)
	}
	if got, err := fetchForecastData(context.Background(), forecastQueryStub{}, query); err != nil || got != nil {
		t.Fatalf("empty query provider fetch = %+v, %v", got, err)
	}
	base := forecastBaseStub{data: map[int]ForecastScore{100: {Score: 104}}}
	if got, err := fetchForecastData(context.Background(), base, query); err != nil || got["forecast"].Scores[100].Score != 104 {
		t.Fatalf("base provider fetch = %+v, %v", got, err)
	}
	if _, err := fetchForecastData(context.Background(), forecastBaseStub{err: errors.New("failed")}, query); err == nil {
		t.Fatal("base provider failure unexpectedly succeeded")
	}
}

func TestForecastDataCachePruneAndOrderingEdges(t *testing.T) {
	if normalizedForecastCacheTime(time.Time{}).IsZero() {
		t.Fatal("zero cache time stayed zero")
	}
	cache := &forecastDataCache{entries: map[forecastDataCacheKey]*forecastDataCacheEntry{}, inFlight: map[forecastDataCacheKey]struct{}{}}
	entryTTL, failureTTL, maxEntries := cache.prunePolicy()
	if entryTTL != forecastDataEntryTTL || failureTTL != forecastDataFailureEntryTTL || maxEntries != forecastDataMaxEntries {
		t.Fatalf("default prune policy = %v, %v, %d", entryTTL, failureTTL, maxEntries)
	}
	now := time.Now().UTC()
	refreshingKey := forecastDataCacheKey{Region: "jp", EventID: 1, Scope: ForecastScopeTotal}
	cache.entries[refreshingKey] = nil
	cache.inFlight[refreshingKey] = struct{}{}
	cache.pruneExpired(now, time.Minute, time.Minute)
	if _, ok := cache.entries[refreshingKey]; !ok {
		t.Fatal("refreshing entry was pruned")
	}
	if activity, hasData := forecastDataCacheEntryActivity(nil); !activity.IsZero() || hasData {
		t.Fatalf("nil entry activity = %v, %v", activity, hasData)
	}
}

func TestForecastDataCacheHelperMapEdges(t *testing.T) {
	if got := filterForecastSourceDataMap(nil, nil); got != nil {
		t.Fatalf("nil filter result = %+v", got)
	}
	input := map[string]ForecastSourceData{
		"empty":  {},
		"source": {Scores: map[int]ForecastScore{100: {Score: 1}, 200: {Score: 2}}, FetchedAt: 7},
	}
	filtered := filterForecastSourceDataMap(input, []int{-1, 200})
	if len(filtered) != 1 || filtered["source"].Scores[200].Score != 2 {
		t.Fatalf("filtered data = %+v", filtered)
	}
	merged := mergeForecastSourceDataMap(nil, input)
	if len(merged) != 1 || !reflect.DeepEqual(cloneForecastSourceDataMap(merged), merged) {
		t.Fatalf("merged data = %+v", merged)
	}
	if cloneForecastScores(nil) != nil || cloneForecastSourceDataMap(nil) != nil {
		t.Fatal("nil clone helpers returned non-nil data")
	}
}

func TestForecastRefreshDecisionEdges(t *testing.T) {
	now := time.Now().UTC()
	if shouldRefreshForecastEntry(nil, now, time.Second) {
		t.Fatal("nil entry should not refresh")
	}
	fresh := &forecastDataCacheEntry{data: forecastSourceFixture(1), refreshedAt: now}
	if shouldRefreshForecastEntry(fresh, now, time.Second) {
		t.Fatal("fresh entry should not refresh")
	}
	stale := &forecastDataCacheEntry{data: forecastSourceFixture(1), refreshedAt: now.Add(-forecastDataRefreshInterval - time.Second)}
	if !shouldRefreshForecastEntry(stale, now, time.Second) {
		t.Fatal("stale entry should refresh")
	}
	stale.lastAttemptAt = now
	if shouldRefreshForecastEntry(stale, now, time.Minute) {
		t.Fatal("recently attempted stale entry should not refresh")
	}
}

func forecastSourceFixture(score int) map[string]ForecastSourceData {
	return map[string]ForecastSourceData{
		"source": {Scores: map[int]ForecastScore{100: {Score: score}}, FetchedAt: 1},
	}
}
