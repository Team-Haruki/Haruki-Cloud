package assets

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func countCalls(memory *storagetest.Memory, method string) int {
	count := 0
	for _, op := range memory.Calls() {
		if op.Method == method {
			count++
		}
	}
	return count
}

// storeOnlyHelper is the E1 shape: no local root, the assets slot attached.
func storeOnlyHelper(t *testing.T, memory *storagetest.Memory, cfg StoreProbeConfig) *AssetHelper {
	t.Helper()
	helper := NewAssetHelper("", nil).WithStore(memory, cfg, nil)
	if !helper.ProbesStore() {
		t.Fatal("helper must probe the store")
	}
	return helper
}

func TestWithStoreIgnoresNilAndDisabledStores(t *testing.T) {
	var nilHelper *AssetHelper
	if nilHelper.WithStore(storagetest.NewMemory(), StoreProbeConfig{}, nil) != nil || nilHelper.ProbesStore() {
		t.Fatal("nil helper must stay nil")
	}
	helper := NewAssetHelper("", nil)
	if helper.WithStore(nil, StoreProbeConfig{}, nil).ProbesStore() {
		t.Fatal("nil store must not attach a probe")
	}
	if helper.WithStore(storage.Disabled(), StoreProbeConfig{}, nil).ProbesStore() {
		t.Fatal("disabled store must not attach a probe")
	}
	helper.WithStore(storagetest.NewMemory(), StoreProbeConfig{}, nil)
	if !helper.ProbesStore() || !helper.WithContext(context.Background()).ProbesStore() {
		t.Fatal("configured store must be shared with WithContext copies")
	}
	if got := helper.store.cfg; got != (StoreProbeConfig{PositiveTTL: DefaultStoreProbePositiveTTL, NegativeTTL: DefaultStoreProbeNegativeTTL, Timeout: DefaultStoreProbeTimeout}) {
		t.Fatalf("defaults = %+v", got)
	}
}

func TestResolveRegionAssetPathLocalRootStillWinsWithStore(t *testing.T) {
	root := writeAssetTree(t, map[string]string{
		"asset/jp-assets/ondemand/gacha/g1/logo/logo.png": "local",
	})
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/gacha/g1/logo/logo.png": []byte("store")})
	helper := NewAssetHelper(root, nil).WithStore(memory, StoreProbeConfig{}, nil)

	got := ResolveRegionAssetPath(helper, "jp", filepath.Join("gacha", "g1", "logo", "logo.png"))
	if want := "asset/jp-assets/ondemand/gacha/g1/logo/logo.png"; got != want {
		t.Fatalf("local hit = %q, want %q", got, want)
	}
	if calls := memory.Calls(); len(calls) != 0 {
		t.Fatalf("local hit must not touch the store: %+v", calls)
	}

	// A total local miss then asks the store (mirrors AssetReader.ReadFirst).
	got = ResolveRegionAssetPath(helper, "jp", filepath.Join("gacha", "g2", "logo", "logo.png"))
	if want := "asset/jp-assets/ondemand/gacha/g2/logo/logo.png"; got != want {
		t.Fatalf("local+store miss = %q, want first candidate %q", got, want)
	}
	memory.Seed(map[string][]byte{"jp-assets/startapp/gacha/g3/logo/logo.png": []byte("store")})
	got = ResolveRegionAssetPath(helper, "jp", filepath.Join("gacha", "g3", "logo", "logo.png"))
	if want := "asset/jp-assets/startapp/gacha/g3/logo/logo.png"; got != want {
		t.Fatalf("local miss, store hit = %q, want %q", got, want)
	}
}

func TestResolveRegionAssetPathWithoutStoreKeepsFirstCandidate(t *testing.T) {
	helper := NewAssetHelper("", nil)
	got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "j", "j.png"))
	if want := "asset/jp-assets/startapp/music/jacket/j/j.png"; got != want {
		t.Fatalf("rootless, storeless helper = %q, want %q", got, want)
	}
}

