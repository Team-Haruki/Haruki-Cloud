package assets

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

// derivedSlotReaders builds, for one unchanged config (asset_dirs.primary and
// asset_dirs.legacy set, storage.assets unset), the pre-T6 reader (helper
// only) and the T6 reader over the fs store storage.BuildSet derives from
// Primary.
func derivedSlotReaders(t *testing.T, primary, legacy string) (helper *AssetHelper, before, after *AssetReader) {
	t.Helper()
	helper = NewAssetHelper(primary, []string{legacy})
	store, err := storage.NewLocal(primary, 0)
	if err != nil {
		t.Fatal(err)
	}
	return helper, NewAssetReader(helper, storage.Disabled()), NewAssetReader(helper, store)
}

// With an unchanged config the reads must resolve exactly what 664281d1's
// AssetHelper resolved: the absolute local path of a Primary hit, of a hit
// that exists only in a legacy root, and of a case-mismatched hit.
func TestDerivedSlotKeepsLegacyHitStrings(t *testing.T) {
	primary := writeAssetTree(t, map[string]string{
		"music/jacket/jacket_s_001/jacket_s_001.png": "primary",
		"jp-assets/startapp/Stamp/Stamp0001.png":     "cased",
	})
	legacy := writeAssetTree(t, map[string]string{
		"jp-assets/startapp/honor/legacy_only.png": "legacy",
	})
	helper, before, after := derivedSlotReaders(t, primary, legacy)
	ctx := context.Background()

	absoluteJacket := filepath.ToSlash(filepath.Join(primary, "music/jacket/jacket_s_001/jacket_s_001.png"))
	legacyHit := filepath.ToSlash(filepath.Join(legacy, "jp-assets/startapp/honor/legacy_only.png"))
	cases := map[string]string{
		"primary hit":   "music/jacket/jacket_s_001/jacket_s_001.png",
		"legacy hit":    "asset/jp-assets/startapp/honor/legacy_only.png",
		"case mismatch": "jp-assets/startapp/stamp/stamp0001.png",
	}
	for name, path := range cases {
		want := helper.FirstExisting(path)
		if want == "" {
			t.Fatalf("%s: legacy resolution missed", name)
		}
		wantData, _, err := before.ReadFirst(ctx, path)
		if err != nil {
			t.Fatalf("%s: legacy read: %v", name, err)
		}
		gotData, resolved, err := after.ReadFirst(ctx, path)
		if err != nil || string(gotData) != string(wantData) || resolved != want {
			t.Fatalf("%s: ReadFirst = %q %q %v, want %q %q", name, gotData, resolved, err, wantData, want)
		}
	}
	if got := helper.FirstExisting(cases["primary hit"]); got != absoluteJacket {
		t.Fatalf("jacket hit string = %q, want the absolute path %q", got, absoluteJacket)
	}
	if got := helper.FirstExisting(cases["legacy hit"]); got != legacyHit {
		t.Fatalf("legacy hit string = %q, want %q", got, legacyHit)
	}
	if after.StoreOnly() || before.StoreOnly() {
		t.Fatal("a reader with local roots is not store-only")
	}

	// Misses stay misses on both branches.
	if _, _, err := after.ReadFirst(ctx, "jp-assets/startapp/missing.png"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("ReadFirst miss err = %v", err)
	}
}

// The store still answers what the local roots cannot see, and once no local
// root is left (E1) the reader is store-only.
func TestStoreFallbackBehindLocalRoots(t *testing.T) {
	root := writeAssetTree(t, map[string]string{"jp-assets/startapp/local.png": "local"})
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/remote.png": []byte("remote")})
	reader := NewAssetReader(NewAssetHelper(root, nil), memory)
	ctx := context.Background()

	if data, resolved, err := reader.ReadFirst(ctx, "asset/jp-assets/startapp/remote.png"); err != nil || string(data) != "remote" || resolved != "jp-assets/startapp/remote.png" {
		t.Fatalf("store fallback read = %q %q %v", data, resolved, err)
	}
	if countStoreCalls(memory, "Get") != 1 {
		t.Fatalf("local hit must not reach the store: %+v", memory.Calls())
	}
	if _, _, err := reader.ReadFirst(ctx, "jp-assets/startapp/local.png"); err != nil {
		t.Fatal(err)
	}
	if countStoreCalls(memory, "Get") != 1 {
		t.Fatalf("local hit reached the store: %+v", memory.Calls())
	}

	storeOnly := NewAssetReader(NewAssetHelper("", nil), memory)
	if !storeOnly.StoreOnly() || NewAssetReader(NewAssetHelper(root, nil), storage.Disabled()).StoreOnly() {
		t.Fatal("StoreOnly misreports")
	}
	var nilReader *AssetReader
	if nilReader.StoreOnly() || helperHasLocalRoots(nil) || helperHasLocalRoots(&AssetHelper{}) {
		t.Fatal("nil reader / helper has no local roots")
	}
}

func countStoreCalls(memory *storagetest.Memory, method string) int {
	count := 0
	for _, call := range memory.Calls() {
		if call.Method == method {
			count++
		}
	}
	return count
}
