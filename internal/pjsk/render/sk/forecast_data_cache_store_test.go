package sk

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func forecastStoreTestProvider() *sequencedForecastProvider {
	return &sequencedForecastProvider{
		data: []map[string]ForecastSourceData{{
			"local": {
				Scores:    map[int]ForecastScore{100: {Score: 42, Timestamp: 1_700_000_000, Source: "local"}},
				FetchedAt: 1_700_000_100,
			},
		}},
	}
}

func TestForecastCacheStoreOnLocalWritesTheOldPath(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewLocal(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	controller := NewControllerWithConfig(nil, ForecastConfig{CacheStore: store, CacheKey: "sk_forecast_cache.json"})
	if controller.forecastCache.store != store || controller.forecastCache.storeKey != "sk_forecast_cache.json" {
		t.Fatal("controller did not use the configured cache store")
	}
	cache := newForecastDataCacheWithStore(forecastStoreTestProvider(), store, "sk_forecast_cache.json")
	if err := cache.RefreshNow(context.Background(), "jp", 7); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	stored, err := store.Get(context.Background(), "sk_forecast_cache.json")
	if err != nil {
		t.Fatalf("store get: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "sk_forecast_cache.json"))
	if err != nil || !bytes.Equal(onDisk, stored) {
		t.Fatalf("file at old path = %q, %v; store = %q", onDisk, err, stored)
	}

	viaPath := newForecastDataCacheWithPath(&sequencedForecastProvider{}, filepath.Join(dir, "sk_forecast_cache.json"))
	got, err := viaPath.CachedBySource("jp", 7, []int{100})
	if err != nil || got["local"].Scores[100].Score != 42 {
		t.Fatalf("path wrapper did not load the store object: %+v, %v", got, err)
	}
}

func TestForecastCacheStoreDisabledAndFailing(t *testing.T) {
	disabled := newForecastDataCacheWithStore(forecastStoreTestProvider(), storage.Disabled(), "sk_forecast_cache.json")
	if err := disabled.RefreshNow(context.Background(), "jp", 1); err != nil {
		t.Fatalf("refresh with disabled store: %v", err)
	}
	if disabled.persistedGeneration != 0 {
		t.Fatalf("disabled store recorded a persisted generation %d", disabled.persistedGeneration)
	}

	memory := storagetest.NewMemory()
	memory.FailPut = func(storage.Key) error { return errors.New("put down") }
	memory.FailGet = func(storage.Key) error { return errors.New("get down") }
	failing := newForecastDataCacheWithStore(forecastStoreTestProvider(), memory, "sk_forecast_cache.json")
	if err := failing.RefreshNow(context.Background(), "jp", 1); err != nil {
		t.Fatalf("refresh with failing store: %v", err)
	}
	if failing.persistedGeneration != 0 {
		t.Fatalf("failed put recorded a persisted generation %d", failing.persistedGeneration)
	}
	if len(memory.Calls()) != 2 {
		t.Fatalf("store calls = %+v", memory.Calls())
	}
}

func TestForecastCacheStoreGuards(t *testing.T) {
	if cache := newForecastDataCacheWithStore(nil, nil, "k"); cache.store != nil {
		t.Fatal("nil store enabled persistence")
	}
	if cache := newForecastDataCacheWithStore(nil, storagetest.NewMemory(), ""); cache.store != nil {
		t.Fatal("empty key enabled persistence")
	}
	if cache := newForecastDataCacheWithPath(nil, "  "); cache.store != nil {
		t.Fatal("empty path enabled persistence")
	}
	if cache := newForecastCacheForConfig(nil, ForecastConfig{}); cache.store != nil {
		t.Fatal("zero config enabled persistence")
	}
	(*forecastDataCache)(nil).persistLatest(context.Background(), 1)

	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"sk_forecast_cache.json": []byte("{")})
	cache := newForecastDataCacheWithStore(nil, memory, "sk_forecast_cache.json")
	if len(cache.entries) != 0 {
		t.Fatal("malformed persisted payload produced entries")
	}
}
