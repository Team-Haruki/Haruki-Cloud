package assets

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

// cappedThumbnailStore seeds n card thumbnails in a directory the probe
// cannot list (capped), so every lookup there falls back to one HEAD each,
// as on the cold production store.
func cappedThumbnailStore(t *testing.T, n int, cfg StoreProbeConfig) (*AssetHelper, *storagetest.Memory) {
	t.Helper()
	memory := storagetest.NewMemory()
	seed := map[string][]byte{}
	for i := range n {
		seed[fmt.Sprintf("jp-assets/startapp/thumbnail/chara/res%03d_normal.png", i)] = []byte("x")
	}
	memory.Seed(seed)
	helper := storeOnlyHelper(t, memory, cfg)
	helper.store.maxListEntries = 1
	return helper, memory
}

func thumbnailPath(i int) string {
	return filepath.Join("thumbnail", "chara", fmt.Sprintf("res%03d_normal.png", i))
}

func TestProbeBudgetCapsStoreCallsPerRequest(t *testing.T) {
	helper, memory := cappedThumbnailStore(t, 60, StoreProbeConfig{RequestMaxStoreCalls: 10, RequestBudget: -1, RequestConcurrency: -1})
	ctx, trace := commandtrace.WithTrace(context.Background())
	request := helper.WithContext(ctx)

	for i := range 60 {
		want := "asset/jp-assets/startapp/thumbnail/chara/" + fmt.Sprintf("res%03d_normal.png", i)
		if got := ResolveRegionAssetPath(request, "jp", thumbnailPath(i)); got != want {
			t.Fatalf("lookup %d = %q, want %q (the first candidate is the graceful fallback)", i, got, want)
		}
	}
	if stats := countCalls(memory, "Stat"); stats > 12 {
		t.Fatalf("one request made %d HEADs, budget is 10 store calls", stats)
	}
	if refused, ok := traceOperation(trace.Snapshot(), "asset.store_probe_budget_exhausted"); !ok || refused.Count < 40 {
		t.Fatalf("refusals = %+v", refused)
	}

	// The next request has its own budget and continues where it stopped;
	// results resolved by the first request stay cached.
	// (Listing the capped directory and its parents counted as well, so
	// fewer than 10 keys got a HEAD.)
	before := countCalls(memory, "Stat")
	next := helper.WithContext(context.Background())
	for i := range before {
		ResolveRegionAssetPath(next, "jp", thumbnailPath(i))
	}
	if got := countCalls(memory, "Stat"); got != before {
		t.Fatalf("cached keys must not be probed again: %d -> %d", before, got)
	}
	ResolveRegionAssetPath(next, "jp", thumbnailPath(before))
	if got := countCalls(memory, "Stat"); got != before+1 {
		t.Fatalf("the next request probes the next key: %d -> %d", before, got)
	}
}

func TestProbeBudgetStopsWaitingAfterDeadline(t *testing.T) {
	helper, memory := cappedThumbnailStore(t, 40, StoreProbeConfig{RequestBudget: 100 * time.Millisecond, RequestMaxStoreCalls: -1, RequestConcurrency: -1})
	memory.FailStat = func(storage.Key) error {
		time.Sleep(30 * time.Millisecond)
		return nil
	}
	request := helper.WithContext(context.Background())
	started := time.Now()
	for i := range 40 {
		ResolveRegionAssetPath(request, "jp", thumbnailPath(i))
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("40 slow lookups took %v; the 100ms budget should cut them short", elapsed)
	}
	if stats := countCalls(memory, "Stat"); stats >= 40 {
		t.Fatalf("every key was probed (%d HEADs) despite the budget", stats)
	}
}

func TestProbeBudgetBoundsConcurrency(t *testing.T) {
	helper, memory := cappedThumbnailStore(t, 32, StoreProbeConfig{RequestConcurrency: 2, RequestBudget: -1, RequestMaxStoreCalls: -1})
	var active, peak atomic.Int32
	memory.FailStat = func(storage.Key) error {
		now := active.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return nil
	}
	request := helper.WithContext(context.Background())
	tasks := make([]func(*AssetHelper), 0, 32)
	for i := range 32 {
		tasks = append(tasks, func(h *AssetHelper) { ResolveRegionAssetPath(h, "jp", thumbnailPath(i)) })
	}
	if err := request.Prefetch(tasks); err != nil {
		t.Fatal(err)
	}
	if got := peak.Load(); got > 2 {
		t.Fatalf("peak concurrent HEADs = %d, want <= 2", got)
	}
}

