package assets

import (
	"context"
	"errors"
	"fmt"
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

func requestCount(memory *storagetest.Memory) int {
	return countCalls(memory, "Stat") + countCalls(memory, "ListDir") + countCalls(memory, "List")
}

// storeOnlyHelper is the E1 shape: no local root, the assets slot attached.
func storeOnlyHelper(t *testing.T, store storage.Store, cfg StoreProbeConfig) *AssetHelper {
	t.Helper()
	helper := NewAssetHelper("", nil).WithStore(store, cfg, nil)
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
	want := StoreProbeConfig{
		PositiveTTL: DefaultStoreProbePositiveTTL, ListingTTL: DefaultStoreProbeListingTTL,
		NegativeTTL: DefaultStoreProbeNegativeTTL, Timeout: DefaultStoreProbeTimeout,
	}
	got := helper.store.cfg
	if got.PositiveTTL != want.PositiveTTL || got.ListingTTL != want.ListingTTL || got.NegativeTTL != want.NegativeTTL || got.Timeout != want.Timeout {
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
	memory.Seed(map[string][]byte{"jp-assets/startapp/gacha/g3/logo/logo.png": []byte("store")})
	got = ResolveRegionAssetPath(helper, "jp", filepath.Join("gacha", "g2", "logo", "logo.png"))
	if want := "asset/jp-assets/ondemand/gacha/g2/logo/logo.png"; got != want {
		t.Fatalf("local+store miss = %q, want first candidate %q", got, want)
	}
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
	if got := countCalls(memory, "Stat"); got != 0 {
		t.Fatalf("listing-backed existence must not HEAD: %d Stat calls", got)
	}
}

func TestStoreProbeSiblingsShareOneListing(t *testing.T) {
	memory := storagetest.NewMemory()
	seed := map[string][]byte{}
	for i := range 150 {
		seed[fmt.Sprintf("jp-assets/startapp/thumbnail/chara/res%03d_no001_normal.png", i)] = []byte("x")
	}
	memory.Seed(seed)
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})

	for i := range 150 {
		rel := filepath.Join("thumbnail", "chara", fmt.Sprintf("RES%03d_no001_normal.png", i))
		want := fmt.Sprintf("asset/jp-assets/startapp/thumbnail/chara/res%03d_no001_normal.png", i)
		if got := ResolveRegionAssetPath(helper, "jp", rel); got != want {
			t.Fatalf("card %d = %q, want %q", i, got, want)
		}
	}
	// root, jp-assets/, jp-assets/startapp/, thumbnail/, thumbnail/chara/.
	if lists, stats := countCalls(memory, "ListDir"), countCalls(memory, "Stat"); lists != 5 || stats != 0 {
		t.Fatalf("150 siblings cost ListDir=%d Stat=%d, want 5 listings and no HEAD", lists, stats)
	}
}

func TestStoreProbeBulkListsWideDirectoriesOnce(t *testing.T) {
	memory := storagetest.NewMemory()
	seed := map[string][]byte{}
	for i := range 100 {
		name := fmt.Sprintf("jacket_s_%03d", i)
		seed["jp-assets/startapp/music/jacket/"+name+"/"+name+".png"] = []byte("x")
	}
	seed["jp-assets/startapp/music/jacket/Jacket_Special/Jacket_Special.png"] = []byte("x")
	seed["jp-assets/startapp/music/jacket/jacket_s_000/extra/nested.png"] = []byte("x")
	memory.Seed(seed)
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})

	if got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "jacket_s_000", "jacket_s_000.png")); got != "asset/jp-assets/startapp/music/jacket/jacket_s_000/jacket_s_000.png" {
		t.Fatalf("first jacket = %q", got)
	}
	// root, jp-assets/, startapp/, music/, music/jacket/ by ListDir, then one
	// recursive List of music/jacket/ instead of a ListDir per jacket.
	if lists, bulk := countCalls(memory, "ListDir"), countCalls(memory, "List"); lists != 5 || bulk != 1 {
		t.Fatalf("ListDir=%d List=%d after the first jacket", lists, bulk)
	}
	for i := 1; i < 100; i++ {
		name := fmt.Sprintf("jacket_s_%03d", i)
		if got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", name, name+".png")); got != "asset/jp-assets/startapp/music/jacket/"+name+"/"+name+".png" {
			t.Fatalf("jacket %d = %q", i, got)
		}
	}
	if got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "jacket_special", "jacket_special.png")); got != "asset/jp-assets/startapp/music/jacket/Jacket_Special/Jacket_Special.png" {
		t.Fatalf("bulk-indexed case correction = %q", got)
	}
	if got := requestCount(memory); got != 6 {
		t.Fatalf("100 jackets cost %d store calls, want 6: %+v", got, memory.Calls())
	}
	// Nested directories seen by the bulk listing are indexed as sub-dirs;
	// their own contents are listed on demand.
	if got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "jacket_s_000", "EXTRA", "nested.png")); got != "asset/jp-assets/startapp/music/jacket/jacket_s_000/extra/nested.png" {
		t.Fatalf("nested = %q", got)
	}
	if got := requestCount(memory); got != 7 {
		t.Fatalf("nested lookup cost %d store calls, want 7", got)
	}
}

