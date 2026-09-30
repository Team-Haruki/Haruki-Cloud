package assets

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type delayedAssetStore struct {
	storage.Store
	delay  time.Duration
	active atomic.Int32
	peak   atomic.Int32
}

func (s *delayedAssetStore) ListDir(ctx context.Context, key storage.Key, yield func(storage.DirEntry) error) error {
	active := s.active.Add(1)
	defer s.active.Add(-1)
	for old := s.peak.Load(); active > old && !s.peak.CompareAndSwap(old, active); old = s.peak.Load() {
	}
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.Store.ListDir(ctx, key, yield)
}

func bannerPrefetchFixture(delay time.Duration) (*AssetHelper, *storagetest.Memory, *delayedAssetStore, []func(*AssetHelper), []string) {
	memory := storagetest.NewMemory()
	seed := make(map[string][]byte)
	for i := range 24 {
		name := fmt.Sprintf("event%d", i)
		seed["kr-assets/startapp/home/banner/"+name+"/other.png"] = []byte("other")
		seed["kr-assets/ondemand/event/"+name+"/other.png"] = []byte("other")
		seed["kr-assets/ondemand/event_story/"+name+"/screen_image/banner_event_story.png"] = []byte("fallback")
		if i%2 == 0 {
			seed["kr-assets/startapp/home/banner/"+name+"/"+name+".png"] = []byte("preferred")
		}
	}
	memory.Seed(seed)
	store := &delayedAssetStore{Store: memory, delay: delay}
	helper := NewAssetHelper("", nil).WithStore(store, StoreProbeConfig{}, nil)
	helper.store.bulkMinChildren = 1000
	results := make([]string, 24)
	tasks := make([]func(*AssetHelper), 24)
	for i := range tasks {
		tasks[i] = func(h *AssetHelper) { results[i] = ResolveEventBannerPath(h, "kr", fmt.Sprintf("event%d", i)) }
	}
	return helper, memory, store, tasks, results
}

func TestAssetPrefetchBoundsWorkersAndPreservesCandidatePriority(t *testing.T) {
	serial, serialMemory, _, serialTasks, want := bannerPrefetchFixture(time.Millisecond)
	defer serial.Close()
	for _, task := range serialTasks {
		task(serial)
	}
	parallel, parallelMemory, store, tasks, got := bannerPrefetchFixture(time.Millisecond)
	defer parallel.Close()
	ctx, trace := commandtrace.WithTrace(t.Context())
	if err := parallel.WithContext(ctx).Prefetch(tasks); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("parallel output differs: got %v, want %v", got, want)
	}
	if peak := store.peak.Load(); peak < 2 || peak > assetPrefetchWorkers {
		t.Fatalf("concurrent I/O = %d", peak)
	}
	if a, b := requestCount(serialMemory), requestCount(parallelMemory); a != b {
		t.Fatalf("serial/parallel store calls %d/%d", a, b)
	}
	if counted := operationStatsByName(trace.Snapshot())["asset.store_list"].Count; counted != countCalls(parallelMemory, "ListDir") {
		t.Fatalf("trace duplicated shared calls: counted %d, actual %d", counted, countCalls(parallelMemory, "ListDir"))
	}
}

func BenchmarkAssetBannerPrefetch(b *testing.B) {
	for _, concurrent := range []bool{false, true} {
		b.Run(fmt.Sprintf("parallel=%v", concurrent), func(b *testing.B) {
			for b.Loop() {
				helper, _, _, tasks, _ := bannerPrefetchFixture(5 * time.Millisecond)
				if concurrent {
					_ = helper.Prefetch(tasks)
				} else {
					for _, task := range tasks {
						task(helper)
					}
				}
				helper.Close()
			}
		})
	}
}

func TestPrefetchCanceledRequestStartsNoWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	helper := NewAssetHelper("", nil).WithContext(ctx)
	var calls atomic.Int32
	tasks := make([]func(*AssetHelper), 100)
	for i := range tasks {
		tasks[i] = func(*AssetHelper) { calls.Add(1) }
	}
	if err := helper.Prefetch(tasks); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("canceled request started %d tasks", calls.Load())
	}
}

