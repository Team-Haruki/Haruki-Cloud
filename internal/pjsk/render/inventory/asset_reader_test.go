package inventory

import (
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/storage/storagetest"
)

// The inventory probe emits the same Drawing path through a store as through
// the local helper: the region hit, then the JP fallback, then the region miss.
func TestResolveInventoryAssetPathStoreParity(t *testing.T) {
	const jpKey = "jp-assets/startapp/thumbnail/common_material/jewel.png"
	root := t.TempDir()
	full := filepath.Join(root, filepath.FromSlash(jpKey))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{jpKey: []byte("png")})

	legacy := NewController(nil, assets.NewAssetHelper(root, nil), nil, renderregion.TW, MasterdataOptions{})
	store := NewController(nil, assets.NewAssetHelper("", nil), nil, renderregion.TW, MasterdataOptions{})
	store.SetAssetReader(assets.NewAssetReader(nil, memory))

	for name, controller := range map[string]*Controller{"legacy": legacy, "store": store} {
		if got := controller.inventoryIconPath(renderregion.TW, "jewel", 0); got != "asset/jp-assets/startapp/thumbnail/common_material/jewel.png" {
			t.Fatalf("%s jewel = %q", name, got)
		}
		if got := controller.inventoryIconPath(renderregion.TW, "coin", 0); got != "asset/tw-assets/startapp/thumbnail/common_material/coin.png" {
			t.Fatalf("%s coin = %q", name, got)
		}
	}
	var nilController *Controller
	nilController.SetAssetReader(nil)
}
