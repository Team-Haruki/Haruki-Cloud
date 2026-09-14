package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

// C1 (T15): the inventory icon no longer probes existence. It emits the
// ordered candidate list - the requested region first (today's region hit and
// miss string), the JP fallback last - and Drawing takes the first existing.
// The list is identical whether or not the asset exists on local disk.
func TestInventoryIconPathEmitsRegionThenJPCandidates(t *testing.T) {
	const jpKey = "jp-assets/startapp/thumbnail/common_material/jewel.png"
	root := t.TempDir()
	full := filepath.Join(root, filepath.FromSlash(jpKey))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := NewController(nil, assets.NewAssetHelper(root, nil), nil, renderregion.TW, MasterdataOptions{})
	store := NewController(nil, assets.NewAssetHelper("", nil), nil, renderregion.TW, MasterdataOptions{})

	wantJewel := drawing.AssetKey{
		"asset/tw-assets/startapp/thumbnail/common_material/jewel.png",
		"asset/jp-assets/startapp/thumbnail/common_material/jewel.png",
	}
	wantCoin := drawing.AssetKey{
		"asset/tw-assets/startapp/thumbnail/common_material/coin.png",
		"asset/jp-assets/startapp/thumbnail/common_material/coin.png",
	}
	for name, controller := range map[string]*Controller{"local-hit": legacy, "no-roots": store} {
		if got := controller.inventoryIconPath(renderregion.TW, "jewel", 0); !slices.Equal(got, wantJewel) {
			t.Fatalf("%s jewel = %v", name, got)
		}
		if got := controller.inventoryIconPath(renderregion.TW, "coin", 0); !slices.Equal(got, wantCoin) {
			t.Fatalf("%s coin = %v", name, got)
		}
		// JP collapses to one path: the wire shape stays a plain string (C2).
		jp := controller.inventoryIconPath(renderregion.JP, "coin", 0)
		body, err := json.Marshal(drawing.InventoryItem{IconPath: jp})
		if err != nil || !slices.Equal(jp, drawing.AssetKey{wantCoin[1]}) {
			t.Fatalf("%s jp coin = %v (%v)", name, jp, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil || decoded["icon_path"] != wantCoin[1] {
			t.Fatalf("%s jp wire = %s (%v)", name, body, err)
		}
	}
	var nilController *Controller
	if got := nilController.resolveInventoryAssetPath(renderregion.Value(""), "thumbnail/x.png"); got.First() != "asset/jp-assets/startapp/thumbnail/x.png" || len(got) != 1 {
		t.Fatalf("nil controller path = %v", got)
	}
}
