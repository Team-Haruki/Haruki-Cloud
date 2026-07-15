package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestCustomProfileMasterCacheInvalidatesAndOwnsRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"first","nested":{"value":1}}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &renderapp.App{Config: renderapp.Config{
		LocalMasterdata: renderapp.LocalMasterdataConfig{Dir: dir},
	}}
	ctx, trace := commandtrace.WithTrace(context.Background())
	ids := map[int]struct{}{1: {}}

	first, err := loadCustomProfileMasterTable(ctx, app, renderregion.JP, "test.json", ids)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	first[1]["name"] = "mutated"
	first[1]["nested"].(map[string]any)["value"] = float64(99)
	second, err := loadCustomProfileMasterTable(ctx, app, renderregion.JP, "test.json", ids)
	if err != nil {
		t.Fatalf("cached load: %v", err)
	}
	if second[1]["name"] != "first" || second[1]["nested"].(map[string]any)["value"] != float64(1) {
		t.Fatalf("cached row was mutated through caller ownership: %#v", second[1])
	}

	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"updated-longer","nested":{"value":2}}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	nextTime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, nextTime, nextTime); err != nil {
		t.Fatal(err)
	}
	updated, err := loadCustomProfileMasterTable(ctx, app, renderregion.JP, "test.json", ids)
	if err != nil {
		t.Fatalf("updated load: %v", err)
	}
	if updated[1]["name"] != "updated-longer" {
		t.Fatalf("mtime update was not observed: %#v", updated[1])
	}

	operations := make(map[string]int)
	for _, stat := range trace.Snapshot().Operations {
		operations[stat.Name] = stat.Count
	}
	if operations["custom_profile.master.load"] < 2 || operations["custom_profile.master.cache"] < 1 {
		t.Fatalf("unexpected cache/load operations: %#v", operations)
	}
}

func TestCustomProfileMasterCacheCoalescesConcurrentLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(`[{"id":1,"nested":[1,2,3]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &renderapp.App{Config: renderapp.Config{
		LocalMasterdata: renderapp.LocalMasterdataConfig{Dir: dir},
	}}
	ids := map[int]struct{}{1: {}}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			rows, err := loadCustomProfileMasterTable(context.Background(), app, renderregion.JP, "test.json", ids)
			if err != nil {
				t.Errorf("load: %v", err)
				return
			}
			rows[1]["nested"].([]any)[0] = float64(value)
		}(i)
	}
	wg.Wait()
	rows, err := loadCustomProfileMasterTable(context.Background(), app, renderregion.JP, "test.json", ids)
	if err != nil {
		t.Fatal(err)
	}
	if rows[1]["nested"].([]any)[0] != float64(1) {
		t.Fatal("concurrent caller mutation escaped into cache")
	}
}

func TestCustomProfileMasterCacheIsBounded(t *testing.T) {
	cache := newCustomProfileMasterIndexCache()
	dir := t.TempDir()
	for i := 0; i < customProfileMasterCacheMaxItems+5; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%d.json", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf(`[{"id":%d}]`, i+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := cache.load(path, info); err != nil {
			t.Fatal(err)
		}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.entries) > customProfileMasterCacheMaxItems || cache.charge > customProfileMasterCacheMaxCharge {
		t.Fatalf("cache exceeded bounds: entries=%d charge=%d", len(cache.entries), cache.charge)
	}
}

func BenchmarkCustomProfileMasterCacheWarm(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"cached","nested":{"value":1}}]`), 0o644); err != nil {
		b.Fatal(err)
	}
	app := &renderapp.App{Config: renderapp.Config{
		LocalMasterdata: renderapp.LocalMasterdataConfig{Dir: dir},
	}}
	ids := map[int]struct{}{1: {}}
	if _, err := loadCustomProfileMasterTable(context.Background(), app, renderregion.JP, "test.json", ids); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loadCustomProfileMasterTable(context.Background(), app, renderregion.JP, "test.json", ids); err != nil {
			b.Fatal(err)
		}
	}
}
