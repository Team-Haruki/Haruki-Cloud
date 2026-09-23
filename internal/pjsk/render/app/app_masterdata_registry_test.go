package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

type fakeMasterdataRegistry struct {
	mu       sync.Mutex
	hashes   map[string]string
	etagOnly bool
	requests int
	served   int
}

func (r *fakeMasterdataRegistry) set(region, hash string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hashes[region] = hash
}

func (r *fakeMasterdataRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests++
	var region string
	if _, err := fmt.Sscanf(req.URL.Path, "/v1/master/%s", &region); err != nil {
		http.NotFound(w, req)
		return
	}
	region = region[:len(region)-len("/current")]
	hash, ok := r.hashes[region]
	if !ok {
		http.NotFound(w, req)
		return
	}
	etag := `"` + hash + `"`
	if req.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	r.served++
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	if r.etagOnly {
		fmt.Fprintf(w, `{"region":%q}`, region)
		return
	}
	fmt.Fprintf(w, `{"region":%q,"contentHash":%q,"createdAt":"2026-09-24T00:00:00Z"}`, region, hash)
}

type atomicMasterdataResetter struct {
	mu    sync.Mutex
	count int
}

func (r *atomicMasterdataResetter) ResetMasterdataCache() {
	r.mu.Lock()
	r.count++
	r.mu.Unlock()
}

func (r *atomicMasterdataResetter) resets() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func waitForResets(t *testing.T, r *atomicMasterdataResetter, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.resets() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("resets = %d, want at least %d", r.resets(), want)
}

func newRegistryTestState(t *testing.T, registry *fakeMasterdataRegistry, delays []time.Duration, regions ...renderregion.Value) (*registryMasterdataRefreshState, map[renderregion.Value]*atomicMasterdataResetter, *atomicMasterdataResetter, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(registry)
	t.Cleanup(server.Close)
	resetters := make(map[renderregion.Value]*atomicMasterdataResetter, len(regions))
	providers := make(map[renderregion.Value]masterdataCacheResetter, len(regions))
	for _, region := range regions {
		resetter := &atomicMasterdataResetter{}
		resetters[region] = resetter
		providers[region] = resetter
	}
	additional := &atomicMasterdataResetter{}
	state := newRegistryMasterdataRefreshState(server.URL+"/", server.Client(), providers, additional)
	state.settleDelays = delays
	t.Cleanup(state.stop)
	return state, resetters, additional, server
}

func TestRegistryMasterdataRefreshResetsOnContentHashChange(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1", "cn": "sha256:cn-1"}}
	state, resetters, additional, _ := newRegistryTestState(t, registry, nil, renderregion.JP, renderregion.CN)
	jp, cn := resetters[renderregion.JP], resetters[renderregion.CN]
	ctx := context.Background()

	state.poll(ctx)
	if jp.resets() != 0 || cn.resets() != 0 || additional.resets() != 0 {
		t.Fatalf("first observation must only record: jp=%d cn=%d additional=%d", jp.resets(), cn.resets(), additional.resets())
	}
	if state.signals[renderregion.JP] != "sha256:jp-1" || state.etags[renderregion.JP] != `"sha256:jp-1"` {
		t.Fatalf("recorded state = signals:%v etags:%v", state.signals, state.etags)
	}

	state.poll(ctx)
	if jp.resets() != 0 || cn.resets() != 0 || additional.resets() != 0 {
		t.Fatalf("unchanged registry must not reset: jp=%d cn=%d additional=%d", jp.resets(), cn.resets(), additional.resets())
	}
	if registry.served != 2 {
		t.Fatalf("unchanged poll should be answered with 304 via ETag, served=%d requests=%d", registry.served, registry.requests)
	}

	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	if jp.resets() != 1 || cn.resets() != 0 || additional.resets() != 1 {
		t.Fatalf("jp change must reset jp and shared caches only: jp=%d cn=%d additional=%d", jp.resets(), cn.resets(), additional.resets())
	}

	state.poll(ctx)
	if jp.resets() != 1 || cn.resets() != 0 || additional.resets() != 1 {
		t.Fatalf("settled registry must not reset again: jp=%d cn=%d additional=%d", jp.resets(), cn.resets(), additional.resets())
	}

	registry.set("jp", "sha256:jp-3")
	registry.set("cn", "sha256:cn-2")
	state.poll(ctx)
	if jp.resets() != 2 || cn.resets() != 1 || additional.resets() != 3 {
		t.Fatalf("both regions changed: jp=%d cn=%d additional=%d", jp.resets(), cn.resets(), additional.resets())
	}
}

