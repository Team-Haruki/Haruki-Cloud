package music

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type mutableBPMIndexSource struct {
	mu       sync.Mutex
	revision string
	key      storage.Key
}

func (s *mutableBPMIndexSource) BPMIndex(string) (string, storage.Key, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision, s.key, s.key != ""
}
func (s *mutableBPMIndexSource) set(revision string, key storage.Key) {
	s.mu.Lock()
	s.revision = revision
	s.key = key
	s.mu.Unlock()
}

func publishTestBPMIndex(t *testing.T, store storage.Store, revision string) (*BPMIndex, storage.Key) {
	t.Helper()
	index, err := BuildBPMIndex(context.Background(), store, "jp", revision)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := PublishBPMIndex(context.Background(), store, index)
	if err != nil {
		t.Fatal(err)
	}
	return index, key
}

func TestBPMIndexBuildPublishAndReadUsesOneIndexGet(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{
		storeChartKey: []byte("#BPM01:128\n#00008:01"),
		"jp-assets/ondemand/music/music_score/0001_01/expert.txt": []byte("#BPM01:180\n#00008:01"),
		"jp-assets/ondemand/music/music_score/0001_01/master.txt": []byte("#BPM01:196\n#00008:01"),
		"jp-assets/startapp/music/music_score/0001_02/expert.txt": []byte("not a candidate"),
	})
	index, key := publishTestBPMIndex(t, store, "r1")
	if len(index.Charts) != 3 {
		t.Fatalf("indexed %d charts", len(index.Charts))
	}
	puts := countCalls(store, "Put")
	again, _, err := PublishBPMIndex(context.Background(), store, index)
	if err != nil || again != key || countCalls(store, "Put") != puts {
		t.Fatalf("immutable republish = %s, %v", again, err)
	}
	source := &mutableBPMIndexSource{revision: "r1", key: key}
	controller := newStoreChartController(store)
	controller.SetBPMIndexSource(source, store)
	before := countCalls(store, "Get")
	for _, tc := range []struct {
		difficulty string
		want       float64
		found      bool
	}{{"expert", 128, true}, {"master", 196, true}, {"append", 0, false}} {
		chart, found, err := controller.loadChartBPM(context.Background(), "jp", 1, tc.difficulty)
		if err != nil || found != tc.found || (found && chart.MainBPM != tc.want) {
			t.Fatalf("%s = %+v,%v,%v", tc.difficulty, chart, found, err)
		}
		if found {
			chart.Events[0].BPM = -1
		}
	}
	chart, _, err := controller.loadChartBPM(context.Background(), "jp", 1, "expert")
	if err != nil || chart.Events[0].BPM != 128 {
		t.Fatalf("index was mutated: %+v %v", chart, err)
	}
	if gets := countCalls(store, "Get") - before; gets != 1 {
		t.Fatalf("index-backed lookups GETs=%d,want 1", gets)
	}
}

func TestBPMIndexRevisionSwitchAndFallbackRevision(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:128\n#00008:01")})
	_, first := publishTestBPMIndex(t, store, "r1")
	source := &mutableBPMIndexSource{revision: "r1", key: first}
	controller := newStoreChartController(store)
	controller.SetBPMIndexSource(source, store)
	load := func(want float64) {
		t.Helper()
		chart, found, err := controller.loadChartBPM(context.Background(), "jp", 1, "expert")
		if err != nil || !found || chart.MainBPM != want {
			t.Fatalf("lookup=%+v,%v,%v", chart, found, err)
		}
	}
	load(128)
	store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:196\n#00008:01")})
	_, second := publishTestBPMIndex(t, store, "r2")
	source.set("r2", second)
	load(196)
	source.set("r3", "")
	load(196)
	store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:220\n#00008:01")})
	source.set("r4", "")
	load(220)
}

