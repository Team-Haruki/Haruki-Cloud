package sk

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
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
