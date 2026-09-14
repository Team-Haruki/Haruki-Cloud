package music

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

const storeChartKey = "jp-assets/startapp/music/music_score/0001_01/expert.txt"

func storeChartSource() *lookupTestSource {
	return &lookupTestSource{
		musics: map[int]*masterdata.Music{1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_test"}},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 27, TotalNoteCount: 777}},
		},
	}
}

func newStoreChartController(store storage.Store) *Controller {
	helper := assets.NewAssetHelper("", nil)
	controller := NewController(storeChartSource(), nil, helper, nil, nil)
	controller.SetAssetReader(assets.NewAssetReader(helper, store))
	return controller
}

func countCalls(memory *storagetest.Memory, method string) int {
	count := 0
	for _, call := range memory.Calls() {
		if call.Method == method {
			count++
		}
	}
	return count
}

func TestChartBPMReadsThroughStoreWithLRU(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:128\n#BPM02:196\n#00008:0100\n#00108:0200")})
	controller := newStoreChartController(memory)

	bpm, err := controller.ResolveMusicBPM(Query{Query: "Song A", Region: "jp", Difficulty: "expert"})
	if err != nil || bpm.Difficulty != "expert" || bpm.MainBPM != 128 || len(bpm.Events) != 2 {
		t.Fatalf("ResolveMusicBPM() = %+v, %v", bpm, err)
	}
	gets := countCalls(memory, "Get")
	if gets == 0 {
		t.Fatal("chart was not read from the store")
	}
	bpm.Events[0].BPM = -1 // callers must not be able to corrupt the cached parse

	matches, err := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 196})
	if err != nil || len(matches) != 1 || matches[0].Events[0].BPM != 128 {
		t.Fatalf("FindMusicChartsByBPM() = %+v, %v", matches, err)
	}
	if detail := controller.resolveMusicDetailBPM("jp", 1, "expert"); detail == nil || *detail != 128 {
		t.Fatalf("resolveMusicDetailBPM() = %v", detail)
	}
	if again := countCalls(memory, "Get"); again != gets {
		t.Fatalf("repeat lookups re-read the store: %d -> %d", gets, again)
	}
}

func TestChartBPMStoreMissAndFailure(t *testing.T) {
	controller := newStoreChartController(storagetest.NewMemory())
	_, err := controller.ResolveMusicBPM(Query{Query: "Song A", Region: "jp", Difficulty: "expert"})
	if err == nil || err.Error() != "当前环境没有可读取的本地谱面文件，无法查询 BPM" {
		t.Fatalf("store miss error = %v", err)
	}
	if detail := controller.resolveMusicDetailBPM("jp", 1, "expert"); detail != nil {
		t.Fatalf("store miss detail = %v", *detail)
	}

	failing := storagetest.NewMemory()
	failing.FailGet = func(storage.Key) error { return errors.New("backend down") }
	failed := newStoreChartController(failing)
	if _, err := failed.ResolveMusicBPM(Query{Query: "Song A", Region: "jp", Difficulty: "expert"}); err == nil || !strings.Contains(err.Error(), "failed to open chart file") {
		t.Fatalf("store failure error = %v", err)
	}
	if matches, err := failed.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 128}); err == nil || matches != nil {
		t.Fatalf("store failure scan = %+v, %v", matches, err)
	}

	broken := storagetest.NewMemory()
	broken.Seed(map[string][]byte{storeChartKey: []byte("no bpm here")})
	if _, err := newStoreChartController(broken).ResolveMusicBPM(Query{Query: "Song A", Region: "jp", Difficulty: "expert"}); err == nil || !strings.Contains(err.Error(), "没有可用") {
		t.Fatalf("unparseable chart error = %v", err)
	}

	// A Disabled store keeps the local probe: nothing under the empty root.
	if _, err := newStoreChartController(storage.Disabled()).ResolveMusicBPM(Query{Query: "Song A", Region: "jp", Difficulty: "expert"}); err == nil {
		t.Fatal("disabled store with no local chart must miss")
	}
}

// C1 (T15): static icons are fixed Drawing paths; nothing is probed, so the
// store sees no Stat for them.
func TestMusicStaticIconsAreNotProbed(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"static_images/jewel.png": []byte("png")})
	controller := newStoreChartController(memory)
	if got := controller.resolveStaticIcon(nil, "jewel.png"); got == nil || *got != "static_images/jewel.png" {
		t.Fatalf("store static icon = %v", got)
	}
	if got := controller.resolveStaticIcon(nil, "shard.png"); got == nil || *got != "static_images/shard.png" {
		t.Fatalf("store static icon miss = %v", got)
	}
	if got := controller.resolveStaticIcon(new(" explicit.png "), "jewel.png"); got == nil || *got != "explicit.png" {
		t.Fatalf("explicit static icon = %v", got)
	}
	if calls := memory.Calls(); len(calls) != 0 {
		t.Fatalf("static icons must not touch the store: %+v", calls)
	}
	var nilController *Controller
	nilController.SetAssetReader(nil)
	if _, found, err := nilController.loadChartBPM(context.TODO(), "jp", 1, "expert"); found || err != nil {
		t.Fatal("nil controller must not find charts")
	}
}

