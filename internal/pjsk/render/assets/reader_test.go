package assets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if _, ok := nilReader.Stat(context.Background(), "jp-assets/x"); ok {
		t.Fatal("nil reader Stat")
	}
	if _, _, err := nilReader.ReadFirst(context.Background(), "jp-assets/x"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("nil reader ReadFirst = %v", err)
	}
	reader := NewAssetReader(nil, nil)
	if reader.Exists(context.Background(), "jp-assets/x") {
		t.Fatal("nil helper Exists")
	}
}

func TestAssetReaderStatParity(t *testing.T) {
	legacy, store, root := parityReaders(t, parityTree)
	ctx := context.Background()
	cases := []struct {
		candidates []string
		wantKey    string
	}{
		{[]string{"asset/jp-assets/startapp/character/member/card_after.png"}, "jp-assets/startapp/character/member/card_after.png"},
		{[]string{"asset/jp-assets/startapp/nope.png", "cn-assets/ondemand/honor/frame/frame.png"}, "cn-assets/ondemand/honor/frame/frame.png"},
		{[]string{"asset/jp-assets/startapp/nope.png"}, ""},
		{[]string{"", "../x"}, ""},
	}
	for _, tc := range cases {
		legacyResolved, legacyOK := legacy.Stat(ctx, tc.candidates...)
		storeResolved, storeOK := store.Stat(ctx, tc.candidates...)
		if legacyOK != storeOK || storeResolved != tc.wantKey {
			t.Fatalf("%v: legacy=%q,%v store=%q,%v", tc.candidates, legacyResolved, legacyOK, storeResolved, storeOK)
		}
		if legacyOK && legacyResolved != filepath.ToSlash(filepath.Join(root, tc.wantKey)) {
			t.Fatalf("%v: legacy resolved %q", tc.candidates, legacyResolved)
		}
		if legacy.Exists(ctx, tc.candidates[0]) != store.Exists(ctx, tc.candidates[0]) {
			t.Fatalf("%v: Exists disagrees", tc.candidates)
		}
	}
}

func TestAssetReaderLegacyStatVanishedFile(t *testing.T) {
	root := writeAssetTree(t, map[string]string{"jp-assets/a.png": "a"})
	helper := NewAssetHelper(root, nil)
	reader := NewAssetReader(helper, storage.Disabled())
	if !reader.Exists(context.Background(), "jp-assets/a.png") {
		t.Fatal("expected hit")
	}
	if err := os.Remove(filepath.Join(root, "jp-assets", "a.png")); err != nil {
		t.Fatal(err)
	}
	// FirstExisting keeps its positive resolution for a short TTL; os.Stat is
	// the final word, exactly as the probe sites check today.
	if reader.Exists(context.Background(), "jp-assets/a.png") {
		t.Fatal("a vanished file must not be reported")
	}
}

func TestAssetReaderStoreStatMemo(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/hit.png": []byte("hit")})
	reader := NewAssetReader(nil, memory)
	now := time.Unix(1_700_000_000, 0)
	reader.memo.now = func() time.Time { return now }
	ctx := context.Background()

	statCalls := func() int {
		n := 0
		for _, op := range memory.Calls() {
			if op.Method == "Stat" {
				n++
			}
		}
		return n
	}

	for range 3 {
		if !reader.Exists(ctx, "asset/jp-assets/hit.png") || reader.Exists(ctx, "asset/jp-assets/late.png") {
			t.Fatal("unexpected Stat answers")
		}
	}
	if got := statCalls(); got != 2 {
		t.Fatalf("memo hit/miss must avoid repeat HEADs, Stat calls = %d", got)
	}

	// A later object shows up; a successful read records the hit, so the memo
	// never keeps turning it into a miss.
	if err := memory.Put(ctx, "jp-assets/late.png", []byte("late"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if data, resolved, err := reader.ReadFirst(ctx, "asset/jp-assets/late.png"); err != nil || string(data) != "late" || resolved != "jp-assets/late.png" {
		t.Fatalf("ReadFirst = %q %q %v", data, resolved, err)
	}
	if !reader.Exists(ctx, "asset/jp-assets/late.png") {
		t.Fatal("memo turned a later hit into a miss")
	}

	// Expiry re-probes the store.
	if err := memory.Delete(ctx, "jp-assets/hit.png"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(assetStatMemoTTL)
	if reader.Exists(ctx, "asset/jp-assets/hit.png") {
		t.Fatal("expired memo entry must be re-probed")
	}
}

func TestAssetReaderStoreErrors(t *testing.T) {
	boom := errors.New("backend down")
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/a.png": []byte("a")})
	memory.FailGet = func(storage.Key) error { return boom }
	memory.FailStat = func(storage.Key) error { return boom }
	reader := NewAssetReader(nil, memory)
	ctx := context.Background()

	if _, _, err := reader.ReadFirst(ctx, "jp-assets/a.png"); !errors.Is(err, boom) {
		t.Fatalf("ReadFirst must return a non-NotExist store error, got %v", err)
	}
	if reader.Exists(ctx, "jp-assets/a.png") {
		t.Fatal("a failing Stat is a miss")
	}
	memory.FailStat = nil
	if !reader.Exists(ctx, "jp-assets/a.png") {
		t.Fatal("a transient Stat error must not be memoised")
	}
}

func TestAssetStatMemoBound(t *testing.T) {
	memo := newAssetStatMemo(time.Minute, 2)
	now := time.Unix(0, 0)
	memo.now = func() time.Time { return now }
	memo.store("a", true)
	now = now.Add(2 * time.Minute)
	memo.store("b", false)
	memo.store("c", true) // evicts expired "a"
	if _, cached := memo.lookup("a"); cached {
		t.Fatal("expired entry survived")
	}
	if exists, cached := memo.lookup("b"); !cached || exists {
		t.Fatal("b lost")
	}
	memo.store("d", true) // full with live entries: cleared
	if len(memo.entries) != 1 {
		t.Fatalf("memo size = %d", len(memo.entries))
	}
	memo.store("d", false) // overwrite does not evict
	if exists, cached := memo.lookup("d"); !cached || exists {
		t.Fatal("overwrite failed")
	}
}
