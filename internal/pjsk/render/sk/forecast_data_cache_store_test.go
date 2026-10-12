package sk

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	json "haruki-cloud/internal/jsonutil"
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
	if err := cache.configurePersistence(t.Context(), "test-node"); err != nil {
		t.Fatal(err)
	}
	if err := cache.RefreshNow(context.Background(), "jp", 7); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if err := cache.closePersistence(t.Context()); err != nil {
		t.Fatal(err)
	}
	stored, err := store.Get(context.Background(), cache.storeKey)
	if err != nil {
		t.Fatalf("store get: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, string(cache.storeKey)))
	if err != nil || !bytes.Equal(onDisk, stored) {
		t.Fatalf("file at old path = %q, %v; store = %q", onDisk, err, stored)
	}

	viaPath := newForecastDataCacheWithPath(&sequencedForecastProvider{}, filepath.Join(dir, string(cache.storeKey)))
	got, err := viaPath.CachedBySource("jp", 7, []int{100})
	if err != nil || got["local"].Scores[100].Score != 42 {
		t.Fatalf("path wrapper did not load the store object: %+v, %v", got, err)
	}
}

func TestForecastCacheStoreDisabledAndFailing(t *testing.T) {
	disabled := newForecastDataCacheWithStore(forecastStoreTestProvider(), storage.Disabled(), "sk_forecast_cache.json")
	if err := disabled.configurePersistence(t.Context(), "test-node"); err != nil {
		t.Fatal(err)
	}
	if err := disabled.RefreshNow(context.Background(), "jp", 1); err != nil {
		t.Fatalf("refresh with disabled store: %v", err)
	}
	if err := disabled.closePersistence(t.Context()); err == nil {
		t.Fatal("disabled persistence should report error")
	}
	if disabled.persistedGeneration != 0 {
		t.Fatalf("disabled store recorded a persisted generation %d", disabled.persistedGeneration)
	}

	memory := storagetest.NewMemory()
	memory.FailPut = func(storage.Key) error { return errors.New("put down") }
	memory.FailGet = func(storage.Key) error { return errors.New("get down") }
	failing := newForecastDataCacheWithStore(forecastStoreTestProvider(), memory, "sk_forecast_cache.json")
	if err := failing.configurePersistence(t.Context(), "test-node"); err != nil {
		t.Fatal(err)
	}
	if err := failing.RefreshNow(context.Background(), "jp", 1); err != nil {
		t.Fatalf("refresh with failing store: %v", err)
	}
	// Persistence never started, so there is no writer to flush or fail.
	if err := failing.closePersistence(t.Context()); err != nil {
		t.Fatalf("close without a writer: %v", err)
	}
	if failing.persistedGeneration != 0 {
		t.Fatalf("failed put recorded a persisted generation %d", failing.persistedGeneration)
	}
	// The namespaced read failed for a reason other than absence, so neither
	// the legacy fallback read nor any write happens.
	if calls := memory.Calls(); len(calls) != 1 || calls[0].Method != "Get" {
		t.Fatalf("store calls = %+v", calls)
	}
	if !failing.persistenceDisabled {
		t.Fatal("read failure did not keep persistence disabled")
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
	if err := cache.configurePersistence(t.Context(), "test-node"); err != nil {
		t.Fatal(err)
	}
	if len(cache.entries) != 0 {
		t.Fatal("malformed persisted payload produced entries")
	}
	if got := memory.Calls(); len(got) != 2 {
		t.Fatalf("expected the namespaced read and the legacy fallback read, got %+v", got)
	}
}

func TestForecastNamespaceReadFailureKeepsPersistenceDisabled(t *testing.T) {
	memory := storagetest.NewMemory()
	source := newForecastDataCache(&keyedForecastProvider{})
	if err := source.RefreshNow(t.Context(), "jp", 8); err != nil {
		t.Fatal(err)
	}
	source.mu.Lock()
	payload, err := json.Marshal(source.snapshotForPersistenceLocked())
	source.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	memory.Seed(map[string][]byte{"forecast.json": payload, "instances/node-a/forecast.json": payload})
	memory.FailGet = func(key storage.Key) error {
		if key == "instances/node-a/forecast.json" {
			return errors.New("transient read failure")
		}
		return nil
	}
	cache := newForecastDataCacheWithStore(&keyedForecastProvider{}, memory, "forecast.json")
	if err := cache.configurePersistence(t.Context(), "node-a"); err != nil {
		t.Fatal(err)
	}
	if !cache.persistenceDisabled {
		t.Fatal("transient namespaced read failure did not keep persistence disabled")
	}
	if len(cache.entries) != 0 {
		t.Fatal("legacy snapshot was read although the namespaced one exists")
	}
	if err := cache.RefreshNow(t.Context(), "jp", 9); err != nil {
		t.Fatal(err)
	}
	cache.persistLatest(t.Context(), 1)
	if err := cache.closePersistence(t.Context()); err != nil {
		t.Fatalf("close without a writer: %v", err)
	}
	for _, call := range memory.Calls() {
		if call.Method != "Get" || call.Key != "instances/node-a/forecast.json" {
			t.Fatalf("unexpected store call after a failed namespaced read: %+v", memory.Calls())
		}
	}
	memory.FailGet = nil
	if stored, err := memory.Get(t.Context(), "instances/node-a/forecast.json"); err != nil || !bytes.Equal(stored, payload) {
		t.Fatalf("namespaced snapshot was replaced: %v", err)
	}
}
