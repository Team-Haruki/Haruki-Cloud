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
	fmt.Fprintf(w, `{"region":%q,"contentHash":%q,"createdAt":"2026-09-24T00:00:00Z"}`, region, hash)
}

func TestRegistryMasterdataRefreshResetsOnContentHashChange(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1", "cn": "sha256:cn-1"}}
	server := httptest.NewServer(registry)
	defer server.Close()

	jp := &countingMasterdataResetter{}
	cn := &countingMasterdataResetter{}
	additional := &countingMasterdataResetter{}
	state := newRegistryMasterdataRefreshState(server.URL+"/", server.Client(), map[renderregion.Value]masterdataCacheResetter{
		renderregion.JP: jp,
		renderregion.CN: cn,
	}, additional)
	ctx := context.Background()

	state.poll(ctx)
	if jp.count != 0 || cn.count != 0 || additional.count != 0 {
		t.Fatalf("first observation must only record: jp=%d cn=%d additional=%d", jp.count, cn.count, additional.count)
	}
	if state.hashes[renderregion.JP] != "sha256:jp-1" || state.etags[renderregion.JP] != `"sha256:jp-1"` {
		t.Fatalf("recorded state = hashes:%v etags:%v", state.hashes, state.etags)
	}

	state.poll(ctx)
	if jp.count != 0 || cn.count != 0 || additional.count != 0 {
		t.Fatalf("unchanged registry must not reset: jp=%d cn=%d additional=%d", jp.count, cn.count, additional.count)
	}
	if registry.served != 2 {
		t.Fatalf("unchanged poll should be answered with 304 via ETag, served=%d requests=%d", registry.served, registry.requests)
	}

	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	if jp.count != 1 || cn.count != 0 || additional.count != 1 {
		t.Fatalf("jp change must reset jp and shared caches only: jp=%d cn=%d additional=%d", jp.count, cn.count, additional.count)
	}

	state.poll(ctx)
	if jp.count != 1 || cn.count != 0 || additional.count != 1 {
		t.Fatalf("settled registry must not reset again: jp=%d cn=%d additional=%d", jp.count, cn.count, additional.count)
	}

	registry.set("jp", "sha256:jp-3")
	registry.set("cn", "sha256:cn-2")
	state.poll(ctx)
	if jp.count != 2 || cn.count != 1 || additional.count != 2 {
		t.Fatalf("both regions changed: jp=%d cn=%d additional=%d", jp.count, cn.count, additional.count)
	}
}

func TestRegistryMasterdataRefreshToleratesRegistryErrors(t *testing.T) {
	registry := &fakeMasterdataRegistry{hashes: map[string]string{"jp": "sha256:jp-1"}}
	server := httptest.NewServer(registry)
	defer server.Close()

	jp := &countingMasterdataResetter{}
	kr := &countingMasterdataResetter{}
	state := newRegistryMasterdataRefreshState(server.URL, server.Client(), map[renderregion.Value]masterdataCacheResetter{
		renderregion.JP: jp,
		renderregion.KR: kr,
	})
	ctx := context.Background()

	state.poll(ctx)
	registry.set("jp", "sha256:jp-2")
	state.poll(ctx)
	if jp.count != 1 || kr.count != 0 {
		t.Fatalf("missing region must not block other regions: jp=%d kr=%d", jp.count, kr.count)
	}
	if _, ok := state.hashes[renderregion.KR]; ok {
		t.Fatalf("404 region must not record a hash: %v", state.hashes)
	}

	server.Close()
	registry.set("jp", "sha256:jp-3")
	state.poll(ctx)
	if jp.count != 1 {
		t.Fatalf("transport failure must not reset caches: jp=%d", jp.count)
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