func TestSharedAssetMetadataAvoidsReaderHEAD(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{"jp-assets/ondemand/music/jacket/J/J.PNG": []byte("image")})
	helper := NewAssetHelper("", nil).WithStore(store, StoreProbeConfig{}, nil)
	defer helper.Close()
	path := ResolveRegionAssetPath(helper, "jp", "music/jacket/j/j.png")
	reader := NewAssetReader(helper, store)
	before := requestCount(store)
	if resolved, found, err := reader.StatResult(t.Context(), path); err != nil || !found || resolved != "jp-assets/ondemand/music/jacket/J/J.PNG" {
		t.Fatalf("StatResult = %q %v %v", resolved, found, err)
	}
	if requestCount(store) != before || countCalls(store, "Stat") != 0 {
		t.Fatal("reader repeated known existence I/O")
	}
}

func TestReaderUnknownErrorIsNotMissingAndConcurrentHEADIsShared(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{"hit.png": []byte("image")})
	boom := errors.New("unavailable")
	store.FailStat = func(storage.Key) error { return boom }
	reader := NewAssetReader(nil, store)
	if _, found, err := reader.StatResult(t.Context(), "hit.png"); found || !errors.Is(err, boom) {
		t.Fatalf("unknown = %v %v", found, err)
	}
	store.FailStat = nil
	start, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	store.FailStat = func(storage.Key) error { once.Do(func() { close(start); <-release }); return nil }
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			_, found, err := reader.StatResult(t.Context(), "hit.png")
			if !found || err != nil {
				t.Errorf("shared HEAD: %v %v", found, err)
			}
		})
	}
	<-start
	close(release)
	wg.Wait()
	if countCalls(store, "Stat") != 2 {
		t.Fatalf("HEAD calls = %d (one error + one shared success)", countCalls(store, "Stat"))
	}
}

type mutableMetadataIndex struct {
	mu   sync.RWMutex
	keys map[storage.Key]storage.Key
}

func (m *mutableMetadataIndex) Lookup(key storage.Key) (storage.Key, bool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resolved, found := m.keys[key]
	return resolved, found, true
}
func (m *mutableMetadataIndex) replace(keys map[storage.Key]storage.Key) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys = keys
}

func TestMetadataIndexOverridesCachedChoiceAndReaderMemo(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{"jp-assets/startapp/b.png": []byte("b")})
	index := &mutableMetadataIndex{keys: map[storage.Key]storage.Key{"jp-assets/startapp/b.png": "jp-assets/startapp/b.png"}}
	helper := NewAssetHelper("", nil).WithStore(store, StoreProbeConfig{}, nil).WithMetadataIndex(index)
	defer helper.Close()
	reader := NewAssetReader(helper, store)
	if got := ResolveRegionAssetPath(helper, "jp", "a.png", "b.png"); got != "asset/jp-assets/startapp/b.png" {
		t.Fatal(got)
	}
	if _, found, err := reader.StatResult(t.Context(), "jp-assets/startapp/a.png"); err != nil || found {
		t.Fatalf("missing = %v %v", found, err)
	}
	index.replace(map[storage.Key]storage.Key{"jp-assets/startapp/a.png": "jp-assets/startapp/A.PNG"})
	if got := ResolveRegionAssetPath(helper, "jp", "a.png", "b.png"); got != "asset/jp-assets/startapp/A.PNG" {
		t.Fatal(got)
	}
	if _, found, err := reader.StatResult(t.Context(), "jp-assets/startapp/b.png"); err != nil || found {
		t.Fatalf("removed = %v %v", found, err)
	}
	if _, _, err := reader.ReadFirst(t.Context(), "jp-assets/startapp/b.png"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatal(err)
	}
	if len(store.Calls()) != 0 {
		t.Fatalf("authoritative metadata performed storage I/O: %v", store.Calls())
	}
}