func TestResolveRegionAssetPathStoreOnlyChoosesMode(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{
		"jp-assets/ondemand/music/jacket/jacket_s_002/jacket_s_002.png": []byte("ondemand only"),
		"jp-assets/startapp/gacha/g1/logo/logo.png":                     []byte("startapp only"),
		"jp-assets/startapp/home/banner/b1/b1.png":                      []byte("banner"),
	})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})

	cases := map[string]struct {
		rels []string
		want string
	}{
		"ondemand fallback": {
			rels: []string{filepath.Join("music", "jacket", "jacket_s_002", "jacket_s_002.png")},
			want: "asset/jp-assets/ondemand/music/jacket/jacket_s_002/jacket_s_002.png",
		},
		"startapp fallback for ondemand-first top level": {
			rels: []string{filepath.Join("gacha", "g1", "logo", "logo.png")},
			want: "asset/jp-assets/startapp/gacha/g1/logo/logo.png",
		},
		"second relative candidate": {
			rels: []string{filepath.Join("event", "b1", "banner.png"), filepath.Join("home", "banner", "b1", "b1.png")},
			want: "asset/jp-assets/startapp/home/banner/b1/b1.png",
		},
		"total miss returns first candidate": {
			rels: []string{filepath.Join("music", "jacket", "nope", "nope.png")},
			want: "asset/jp-assets/startapp/music/jacket/nope/nope.png",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ResolveRegionAssetPath(helper, "jp", tc.rels...); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	if got := ResolveEventBannerPath(helper, "jp", "b1"); got != "asset/jp-assets/startapp/home/banner/b1/b1.png" {
		t.Fatalf("event banner = %q", got)
	}
}

func TestResolveRegionAssetPathStoreOnlyCorrectsCaseThroughListings(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{
		"jp-assets/startapp/music/jacket/Jacket_S_001/Jacket_S_001.PNG": []byte("cased"),
		"jp-assets/startapp/music/jacket/other/other.png":               []byte("x"),
	})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})

	got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "jacket_s_001", "jacket_s_001.png"))
	if want := "asset/jp-assets/startapp/music/jacket/Jacket_S_001/Jacket_S_001.PNG"; got != want {
		t.Fatalf("case-corrected path = %q, want %q", got, want)
	}
	// One HEAD for the startapp candidate, then one listing per level of the
	// walk (root, jp-assets, startapp, music, jacket, Jacket_S_001).
	if stats, lists := countCalls(memory, "Stat"), countCalls(memory, "ListDir"); stats != 1 || lists != 6 {
		t.Fatalf("Stat=%d ListDir=%d calls: %+v", stats, lists, memory.Calls())
	}

	// A second miscased key under the same parents reuses their listings and
	// only lists its own leaf directory.
	before := len(memory.Calls())
	got = ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "OTHER", "OTHER.png"))
	if want := "asset/jp-assets/startapp/music/jacket/other/other.png"; got != want {
		t.Fatalf("second correction = %q, want %q", got, want)
	}
	if lists := countCalls(memory, "ListDir"); lists != 7 {
		t.Fatalf("directory listings must be cached, got %d after %d calls", lists, len(memory.Calls())-before)
	}

	// The corrected path is served from the key cache afterwards.
	before = len(memory.Calls())
	ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "jacket_s_001", "jacket_s_001.png"))
	if len(memory.Calls()) != before {
		t.Fatalf("positive result must be cached: %+v", memory.Calls()[before:])
	}
}