func TestBPMIndexInvalidFallsBackWithoutCachingChartError(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:128\n#00008:01")})
	index, key := publishTestBPMIndex(t, store, "r1")
	// Deliberately wrong bytes under the content-addressed key must be rejected.
	store.Seed(map[string][]byte{string(key): []byte("{}")})
	source := &mutableBPMIndexSource{revision: "r1", key: key}
	controller := newStoreChartController(store)
	controller.SetBPMIndexSource(source, store)
	var fail atomic.Bool
	fail.Store(true)
	store.FailGet = func(key storage.Key) error {
		if key == storeChartKey && fail.Load() {
			return errors.New("backend down")
		}
		return nil
	}
	before := countCalls(store, "Get")
	if _, _, err := controller.loadChartBPM(context.Background(), "jp", 1, "expert"); err == nil {
		t.Fatal("expected chart read failure")
	}
	fail.Store(false)
	chart, found, err := controller.loadChartBPM(context.Background(), "jp", 1, "expert")
	if err != nil || !found || chart.MainBPM != 128 {
		t.Fatalf("transient error cached as miss: %+v %v %v", chart, found, err)
	}
	if gets := countCalls(store, "Get") - before; gets != 3 {
		t.Fatalf("GETs=%d,want one bad index and two chart attempts", gets)
	}
	index.Complete = false
	data, err := jsonutil.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	badKey := storage.Key("indexes/bpm/v1/jp/" + hex.EncodeToString(digest[:]) + ".json")
	if _, err := decodeBPMIndex(data, "jp", "r1", badKey); err == nil {
		t.Fatal("incomplete index accepted")
	}
	index.Complete = true
	index.Prefixes = index.Prefixes[:1]
	if _, _, err := PublishBPMIndex(context.Background(), store, index); err == nil {
		t.Fatal("partial prefix coverage published")
	}
}

func TestBPMIndexBuildDoesNotPublishPartialScans(t *testing.T) {
	for _, kind := range []string{"list", "get", "parse"} {
		t.Run(kind, func(t *testing.T) {
			store := storagetest.NewMemory()
			store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:128\n#00008:01")})
			switch kind {
			case "list":
				store.FailList = func(storage.Key) error { return errors.New("partial listing") }
			case "get":
				store.FailGet = func(storage.Key) error { return errors.New("read failed") }
			case "parse":
				store.Seed(map[string][]byte{storeChartKey: []byte("invalid chart")})
			}
			if index, err := BuildBPMIndex(context.Background(), store, "jp", "r1"); err == nil || index != nil {
				t.Fatalf("partial build=%+v,%v", index, err)
			}
			if countCalls(store, "Put") != 0 {
				t.Fatal("failed build published an object")
			}
		})
	}
}

func TestChartBPMColdReadersShareGetAndCancelIndependently(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:128\n#00008:01")})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	store.FailGet = func(storage.Key) error { once.Do(func() { close(entered) }); <-release; return nil }
	controller := newStoreChartController(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	canceled := make(chan error, 1)
	go func() { _, _, err := controller.loadChartBPM(ctx, "jp", 1, "expert"); canceled <- err }()
	<-entered
	const readers = 20
	var wg sync.WaitGroup
	errs := make([]error, readers)
	for i := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			chart, found, err := controller.loadChartBPM(context.Background(), "jp", 1, "expert")
			if err == nil && (!found || chart.MainBPM != 128) {
				err = errors.New("wrong chart")
			}
			errs[i] = err
		}()
	}
	cancel()
	select {
	case err := <-canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter stayed blocked")
	}
	close(release)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if gets := countCalls(store, "Get"); gets != 1 {
		t.Fatalf("concurrent cold GETs=%d,want 1", gets)
	}
}