func TestProbeBudgetDisabledAndShared(t *testing.T) {
	helper, memory := cappedThumbnailStore(t, 30, StoreProbeConfig{RequestBudget: -1, RequestMaxStoreCalls: -1, RequestConcurrency: -1})
	request := helper.WithContext(context.Background())
	for i := range 30 {
		ResolveRegionAssetPath(request, "jp", thumbnailPath(i))
	}
	if got := countCalls(memory, "Stat"); got != 30 {
		t.Fatalf("disabled limits: %d HEADs, want 30", got)
	}
	if helper.store.budgetFor(WithProbeBudget(context.Background())) != nil {
		t.Fatal("all limits disabled means no budget")
	}
	if helper.store.budgetFor(context.Background()) != nil {
		t.Fatal("a context without a budget is unlimited")
	}

	ctx := WithProbeBudget(context.Background())
	if WithProbeBudget(ctx) != ctx {
		t.Fatal("WithProbeBudget must keep an existing budget")
	}
	var unset context.Context
	if WithProbeBudget(unset) == nil {
		t.Fatal("a nil context gets a background context")
	}
	first := helper.WithContext(ctx).ctx.Value(probeBudgetKey{})
	second := helper.WithContext(ctx).ctx.Value(probeBudgetKey{})
	if first == nil || first != second {
		t.Fatal("helper copies bound to one request context share its budget")
	}
}

func TestProbeBudgetDefaults(t *testing.T) {
	cfg := StoreProbeConfig{}.withDefaults()
	if cfg.RequestBudget != DefaultStoreProbeRequestBudget || cfg.RequestMaxStoreCalls != DefaultStoreProbeRequestMaxStoreCalls ||
		cfg.RequestConcurrency != DefaultStoreProbeRequestConcurrency || cfg.WarmInterval != DefaultStoreProbeListingTTL*3/4 {
		t.Fatalf("defaults = %+v", cfg)
	}
}

func TestResolveWarmPrefixes(t *testing.T) {
	defaults := DefaultWarmPrefixes()
	if len(defaults) != 15 || defaults[0] != "jp-assets/startapp/thumbnail/chara" {
		t.Fatalf("defaults = %v", defaults)
	}
	if got := ResolveWarmPrefixes(nil); !reflect.DeepEqual(got, defaults) {
		t.Fatalf("nil = %v", got)
	}
	if got := ResolveWarmPrefixes([]string{}); !reflect.DeepEqual(got, defaults) {
		t.Fatalf("empty = %v", got)
	}
	if got := ResolveWarmPrefixes([]string{"none"}); got != nil {
		t.Fatalf("none = %v", got)
	}
	if got := ResolveWarmPrefixes([]string{"jp-assets/x"}); !reflect.DeepEqual(got, []string{"jp-assets/x"}) {
		t.Fatalf("explicit = %v", got)
	}
}

func TestWarmLoopRefreshesListingsAndUsesWarmTimeout(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"jp-assets/startapp/thumbnail/chara/a_normal.png": []byte("x")})
	var slowListing atomic.Bool
	slowListing.Store(true)
	memory.FailListDir = func(key storage.Key) error {
		if slowListing.Load() && key == "jp-assets/startapp/thumbnail/chara" {
			time.Sleep(40 * time.Millisecond)
		}
		return nil
	}
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{
		Timeout:      10 * time.Millisecond,
		WarmPrefixes: []string{"jp-assets/startapp/thumbnail/chara"},
		WarmInterval: 20 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		helper.WarmLoop(ctx)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		index, ok := helper.store.dirs.lookup("jp-assets/startapp/thumbnail/chara/", time.Now())
		if ok {
			if index.unavailable {
				t.Fatal("the warm listing must use the longer warm timeout, not the 10ms request timeout")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("warm-up did not list the prefix")
		}
		time.Sleep(5 * time.Millisecond)
	}
	slowListing.Store(false)

	// A thumbnail published after the first warm-up appears through the
	// periodic refresh, without a request-time listing.
	memory.Seed(map[string][]byte{"jp-assets/startapp/thumbnail/chara/b_normal.png": []byte("x")})
	for {
		index, ok := helper.store.dirs.lookup("jp-assets/startapp/thumbnail/chara/", time.Now())
		if ok {
			if _, found := index.match("b_normal.png", true); found {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the refresh did not pick up the new thumbnail")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WarmLoop did not stop with its context")
	}
	NewAssetHelper("", nil).WarmLoop(context.Background()) // storeless: no-op
}

func traceOperation(snapshot commandtrace.Snapshot, name string) (commandtrace.Stats, bool) {
	for _, operation := range snapshot.Operations {
		if operation.Name == name {
			return operation, true
		}
	}
	return commandtrace.Stats{}, false
}