func TestStoreProbeBulkListingOverCapFallsBackToPerDirectoryListings(t *testing.T) {
	memory := storagetest.NewMemory()
	seed := map[string][]byte{}
	for i := range 70 {
		for j := range 3 {
			seed[fmt.Sprintf("jp-assets/startapp/honor/h%03d/f%d.png", i, j)] = []byte("x")
		}
	}
	memory.Seed(seed)
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	helper.store.bulkMaxObjects = 50

	rel := filepath.Join("honor", "h001", "F1.png")
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/startapp/honor/h001/f1.png" {
		t.Fatalf("got %q", got)
	}
	// root, jp-assets/, startapp/, honor/ by ListDir, one abandoned bulk
	// listing of honor/, then the honor directory h001/ itself.
	if bulk, lists := countCalls(memory, "List"), countCalls(memory, "ListDir"); bulk != 1 || lists != 5 {
		t.Fatalf("List=%d ListDir=%d", bulk, lists)
	}
	ResolveRegionAssetPath(helper, "jp", filepath.Join("honor", "h002", "f0.png"))
	if bulk, lists := countCalls(memory, "List"), countCalls(memory, "ListDir"); bulk != 1 || lists != 6 {
		t.Fatalf("the failed bulk listing must not be retried: List=%d ListDir=%d", bulk, lists)
	}
}

func TestStoreProbeCappedOrFailedListingFallsBackToHead(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{
		"jp-assets/startapp/stamp/s1/s1.png": []byte("a"),
		"jp-assets/startapp/stamp/s2/s2.png": []byte("b"),
	})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	helper.store.maxListEntries = 1
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	ctx := context.Background()

	// stamp/ has two children: over the cap, so the rest of the key is HEADed.
	if key, found, err := helper.store.resolve(ctx, "jp-assets/startapp/stamp/s1/s1.png"); err != nil || !found || key != "jp-assets/startapp/stamp/s1/s1.png" {
		t.Fatalf("head fallback = %q %v %v", key, found, err)
	}
	if _, found, err := helper.store.resolve(ctx, "jp-assets/startapp/stamp/S1/S1.png"); err != nil || found {
		t.Fatalf("no case correction without a listing: found=%v err=%v", found, err)
	}
	if stats := countCalls(memory, "Stat"); stats != 2 {
		t.Fatalf("Stat calls = %d, want 2", stats)
	}
	capped, ok := helper.store.dirs.lookup("jp-assets/startapp/stamp/", now)
	if !ok || !capped.unavailable {
		t.Fatalf("capped listing must be remembered as unavailable: %+v %v", capped, ok)
	}
	if got := helper.store.dirs.weight; got > 5 {
		t.Fatalf("an unavailable listing must weigh one, total weight = %d", got)
	}
	// It is retried after NegativeTTL, not ListingTTL.
	lists := countCalls(memory, "ListDir")
	now = now.Add(DefaultStoreProbeNegativeTTL + time.Second)
	helper.store.resolve(ctx, "jp-assets/startapp/stamp/s2/s2.png")
	if got := countCalls(memory, "ListDir"); got <= lists {
		t.Fatal("unavailable listing must be retried after the negative TTL")
	}

	failing := storagetest.NewMemory()
	failing.Seed(map[string][]byte{"jp-assets/startapp/x/y.png": []byte("x")})
	failing.FailListDir = func(storage.Key) error { return errors.New("list failed") }
	helper = storeOnlyHelper(t, failing, StoreProbeConfig{})
	if got := ResolveRegionAssetPath(helper, "jp", filepath.Join("x", "y.png")); got != "asset/jp-assets/startapp/x/y.png" {
		t.Fatalf("failed listing -> HEAD = %q", got)
	}
	if stats := countCalls(failing, "Stat"); stats != 1 {
		t.Fatalf("Stat calls = %d, want 1", stats)
	}
}