func TestChartBPMCacheLRU(t *testing.T) {
	cache := newChartBPMCache(2, time.Minute)
	now := time.Unix(0, 0)
	cache.now = func() time.Time { return now }
	a, b, c := &parsedChartBPM{MainBPM: 1}, &parsedChartBPM{MainBPM: 2}, &parsedChartBPM{MainBPM: 3}

	cache.put("a", a)
	cache.put("b", b)
	cache.put("a", a) // refresh moves a to the front
	cache.put("c", c) // evicts b
	if _, ok := cache.get("b"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if got, ok := cache.get("a"); !ok || got.MainBPM != 1 || got == a {
		t.Fatalf("get(a) = %+v %v (must be a copy)", got, ok)
	}
	cache.put("nil", nil)
	if _, ok := cache.get("nil"); ok {
		t.Fatal("nil parse must not be cached")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := cache.get("c"); ok {
		t.Fatal("expired entry was returned")
	}
	var nilCache *chartBPMCache
	nilCache.put("a", a)
	if _, ok := nilCache.get("a"); ok {
		t.Fatal("nil cache must miss")
	}
}

func writeMusicAssetTree(t *testing.T, files map[string]string) string {
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

// An unchanged config (asset_dirs set, storage.assets derived from Primary)
// still reads charts that exist only in a legacy root (bare layout), and the
// jacket is the Drawing-relative path (the bare jacket probe is gone, T15).
func TestDerivedAssetsSlotKeepsLegacyMusicLookups(t *testing.T) {
	primary := writeMusicAssetTree(t, map[string]string{
		"static_images/jewel.png": "png",
	})
	legacy := writeMusicAssetTree(t, map[string]string{
		"music/music_score/0001_01/expert.txt": "#BPM01:150\n#00008:01",
	})
	helper := assets.NewAssetHelper(primary, []string{legacy})
	store, err := storage.NewLocal(primary, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := NewController(storeChartSource(), nil, helper, nil, nil)
	after := NewController(storeChartSource(), nil, helper, nil, nil)
	after.SetAssetReader(assets.NewAssetReader(helper, store))

	for name, controller := range map[string]*Controller{"legacy": before, "derived slot": after} {
		if got := controller.resolveStaticIcon(nil, "jewel.png"); got == nil || *got != "static_images/jewel.png" {
			t.Fatalf("%s static icon = %v", name, got)
		}
		bpm, err := controller.ResolveMusicBPM(Query{Query: "Song A", Region: "jp", Difficulty: "expert"})
		if err != nil || bpm.MainBPM != 150 {
			t.Fatalf("%s legacy-root chart = %+v, %v", name, bpm, err)
		}
		if filepath.IsAbs(bpm.JacketPath) || !strings.HasPrefix(bpm.JacketPath, "asset/jp-assets/") {
			t.Fatalf("%s jacket = %q, want a Drawing-relative path", name, bpm.JacketPath)
		}
	}
}

func TestChartScoreCandidatesDropBareLayoutWhenStoreOnly(t *testing.T) {
	local := chartScoreCandidates("", 1, "expert", false)
	storeOnly := chartScoreCandidates("", 1, "expert", true)
	if len(local) != 3 || local[0] != "music/music_score/0001_01/expert.txt" {
		t.Fatalf("local candidates = %v", local)
	}
	if len(storeOnly) != 2 || storeOnly[0] != "asset/"+storeChartKey || storeOnly[1] != "asset/jp-assets/ondemand/music/music_score/0001_01/expert.txt" {
		t.Fatalf("store-only candidates = %v", storeOnly)
	}
}

// A BPM scan over a store caches misses, so a repeated scan issues no GETs.
func TestChartBPMScanCachesMisses(t *testing.T) {
	memory := storagetest.NewMemory()
	controller := newStoreChartController(memory)
	if matches, _ := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 120}); len(matches) != 0 {
		t.Fatalf("first scan = %+v", matches)
	}
	gets := countCalls(memory, "Get")
	if gets != 2 {
		t.Fatalf("store-only scan GETs = %d, want the two regional candidates", gets)
	}
	if matches, _ := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 120}); len(matches) != 0 {
		t.Fatalf("second scan = %+v", matches)
	}
	if again := countCalls(memory, "Get"); again != gets {
		t.Fatalf("cached miss re-read the store: %d -> %d", gets, again)
	}
}

func TestChartBPMCacheMissEntries(t *testing.T) {
	cache := newChartBPMCache(4, time.Hour)
	now := time.Unix(0, 0)
	cache.now = func() time.Time { return now }
	cache.putMiss("gone")
	if parsed, ok := cache.get("gone"); !ok || parsed != nil {
		t.Fatalf("miss entry = %+v %v", parsed, ok)
	}
	now = now.Add(chartBPMCacheMissTTL)
	if _, ok := cache.get("gone"); ok {
		t.Fatal("miss entry outlived its TTL")
	}
	cache.putMiss("gone")
	cache.put("gone", &parsedChartBPM{MainBPM: 9})
	if parsed, ok := cache.get("gone"); !ok || parsed == nil || parsed.MainBPM != 9 {
		t.Fatalf("hit must replace a miss: %+v %v", parsed, ok)
	}
	var nilCache *chartBPMCache
	nilCache.putMiss("x")
}
