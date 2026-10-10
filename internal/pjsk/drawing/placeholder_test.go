package drawing

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

// placeholderDrawingServer answers like Drawing: the first `flagged` renders
// drew missing-asset placeholders (Cache-Store: 0 plus the count header), the
// rest are complete bytes.
type placeholderDrawingServer struct {
	*httptest.Server
	flagged atomic.Int32
	calls   atomic.Int32
}

func newPlaceholderDrawingServer(t *testing.T, flagged int32) *placeholderDrawingServer {
	t.Helper()
	s := &placeholderDrawingServer{}
	s.flagged.Store(flagged)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		w.Header().Set(headerNode, "render-1")
		w.Header().Set("Content-Type", "image/png")
		if s.flagged.Add(-1) >= 0 {
			w.Header().Set(headerCacheStore, "0")
			w.Header().Set(headerRenderMissingAssets, "2")
			_, _ = w.Write([]byte("placeholder"))
			return
		}
		_, _ = w.Write([]byte("complete"))
	}))
	t.Cleanup(s.Close)
	return s
}

func placeholderCacheEntry(t *testing.T, lc *localRenderCache) *localRenderEntry {
	t.Helper()
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if len(lc.entries) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(lc.entries))
	}
	for _, entry := range lc.entries {
		return entry
	}
	return nil
}

func expireAll(lc *localRenderCache) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	for _, entry := range lc.entries {
		entry.permanent = false
		entry.expiresAt = time.Now().Add(-time.Second)
	}
}

func renderCardList(t *testing.T, client *HarukiDrawingClient) (string, map[string]int) {
	t.Helper()
	ctx, trace := commandtrace.WithNewTrace(t.Context())
	data, err := client.WithContext(ctx).GenerateCardList(&CardListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return string(data), traceOps(trace)
}

// card/list is an Infinite rule: a flagged render must still expire, after the
// placeholder TTL, and a hit must not extend it.
func TestPlaceholderRenderIsCachedBrieflyAndNeverSlides(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote=%v", remote), func(t *testing.T) {
			server := newPlaceholderDrawingServer(t, 1)
			client := NewHarukiDrawingClient(server.URL,
				WithArtifactConfig(ArtifactConfig{Endpoints: []string{"*"}}),
				WithPlaceholderCacheTTL(30*time.Minute))
			cache := client.localCache
			if remote {
				index := &fakeRenderIndex{}
				remoteCache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: index, PlaceholderTTL: 30 * time.Minute})
				t.Cleanup(func() { _ = remoteCache.Close() })
				client.SetRenderCache(remoteCache)
				cache = remoteCache.placeholder
			}

			before := time.Now()
			data, ops := renderCardList(t, client)
			if data != "placeholder" || ops["drawing.placeholder_render"] != 1 || ops["drawing.cache_placeholder_store"] != 1 {
				t.Fatalf("first render = %q, ops %v", data, ops)
			}
			entry := placeholderCacheEntry(t, cache)
			if entry.permanent {
				t.Fatal("a flagged render on an infinite endpoint was cached permanently")
			}
			expires := entry.expiresAt
			if expires.Before(before.Add(30*time.Minute)) || expires.After(time.Now().Add(30*time.Minute)) {
				t.Fatalf("expiresAt = %v, want now+30m", expires)
			}

			data, ops = renderCardList(t, client)
			if data != "placeholder" || server.calls.Load() != 1 || ops[drawingCacheHitTraceField] != 1 {
				t.Fatalf("second request = %q, drawing calls %d, ops %v", data, server.calls.Load(), ops)
			}
			if remote && ops["drawing.cache_placeholder_hit"] != 1 {
				t.Fatalf("ops = %v", ops)
			}
			if got := placeholderCacheEntry(t, cache).expiresAt; !got.Equal(expires) {
				t.Fatalf("hit moved expiresAt %v -> %v", expires, got)
			}

			// The asset has shipped and the entry expired: the next request renders the real image.
			expireAll(cache)
			if data, _ := renderCardList(t, client); data != "complete" || server.calls.Load() != 2 {
				t.Fatalf("after expiry = %q, drawing calls %d", data, server.calls.Load())
			}
			if data, _ := renderCardList(t, client); data != "complete" || server.calls.Load() != 2 {
				t.Fatalf("complete render was not cached: %q, drawing calls %d", data, server.calls.Load())
			}
		})
	}
}

