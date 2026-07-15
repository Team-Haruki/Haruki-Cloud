package sk

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"haruki-cloud/internal/observability/commandtrace"
)

type sequencedForecastProvider struct {
	calls atomic.Int32
	data  []map[string]ForecastSourceData
	errs  []error
}

func (p *sequencedForecastProvider) Fetch(context.Context, string, int, []int) (map[int]ForecastScore, error) {
	return nil, errors.New("not implemented")
}

func (p *sequencedForecastProvider) FetchBySource(context.Context, string, int, []int) (map[string]ForecastSourceData, error) {
	idx := int(p.calls.Add(1)) - 1
	if idx < len(p.errs) && p.errs[idx] != nil {
		return nil, p.errs[idx]
	}
	if idx < len(p.data) {
		return cloneForecastSourceDataMap(p.data[idx]), nil
	}
	return nil, errors.New("unexpected forecast fetch")
}

func TestForecastDataCacheKeepsPreviousDataWhenRefreshFails(t *testing.T) {
	prevRetryLimit := forecastDataRefreshRetryLimit
	prevRetryInterval := forecastDataRefreshRetryInterval
	t.Cleanup(func() {
		forecastDataRefreshRetryLimit = prevRetryLimit
		forecastDataRefreshRetryInterval = prevRetryInterval
	})
	forecastDataRefreshRetryLimit = 1
	forecastDataRefreshRetryInterval = time.Millisecond

	provider := &sequencedForecastProvider{
		data: []map[string]ForecastSourceData{
			{
				"33kit": {
					Scores: map[int]ForecastScore{
						100: {Score: 1_234_567, Timestamp: 1_700_000_000, Source: "33kit"},
					},
					FetchedAt: 1_700_000_100,
				},
			},
		},
		errs: []error{
			nil,
			errors.New("forecast source down"),
		},
	}
	cache := newForecastDataCache(provider)

	if err := cache.RefreshNow(context.Background(), "jp", 101); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	if err := cache.RefreshNow(context.Background(), "jp", 101); err == nil {
		t.Fatal("expected failed refresh, got nil")
	}

	got, err := cache.CachedBySource("jp", 101, []int{100})
	if err != nil {
		t.Fatalf("cached data after failed refresh: %v", err)
	}
	score := got["33kit"].Scores[100]
	if score.Score != 1_234_567 {
		t.Fatalf("failed refresh overwrote cached score: %+v", score)
	}
}

func TestForecastDataCacheLoadsPersistedData(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "sk_forecast_cache.json")
	provider := &sequencedForecastProvider{
		data: []map[string]ForecastSourceData{
			{
				"local": {
					Scores: map[int]ForecastScore{
						100: {Score: 8_765_432, Timestamp: 1_700_000_000, Source: "local"},
					},
					FetchedAt: 1_700_000_100,
				},
			},
		},
	}
	cache := newForecastDataCacheWithPath(provider, cachePath)
	if err := cache.RefreshNow(context.Background(), "cn", 202); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}

	loadedProvider := &sequencedForecastProvider{
		errs: []error{errors.New("should not fetch while persisted cache is fresh")},
	}
	loaded := newForecastDataCacheWithPath(loadedProvider, cachePath)
	got, err := loaded.CachedBySource("cn", 202, []int{100})
	if err != nil {
		t.Fatalf("read persisted forecast cache: %v", err)
	}
	score := got["local"].Scores[100]
	if score.Score != 8_765_432 {
		t.Fatalf("unexpected persisted score: %+v", score)
	}
	if calls := loadedProvider.calls.Load(); calls != 0 {
		t.Fatalf("persisted cache should avoid cold fetch, got %d calls", calls)
	}
}

type keyedForecastProvider struct {
	calls atomic.Int32
}

func (p *keyedForecastProvider) Fetch(context.Context, string, int, []int) (map[int]ForecastScore, error) {
	return nil, errors.New("not implemented")
}

func (p *keyedForecastProvider) FetchBySource(_ context.Context, _ string, eventID int, _ []int) (map[string]ForecastSourceData, error) {
	p.calls.Add(1)
	return map[string]ForecastSourceData{
		"local": {
			Scores: map[int]ForecastScore{
				100: {Score: eventID, Timestamp: int64(eventID), Source: "local"},
			},
			FetchedAt: int64(eventID),
		},
	}, nil
}

