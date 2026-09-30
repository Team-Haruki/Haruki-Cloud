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

	"haruki-cloud/internal/core/upstream"
)

func TestIdentityRefreshRetainsTransientFailureThenExpiresAndRecovers(t *testing.T) {
	var unhealthy atomic.Bool
	var epoch atomic.Value
	epoch.Store(strings.Repeat("a", 64))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cache/identity" {
			t.Error(r.URL.Path)
		}
		if unhealthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprintf(w, `{"version":1,"renderer_epoch":%q}`, epoch.Load())
	}))
	defer server.Close()
	v := &cacheVersions{cfg: CacheVersionConfig{MaxStale: time.Minute}, http: server.Client(), targets: []upstream.TargetConfig{{BaseURL: server.URL}}}
	v.refresh(t.Context())
	initial := v.snapshot(nil)
	if !initial.ready {
		t.Fatal("healthy identity unavailable")
	}
	unhealthy.Store(true)
	v.refresh(t.Context())
	if got := v.snapshot(nil); !got.ready || got.renderer != initial.renderer {
		t.Fatal("transient failure discarded fresh identity")
	}
	state := v.current.Load()
	v.current.Store(&rendererIdentities{epochs: state.epochs, checked: map[string]time.Time{server.URL: time.Now().Add(-2 * time.Minute)}})
	if v.snapshot(nil).ready {
		t.Fatal("stale identity remained cacheable")
	}
	unhealthy.Store(false)
	epoch.Store(strings.Repeat("b", 64))
	v.refresh(t.Context())
	if got := v.snapshot(nil); !got.ready || got.renderer == initial.renderer {
		t.Fatal("recovery failed to invalidate old renderer")
	}
}

func TestIdentityRejectsMalformedAndOversizedResponses(t *testing.T) {
	for _, body := range []string{"{", strings.Repeat("x", 4097), `{"version":2,"renderer_epoch":"` + strings.Repeat("a", 64) + `"}`, `{"version":1,"renderer_epoch":"` + strings.Repeat("A", 64) + `"}`, `{"version":1,"renderer_epoch":"short"}`} {
		t.Run(fmt.Sprint(len(body), body[:1]), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			v := &cacheVersions{http: server.Client()}
			if _, err := v.identity(t.Context(), server.URL); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	v := &cacheVersions{http: &http.Client{}}
	if _, err := v.identity(t.Context(), "://invalid"); err == nil {
		t.Fatal("invalid URL accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := v.identity(ctx, "http://127.0.0.1:1"); err == nil {
		t.Fatal("cancelled identity accepted")
	}
}

func TestIdentityWatcherPollsAndCloseCancelsInflightHTTP(t *testing.T) {
	entered := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	defer server.Close()
	client := NewHarukiDrawingClient(server.URL, WithCacheVersions(t.Context(), CacheVersionConfig{Enabled: true}, nil, nil))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("watcher never polled")
	}
	client.versions.Close()
	client.versions.Close()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("close did not cancel HTTP")
	}
	disabled := NewHarukiDrawingClient(server.URL, WithCacheVersions(t.Context(), CacheVersionConfig{}, nil, nil))
	if disabled.versions != nil {
		t.Fatal("disabled watcher started")
	}
}

func TestConfiguredRendererEpochSkipsHTTPAndPollsUntilCancelled(t *testing.T) {
	client := NewHarukiDrawingClient("http://127.0.0.1:1", WithCacheVersions(t.Context(), CacheVersionConfig{Enabled: true, RendererEpoch: strings.Repeat("a", 64), PollInterval: time.Millisecond}, nil, nil))
	defer client.versions.Close()
	deadline := time.Now().Add(time.Second)
	for !client.versions.snapshot(nil).ready && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !client.versions.snapshot(nil).ready {
		t.Fatal("configured identity not published")
	}
	first := client.versions.current.Load()
	for client.versions.current.Load() == first && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if client.versions.current.Load() == first {
		t.Fatal("poll ticker did not refresh")
	}
}