func TestRegistryMasterdataRefreshSchedulesSettleResets(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1", "cn": "sha256:cn-1"}}
	state, resetters, additional, _ := newRegistryTestState(t, registry, []time.Duration{20 * time.Millisecond, 60 * time.Millisecond}, renderregion.JP, renderregion.CN)
	jp, cn := resetters[renderregion.JP], resetters[renderregion.CN]
	ctx := context.Background()

	state.poll(ctx)
	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	if jp.resets() != 1 {
		t.Fatalf("immediate reset expected, got %d", jp.resets())
	}
	waitForResets(t, jp, 3)
	time.Sleep(80 * time.Millisecond)
	if jp.resets() != 3 || cn.resets() != 0 || additional.resets() != 3 {
		t.Fatalf("one immediate plus two settle resets expected: jp=%d cn=%d additional=%d", jp.resets(), cn.resets(), additional.resets())
	}
	if _, pending := state.settling[renderregion.JP]; !pending {
		t.Fatalf("fired timers stay recorded until the next change: %v", state.settling)
	}
}

func TestRegistryMasterdataRefreshDeduplicatesSettleResetsPerRegion(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1"}}
	state, resetters, _, _ := newRegistryTestState(t, registry, []time.Duration{40 * time.Millisecond, 80 * time.Millisecond}, renderregion.JP)
	jp := resetters[renderregion.JP]
	ctx := context.Background()

	state.poll(ctx)
	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	registry.set("jp", "sha256:jp-3")
	state.poll(ctx)
	if jp.resets() != 2 {
		t.Fatalf("two immediate resets expected, got %d", jp.resets())
	}
	waitForResets(t, jp, 4)
	time.Sleep(120 * time.Millisecond)
	if jp.resets() != 4 {
		t.Fatalf("a second change inside the settle window must replace the pending follow-ups (2 immediate + 2 settle), got %d", jp.resets())
	}

	registry.set("jp", "sha256:jp-4")
	state.poll(ctx)
	state.stop()
	time.Sleep(120 * time.Millisecond)
	if jp.resets() != 5 {
		t.Fatalf("stop must cancel pending settle resets, got %d", jp.resets())
	}
	if len(state.settling) != 0 {
		t.Fatalf("stop must drop pending timers: %v", state.settling)
	}
	state.scheduleSettleResets(renderregion.JP, "after-stop")
	if len(state.settling) != 0 {
		t.Fatalf("scheduling after stop must be a no-op: %v", state.settling)
	}
}

func TestRegistryMasterdataRefreshUsesETagWhenContentHashMissing(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "v1"}, etagOnly: true}
	state, resetters, _, _ := newRegistryTestState(t, registry, nil, renderregion.JP)
	jp := resetters[renderregion.JP]
	ctx := context.Background()

	state.poll(ctx)
	if state.signals[renderregion.JP] != `"v1"` {
		t.Fatalf("ETag must be the signal when contentHash is missing: %v", state.signals)
	}
	state.poll(ctx)
	registry.set("jp", "v2")
	state.poll(ctx)
	if jp.resets() != 1 {
		t.Fatalf("ETag change must reset, got %d", jp.resets())
	}
}