func TestPlaceholderTTLIsCappedByTheEndpointTTL(t *testing.T) {
	server := newPlaceholderDrawingServer(t, 1)
	client := NewHarukiDrawingClient(server.URL)
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}})
	t.Cleanup(func() { _ = cache.Close() })
	client.SetRenderCache(cache)

	if _, err := client.GenerateSKWinRateImage(&WinRateRequest{}); err != nil {
		t.Fatal(err)
	}
	entry := placeholderCacheEntry(t, cache.placeholder)
	if left := time.Until(entry.expiresAt); left > skRenderCacheBucketJPAndCN || left <= 0 {
		t.Fatalf("sk/winrate placeholder entry lives %v, want <= %v", left, skRenderCacheBucketJPAndCN)
	}
}

func TestNegativePlaceholderTTLNeverCachesAFlaggedRender(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote=%v", remote), func(t *testing.T) {
			server := newPlaceholderDrawingServer(t, 2)
			client := NewHarukiDrawingClient(server.URL, WithPlaceholderCacheTTL(-1))
			if remote {
				cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}, PlaceholderTTL: -1})
				t.Cleanup(func() { _ = cache.Close() })
				if cache.placeholder != nil {
					t.Fatal("disabled placeholder cache was constructed")
				}
				client.SetRenderCache(cache)
			}
			for i, want := range []string{"placeholder", "placeholder", "complete", "complete"} {
				if data, _ := renderCardList(t, client); data != want {
					t.Fatalf("request %d = %q, want %q", i, data, want)
				}
			}
			if server.calls.Load() != 3 {
				t.Fatalf("drawing calls = %d; flagged renders must not be cached when disabled", server.calls.Load())
			}
		})
	}
}

func TestForcedCompleteRenderReplacesAFlaggedEntry(t *testing.T) {
	server := newPlaceholderDrawingServer(t, 1)
	client := NewHarukiDrawingClient(server.URL)
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}})
	t.Cleanup(func() { _ = cache.Close() })
	client.SetRenderCache(cache)

	if data, _ := renderCardList(t, client); data != "placeholder" {
		t.Fatalf("first render = %q", data)
	}
	forced, err := client.WithContext(WithForceRender(t.Context())).GenerateCardList(&CardListRequest{})
	if err != nil || string(forced) != "complete" {
		t.Fatalf("forced render = %q, %v", forced, err)
	}
	cache.placeholder.mu.Lock()
	left := len(cache.placeholder.entries)
	cache.placeholder.mu.Unlock()
	if left != 0 {
		t.Fatalf("flagged entry survived a complete render (%d entries)", left)
	}
	if data, _ := renderCardList(t, client); data != "complete" || server.calls.Load() != 2 {
		t.Fatalf("after force = %q, drawing calls %d", data, server.calls.Load())
	}
}

func TestFlaggedRenderOnANoStorePathIsCountedButNotCached(t *testing.T) {
	server := newPlaceholderDrawingServer(t, 2)
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{
		Endpoints: []string{"*"}, NoStorePaths: []string{"api/pjsk/sk"},
	}))
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}})
	t.Cleanup(func() { _ = cache.Close() })
	client.SetRenderCache(cache)

	for range 2 {
		ctx, trace := commandtrace.WithNewTrace(t.Context())
		if _, err := client.WithContext(ctx).GenerateSKSpeed(&SpeedRequest{}); err != nil {
			t.Fatal(err)
		}
		if ops := traceOps(trace); ops["drawing.placeholder_render"] != 1 || ops["drawing.cache_placeholder_store"] != 0 {
			t.Fatalf("ops = %v", ops)
		}
	}
	if server.calls.Load() != 2 {
		t.Fatalf("drawing calls = %d", server.calls.Load())
	}
}

