package card

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func storeLookupSource() *lookupTestSource {
	return &lookupTestSource{card: &masterdata.Card{
		ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Test Card", AssetBundleName: "card_test",
	}}
}

func newStoreLookupController(t *testing.T, store storage.Store) *Controller {
	t.Helper()
	source := storeLookupSource()
	helper := assets.NewAssetHelper("", nil)
	controller := NewController(source, nil, nil, helper)
	controller.SetAssetReader(assets.NewAssetReader(helper, store))
	return controller
}

// With a store the probe answers through AssetReader.Stat and, having no local
// path, emits exactly the Drawing path the local miss branch emits (C2).
func TestResolveCardImagesThroughStoreKeepsDrawingPaths(t *testing.T) {
	want := []string{
		"asset/jp-assets/startapp/character/member/card_test/card_normal.png",
		"asset/jp-assets/startapp/character/member/card_test/card_after_training.png",
	}
	legacy := NewController(storeLookupSource(), nil, nil, assets.NewAssetHelper("", nil))
	legacyResult, err := legacy.ResolveCardImages(Query{Query: "1001", Region: "jp"})
	if err != nil || !slices.Equal(legacyResult.Paths, want) {
		t.Fatalf("legacy miss paths = %v, %v", legacyResult, err)
	}

	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/character/member/card_test/card_normal.png": []byte("png")})
	result, err := newStoreLookupController(t, memory).ResolveCardImages(Query{Query: "1001", Region: "jp"})
	if err != nil || !slices.Equal(result.Paths, want) {
		t.Fatalf("store paths = %v, %v", result, err)
	}
	stats := 0
	for _, call := range memory.Calls() {
		if call.Method == "Stat" {
			stats++
		}
	}
	if stats == 0 {
		t.Fatalf("store was not probed: %+v", memory.Calls())
	}

	failing := storagetest.NewMemory()
	failing.FailStat = func(storage.Key) error { return errors.New("boom") }
	failed, err := newStoreLookupController(t, failing).ResolveCardImages(Query{Query: "1001", Region: "jp"})
	if err != nil || !slices.Equal(failed.Paths, want) {
		t.Fatalf("failing store paths = %v, %v", failed, err)
	}

	var nilController *Controller
	nilController.SetAssetReader(nil)
}

// An unchanged config (asset_dirs.primary + legacy, storage.assets derived
// from Primary) keeps 664281d1's hit strings, including a legacy-root hit.
func TestResolveCardImagesDerivedSlotKeepsLocalHitPaths(t *testing.T) {
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
	want := []string{filepath.ToSlash(normal), filepath.ToSlash(trained)}
	helper := assets.NewAssetHelper(primary, []string{legacyRoot})

	legacy := NewController(storeLookupSource(), nil, nil, helper)
	legacyResult, err := legacy.ResolveCardImages(Query{Query: "1001", Region: "jp"})
	if err != nil || !slices.Equal(legacyResult.Paths, want) {
		t.Fatalf("legacy hit paths = %v, %v", legacyResult, err)
	}
	store, err := storage.NewLocal(primary, 0)
	if err != nil {
		t.Fatal(err)
	}
	derived := NewController(storeLookupSource(), nil, nil, helper)
	derived.SetAssetReader(assets.NewAssetReader(helper, store))
	result, err := derived.ResolveCardImages(Query{Query: "1001", Region: "jp"})
	if err != nil || !slices.Equal(result.Paths, want) {
		t.Fatalf("derived slot paths = %v, %v, want %v", result, err, want)
	}
}