func TestBPMFallbackScanBoundedAndIndexScanAvoidsChartGets(t *testing.T) {
	const charts = 32
	store := storagetest.NewMemory()
	seeds := map[string][]byte{}
	source := &lookupTestSource{musics: map[int]*masterdata.Music{}, difficulties: map[int][]*masterdata.MusicDifficulty{}}
	for i := 1; i <= charts; i++ {
		source.musics[i] = &masterdata.Music{ID: i, Title: fmt.Sprintf("song%d", i)}
		source.difficulties[i] = []*masterdata.MusicDifficulty{{MusicID: i, MusicDifficulty: "expert"}}
		seeds[fmt.Sprintf("jp-assets/startapp/music/music_score/%04d_01/expert.txt", i)] = []byte("#BPM01:128\n#00008:01")
	}
	store.Seed(seeds)
	var active, peak atomic.Int64
	store.FailGet = func(storage.Key) error {
		n := active.Add(1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return nil
	}
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	controller.SetAssetReader(assets.NewAssetReader(nil, store))
	started := time.Now()
	matches, err := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 128})
	if err != nil || len(matches) != charts {
		t.Fatalf("fallback matches=%d,error=%v", len(matches), err)
	}
	for i, item := range matches {
		if item.Music.ID != i+1 {
			t.Fatalf("unordered match %d: %+v", i, item)
		}
	}
	if peak.Load() < 2 || peak.Load() > 8 {
		t.Fatalf("fallback peak=%d", peak.Load())
	}
	t.Logf("fallback %d charts: %s, GET=%d, peak=%d", charts, time.Since(started), countCalls(store, "Get"), peak.Load())
	before := countCalls(store, "Get")
	if _, err := controller.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 128}); err != nil {
		t.Fatal(err)
	}
	if countCalls(store, "Get") != before {
		t.Fatal("warm fallback repeated downloads")
	}
	_, key := publishTestBPMIndex(t, store, "r1")
	indexed := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	indexed.SetAssetReader(assets.NewAssetReader(nil, store))
	indexed.SetBPMIndexSource(&mutableBPMIndexSource{revision: "r1", key: key}, store)
	before = countCalls(store, "Get")
	started = time.Now()
	matches, err = indexed.FindMusicChartsByBPM(BPMQuery{Region: "jp", BPM: 128})
	if err != nil || len(matches) != charts {
		t.Fatalf("index matches=%d,error=%v", len(matches), err)
	}
	if gets := countCalls(store, "Get") - before; gets != 1 {
		t.Fatalf("indexed scan GETs=%d,want 1 index object", gets)
	}
	t.Logf("index %d charts: %s, GET=%d (one index, zero chart downloads)", charts, time.Since(started), countCalls(store, "Get")-before)
}

func TestBPMCacheRetainsFiveRegionScans(t *testing.T) {
	cache := newChartBPMCache(chartBPMCacheEntries, chartBPMCacheTTL)
	for _, region := range []string{"jp", "en", "tw", "kr", "cn"} {
		for i := range 5000 {
			cache.put(region+fmt.Sprint(i), &parsedChartBPM{MainBPM: 128})
		}
	}
	for _, region := range []string{"jp", "en", "tw", "kr", "cn"} {
		for i := range 5000 {
			if _, ok := cache.get(region + fmt.Sprint(i)); !ok {
				t.Fatalf("evicted %s chart %d", region, i)
			}
		}
	}
}

func TestBPMIndexKeepsLocalLegacyPrecedence(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:128\n#00008:01")})
	_, key := publishTestBPMIndex(t, store, "r1")
	root := writeMusicAssetTree(t, map[string]string{"music/music_score/0001_01/expert.txt": "#BPM01:200\n#00008:01"})
	helper := assets.NewAssetHelper(root, nil)
	controller := NewController(storeChartSource(), nil, helper, nil, nil)
	controller.SetAssetReader(assets.NewAssetReader(helper, store))
	controller.SetBPMIndexSource(&mutableBPMIndexSource{revision: "r1", key: key}, store)
	parsed, found, err := controller.loadChartBPM(context.Background(), "jp", 1, "expert")
	if err != nil || !found || parsed.MainBPM != 200 {
		t.Fatalf("legacy precedence=%+v,%v,%v", parsed, found, err)
	}
	if strings.Contains(fmt.Sprint(parsed.Events), "128") {
		t.Fatal("used store index over local chart")
	}
}

func TestBPMIndexRejectsInvalidSchemaAndEvents(t *testing.T) {
	valid := BPMIndex{SchemaVersion: 1, Region: "jp", ResourceRevision: "r1", Complete: true, Prefixes: bpmIndexPrefixes("jp"), Charts: map[string]BPMIndexChart{storeChartKey: {MainBPM: 128, BarCount: 1, Duration: 1.875, Events: []BPMIndexEvent{{Bar: 0, BPM: 128, Duration: 1.875}}}}}
	for name, change := range map[string]func(*BPMIndex){
		"region":   func(index *BPMIndex) { index.Region = "other" },
		"version":  func(index *BPMIndex) { index.SchemaVersion = 2 },
		"revision": func(index *BPMIndex) { index.ResourceRevision = "" },
		"prefixes": func(index *BPMIndex) { index.Prefixes = []string{"jp-assets/"} },
		"invalid_key": func(index *BPMIndex) {
			index.Charts = map[string]BPMIndexChart{"en-assets/startapp/music/music_score/0001_01/expert.txt": valid.Charts[storeChartKey]}
		},
		"invalid_bpm": func(index *BPMIndex) {
			chart := valid.Charts[storeChartKey]
			chart.MainBPM = -1
			index.Charts = map[string]BPMIndexChart{storeChartKey: chart}
		},
		"invalid_event": func(index *BPMIndex) {
			chart := valid.Charts[storeChartKey]
			chart.Events = []BPMIndexEvent{{Bar: 0, BPM: 0}}
			index.Charts = map[string]BPMIndexChart{storeChartKey: chart}
		},
	} {
		t.Run(name, func(t *testing.T) {
			index := valid
			change(&index)
			if err := index.validate(); err == nil {
				t.Fatal("invalid index accepted")
			}
		})
	}
	for _, key := range []storage.Key{"jp-assets/startapp/music/music_score/x_01/expert.txt", "jp-assets/startapp/music/music_score/0001_01/readme.txt", "jp-assets/startapp/music/music_score/0001_01/expert.png"} {
		if bpmIndexScoreKey("jp", key) {
			t.Fatalf("accepted non-score key %s", key)
		}
	}
}