func TestStoreProbeStaleListingIsRefreshedOnMiss(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/x/old.png": []byte("x")})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{NegativeTTL: 5 * time.Minute, ListingTTL: time.Hour})
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	helper.store.now = func() time.Time { return now }
	rel := filepath.Join("x", "new.png")
	first := "asset/jp-assets/startapp/x/new.png"

	if got := ResolveRegionAssetPath(helper, "jp", rel); got != first {
		t.Fatalf("miss = %q", got)
	}
	memory.Seed(map[string][]byte{"jp-assets/ondemand/x/new.png": []byte("late")})
	before := len(memory.Calls())
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != first || len(memory.Calls()) != before {
		t.Fatalf("miss must stay cached inside the negative TTL: %q, %d new calls", got, len(memory.Calls())-before)
	}

	// Past the negative TTL (but inside the listing TTL) the miss re-lists
	// the directory once and finds the new file.
	now = now.Add(5*time.Minute + time.Second)
	lists := countCalls(memory, "ListDir")
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/ondemand/x/new.png" {
		t.Fatalf("after negative TTL = %q", got)
	}
	if got := countCalls(memory, "ListDir"); got == lists {
		t.Fatal("stale listing must be refreshed")
	}
	// A hit found in a fresh listing does not re-list, and another miss in
	// the refreshed listing does not re-list either.
	lists = countCalls(memory, "ListDir")
	ResolveRegionAssetPath(helper, "jp", filepath.Join("x", "gone.png"))
	if got := countCalls(memory, "ListDir"); got != lists {
		t.Fatalf("fresh listing re-listed on a miss: %d -> %d", lists, got)
	}
}

func TestStoreProbeListingExpiresAfterListingTTL(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/x/y.png": []byte("x")})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{ListingTTL: 30 * time.Minute, PositiveTTL: time.Hour})
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	ctx := context.Background()

	helper.store.resolve(ctx, "jp-assets/startapp/x/y.png")
	helper.store.resolve(ctx, "jp-assets/startapp/x/z.png")
	lists := countCalls(memory, "ListDir")
	now = now.Add(31 * time.Minute)
	helper.store.resolve(ctx, "jp-assets/startapp/x/w.png")
	if got := countCalls(memory, "ListDir"); got <= lists {
		t.Fatal("listings must expire after listing_ttl")
	}
	// The positive key result outlives the listing.
	before := len(memory.Calls())
	if key, found, _ := helper.store.resolve(ctx, "jp-assets/startapp/x/y.png"); !found || key == "" || len(memory.Calls()) != before {
		t.Fatalf("positive key cache = %q %v, %d new calls", key, found, len(memory.Calls())-before)
	}
	now = now.Add(time.Hour)
	helper.ClearResolutionCache()
	helper.store.resolve(ctx, "jp-assets/startapp/x/y.png")
	if len(memory.Calls()) == before {
		t.Fatal("clear must drop the caches")
	}
}

