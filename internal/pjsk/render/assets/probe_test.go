package assets

import (
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

func TestAssetHelperRelativePath(t *testing.T) {
	var nilHelper *AssetHelper
	if got := nilHelper.RelativePath("/abs/x.png"); got != "/abs/x.png" {
		t.Fatalf("nil helper = %q", got)
	}
	root := t.TempDir()
	legacy := t.TempDir()
	helper := NewAssetHelper(root, []string{"https://assets.example", legacy})
	if got := helper.RelativePath(" "); got != " " {
		t.Fatalf("blank target = %q", got)
	}
	if got := helper.RelativePath(root + "/static_images/a.png"); got != "static_images/a.png" {
		t.Fatalf("primary hit = %q", got)
	}
	if got := helper.RelativePath(legacy + "/jp-assets/b.png"); got != "jp-assets/b.png" {
		t.Fatalf("legacy hit = %q", got)
	}
	if got := helper.RelativePath("asset/jp-assets/startapp/c.png"); got != "asset/jp-assets/startapp/c.png" {
		t.Fatalf("relative target changed = %q", got)
	}
	rootless := NewAssetHelper("", nil)
	if got := rootless.RelativePath("static_images/d.png"); got != "static_images/d.png" {
		t.Fatalf("rootless helper = %q", got)
	}
}
