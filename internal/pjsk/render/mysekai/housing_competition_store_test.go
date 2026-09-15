package mysekai

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func seedHousingStatsForPersist(cache *housingCompetitionStatsCache) {
	cache.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 3}] = &housingCompetitionStatsBucket{
		entries:       map[string]HousingCompetitionEntry{"a": {CacheKey: "a", ReviewCount: 9, LastSeenAt: time.Now().UnixMilli()}},
		refreshedAt:   time.Now().UTC(),
		snapshotDirty: true,
	}
	cache.generation = 1
}

func TestHousingStatsCacheStoreOnLocalWritesTheOldPath(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewLocal(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	cache := newHousingCompetitionStatsCacheWithStore(store, DefaultHousingCompetitionStatsCacheKey, time.Second)
	seedHousingStatsForPersist(cache)
	cache.persistLatest(context.Background(), 1)
	if cache.persistedGeneration != 1 {
		t.Fatalf("persisted generation = %d", cache.persistedGeneration)
	}
	stored, err := store.Get(context.Background(), DefaultHousingCompetitionStatsCacheKey)
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "mysekai_housing_competition_stats.json"))
	if err != nil || !bytes.Equal(onDisk, stored) {
		t.Fatalf("file at old path = %q, %v", onDisk, err)
	}

	reloaded := newHousingCompetitionStatsCache(filepath.Join(dir, "mysekai_housing_competition_stats.json"), time.Second)
	if bucket := reloaded.buckets[housingCompetitionStatsCacheKey{Region: "jp", HousingID: 3}]; bucket == nil || bucket.entries["a"].ReviewCount != 9 {
		t.Fatalf("reloaded buckets = %+v", reloaded.buckets)
	}
}

func TestHousingStatsCacheStoreDisabledAndFailing(t *testing.T) {
	disabled := newHousingCompetitionStatsCacheWithStore(storage.Disabled(), DefaultHousingCompetitionStatsCacheKey, time.Second)
	seedHousingStatsForPersist(disabled)
	disabled.persistLatest(context.Background(), 1)
	if disabled.persistedGeneration != 0 {
		t.Fatal("disabled store recorded a persisted generation")
	}

	memory := storagetest.NewMemory()
	memory.FailGet = func(storage.Key) error { return errors.New("get down") }
	memory.FailPut = func(storage.Key) error { return errors.New("put down") }
	failing := newHousingCompetitionStatsCacheWithStore(memory, "stats.json", time.Second)
	seedHousingStatsForPersist(failing)
	failing.persistLatest(context.Background(), 1)
	if failing.persistedGeneration != 0 || len(memory.Calls()) != 2 {
		t.Fatalf("failing store generation=%d calls=%+v", failing.persistedGeneration, memory.Calls())
	}

	if cache := newHousingCompetitionStatsCacheWithStore(memory, "", time.Second); cache.store != nil {
		t.Fatal("empty key enabled persistence")
	}
	if store, key := localHousingCompetitionCacheStore(" "); store != nil || key != "" {
		t.Fatal("blank path produced a store")
	}
}

func TestHousingCompetitionCacheStoreSelection(t *testing.T) {
	memory := storagetest.NewMemory()
	store, key := housingCompetitionCacheStore(MasterdataOptions{HousingCompetitionCacheStore: memory, HousingCompetitionStatsCachePath: "/ignored/x.json"})
	if store != storage.Store(memory) || key != DefaultHousingCompetitionStatsCacheKey {
		t.Fatalf("store selection = %v %q", store, key)
	}
	store, key = housingCompetitionCacheStore(MasterdataOptions{HousingCompetitionCacheStore: memory, HousingCompetitionStatsCacheKey: "custom.json"})
	if store != storage.Store(memory) || key != "custom.json" {
		t.Fatalf("custom key selection = %q", key)
	}
	if store, key = housingCompetitionCacheStore(MasterdataOptions{}); store != nil || key != "" {
		t.Fatal("zero options produced a store")
	}

	controller := NewController(nil, nil, "", nil, MasterdataOptions{HousingCompetitionCacheStore: memory})
	if controller.housingCompetitionStats.store != storage.Store(memory) || controller.housingCompetitionBanners.store != storage.Store(memory) {
		t.Fatal("controller did not share the cache store between stats and banners")
	}
}

func TestHousingBannerCacheThroughMemoryStore(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{housingCompetitionBannerCacheDirName + "/asset/jp-assets/banner.png": []byte("cached")})
	cache := newHousingCompetitionBannerCache(memory, nil, nil)
	if raw, err := cache.Bytes("asset/jp-assets/banner.png"); err != nil || string(raw) != "cached" {
		t.Fatalf("Bytes() = %q, %v", raw, err)
	}
	calls := memory.Calls()
	if len(calls) != 1 || calls[0].Method != "Get" || calls[0].Key != housingCompetitionBannerCacheDirName+"/asset/jp-assets/banner.png" {
		t.Fatalf("store calls = %+v", calls)
	}
	if key := cache.cacheKey("/abs/banner.png"); len(key) == 0 || !bytes.HasPrefix([]byte(key), []byte(housingCompetitionBannerCacheDirName+"/by_hash/")) {
		t.Fatalf("absolute path key = %q", key)
	}
}
