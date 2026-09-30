package assets

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func TestEventBannerSelectionSurvivesNegativeExpiry(t *testing.T) {
	memory := storagetest.NewMemory()
	seed := map[string][]byte{}
	for i := 0; i < 70; i++ {
		name := fmt.Sprintf("event%d", i)
		seed["kr-assets/startapp/home/banner/"+name+"/other.png"] = []byte("other")
		seed["kr-assets/ondemand/event/"+name+"/other.png"] = []byte("other")
		seed["kr-assets/ondemand/event_story/"+name+"/screen_image/banner_event_story.png"] = []byte("banner")
	}
	memory.Seed(seed)
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	helper.store.bulkMinChildren = 1000
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	helper.store.now = func() time.Time { return now }
	resolve := func(h *AssetHelper) {
		for i := 0; i < 70; i++ {
			name := fmt.Sprintf("event%d", i)
			want := "asset/kr-assets/ondemand/event_story/" + name + "/screen_image/banner_event_story.png"
			if got := ResolveEventBannerPath(h, "kr", name); got != want {
				t.Fatalf("banner %d = %q, want %q", i, got, want)
			}
		}
	}
	resolve(helper)
	for _, minutes := range []int{6, 12, 29} {
		now = time.Date(2026, 9, 30, 0, minutes, 0, 0, time.UTC)
		before := requestCount(memory)
		ctx, trace := commandtrace.WithTrace(context.Background())
		resolve(helper.WithContext(ctx))
		extra := requestCount(memory) - before
		t.Logf("70 fallback banners at minute %d: %d new store calls", minutes, extra)
		if extra != 0 {
			t.Fatalf("cached selections caused %d store calls", extra)
		}
		var hits int
		for _, op := range trace.Snapshot().Operations {
			if op.Name == "asset.store_selection_cache_hit" {
				hits = op.Count
			}
		}
		if hits != 70 {
			t.Fatalf("selection cache hits = %d", hits)
		}
	}
}

func TestCandidateSelectionExpiresWithoutSlidingAndClears(t *testing.T) {
	for _, clearCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("clear=%v", clearCache), func(t *testing.T) {
			memory := storagetest.NewMemory()
			memory.Seed(map[string][]byte{"kr-assets/ondemand/event_story/e/screen_image/banner_event_story.png": []byte("fallback")})
			helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
			now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
			helper.store.now = func() time.Time { return now }
			fallback := ResolveEventBannerPath(helper, "kr", "e")
			memory.Seed(map[string][]byte{"kr-assets/startapp/home/banner/e/e.png": []byte("new preferred")})
			now = now.Add(29 * time.Minute)
			if got := ResolveEventBannerPath(helper, "kr", "e"); got != fallback {
				t.Fatalf("unexpired choice changed: %s", got)
			}
			if clearCache {
				helper.ClearResolutionCache()
			} else {
				now = now.Add(time.Minute)
			}
			want := "asset/kr-assets/startapp/home/banner/e/e.png"
			if got := ResolveEventBannerPath(helper, "kr", "e"); got != want {
				t.Fatalf("refreshed choice = %s, want %s", got, want)
			}
		})
	}
}

func TestCandidateSelectionClearRejectsInflightFill(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"kr-assets/startapp/a.png": []byte("a")})
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	memory.FailListDir = func(storage.Key) error { once.Do(func() { close(started); <-release }); return nil }
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{})
	done := make(chan string, 1)
	go func() { done <- ResolveRegionAssetPath(helper, "kr", "a.png") }()
	<-started
	helper.ClearResolutionCache()
	close(release)
	if got := <-done; got != "asset/kr-assets/startapp/a.png" {
		t.Fatalf("inflight resolution = %q", got)
	}
	if got := helper.store.selections.len(); got != 0 {
		t.Fatalf("old generation refilled %d choices", got)
	}
	ResolveRegionAssetPath(helper, "kr", "a.png")
	if got := helper.store.selections.len(); got != 1 {
		t.Fatalf("fresh generation choices = %d", got)
	}
}

func TestCandidateSelectionOrderRegionAndPositiveTTL(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{
		"kr-assets/startapp/A.PNG": []byte("a"),
		"kr-assets/startapp/b.png": []byte("b"),
		"jp-assets/startapp/b.png": []byte("jp b"),
	})
	helper := storeOnlyHelper(t, memory, StoreProbeConfig{PositiveTTL: 2 * time.Minute, NegativeTTL: time.Second})
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	helper.store.now = func() time.Time { return now }
	for _, tc := range []struct {
		region string
		paths  []string
		want   string
	}{
		{"kr", []string{"a.png", "b.png"}, "asset/kr-assets/startapp/A.PNG"},
		{"kr", []string{"b.png", "a.png"}, "asset/kr-assets/startapp/b.png"},
		{"jp", []string{"a.png", "b.png"}, "asset/jp-assets/startapp/b.png"},
	} {
		if got := ResolveRegionAssetPath(helper, tc.region, tc.paths...); got != tc.want {
			t.Fatalf("choice = %q, want %q", got, tc.want)
		}
	}
	memory.Seed(map[string][]byte{"jp-assets/startapp/a.png": []byte("jp a")})
	now = now.Add(2 * time.Minute)
	if got := ResolveRegionAssetPath(helper, "jp", "a.png", "b.png"); got != "asset/jp-assets/startapp/a.png" {
		t.Fatalf("positive TTL did not bound choice: %q", got)
	}
}