func TestStoreProbeStoreErrorFallsBackAndOpensTheBreaker(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/ondemand/music/jacket/j/j.png": []byte("x")})
	boom := errors.New("garage down")
	memory.FailListDir = func(storage.Key) error { return boom }
	memory.FailStat = func(storage.Key) error { return boom }
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	first := "asset/jp-assets/startapp/music/jacket/j/j.png"

	for i := range 10 {
		rel := filepath.Join("music", "jacket", fmt.Sprintf("j%d", i), "j.png")
		if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/startapp/music/jacket/"+fmt.Sprintf("j%d", i)+"/j.png" {
			t.Fatalf("key %d = %q", i, got)
		}
	}
	// Failures: root listing fails -> HEAD fails (2 calls) for key 0, the
	// same for key 1 opens the breaker at the third failure; every later key
	// is refused without a store call.
	if got := requestCount(memory); got != 3 {
		t.Fatalf("store calls with an open breaker = %d, want 3: %+v", got, memory.Calls())
	}
	if !helper.store.breaker.blocked(now) {
		t.Fatal("breaker must be open")
	}
	if _, _, err := helper.store.resolve(context.Background(), "jp-assets/startapp/music/jacket/j/j.png"); !errors.Is(err, errStoreProbeOpen) {
		t.Fatalf("open breaker error = %v", err)
	}

	// Half-open after the cooldown: one probe goes through; it fails and
	// re-opens the breaker.
	now = now.Add(storeProbeBreakerCooldown + time.Second)
	ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "half", "open.png"))
	if got := requestCount(memory); got != 4 {
		t.Fatalf("half-open probe calls = %d, want 4", got)
	}
	if !helper.store.breaker.blocked(now) {
		t.Fatal("failed half-open probe must re-open the breaker")
	}

	// The store recovers: the next half-open probe succeeds and closes it.
	memory.FailListDir, memory.FailStat = nil, nil
	now = now.Add(storeProbeBreakerCooldown + time.Second)
	helper.ClearResolutionCache()
	if got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "j", "j.png")); got != "asset/jp-assets/ondemand/music/jacket/j/j.png" {
		t.Fatalf("after recovery = %q (first candidate is %q)", got, first)
	}
	if helper.store.breaker.blocked(now) {
		t.Fatal("successful half-open probe must close the breaker")
	}
}

func TestStoreProbeHeadErrorIsCachedBriefly(t *testing.T) {
	memory := storagetest.NewMemory()
	boom := errors.New("head down")
	memory.FailStat = func(storage.Key) error { return boom }
	memory.Seed(map[string][]byte{"jp-assets/startapp/other.png": []byte("x")})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	helper.store.maxListEntries = 0 // every non-empty listing is capped: HEAD path
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	ctx := context.Background()

	if _, _, err := helper.store.resolve(ctx, "jp-assets/startapp/a.png"); !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}
	stats := countCalls(memory, "Stat")
	if _, _, err := helper.store.resolve(ctx, "jp-assets/startapp/a.png"); !errors.Is(err, boom) || countCalls(memory, "Stat") != stats {
		t.Fatalf("error must be served from the cache: %v, Stat %d -> %d", err, stats, countCalls(memory, "Stat"))
	}
	memory.FailStat = nil
	memory.Seed(map[string][]byte{"jp-assets/startapp/a.png": []byte("x")})
	now = now.Add(storeProbeErrorTTL + time.Second)
	helper.store.breaker.success()
	if key, found, err := helper.store.resolve(ctx, "jp-assets/startapp/a.png"); err != nil || !found || key != "jp-assets/startapp/a.png" {
		t.Fatalf("after the error TTL = %q %v %v", key, found, err)
	}
}

func TestStoreProbeSingleflightCollapsesConcurrentProbes(t *testing.T) {
	memory := storagetest.NewMemory()
	const waiters = 16
	seed := map[string][]byte{"jp-assets/startapp/music/jacket/j/j.png": []byte("x")}
	for i := range waiters {
		seed[fmt.Sprintf("jp-assets/startapp/music/jacket/j/other%d.png", i)] = []byte("x")
	}
	memory.Seed(seed)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	memory.FailListDir = func(storage.Key) error {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return nil
	}
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})

	results := make([]string, waiters)
	var wg sync.WaitGroup
	for i := range waiters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Half the callers ask for the same key, half for siblings: both
			// share the in-flight directory listings.
			rel := filepath.Join("music", "jacket", "j", "j.png")
			if i%2 == 1 {
				rel = filepath.Join("music", "jacket", "j", fmt.Sprintf("other%d.png", i))
			}
			results[i] = ResolveRegionAssetPath(helper, "jp", rel)
		}()
	}
	<-entered
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, got := range results {
		want := "asset/jp-assets/startapp/music/jacket/j/j.png"
		if i%2 == 1 {
			want = fmt.Sprintf("asset/jp-assets/startapp/music/jacket/j/other%d.png", i)
		}
		if got != want {
			t.Fatalf("waiter %d = %q, want %q", i, got, want)
		}
	}
	// root, jp-assets/, startapp/, music/, jacket/, jacket/j/: every waiter
	// shared the same six listings.
	if lists := countCalls(memory, "ListDir"); lists != 6 {
		t.Fatalf("ListDir calls = %d, want 6 shared listings: %+v", lists, memory.Calls())
	}
}

