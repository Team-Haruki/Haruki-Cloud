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
