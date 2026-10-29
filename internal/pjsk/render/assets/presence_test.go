package assets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestResolveRegionAssetPresence(t *testing.T) {
	thumbnail := filepath.Join("mysekai", "thumbnail", "fixture", "chair_1.png")
	missing := filepath.Join("mysekai", "thumbnail", "fixture", "nope_1.png")
	const (
		storedPath  = "asset/tw-assets/ondemand/mysekai/thumbnail/fixture/chair_1.png"
		missingPath = "asset/tw-assets/ondemand/mysekai/thumbnail/fixture/nope_1.png"
	)

	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"tw-assets/ondemand/mysekai/thumbnail/fixture/chair_1.png": []byte("x")})
	store := storeOnlyHelper(t, memory, StoreProbeConfig{})
	if path, presence := ResolveRegionAssetPresence(store, "tw", thumbnail); path != storedPath || presence != AssetPresent {
		t.Fatalf("stored = %q, %v", path, presence)
	}
	if path, presence := ResolveRegionAssetPresence(store, "tw", missing); path != missingPath || presence != AssetMissing {
		t.Fatalf("store miss = %q, %v", path, presence)
	}

	// A failing store proves nothing; the path is the usual first candidate.
	broken := storagetest.NewMemory()
	broken.FailListDir = func(storage.Key) error { return errors.New("down") }
	broken.FailList = broken.FailListDir
	broken.FailStat = broken.FailListDir
	if path, presence := ResolveRegionAssetPresence(storeOnlyHelper(t, broken, StoreProbeConfig{}), "tw", missing); path != missingPath || presence != AssetPresenceUnknown {
		t.Fatalf("failing store = %q, %v", path, presence)
	}

	// A later candidate's failure also leaves the answer open: the ondemand
	// miss alone does not prove the startapp copy is absent.
	partial := storagetest.NewMemory()
	partial.Seed(map[string][]byte{
		"tw-assets/ondemand/mysekai/other.png": []byte("x"),
		"tw-assets/startapp/other.png":         []byte("x"),
	})
	failStartapp := func(key storage.Key) error {
		if strings.HasPrefix(string(key), "tw-assets/startapp") {
			return errors.New("down")
		}
		return nil
	}
	partial.FailListDir, partial.FailList, partial.FailStat = failStartapp, failStartapp, failStartapp
	if path, presence := ResolveRegionAssetPresence(storeOnlyHelper(t, partial, StoreProbeConfig{}), "tw", missing); path != missingPath || presence != AssetPresenceUnknown {
		t.Fatalf("later candidate failing = %q, %v", path, presence)
	}

	// Without a store only a configured filesystem root answers.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tw-assets", "ondemand", "mysekai", "thumbnail", "fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tw-assets", "ondemand", "mysekai", "thumbnail", "fixture", "chair_1.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	local := NewAssetHelper(root, nil)
	if path, presence := ResolveRegionAssetPresence(local, "tw", thumbnail); path != storedPath || presence != AssetPresent {
		t.Fatalf("local = %q, %v", path, presence)
	}
	if path, presence := ResolveRegionAssetPresence(local, "tw", missing); path != missingPath || presence != AssetMissing {
		t.Fatalf("local miss = %q, %v", path, presence)
	}
	if path, presence := ResolveRegionAssetPresence(NewAssetHelper("", nil), "tw", missing); path != missingPath || presence != AssetPresenceUnknown {
		t.Fatalf("rootless, storeless = %q, %v", path, presence)
	}
	if path, presence := ResolveRegionAssetPresence(NewAssetHelper("https://assets.example", nil), "tw", missing); path != missingPath || presence != AssetPresenceUnknown {
		t.Fatalf("URL root = %q, %v", path, presence)
	}
	// A URL root is never checked, so a local miss beside it proves nothing.
	if path, presence := ResolveRegionAssetPresence(NewAssetHelper(root, []string{"https://assets.example"}), "tw", missing); path != missingPath || presence != AssetPresenceUnknown {
		t.Fatalf("filesystem and URL roots = %q, %v", path, presence)
	}
	// Only not-exist counts as a miss; here a file stands where a directory
	// should be.
	blocked := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocked, "tw-assets"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if path, presence := ResolveRegionAssetPresence(NewAssetHelper(blocked, nil), "tw", missing); path != missingPath || presence != AssetPresenceUnknown {
		t.Fatalf("stat error = %q, %v", path, presence)
	}
	if path, presence := ResolveRegionAssetPresence(nil, "tw", missing); path != missingPath || presence != AssetPresenceUnknown {
		t.Fatalf("nil helper = %q, %v", path, presence)
	}
	if path, presence := ResolveRegionAssetPresence(local, "tw"); path != "" || presence != AssetPresenceUnknown {
		t.Fatalf("no paths = %q, %v", path, presence)
	}
}