func TestReaderClearRejectsInflightStatMemo(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{"hit.png": []byte("x")})
	start, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	store.FailStat = func(storage.Key) error { once.Do(func() { close(start); <-release }); return nil }
	reader := NewAssetReader(nil, store)
	done := make(chan struct{})
	go func() { defer close(done); reader.Stat(t.Context(), "hit.png") }()
	<-start
	reader.ClearMetadataCache()
	close(release)
	<-done
	if _, cached := reader.memo.lookup("hit.png"); cached {
		t.Fatal("old publication refilled memo")
	}
	if _, found := reader.Stat(t.Context(), "hit.png"); !found {
		t.Fatal("fresh lookup failed")
	}
	if countCalls(store, "Stat") != 2 {
		t.Fatal("fresh generation reused old flight")
	}
}

func TestHelperCloseCancelsSharedProbe(t *testing.T) {
	store := &blockingStore{Store: storagetest.NewMemory()}
	helper := NewAssetHelper("", nil).WithStore(store, StoreProbeConfig{Timeout: time.Hour}, nil)
	done := make(chan error, 1)
	go func() { _, _, err := helper.store.resolve(t.Context(), "jp-assets/a.png"); done <- err }()
	deadline := time.Now().Add(time.Second)
	for store.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	helper.Close()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("close result = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shared flight survived Close")
	}
}

type blockingBulkStore struct {
	storage.Store
	entered chan bool
	release chan struct{}
	calls   atomic.Int32
}

func (s *blockingBulkStore) List(ctx context.Context, _ storage.Key, _ func(storage.Object) error) error {
	s.calls.Add(1)
	s.entered <- storage.IsBackgroundIO(ctx)
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestBulkWarmupAdmissionIsBoundedAndMarkedBackground(t *testing.T) {
	store := &blockingBulkStore{Store: storagetest.NewMemory(), entered: make(chan bool, 8), release: make(chan struct{})}
	helper := NewAssetHelper("", nil).WithStore(store, StoreProbeConfig{}, nil)
	defer helper.Close()
	probe := helper.store
	generation := probe.dirs.currentGeneration()
	for i := range 8 {
		grand := fmt.Sprintf("jp-assets/wide%d/", i)
		index := &storeDirIndex{listedAt: time.Now()}
		for j := range probe.bulkMinChildren {
			index.add(storage.DirEntry{Name: fmt.Sprintf("child%d", j), Dir: true})
		}
		probe.rememberDir(grand, index, generation)
		probe.startBulkList(grand+"child0/", generation)
	}
	for range 2 {
		select {
		case marked := <-store.entered:
			if !marked {
				t.Fatal("bulk work lost background classification")
			}
		case <-time.After(time.Second):
			t.Fatal("admitted bulk work did not start")
		}
	}
	if got := store.calls.Load(); got != 2 {
		t.Fatalf("started %d background operations, want two", got)
	}
	close(store.release)
	probe.bulkWait.Wait()
	if got := store.calls.Load(); got != 2 {
		t.Fatalf("unbounded queued bulk work started: %d", got)
	}
}

type unknownMetadataIndex struct{}

func (unknownMetadataIndex) Lookup(storage.Key) (storage.Key, bool, bool) { return "", false, false }

func TestIncompleteMetadataKeepsSuccessfulChoiceCache(t *testing.T) {
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{"kr-assets/ondemand/event_story/e/screen_image/banner_event_story.png": []byte("fallback")})
	helper := NewAssetHelper("", nil).WithStore(store, StoreProbeConfig{}, nil).WithMetadataIndex(unknownMetadataIndex{})
	defer helper.Close()
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	want := ResolveEventBannerPath(helper, "kr", "e")
	before := requestCount(store)
	now = now.Add(6 * time.Minute)
	if got := ResolveEventBannerPath(helper, "kr", "e"); got != want || requestCount(store) != before {
		t.Fatalf("incomplete metadata regressed selection cache: got %s calls %d -> %d", got, before, requestCount(store))
	}
}