func TestStoreProbeCancelledCallerFallsBackWithoutAbortingTheFlight(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/music/jacket/j/j.png": []byte("x")})
	release := make(chan struct{})
	memory.FailListDir = func(storage.Key) error {
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
	deadline := time.Now().Add(5 * time.Second)
	for helper.store.keys.len() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("flight result was not cached")
		}
		time.Sleep(5 * time.Millisecond)
	}
	before := len(memory.Calls())
	if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/startapp/music/jacket/j/j.png" || len(memory.Calls()) != before {
		t.Fatalf("next caller = %q, calls %d -> %d", got, before, len(memory.Calls()))
	}
}

// blockingStore holds every call until its context expires, like a backend
// that stopped answering.
type blockingStore struct {
	storage.Store
	calls atomic.Int32
}

func (s *blockingStore) Stat(ctx context.Context, _ storage.Key) (storage.Object, error) {
	s.calls.Add(1)
	<-ctx.Done()
	return storage.Object{}, ctx.Err()
}

func (s *blockingStore) ListDir(ctx context.Context, _ storage.Key, _ func(storage.DirEntry) error) error {
	s.calls.Add(1)
	<-ctx.Done()
	return ctx.Err()
}

func TestStoreProbeTimeoutBoundsEachCallAndTripsTheBreaker(t *testing.T) {
	store := &blockingStore{Store: storagetest.NewMemory()}
	helper := storeOnlyHelper(t, store, StoreProbeConfig{Timeout: 20 * time.Millisecond})

	startedAt := time.Now()
	for i := range 20 {
		rel := filepath.Join("music", "jacket", fmt.Sprintf("j%d", i), "j.png")
		if got := ResolveRegionAssetPath(helper, "jp", rel); got != "asset/jp-assets/startapp/music/jacket/"+fmt.Sprintf("j%d", i)+"/j.png" {
			t.Fatalf("timed-out probe %d = %q", i, got)
		}
	}
	if elapsed := time.Since(startedAt); elapsed > 2*time.Second {
		t.Fatalf("20 keys against a hung store took %v", elapsed)
	}
	// ListDir + HEAD time out for key 0, ListDir for key 1 opens the breaker.
	if got := store.calls.Load(); got != 3 {
		t.Fatalf("store calls = %d, want 3 before the breaker opened", got)
	}
}

func TestStoreProbeClearGuardsInFlightResults(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/x/y.png": []byte("x")})
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	memory.FailListDir = func(storage.Key) error {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return nil
	}
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		helper.store.resolve(context.Background(), "jp-assets/startapp/x/y.png")
	}()
	<-entered
	helper.ClearResolutionCache()
	close(release)
	<-done
	// The key result and the root listing were computed before the clear
	// and must not be stored; the deeper listings were fetched afterwards.
	if _, ok := helper.store.dirs.lookup("", time.Now()); ok || helper.store.keys.len() != 0 {
		t.Fatalf("results computed before the clear must not be stored: root cached=%v keys=%d", ok, helper.store.keys.len())
	}
	if _, ok := helper.store.dirs.lookup("jp-assets/", time.Now()); !ok {
		t.Fatal("listings fetched after the clear are fresh and stay cached")
	}
}

func TestStoreProbeWarmUpListsConfiguredPrefixes(t *testing.T) {
	memory := storagetest.NewMemory()
	seed := map[string][]byte{}
	for i := range 70 {
		name := fmt.Sprintf("jacket_s_%03d", i)
		seed["jp-assets/startapp/music/jacket/"+name+"/"+name+".png"] = []byte("x")
	}
	seed["jp-assets/startapp/thumbnail/chara/a.png"] = []byte("x")
	memory.Seed(seed)
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{WarmPrefixes: []string{
		"jp-assets/startapp/thumbnail/chara", "/jp-assets/startapp/music/jacket/", "..", "",
	}})
	NewAssetHelper("", nil).WarmUp(context.Background()) // storeless: no-op
	helper.WarmUp(context.Background())
	warmed := requestCount(memory)
	if warmed == 0 {
		t.Fatal("warm-up must list the configured prefixes")
	}
	if got := ResolveRegionAssetPath(helper, "jp", filepath.Join("thumbnail", "chara", "A.png")); got != "asset/jp-assets/startapp/thumbnail/chara/a.png" {
		t.Fatalf("thumbnail = %q", got)
	}
	if got := ResolveRegionAssetPath(helper, "jp", filepath.Join("music", "jacket", "jacket_s_010", "jacket_s_010.png")); got != "asset/jp-assets/startapp/music/jacket/jacket_s_010/jacket_s_010.png" {
		t.Fatalf("jacket = %q", got)
	}
	if got := requestCount(memory); got != warmed {
		t.Fatalf("warmed prefixes must answer without store calls: %d -> %d", warmed, got)
	}
}