func TestBPMDominantDurationTieIsDeterministic(t *testing.T) {
	for range 100 {
		events := []BPMEvent{{Bar: 0, BPM: 120}, {Bar: 1, BPM: 240}}
		_, main := applyChartBPMDurations(events, 3)
		if main != 120 {
			t.Fatalf("equal-duration tie chose BPM %v", main)
		}
	}
}

func TestBPMIndexCanceledLoadDoesNotOverwriteNewRevision(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:128\n#00008:01")})
	_, first := publishTestBPMIndex(t, store, "r1")
	store.Seed(map[string][]byte{storeChartKey: []byte("#BPM01:196\n#00008:01")})
	_, second := publishTestBPMIndex(t, store, "r2")
	entered, release := make(chan struct{}), make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	store.FailGet = func(key storage.Key) error {
		if key == first {
			enterOnce.Do(func() { close(entered) })
			<-release
		}
		return nil
	}
	source := &mutableBPMIndexSource{revision: "r1", key: first}
	controller := newStoreChartController(store)
	controller.SetBPMIndexSource(source, store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := controller.loadChartBPM(ctx, "jp", 1, "expert"); done <- err }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled index waiter=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("index waiter did not cancel")
	}
	source.set("r2", second)
	chart, found, err := controller.loadChartBPM(context.Background(), "jp", 1, "expert")
	if err != nil || !found || chart.MainBPM != 196 {
		t.Fatalf("new revision=%+v,%v,%v", chart, found, err)
	}
	releaseOnce.Do(func() { close(release) })
	// Join the old load to ensure its attempted cache fill has finished.
	controller.bpmIndex.load(context.Background(), "jp", "r1", first)
	before := countCalls(store, "Get")
	chart, found, err = controller.loadChartBPM(context.Background(), "jp", 1, "expert")
	if err != nil || !found || chart.MainBPM != 196 || countCalls(store, "Get") != before {
		t.Fatalf("stale index replaced current revision=%+v,%v,%v", chart, found, err)
	}
}

func TestBPMIndexCanonicalScoreSpelling(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{"jp-assets/startapp/music/music_score/0001_01/EXPERT.TXT": []byte("#BPM01:128\n#00008:01")})
	_, key := publishTestBPMIndex(t, store, "r1")
	controller := newStoreChartController(store)
	controller.SetBPMIndexSource(&mutableBPMIndexSource{revision: "r1", key: key}, store)
	before := countCalls(store, "Get")
	chart, found, err := controller.loadChartBPM(context.Background(), "jp", 1, "expert")
	if err != nil || !found || chart.MainBPM != 128 {
		t.Fatalf("canonical chart=%+v,%v,%v", chart, found, err)
	}
	if countCalls(store, "Get")-before != 1 {
		t.Fatal("canonical index lookup downloaded the chart")
	}
}

func TestBPMCrossLanguageFixture(t *testing.T) {
	const fixture = "  #BPM01:90\n#BPM01:120\n#BPM02:240\n#00008:0100\n#00108:0002\n#00211:11\n"
	chart, err := parseChartBPM(context.Background(), strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if chart.MainBPM != 120 || chart.BarCount != 3 || chart.Duration != 4.5 || len(chart.Events) != 2 || chart.Events[1].Bar != 1.5 {
		t.Fatalf("fixture chart=%+v", chart)
	}
	data, err := jsonutil.Marshal(indexedChart(chart))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("BPM_CROSS_LANGUAGE=%s", data)
}
