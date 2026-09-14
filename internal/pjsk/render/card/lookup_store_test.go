package card

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func storeLookupSource() *lookupTestSource {
	return &lookupTestSource{card: &masterdata.Card{
		ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Test Card", AssetBundleName: "card_test",
	}}
}

// C1 (T15): original card images are no longer probed for an absolute local
// path. Whether the files exist locally or not, the controller emits the
// Drawing-relative paths; the handler resolves them through AssetReader or the
// public asset host.
func TestResolveCardImagesEmitsDrawingPathsWithoutProbing(t *testing.T) {
	want := []string{
		"asset/jp-assets/startapp/character/member/card_test/card_normal.png",
		"asset/jp-assets/startapp/character/member/card_test/card_after_training.png",
	}
	noRoots := NewController(storeLookupSource(), nil, nil, assets.NewAssetHelper("", nil))
	result, err := noRoots.ResolveCardImages(Query{Query: "1001", Region: "jp"})
	if err != nil || !slices.Equal(result.Paths, want) {
		t.Fatalf("no-root paths = %v, %v", result, err)
	}

	primary := t.TempDir()
	legacyRoot := t.TempDir()
	normal := filepath.Join(primary, "jp-assets/startapp/character/member/card_test/card_normal.png")
	trained := filepath.Join(legacyRoot, "jp-assets/startapp/character/member/card_test/card_after_training.png")
	for _, file := range []string{normal, trained} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("png"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	local := NewController(storeLookupSource(), nil, nil, assets.NewAssetHelper(primary, []string{legacyRoot}))
	result, err = local.ResolveCardImages(Query{Query: "1001", Region: "jp"})
	if err != nil || !slices.Equal(result.Paths, want) {
		t.Fatalf("local-hit paths = %v, %v, want %v", result, err, want)
	}
	// The relative path still reads the legacy-root file through the reader.
	data, resolved, err := assets.NewAssetReader(assets.NewAssetHelper(primary, []string{legacyRoot}), nil).ReadFirst(t.Context(), result.Paths[1])
	if err != nil || string(data) != "png" || resolved != trained {
		t.Fatalf("read relative path = %q, %q, %v", data, resolved, err)
	}
}