func TestStoreProbeExactNameBeatsCaseFoldAndSkipsHugeDirectories(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{
		"jp-assets/startapp/stamp/Stamp0001/Stamp0001.png": []byte("a"),
		"jp-assets/startapp/stamp/stamp0001/stamp0001.png": []byte("b"),
	})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	ctx := context.Background()

	if key, found, err := helper.store.resolve(ctx, "jp-assets/startapp/stamp/stamp0001/stamp0001.png"); err != nil || !found || key != "jp-assets/startapp/stamp/stamp0001/stamp0001.png" {
		t.Fatalf("exact key = %q %v %v", key, found, err)
	}
	if key, found, err := helper.store.resolve(ctx, "jp-assets/startapp/stamp/STAMP0001/STAMP0001.png"); err != nil || !found || key == "" {
		t.Fatalf("folded key = %q %v %v", key, found, err)
	}

	helper.store.maxDirNames = 1
	helper.ClearResolutionCache()
	if _, found, err := helper.store.resolve(ctx, "jp-assets/startapp/stamp/STAMP0001/STAMP0001.png"); err != nil || found {
		t.Fatalf("a directory over the name cap must not be case-corrected: found=%v err=%v", found, err)
	}
	// The oversized listing is remembered, so the second probe lists nothing new.
	before := countCalls(memory, "ListDir")
	if _, found, _ := helper.store.resolve(ctx, "jp-assets/startapp/stamp/STAMP0002/STAMP0002.png"); found {
		t.Fatal("unexpected hit")
	}
	if got := countCalls(memory, "ListDir"); got != before {
		t.Fatalf("truncated directory listed again: %d -> %d", before, got)
	}
}

func TestStoreProbeNegativeCacheExpires(t *testing.T) {
	memory := storagetest.NewMemory()
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{NegativeTTL: 5 * time.Minute})
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	helper.store.now = func() time.Time { return now }
	rel := filepath.Join("music", "jacket", "j", "j.png")
	first := "asset/jp-assets/startapp/music/jacket/j/j.png"

	if got := ResolveRegionAssetPath(helper, "jp", rel); got != first {
		t.Fatalf("miss = %q", got)
	}
	memory.Seed(map[string][]byte{"jp-assets/ondemand/music/jacket/j/j.png": []byte("late")})
	before := len(memory.Calls())
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != first || len(memory.Calls()) != before {
		t.Fatalf("miss must stay cached inside the negative TTL: %q, %d new calls", got, len(memory.Calls())-before)
	}

	now = now.Add(5*time.Minute + time.Second)
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/ondemand/music/jacket/j/j.png" {
		t.Fatalf("after negative TTL = %q", got)
	}
}

func TestStoreProbePositiveCacheExpiresAndClearResets(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/x/y.png": []byte("x")})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{PositiveTTL: time.Hour})
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	ctx := context.Background()

	helper.store.resolve(ctx, "jp-assets/startapp/x/y.png")
	helper.store.resolve(ctx, "jp-assets/startapp/x/y.png")
	if got := countCalls(memory, "Stat"); got != 1 {
		t.Fatalf("Stat calls = %d, want 1", got)
	}
	now = now.Add(time.Hour + time.Second)
	helper.store.resolve(ctx, "jp-assets/startapp/x/y.png")
	if got := countCalls(memory, "Stat"); got != 2 {
		t.Fatalf("Stat calls after positive TTL = %d, want 2", got)
	}
	helper.ClearResolutionCache()
	helper.store.resolve(ctx, "jp-assets/startapp/x/y.png")
	if got := countCalls(memory, "Stat"); got != 3 {
		t.Fatalf("Stat calls after clear = %d, want 3", got)
	}
}

func TestStoreProbeStoreErrorFallsBackAndIsCachedBriefly(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/ondemand/music/jacket/j/j.png": []byte("x")})
	boom := errors.New("garage down")
	memory.FailStat = func(storage.Key) error { return boom }
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	rel := filepath.Join("music", "jacket", "j", "j.png")

	if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/startapp/music/jacket/j/j.png" {
		t.Fatalf("store error must yield the first candidate, got %q", got)
	}
	// The first candidate's failure ends the search: no second HEAD, no listing.
	if stats, lists := countCalls(memory, "Stat"), countCalls(memory, "ListDir"); stats != 1 || lists != 0 {
		t.Fatalf("Stat=%d ListDir=%d", stats, lists)
	}
	ResolveRegionAssetPath(helper, "jp", rel)
	if got := countCalls(memory, "Stat"); got != 1 {
		t.Fatalf("error must be cached: Stat calls = %d", got)
	}
	if _, _, err := helper.store.resolve(context.Background(), "jp-assets/startapp/music/jacket/j/j.png"); !errors.Is(err, boom) {
		t.Fatalf("cached error = %v", err)
	}

	memory.FailStat = nil
	now = now.Add(storeProbeErrorTTL + time.Second)
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/ondemand/music/jacket/j/j.png" {
		t.Fatalf("after the error TTL the store answers again: %q", got)
	}
}

