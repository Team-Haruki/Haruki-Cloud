package app

import (
	"context"
	"errors"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestMusicMetaPersistencePrefersStore(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.FailGet = func(storage.Key) error { return errors.New("store consulted") }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loader := resolveMetaLoader(ctx, nil, 0, musicMetaPersistence(&Config{MusicMetaStore: memory, MusicMetaOutputDir: t.TempDir()}), "", "")
	if loader == nil {
		t.Fatal("loader not constructed")
	}
	sawGet := false
	for _, call := range memory.Calls() {
		sawGet = sawGet || call.Method == "Get"
	}
	if !sawGet {
		t.Fatalf("music meta store not used for the persisted fallback: %+v", memory.Calls())
	}
	if musicMetaPersistence(&Config{}) == nil {
		t.Fatal("output-dir persistence option missing")
	}
}

func TestNewThreadsHousingCacheStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	memory := storagetest.NewMemory()
	runtime := New(nil, nil, Config{
		InitContext:                         ctx,
		MetaLoader:                          nil,
		MusicMetaStore:                      storage.Disabled(),
		MySekaiHousingCompetitionCacheStore: memory,
		MySekaiHousingCompetitionCacheKey:   "housing.json",
		SKForecast:                          SKForecastConfig{CacheStore: memory, CacheKey: "forecast.json"},
	})
	t.Cleanup(func() { _ = runtime.Close() })
	gets := map[storage.Key]bool{}
	for _, call := range memory.Calls() {
		if call.Method == "Get" {
			gets[call.Key] = true
		}
	}
	if !gets["housing.json"] || !gets["forecast.json"] {
		t.Fatalf("cache store calls = %+v", memory.Calls())
	}
}