func TestStoreProbeLogsErrorsAtMostOncePerInterval(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.FailStat = func(storage.Key) error { return errors.New("down") }
	memory.Seed(map[string][]byte{"jp-assets/startapp/other.png": []byte("x")})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	helper.store.maxListEntries = 0
	helper.store.breaker.threshold = 1 << 30
	now := time.Now()
	helper.store.now = func() time.Time { return now }
	ctx := context.Background()
	for i := range 5 {
		helper.store.resolve(ctx, storage.Key("jp-assets/startapp/k/"+string(rune('a'+i))))
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

func TestStoreBreaker(t *testing.T) {
	now := time.Now()
	breaker := storeBreaker{threshold: 3, cooldown: 30 * time.Second}
	for range 2 {
		if !breaker.allow(now) || breaker.failure(now) {
			t.Fatal("below the threshold the breaker stays closed")
		}
	}
	if !breaker.failure(now) || !breaker.blocked(now) || breaker.allow(now) {
		t.Fatal("third failure must open the breaker")
	}
	if breaker.failure(now) {
		t.Fatal("a failure while open must not report opening again")
	}
	later := now.Add(31 * time.Second)
	if breaker.blocked(later) || !breaker.allow(later) {
		t.Fatal("after the cooldown one probe is allowed")
	}
	if !breaker.blocked(later) || breaker.allow(later) {
		t.Fatal("only one half-open probe at a time")
	}
	breaker.success()
	if breaker.blocked(later) || !breaker.allow(later) || breaker.failures != 0 {
		t.Fatal("success must close the breaker")
	}
}

func TestParentDir(t *testing.T) {
	cases := map[string]struct {
		parent string
		ok     bool
	}{"": {"", false}, "a/": {"", true}, "a/b/": {"a/", true}, "a/b/c/": {"a/b/", true}}
	for dir, want := range cases {
		if parent, ok := parentDir(dir); parent != want.parent || ok != want.ok {
			t.Fatalf("parentDir(%q) = %q %v, want %q %v", dir, parent, ok, want.parent, want.ok)
		}
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

func TestProbeCacheBoundsEntriesWeightAndGeneration(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)
	cache := newProbeCache[int](2, 0)
	gen := cache.currentGeneration()
	cache.store("a", 1, 1, later, gen)
	cache.store("b", 2, 1, later, gen)
	cache.lookup("a", now)
	cache.store("c", 3, 1, later, gen)
	if _, ok := cache.lookup("b", now); ok {
		t.Fatal("least recently used entry must be evicted")
	}
	if v, ok := cache.lookup("a", now); !ok || v != 1 {
		t.Fatal("recently used entry must survive")
	}
	if _, ok := cache.lookup("a", later); ok {
		t.Fatal("expired entry must be dropped")
	}
	cache.clear()
	if cache.store("stale", 9, 1, later, gen) || cache.len() != 0 {
		t.Fatal("a store from before the clear must be rejected")
	}
	if !cache.store("fresh", 9, 1, later, cache.currentGeneration()) {
		t.Fatal("a store after the clear must be accepted")
	}

	weighted := newProbeCache[int](10, 5)
	gen = weighted.currentGeneration()
	if weighted.store("big", 1, 6, later, gen) || weighted.len() != 0 {
		t.Fatal("an entry over the weight budget must be rejected")
	}
	weighted.store("x", 1, 3, later, gen)
	weighted.store("y", 2, 3, later, gen)
	if _, ok := weighted.lookup("x", now); ok || weighted.len() != 1 || weighted.weight != 3 {
		t.Fatalf("weight budget not enforced: len=%d weight=%d", weighted.len(), weighted.weight)
	}
}