func TestStoreProbeListingErrorFallsBack(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/music/jacket/J/J.png": []byte("x")})
	memory.FailListDir = func(storage.Key) error { return errors.New("list failed") }
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	rel := filepath.Join("music", "jacket", "j", "j.png")
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/startapp/music/jacket/j/j.png" {
		t.Fatalf("listing error must yield the first candidate, got %q", got)
	}
}

func TestStoreProbeSingleflightCollapsesConcurrentProbes(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/music/jacket/j/j.png": []byte("x")})
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	memory.FailStat = func(storage.Key) error {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return nil
	}
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	rel := filepath.Join("music", "jacket", "j", "j.png")

	const waiters = 16
	results := make([]string, waiters)
	var wg sync.WaitGroup
	for i := range waiters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = ResolveRegionAssetPath(helper, "jp", rel)
		}()
	}
	<-entered
	// Every goroutine is either queued on the flight or about to be; give the
	// stragglers a moment so they join the in-flight probe instead of hitting
	// the positive cache afterwards.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, got := range results {
		if got != "asset/jp-assets/startapp/music/jacket/j/j.png" {
			t.Fatalf("waiter %d = %q", i, got)
		}
	}
	if got := countCalls(memory, "Stat"); got != 1 {
		t.Fatalf("Stat calls = %d, want 1 shared flight", got)
	}
}

func TestStoreProbeCancelledCallerFallsBackWithoutAbortingTheFlight(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/music/jacket/j/j.png": []byte("x")})
	release := make(chan struct{})
	memory.FailStat = func(storage.Key) error {
		<-release
		return nil
	}
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rel := filepath.Join("music", "jacket", "j", "j.png")

	done := make(chan string, 1)
	go func() { done <- ResolveRegionAssetPath(helper.WithContext(ctx), "jp", rel) }()
	select {
	case got := <-done:
		if got != "asset/jp-assets/startapp/music/jacket/j/j.png" {
			t.Fatalf("cancelled caller = %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled caller must not wait for the store")
	}
	close(release)
	// The shared flight finishes and populates the cache for the next caller.
	deadline := time.Now().Add(5 * time.Second)
	for helper.store.keys.len() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("flight result was not cached")
		}
		time.Sleep(5 * time.Millisecond)
	}
	before := countCalls(memory, "Stat")
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/startapp/music/jacket/j/j.png" || countCalls(memory, "Stat") != before {
		t.Fatalf("next caller = %q, Stat calls %d -> %d", got, before, countCalls(memory, "Stat"))
	}
}

// blockingStatStore holds every Stat until its context expires, like a
// backend that stopped answering.
type blockingStatStore struct {
	storage.Store
	calls atomic.Int32
}

func (s *blockingStatStore) Stat(ctx context.Context, _ storage.Key) (storage.Object, error) {
	s.calls.Add(1)
	<-ctx.Done()
	return storage.Object{}, ctx.Err()
}

