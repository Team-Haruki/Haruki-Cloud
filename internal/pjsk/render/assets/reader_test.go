package assets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func writeAssetTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// parityReaders returns a legacy reader (helper root, Disabled store) and a
// store reader (the same tree as an fs root) over one directory tree.
func parityReaders(t *testing.T, files map[string]string) (legacy, store *AssetReader, root string) {
	t.Helper()
	root = writeAssetTree(t, files)
	local, err := storage.NewLocal(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	return NewAssetReader(NewAssetHelper(root, nil), storage.Disabled()),
		NewAssetReader(NewAssetHelper("", nil), local), root
}

var parityTree = map[string]string{
	"jp-assets/startapp/character/member/card_after.png": "after",
	"jp-assets/startapp/music/jacket/j001/j001.png":      "jacket",
	"cn-assets/ondemand/honor/frame/frame.png":           "frame",
	"static_images/pjsk_3d_preview/1.png":                "preview",
}

func TestAssetReaderReadFirstParity(t *testing.T) {
	legacy, store, root := parityReaders(t, parityTree)
	ctx := context.Background()
	cases := [][]string{
		{"asset/jp-assets/startapp/character/member/card_after.png"},
		{"asset/jp-assets/startapp/missing.png", "asset/jp-assets/startapp/music/jacket/j001/j001.png"},
		{"", "cn-assets/ondemand/honor/frame/frame.png"},
		{"static_images/pjsk_3d_preview/1.png"},
	}
	for _, candidates := range cases {
		legacyData, legacyResolved, legacyErr := legacy.ReadFirst(ctx, candidates...)
		storeData, storeResolved, storeErr := store.ReadFirst(ctx, candidates...)
		if legacyErr != nil || storeErr != nil || string(legacyData) != string(storeData) {
			t.Fatalf("%v: legacy=%q,%v store=%q,%v", candidates, legacyData, legacyErr, storeData, storeErr)
		}
		key, _ := ObjectKey(storeResolved)
		if storeResolved != string(key) || filepath.ToSlash(filepath.Join(root, storeResolved)) != legacyResolved {
			t.Fatalf("%v: resolved legacy=%q store=%q", candidates, legacyResolved, storeResolved)
		}
	}
	for _, reader := range []*AssetReader{legacy, store} {
		if data, resolved, err := reader.ReadFirst(ctx, "asset/jp-assets/none.png", "../escape.png"); !errors.Is(err, storage.ErrNotExist) || data != nil || resolved != "" {
			t.Fatalf("all missing = %q %q %v", data, resolved, err)
		}
	}
}

// The legacy branch must be byte-for-byte FirstExisting + os.ReadFile,
// including the case-insensitive fallback a plain store.Get does not have.
func TestAssetReaderLegacyMatchesFirstExisting(t *testing.T) {
	root := writeAssetTree(t, map[string]string{"jp-assets/StartApp/Stamp/S1.png": "stamp"})
	helper := NewAssetHelper(root, nil)
	reader := NewAssetReader(helper, nil)
	path := "asset/jp-assets/startapp/stamp/s1.png"
	want, err := os.ReadFile(helper.FirstExisting(path))
	if err != nil {
		t.Fatal(err)
	}
	//lint:ignore SA1012 the reader normalises a nil context
	got, resolved, err := reader.ReadFirst(nil, path)
	if err != nil || string(got) != string(want) || resolved != helper.FirstExisting(path) {
		t.Fatalf("ReadFirst = %q %q %v", got, resolved, err)
	}
}

func TestAssetReaderLegacyReadError(t *testing.T) {
	root := writeAssetTree(t, map[string]string{"jp-assets/dir/file.png": "x"})
	reader := NewAssetReader(NewAssetHelper(root, nil), storage.Disabled())
	if _, _, err := reader.ReadFirst(context.Background(), "jp-assets/dir"); err == nil || errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("reading a directory must surface the read error, got %v", err)
	}
}

func TestAssetReaderNilHelperAndNilReader(t *testing.T) {
	var nilReader *AssetReader
	if _, _, err := nilReader.ReadFirst(context.Background(), "jp-assets/x"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("nil reader ReadFirst = %v", err)
	}
	if _, _, err := NewAssetReader(nil, nil).ReadFirst(context.Background(), "jp-assets/x"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("nil helper ReadFirst = %v", err)
	}
}

func TestAssetReaderStoreErrors(t *testing.T) {
	boom := errors.New("backend down")
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/a.png": []byte("a")})
	memory.FailGet = func(storage.Key) error { return boom }
	reader := NewAssetReader(nil, memory)
	ctx := context.Background()

	if _, _, err := reader.ReadFirst(ctx, "jp-assets/a.png"); !errors.Is(err, boom) {
		t.Fatalf("ReadFirst must return a non-NotExist store error, got %v", err)
	}
	memory.FailGet = nil
	if data, _, err := reader.ReadFirst(ctx, "jp-assets/a.png"); err != nil || string(data) != "a" {
		t.Fatalf("a transient Get error must not stick: %q %v", data, err)
	}
}
