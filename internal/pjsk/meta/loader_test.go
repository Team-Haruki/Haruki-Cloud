package meta

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestLoaderPersistsUpdatedMusicMetas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{
			"music_id": 1,
			"difficulty": "master",
			"music_time": 100,
			"event_rate": 1,
			"base_score": 2,
			"base_score_auto": 3,
			"fever_score": 4,
			"fever_end_time": 5,
			"tap_count": 6,
			"skill_score_solo": [1,2,3,4,5,6],
			"skill_score_auto": [1,2,3,4,5,6],
			"skill_score_multi": [1,2,3,4,5,6]
		}]`))
	}))
	defer server.Close()

	oldURL := regionURLs["jp"]
	regionURLs["jp"] = server.URL
	defer func() {
		regionURLs["jp"] = oldURL
	}()

	dir := t.TempDir()
	loader := NewLoader(nil, WithOutputDir(dir))
	if err := loader.load(context.Background(), "jp"); err != nil {
		t.Fatalf("load jp: %v", err)
	}

	target := filepath.Join(dir, "music_metas.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read persisted meta: %v", err)
	}
	if !bytes.Contains(data, []byte(`"music_id":10000`)) {
		t.Fatalf("expected persisted meta to contain injected omakase entry: %s", data)
	}
	if cached := loader.Get("jp"); !bytes.Equal(cached, data) {
		t.Fatalf("persisted payload differs from cached payload")
	}
}

func TestLoaderPersistsRegionSpecificFilename(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(nil, WithOutputDir(dir))

	if err := loader.persist("tw", []byte(`[]`)); err != nil {
		t.Fatalf("persist tw: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "music_metas-tc.json")); err != nil {
		t.Fatalf("expected tw meta filename: %v", err)
	}
}

func TestLoaderBoundsRemoteResponses(t *testing.T) {
	loader := NewLoader(nil)
	if got := loader.http.ResponseBodyLimit; got != musicMetaMaxResponseBytes {
		t.Fatalf("response body limit = %d, want %d", got, musicMetaMaxResponseBytes)
	}
	if loader.http.GetClient().Timeout <= 0 {
		t.Fatal("metadata HTTP client has no timeout")
	}
}

func TestLoaderFallsBackToPersistedMusicMetas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	oldURL := regionURLs["jp"]
	regionURLs["jp"] = server.URL
	defer func() { regionURLs["jp"] = oldURL }()

	dir := t.TempDir()
	persisted := []byte(`[{"music_id":1,"difficulty":"master","tap_count":123}]`)
	if err := os.WriteFile(filepath.Join(dir, "music_metas.json"), persisted, 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(nil, WithOutputDir(dir))
	if err := loader.load(context.Background(), "jp"); err != nil {
		t.Fatalf("load jp with persisted fallback: %v", err)
	}
	entry, ok := loader.View("jp").Find(1, "master")
	if !ok || entry.Int("tap_count") != 123 {
		t.Fatal("persisted fallback was not loaded into the cache")
	}
}

func TestLoaderReportsRemoteAndPersistedFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	oldURL := regionURLs["jp"]
	regionURLs["jp"] = server.URL
	defer func() { regionURLs["jp"] = oldURL }()

	loader := NewLoader(nil, WithOutputDir(t.TempDir()))
	err := loader.load(context.Background(), "jp")
	if err == nil {
		t.Fatal("expected remote and persisted metadata failures")
	}
	if cached := loader.Get("jp"); len(cached) != 0 {
		t.Fatal("unexpected cache entry after both metadata sources failed")
	}
}

func TestLoaderKeepsCurrentCacheWhenRemoteRefreshFails(t *testing.T) {
	var unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if unavailable.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[{"music_id":1,"difficulty":"master","tap_count":456}]`))
	}))
	defer server.Close()

	oldURL := regionURLs["jp"]
	regionURLs["jp"] = server.URL
	defer func() { regionURLs["jp"] = oldURL }()

	loader := NewLoader(nil, WithOutputDir(t.TempDir()))
	if err := loader.load(context.Background(), "jp"); err != nil {
		t.Fatal(err)
	}
	unavailable.Store(true)
	if err := loader.load(context.Background(), "jp"); err == nil {
		t.Fatal("expected failed refresh to remain observable")
	}
	entry, ok := loader.View("jp").Find(1, "master")
	if !ok || entry.Int("tap_count") != 456 {
		t.Fatal("failed refresh replaced the current cache")
	}
}

func TestLoaderKeepsFreshRemoteCacheWhenPersistenceFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"music_id":1,"difficulty":"master","tap_count":789}]`))
	}))
	defer server.Close()

	oldURL := regionURLs["jp"]
	regionURLs["jp"] = server.URL
	defer func() { regionURLs["jp"] = oldURL }()

	notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(notDirectory, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := NewLoader(nil, WithOutputDir(notDirectory))
	if err := loader.load(context.Background(), "jp"); err == nil {
		t.Fatal("expected persistence failure")
	}
	entry, ok := loader.View("jp").Find(1, "master")
	if !ok || entry.Int("tap_count") != 789 {
		t.Fatal("persistence failure replaced the fresh remote cache")
	}
}

func TestLoaderRefreshReplacesImmutableViewGeneration(t *testing.T) {
	var version atomic.Int64
	version.Store(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"music_id":1,"difficulty":"master","tap_count":` + strconv.FormatInt(version.Load(), 10) + `}]`))
	}))
	defer server.Close()

	oldURL := regionURLs["jp"]
	regionURLs["jp"] = server.URL
	defer func() { regionURLs["jp"] = oldURL }()

	loader := NewLoader(nil)
	if err := loader.load(context.Background(), "jp"); err != nil {
		t.Fatal(err)
	}
	first := loader.View("jp")
	firstEntry, ok := first.Find(1, "master")
	if !ok || firstEntry.Int("tap_count") != 1 {
		t.Fatal("first generation lookup failed")
	}

	version.Store(2)
	if err := loader.load(context.Background(), "jp"); err != nil {
		t.Fatal(err)
	}
	second := loader.View("jp")
	secondEntry, ok := second.Find(1, "master")
	if !ok || secondEntry.Int("tap_count") != 2 {
		t.Fatal("refreshed generation lookup failed")
	}
	if first == second || firstEntry.Int("tap_count") != 1 {
		t.Fatal("refresh mutated an existing immutable generation")
	}
}