// A store-ref carries the count in the ref too; Cloud reads it when the
// header is absent.
func TestStoreRefMissingAssetsFieldIsNoted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerArtifact, "1")
		w.Header().Set(headerArtifactMode, artifactModeStoreRef)
		w.Header().Set(headerCacheStore, "0")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testArtifactRefJSON(map[string]string{
			"hash": `"` + storeRefHash + `"`, "cdn_path": `"` + storeRefPath + `"`, "object_key": `"` + storeRefPath + `"`,
			"media_type": `"image/jpeg"`, "index_written": "false", "node_name": `"gw-1"`, "expires_at": `null`,
			"missing_assets": "3",
		})))
	}))
	t.Cleanup(server.Close)
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(storeRefConfig(nil, "api/pjsk/sk")))
	d := newRenderDirective(strings.Repeat("e", 64), renderCachePolicy{APIPath: "api/pjsk/sk/speed", UserID: "public"}, time.Minute, false)
	d.StoreRef = true
	resp, err := client.client.R().Post(server.URL + "/api/pjsk/sk/speed")
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace := commandtrace.WithNewTrace(withResponseCacheability(t.Context()))
	if _, err := client.WithContext(ctx).successBody("/api/pjsk/sk/speed", d, resp); err != nil {
		t.Fatal(err)
	}
	if d.outcome.Ref == nil || d.outcome.Ref.MissingAssets != 3 {
		t.Fatalf("outcome = %+v", d.outcome)
	}
	if got := renderPlaceholders(ctx); got != 3 {
		t.Fatalf("placeholders = %d", got)
	}
	if ops := traceOps(trace); ops["drawing.placeholder_render"] != 1 {
		t.Fatalf("ops = %v", ops)
	}
}

func TestPlaceholderTTLHelpers(t *testing.T) {
	for _, tc := range []struct {
		configured, want time.Duration
	}{{0, time.Hour}, {-time.Second, 0}, {5 * time.Minute, 5 * time.Minute}} {
		if got := effectivePlaceholderTTL(tc.configured); got != tc.want {
			t.Errorf("effectivePlaceholderTTL(%v) = %v, want %v", tc.configured, got, tc.want)
		}
	}
	for _, tc := range []struct {
		rule        time.Duration
		infinite    bool
		placeholder time.Duration
		want        time.Duration
	}{
		{0, true, time.Hour, time.Hour},
		{7 * 24 * time.Hour, false, time.Hour, time.Hour},
		{10 * time.Second, false, time.Hour, 10 * time.Second},
		{0, false, time.Hour, time.Hour},
		{time.Minute, false, 0, 0},
	} {
		if got := placeholderEntryTTL(tc.rule, tc.infinite, tc.placeholder); got != tc.want {
			t.Errorf("placeholderEntryTTL(%v, %v, %v) = %v, want %v", tc.rule, tc.infinite, tc.placeholder, got, tc.want)
		}
	}
	for value, want := range map[string]int64{"": 0, "0": 0, "2": 2, " 7 ": 7, "-1": 0, "x": 0} {
		if got := parseRenderMissingAssets(value); got != want {
			t.Errorf("parseRenderMissingAssets(%q) = %d, want %d", value, got, want)
		}
	}
	markRenderPlaceholders(context.Background(), 1)
	if renderPlaceholders(context.Background()) != 0 {
		t.Fatal("a context without cacheability state reported placeholders")
	}
	ctx := withResponseCacheability(context.Background())
	markRenderPlaceholders(ctx, 4)
	markRenderPlaceholders(ctx, 2)
	if renderPlaceholders(ctx) != 4 {
		t.Fatalf("placeholders = %d, want the largest count", renderPlaceholders(ctx))
	}
	var nilCache *localRenderCache
	nilCache.delete("k")
	nilCache.setPlaceholder(ctx, "k", []byte("x"), time.Minute, false)
}
