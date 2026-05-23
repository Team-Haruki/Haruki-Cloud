package meta

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
