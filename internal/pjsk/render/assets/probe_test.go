package assets

import (
	"context"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestAssetReaderUsesStore(t *testing.T) {
	var nilReader *AssetReader
	if nilReader.UsesStore() {
		t.Fatal("nil reader must not use a store")
	}
	if NewAssetReader(nil, nil).UsesStore() || NewAssetReader(nil, storage.Disabled()).UsesStore() {
		t.Fatal("nil / Disabled store must use the legacy branch")
	}
	if !NewAssetReader(nil, storagetest.NewMemory()).UsesStore() {
		t.Fatal("configured store must be used")
	}
}

func TestProbeExistingStoreBranch(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/music/jacket/j/j.png": []byte("x")})
	reader := NewAssetReader(NewAssetHelper(t.TempDir(), nil), memory)
	ctx := context.Background()

	local, ok := ProbeExisting(ctx, reader, nil, "asset/jp-assets/startapp/music/jacket/j/j.png")
	if !ok || local != "" {
		t.Fatalf("store hit = %q %v, want no local path", local, ok)
	}
	if local, ok := ProbeExisting(ctx, reader, NewAssetHelper("", nil), "asset/jp-assets/startapp/missing.png"); ok || local != "" {
		t.Fatalf("store miss = %q %v", local, ok)
	}
}

func TestProbeExistingLegacyBranch(t *testing.T) {
	root := writeAssetTree(t, map[string]string{"jp-assets/startapp/x.png": "x"})
	helper := NewAssetHelper(root, nil)
	ctx := context.Background()
	want := filepath.ToSlash(filepath.Join(root, "jp-assets/startapp/x.png"))

	for name, reader := range map[string]*AssetReader{
		"nil reader":      nil,
		"disabled reader": NewAssetReader(helper, storage.Disabled()),
	} {
		local, ok := ProbeExisting(ctx, reader, helper, "asset/jp-assets/startapp/x.png")
		if !ok || local != want {
			t.Fatalf("%s: hit = %q %v, want %q", name, local, ok, want)
		}
		if local, ok := ProbeExisting(ctx, reader, helper, "asset/jp-assets/startapp/y.png"); ok || local != "" {
			t.Fatalf("%s: miss = %q %v", name, local, ok)
		}
	}

	// Without a caller helper the reader's own helper answers.
	if local, ok := ProbeExisting(ctx, NewAssetReader(helper, nil), nil, "jp-assets/startapp/x.png"); !ok || local != want {
		t.Fatalf("reader helper = %q %v", local, ok)
	}
	if local, ok := ProbeExisting(ctx, nil, nil, "jp-assets/startapp/x.png"); ok || local != "" {
		t.Fatalf("no helper and no reader = %q %v", local, ok)
	}
}

func TestReaderOr(t *testing.T) {
	reader := NewAssetReader(nil, storagetest.NewMemory())
	if ReaderOr(reader, nil) != reader {
		t.Fatal("configured reader must be returned as is")
	}
	helper := NewAssetHelper("", nil)
	fallback := ReaderOr(nil, helper)
	if fallback == nil || fallback.UsesStore() || fallback.helper != helper {
		t.Fatalf("fallback reader = %#v", fallback)
	}
}
