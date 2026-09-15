package deck

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"haruki-cloud/utils/logger"
)

func TestRegistryCurrentURLNormalizesBaseAndRegion(t *testing.T) {
	if got := registryCurrentURL("http://registry:9998/", " JP "); got != "http://registry:9998/v1/master/jp/current" {
		t.Fatalf("unexpected registry url: %s", got)
	}
}

func TestRemoteRegistryPollRecordsThenInvalidatesOnHashChange(t *testing.T) {
	var hash atomic.Value
	hash.Store("aaa")
	var conditional atomic.Int64
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/master/jp/current" {
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		current := hash.Load().(string)
		etag := `"` + current + `"`
		if r.Header.Get("If-None-Match") == etag {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"server":"jp","dataVersion":"6.0.0.1","contentHash":"` + current + `","gitCommit":"abc","files":[]}`))
	}))
	defer registry.Close()

	recommender := newStandaloneTestRemoteDeckRecommender("http://127.0.0.1:1", http.DefaultClient)
	recommender.registryURL = registry.URL
	recommender.region = "jp"
	recommender.logger = logger.NewLogger("DeckRemoteTest", "ERROR", nil)
	state := testRemoteTargetState(t, recommender)
	state.masterdataReady = true

	// Initial capture only records the hash.
	recommender.captureMasterdataSignature()
	if got := recommender.currentRegistryContentHash(); got != "aaa" {
		t.Fatalf("expected initial hash aaa, got %q", got)
	}
	if !targetMasterdataReady(state) {
		t.Fatalf("initial capture must not invalidate")
	}

	// Unchanged manifest answers 304 and changes nothing.
	recommender.refreshMasterdataSignature()
	if conditional.Load() != 1 {
		t.Fatalf("expected one conditional 304, got %d", conditional.Load())
	}
	if !targetMasterdataReady(state) {
		t.Fatalf("304 must not invalidate")
	}

	// A new publish invalidates every target.
	hash.Store("bbb")
	recommender.refreshMasterdataSignature()
	if got := recommender.currentRegistryContentHash(); got != "bbb" {
		t.Fatalf("expected hash bbb after publish, got %q", got)
	}
	if targetMasterdataReady(state) {
		t.Fatalf("expected target masterdata to be invalidated after hash change")
	}
}

func TestRemoteRegistryPollToleratesFailures(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer registry.Close()

	recommender := newStandaloneTestRemoteDeckRecommender("http://127.0.0.1:1", http.DefaultClient)
	recommender.registryURL = registry.URL
	recommender.region = "jp"
	state := testRemoteTargetState(t, recommender)
	state.masterdataReady = true
	recommender.captureMasterdataSignature()
	recommender.refreshMasterdataSignature()
	if got := recommender.currentRegistryContentHash(); got != "" {
		t.Fatalf("expected no hash after failed polls, got %q", got)
	}
	if !targetMasterdataReady(state) {
		t.Fatalf("failed polls must not invalidate")
	}
	recommender.adoptRegistryContentHash(" ccc ")
	if got := recommender.currentRegistryContentHash(); got != "ccc" {
		t.Fatalf("expected adopted hash ccc, got %q", got)
	}
	recommender.adoptRegistryContentHash("ddd")
	if got := recommender.currentRegistryContentHash(); got != "ccc" {
		t.Fatalf("adopt must not overwrite a known hash, got %q", got)
	}
}

func TestUpdateRemoteMasterdataPostsRegistryRequest(t *testing.T) {
	var bodies []string
	deckServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/update/masterdata/registry" {
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","region":"jp","contentHash":"served","gitCommit":"abc","dataVersion":"6.0.0.1","reloaded":true}`))
	}))
	defer deckServer.Close()

	recommender := newStandaloneTestRemoteDeckRecommender(deckServer.URL, deckServer.Client())
	recommender.registryURL = "http://registry.invalid"
	recommender.region = "jp"
	recommender.logger = logger.NewLogger("DeckRemoteTest", "ERROR", nil)
	exec := testRemoteExecution(t, recommender)
	defer exec.Release()

	// No hash known yet: the request carries only the region and Cloud
	// adopts the hash deck-service reports.
	if err := recommender.updateRemoteMasterdata(context.Background(), exec, "jp"); err != nil {
		t.Fatalf("updateRemoteMasterdata: %v", err)
	}
	if got := recommender.currentRegistryContentHash(); got != "served" {
		t.Fatalf("expected adopted hash from response, got %q", got)
	}

	recommender.masterdataMu.Lock()
	recommender.registryContentHash = "abc123"
	recommender.masterdataMu.Unlock()
	if err := recommender.updateRemoteMasterdata(context.Background(), exec, "jp"); err != nil {
		t.Fatalf("updateRemoteMasterdata with hash: %v", err)
	}

	if len(bodies) != 2 {
		t.Fatalf("expected two requests, got %d", len(bodies))
	}
	if bodies[0] != `{"region":"jp"}` {
		t.Fatalf("unexpected first body: %s", bodies[0])
	}
	if bodies[1] != `{"content_hash":"abc123","region":"jp"}` {
		t.Fatalf("unexpected second body: %s", bodies[1])
	}
}

func TestUpdateRemoteMasterdataKeepsDirectoryRequestWithoutRegistry(t *testing.T) {
	var body string
	deckServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/update/masterdata" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer deckServer.Close()

	recommender := newStandaloneTestRemoteDeckRecommender(deckServer.URL, deckServer.Client())
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	exec := testRemoteExecution(t, recommender)
	defer exec.Release()
	if err := recommender.updateRemoteMasterdata(context.Background(), exec, "jp"); err != nil {
		t.Fatalf("updateRemoteMasterdata: %v", err)
	}
	if body != `{"base_dir":"/masterdata","region":"jp"}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestRemoteEngineProviderAcceptsRegistryWithoutMasterdataDir(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		RegistryURL:    "http://registry.invalid/",
	})
	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}
	remote, ok := recommender.(*RemoteDeckRecommender)
	if !ok {
		t.Fatalf("unexpected recommender type: %T", recommender)
	}
	if remote.registryURL != "http://registry.invalid" {
		t.Fatalf("expected trimmed registry url, got %q", remote.registryURL)
	}
	if !strings.HasPrefix(registryCurrentURL(remote.registryURL, remote.region), "http://registry.invalid/v1/master/jp/") {
		t.Fatalf("unexpected registry url for region")
	}

	_, err = newRemoteEngineProvider(RecommendConfig{ServiceBaseURL: "http://example.com"}).Get("jp")
	if err == nil {
		t.Fatalf("expected an error without masterdata_dir and registry_url")
	}
}

func targetMasterdataReady(state *remoteTargetState) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.masterdataReady
}
