package honor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

// assetExists answers identically through a store-backed reader and through
// the local helper probe over the same tree.
func TestBuilderAssetExistsParity(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "jp-assets", "startapp", "honor", "frame.png")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/honor/frame.png": []byte("png")})
	source := newTestHonorSource(renderregion.JP)
	ctx := context.Background()

	legacy := NewBuilder(source, assets.NewAssetHelper(root, nil))
	store := NewBuilder(source, assets.NewAssetHelper("", nil)).WithAssetReader(ctx, assets.NewAssetReader(nil, memory))
	disabled := NewBuilder(source, assets.NewAssetHelper(root, nil)).WithAssetReader(ctx, assets.NewAssetReader(nil, storage.Disabled()))
	for _, builder := range []*Builder{legacy, store, disabled} {
		if !builder.assetExists("asset/jp-assets/startapp/honor/frame.png") {
			t.Fatal("hit branch mismatch")
		}
		if builder.assetExists("asset/jp-assets/startapp/honor/missing.png") {
			t.Fatal("miss branch mismatch")
		}
	}

	storeOnly := NewBuilder(source, nil).WithAssetReader(ctx, assets.NewAssetReader(nil, memory))
	if !storeOnly.assetExists("jp-assets/startapp/honor/frame.png") {
		t.Fatal("store reader without helper must still answer")
	}
	var nilBuilder *Builder
	if nilBuilder.WithAssetReader(ctx, nil) != nil {
		t.Fatal("nil builder must stay nil")
	}

	controller := NewController(source, nil, nil)
	controller.SetAssetReader(assets.NewAssetReader(nil, memory))
	if controller.assetReader == nil {
		t.Fatal("controller reader not set")
	}
	var nilController *Controller
	nilController.SetAssetReader(nil)
}
