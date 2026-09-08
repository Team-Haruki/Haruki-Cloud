package drawing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPendingRenderReusedUntilStoreCompletes(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(404)
			return
		}
		close(entered)
		<-release
		w.WriteHeader(200)
	}))
	defer server.Close()
	client := NewRenderCacheClient(RenderCacheConfig{BaseURL: server.URL, StorageDir: t.TempDir(), TTL: time.Hour})
	defer client.waitForPendingStores()
	defer unblock.Do(func() { close(release) })
	var renders atomic.Int32
	render := func(context.Context) ([]byte, error) { renders.Add(1); return []byte("original image"), nil }
	policy := renderCachePolicy{APIPath: "api/pjsk/profile", UserID: "public", TTL: time.Hour}
	key := strings.Repeat("a", 64)
	first, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/profile", key, policy, render)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("store not started")
	}
	first[0] = 'X'
	for range 10 {
		got, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/profile", key, policy, render)
		if err != nil || string(got) != "original image" {
			t.Fatalf("result=%q err=%v", got, err)
		}
	}
	if renders.Load() != 1 {
		t.Fatalf("renders=%d want 1", renders.Load())
	}
	unblock.Do(func() { close(release) })
	client.waitForPendingStores()
	if _, hit := client.pending.get(key); hit {
		t.Fatal("successful store must retire pending result")
	}
}

func TestPendingRenderLimitsExpiryAndGeneration(t *testing.T) {
	cache := newLocalRenderCacheWithLimits(time.Second, 2, 12)
	cache.set("a", []byte("aaaaaa"), time.Second, false)
	old := cache.peekGeneration("a")
	cache.set("a", []byte("newaaa"), time.Second, false)
	cache.deleteGeneration("a", old)
	if got, _ := cache.get("a"); string(got) != "newaaa" {
		t.Fatalf("old store removed new generation: %q", got)
	}
	cache.set("b", []byte("bbbbbb"), time.Second, false)
	cache.set("c", []byte("cccccc"), time.Second, false)
	if cache.totalBytes > 12 || len(cache.entries) > 2 {
		t.Fatal("pending memory limit exceeded")
	}
	cache.mu.Lock()
	cache.entries["c"].expiresAt = time.Now().Add(-time.Second)
	cache.mu.Unlock()
	if _, hit := cache.get("c"); hit {
		t.Fatal("expired pending result returned")
	}
	cache.set("oversize", make([]byte, 13), time.Second, false)
	if _, hit := cache.get("oversize"); hit {
		t.Fatal("oversized result retained")
	}
}

func TestPendingRenderSurvivesStoreFailureUntilExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(404)
		} else {
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	client := NewRenderCacheClient(RenderCacheConfig{BaseURL: server.URL, StorageDir: t.TempDir(), TTL: time.Hour})
	defer client.waitForPendingStores()
	var renders atomic.Int32
	render := func(context.Context) ([]byte, error) { renders.Add(1); return []byte("image"), nil }
	key := strings.Repeat("b", 64)
	policy := renderCachePolicy{APIPath: "api/pjsk/card/list", UserID: "public", TTL: time.Hour}
	for range 2 {
		if _, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/card/list", key, policy, render); err != nil {
			t.Fatal(err)
		}
		client.waitForPendingStores()
	}
	if renders.Load() != 1 {
		t.Fatalf("failed store caused immediate rerender: %d", renders.Load())
	}
	client.pending.mu.Lock()
	client.pending.entries[key].expiresAt = time.Now().Add(-time.Second)
	client.pending.mu.Unlock()
	if _, err := client.renderRemoteFlight(t.Context(), "/api/pjsk/card/list", key, policy, render); err != nil {
		t.Fatal(err)
	}
	if renders.Load() != 2 {
		t.Fatalf("expired pending image prevented retry: %d", renders.Load())
	}
}
