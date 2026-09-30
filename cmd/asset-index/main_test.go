package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/render/assetindex"
	"haruki-cloud/internal/storage"
)

func TestBootstrapDryRunThenPublishProducesUsableIndex(t *testing.T) {
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "HARUKI_") {
			t.Setenv(name, "")
		}
	}
	root := t.TempDir()
	score := filepath.Join(root, "jp-assets/startapp/music/music_score/0001_01/expert.txt")
	if err := os.MkdirAll(filepath.Dir(score), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(score, []byte("#BPM01:120\n#00008:01\n#00111:11\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfg, []byte("pjsk_render:\n  storage:\n    assets:\n      scheme: fs\n      root: "+root+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-config", cfg, "-region", "jp"}
	if err := run(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	pointer := filepath.Join(root, string(assetindex.PointerKey("jp")))
	if _, err := os.Stat(pointer); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote a pointer: %v", err)
	}
	if err := run(t.Context(), append(args, "-publish")); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewLocal(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	index := assetindex.New(store, assetindex.Config{Enabled: true}, nil)
	if err = index.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	key, found, authoritative := index.Lookup("jp-assets/startapp/music/music_score/0001_01/EXPERT.TXT")
	if !found || !authoritative || string(key) != "jp-assets/startapp/music/music_score/0001_01/expert.txt" {
		t.Fatalf("lookup=(%q,%v,%v)", key, found, authoritative)
	}
	if _, key, ok := index.BPMIndex("jp"); !ok || key == "" {
		t.Fatal("published catalog has no usable BPM descriptor")
	}
}