func TestStoreProbeTimeoutBoundsOneCall(t *testing.T) {
	store := &blockingStatStore{Store: storagetest.NewMemory()}
	helper := NewAssetHelper("", nil).WithStore(store, StoreProbeConfig{Timeout: 20 * time.Millisecond}, nil)
	rel := filepath.Join("music", "jacket", "j", "j.png")

	startedAt := time.Now()
	got := ResolveRegionAssetPath(helper, "jp", rel)
	elapsed := time.Since(startedAt)
	if got != "asset/jp-assets/startapp/music/jacket/j/j.png" {
		t.Fatalf("timed-out probe = %q", got)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("probe took %v, want roughly the configured timeout", elapsed)
	}
	if got := store.calls.Load(); got != 1 {
		t.Fatalf("Stat calls = %d, want 1 (timeout is a cached store error)", got)
	}
}

func TestStoreProbeLogsErrorsAtMostOncePerInterval(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.FailStat = func(storage.Key) error { return errors.New("down") }
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	ctx := context.Background()
	for i := range 5 {
		helper.store.resolve(ctx, storage.Key(filepath.ToSlash(filepath.Join("jp-assets/startapp", "k", string(rune('a'+i))))))
	}
	if got := helper.store.suppressed.Load(); got != 4 {
		t.Fatalf("suppressed = %d, want 4", got)
	}
	now = now.Add(storeProbeLogInterval + time.Second)
	helper.store.resolve(ctx, "jp-assets/startapp/k/z")
	if got := helper.store.suppressed.Load(); got != 0 {
		t.Fatalf("suppressed after the interval = %d, want 0", got)
	}
}

func TestReplaceDrawingKey(t *testing.T) {
	cases := []struct {
		candidate     string
		key, resolved storage.Key
		want          string
	}{
		{"asset/jp-assets/startapp/a/B.png", "jp-assets/startapp/a/B.png", "jp-assets/startapp/A/b.png", "asset/jp-assets/startapp/A/b.png"},
		{"/asset/jp-assets/startapp/a.png", "jp-assets/startapp/a.png", "jp-assets/startapp/A.png", "/asset/jp-assets/startapp/A.png"},
		{"jp-assets/startapp/a.png", "jp-assets/startapp/a.png", "jp-assets/startapp/a.png", "jp-assets/startapp/a.png"},
		{"asset/./jp-assets/startapp/a.png", "jp-assets/startapp/a.png", "jp-assets/startapp/A.png", "asset/./jp-assets/startapp/A.png"},
		{"asset/x/y.png", "y.png", "Y.png", "asset/x/Y.png"},
		{"unrelated.png", "jp-assets/startapp/a.png", "jp-assets/startapp/A.png", "unrelated.png"},
	}
	for _, tc := range cases {
		if got := replaceDrawingKey(tc.candidate, tc.key, tc.resolved); got != tc.want {
			t.Fatalf("replaceDrawingKey(%q, %q, %q) = %q, want %q", tc.candidate, tc.key, tc.resolved, got, tc.want)
		}
	}
}

func TestProbeCacheBoundsEntriesAndWeight(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)
	cache := newProbeCache[int](2, 0)
	cache.store("a", 1, 1, later)
	cache.store("b", 2, 1, later)
	cache.lookup("a", now)
	cache.store("c", 3, 1, later)
	if _, ok := cache.lookup("b", now); ok {
		t.Fatal("least recently used entry must be evicted")
	}
	if v, ok := cache.lookup("a", now); !ok || v != 1 {
		t.Fatal("recently used entry must survive")
	}
	if _, ok := cache.lookup("a", later); ok {
		t.Fatal("expired entry must be dropped")
	}

	weighted := newProbeCache[int](10, 5)
	weighted.store("big", 1, 6, later)
	if weighted.len() != 0 {
		t.Fatal("an entry over the weight budget must be rejected")
	}
	weighted.store("x", 1, 3, later)
	weighted.store("y", 2, 3, later)
	if _, ok := weighted.lookup("x", now); ok || weighted.len() != 1 || weighted.weight != 3 {
		t.Fatalf("weight budget not enforced: len=%d weight=%d", weighted.len(), weighted.weight)
	}
	weighted.clear()
	if weighted.len() != 0 || weighted.weight != 0 {
		t.Fatal("clear must reset the cache")
	}
}