func TestForecastDataCacheRetentionBoundsSuccessAndFailureEntries(t *testing.T) {
	cache := newForecastDataCache(&keyedForecastProvider{})
	cache.entryTTL = time.Hour
	cache.failureTTL = time.Minute
	cache.maxEntries = 3
	now := time.Now().UTC()
	success := func(at time.Time, score int) *forecastDataCacheEntry {
		return &forecastDataCacheEntry{
			data: map[string]ForecastSourceData{
				"local": {
					Scores:    map[int]ForecastScore{100: {Score: score}},
					FetchedAt: at.UnixMilli(),
				},
			},
			refreshedAt:   at,
			lastAttemptAt: at,
		}
	}
	failure := func(at time.Time) *forecastDataCacheEntry {
		return &forecastDataCacheEntry{lastAttemptAt: at, lastError: "unavailable"}
	}
	key := func(eventID int) forecastDataCacheKey {
		return forecastDataCacheKey{Region: "jp", EventID: eventID, Scope: ForecastScopeTotal}
	}
	cache.entries[key(1)] = success(now.Add(-2*time.Hour), 1)
	cache.entries[key(2)] = failure(now.Add(-2 * time.Minute))
	cache.entries[key(3)] = failure(now.Add(-30 * time.Second))
	cache.entries[key(4)] = success(now.Add(-30*time.Second), 4)
	cache.entries[key(5)] = success(now.Add(-20*time.Second), 5)
	cache.entries[key(6)] = success(now.Add(-10*time.Second), 6)

	cache.mu.Lock()
	cache.pruneLocked(now)
	if len(cache.entries) != 3 {
		cache.mu.Unlock()
		t.Fatalf("entry count = %d, want 3", len(cache.entries))
	}
	for _, eventID := range []int{1, 2, 3} {
		if cache.entries[key(eventID)] != nil {
			cache.mu.Unlock()
			t.Fatalf("expired or lower-priority failure entry %d was retained", eventID)
		}
	}
	for _, eventID := range []int{4, 5, 6} {
		if cache.entries[key(eventID)] == nil {
			cache.mu.Unlock()
			t.Fatalf("recent successful entry %d was evicted", eventID)
		}
	}
	cache.mu.Unlock()

	longError := strings.Repeat("错", forecastDataMaxErrorBytes)
	truncated := truncateForecastCacheError(longError)
	if len(truncated) > forecastDataMaxErrorBytes || !utf8.ValidString(truncated) {
		t.Fatalf("truncated error is not bounded valid UTF-8: bytes=%d", len(truncated))
	}
}

func TestForecastDataCacheConcurrentPersistenceIsCompleteAndTraced(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "sk_forecast_cache.json")
	provider := &keyedForecastProvider{}
	cache := newForecastDataCacheWithPath(provider, cachePath)

	const refreshes = 32
	var wg sync.WaitGroup
	errs := make(chan error, refreshes)
	traces := make([]*commandtrace.Trace, refreshes)
	for index := 0; index < refreshes; index++ {
		ctx, trace := commandtrace.WithTrace(context.Background())
		traces[index] = trace
		wg.Add(1)
		go func(ctx context.Context, eventID int) {
			defer wg.Done()
			errs <- cache.RefreshNow(ctx, "jp", eventID)
		}(ctx, index+1)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RefreshNow() error = %v", err)
		}
	}
	if calls := provider.calls.Load(); calls != refreshes {
		t.Fatalf("provider calls = %d, want %d", calls, refreshes)
	}

	payload, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read persisted cache: %v", err)
	}
	var persisted persistedForecastDataCache
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatalf("decode persisted cache: %v", err)
	}
	if len(persisted.Entries) != refreshes {
		t.Fatalf("persisted entries = %d, want %d", len(persisted.Entries), refreshes)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".sk_forecast_cache.json.tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("orphaned temp files: %v", temps)
	}

	stageCounts := make(map[string]int)
	for index, trace := range traces {
		for _, name := range []string{"forecast_cache.fetch", "forecast_cache.merge", "forecast_cache.persist_wait"} {
			if count := forecastCacheTraceOperationCount(trace, name); count != 1 {
				t.Fatalf("trace[%d] %s count = %d, operations=%+v", index, name, count, trace.Snapshot().Operations)
			}
		}
		for _, name := range []string{"forecast_cache.snapshot", "forecast_cache.encode", "forecast_cache.persist"} {
			stageCounts[name] += forecastCacheTraceOperationCount(trace, name)
		}
	}
	for _, name := range []string{"forecast_cache.snapshot", "forecast_cache.encode", "forecast_cache.persist"} {
		if stageCounts[name] == 0 {
			t.Fatalf("no trace recorded %s", name)
		}
	}
	if cache.persistedGeneration != cache.generation {
		t.Fatalf("persisted generation = %d, in-memory generation = %d", cache.persistedGeneration, cache.generation)
	}
}

func forecastCacheTraceOperationCount(trace *commandtrace.Trace, name string) int {
	if trace == nil {
		return 0
	}
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name {
			return operation.Count
		}
	}
	return 0
}

func TestRemoteForecastProviderSourcesMatchSupportedRegions(t *testing.T) {
	provider := NewRemoteForecastProvider()
	tests := []struct {
		region string
		want   []string
	}{
		{region: "jp", want: []string{"33kit", "moesekai", "local"}},
		{region: "cn", want: []string{"moesekai", "local"}},
		{region: "en", want: []string{"sekarun", "local"}},
		{region: "tw", want: []string{"local"}},
		{region: "kr", want: []string{"local"}},
	}
	for _, tt := range tests {
		sources := provider.sourcesForRegion(tt.region)
		var got []string
		for _, source := range sources {
			got = append(got, source.name)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("sources for %s = %v, want %v", tt.region, got, tt.want)
		}
	}
}