func TestRegistryMasterdataRefreshToleratesRegistryErrors(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1"}}
	state, resetters, _, server := newRegistryTestState(t, registry, nil, renderregion.JP, renderregion.KR)
	jp, kr := resetters[renderregion.JP], resetters[renderregion.KR]
	ctx := context.Background()

	state.poll(ctx)
	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	if jp.resets() != 1 || kr.resets() != 0 {
		t.Fatalf("missing region must not block other regions: jp=%d kr=%d", jp.resets(), kr.resets())
	}
	if _, ok := state.signals[renderregion.KR]; ok {
		t.Fatalf("404 region must not record a signal: %v", state.signals)
	}
	if state.failures[renderregion.KR] != 2 || state.failures[renderregion.JP] != 0 {
		t.Fatalf("consecutive failures = %v", state.failures)
	}

	registry.set("kr", "sha256:kr-1")
	state.poll(ctx)
	if state.failures[renderregion.KR] != 0 {
		t.Fatalf("success must clear the failure streak: %v", state.failures)
	}

	server.Close()
	registry.set("jp", "sha256:jp-3")
	for i := 0; i < 5; i++ {
		state.poll(ctx)
	}
	if jp.resets() != 1 {
		t.Fatalf("transport failure must not reset caches: jp=%d", jp.resets())
	}
	if state.failures[renderregion.JP] != 5 {
		t.Fatalf("failure streak = %v", state.failures)
	}
}

func TestResolveMasterdataRegistryURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "explicit", cfg: Config{MasterdataRegistry: MasterdataRegistryConfig{URL: " http://registry:9998/ "}, DeckRecommend: DeckRecommendConfig{RegistryURL: "http://deck"}}, want: "http://registry:9998/"},
		{name: "deck recommend", cfg: Config{DeckRecommend: DeckRecommendConfig{RegistryURL: "http://deck:9998"}, MusicMetaSource: "registry", MusicMetaBaseURL: "http://meta"}, want: "http://deck:9998"},
		{name: "music meta registry", cfg: Config{MusicMetaSource: "Registry", MusicMetaBaseURL: "http://meta:9998"}, want: "http://meta:9998"},
		{name: "music meta legacy", cfg: Config{MusicMetaSource: "legacy", MusicMetaBaseURL: "http://mirror"}, want: ""},
		{name: "none", cfg: Config{}, want: ""},
	}
	for _, tc := range cases {
		if got := resolveMasterdataRegistryURL(tc.cfg); got != tc.want {
			t.Errorf("%s: resolveMasterdataRegistryURL() = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := masterdataRegistryCurrentURL("http://registry:9998/", renderregion.TW); got != "http://registry:9998/v1/master/tw/current" {
		t.Fatalf("masterdataRegistryCurrentURL() = %q", got)
	}

	if got := resolveMasterdataRegistrySettleDelays(nil); len(got) != 2 || got[0] != 5*time.Minute || got[1] != 15*time.Minute {
		t.Fatalf("default settle delays = %v", got)
	}
	if got := resolveMasterdataRegistrySettleDelays([]time.Duration{}); len(got) != 0 {
		t.Fatalf("empty settle delays must disable follow-ups: %v", got)
	}
	if got := resolveMasterdataRegistrySettleDelays([]time.Duration{-time.Second, time.Minute, 0}); len(got) != 1 || got[0] != time.Minute {
		t.Fatalf("non-positive settle delays must be dropped: %v", got)
	}
}

func TestStartRegistryMasterdataRefreshRequiresURLAndProviders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app := &App{}
	app.startRegistryMasterdataRefresh(ctx, Config{MasterdataRegistry: MasterdataRegistryConfig{URL: "http://registry"}})
	(*App)(nil).startRegistryMasterdataRefresh(ctx, Config{})

	masterDir := filepath.Join(t.TempDir(), "haruki-sekai-master", "master")
	if err := os.MkdirAll(masterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(masterDir, "cards.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	app = &App{Providers: map[renderregion.Value]provider.MasterDataProvider{
		renderregion.JP: provider.NewLocalProvider(masterDir, renderregion.JP),
	}}
	app.startRegistryMasterdataRefresh(ctx, Config{})
	app.startRegistryMasterdataRefresh(ctx, Config{MasterdataRegistry: MasterdataRegistryConfig{URL: "http://127.0.0.1:1", PollInterval: -time.Second}})
}
