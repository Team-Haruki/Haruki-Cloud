package music

import (
	"context"
	"errors"
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

func TestMusicProbesThroughStoreKeepDrawingPaths(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{
		"music/jacket/jacket_test/jacket_test.png": []byte("png"),
		"static_images/jewel.png":                  []byte("png"),
	})
	controller := newStoreChartController(memory)
	if got := controller.resolveLocalMusicJacket("jacket_test"); got != "" {
		t.Fatalf("store jacket hit must not emit a local path, got %q", got)
	}
	if got := controller.resolveStaticIcon(nil, "jewel.png"); got == nil || *got != "static_images/jewel.png" {
		t.Fatalf("store static icon = %v", got)
	}
	if got := controller.resolveStaticIcon(nil, "shard.png"); got == nil || *got != "static_images/shard.png" {
		t.Fatalf("store static icon miss = %v", got)
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
