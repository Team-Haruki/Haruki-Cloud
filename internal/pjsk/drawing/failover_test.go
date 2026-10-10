package drawing

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
)

// refusedURL is a loopback address nothing listens on: dialing it is
// refused immediately.
func refusedURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return "http://" + addr
}

type countingServer struct {
	*httptest.Server
	hits atomic.Int64
}

func newCountingServer(t *testing.T, handler http.HandlerFunc) *countingServer {
	t.Helper()
	s := &countingServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func okServer(t *testing.T) *countingServer {
	return newCountingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	})
}

// failoverClient puts the targets in pool order; the first render of a fresh
// pool goes to the first target.
func failoverClient(t *testing.T, urls []string, options ...ClientOption) *HarukiDrawingClient {
	t.Helper()
	targets := make([]upstream.TargetConfig, 0, len(urls))
	for i, url := range urls {
		targets = append(targets, upstream.TargetConfig{Name: fmt.Sprintf("node-%d", i+1), BaseURL: url})
	}
	client := NewHarukiDrawingClientWithTargets("", targets, options...)
	if client == nil {
		t.Fatal("client not built")
	}
	return client
}

func TestRefusedNodeFailsOverToAnotherNode(t *testing.T) {
	healthy := okServer(t)
	client := failoverClient(t, []string{refusedURL(t), healthy.URL})

	ctx, trace := commandtrace.WithNewTrace(t.Context())
	data, err := client.WithContext(ctx).postPrepared("/api/pjsk/card/detail", map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if string(data) != "png-bytes" || healthy.hits.Load() != 1 {
		t.Fatalf("data = %q, healthy hits = %d", data, healthy.hits.Load())
	}
	if ops := traceOps(trace); ops["drawing.failover"] != 1 || ops["drawing.http"] != 2 {
		t.Fatalf("trace ops = %v", ops)
	}
	if client.health.healthy("node-1") {
		t.Fatal("refusing node was not marked unhealthy")
	}
	if !client.health.healthy("node-2") {
		t.Fatal("serving node was marked unhealthy")
	}
}

func TestUnhealthyNodeIsSkippedUntilTheCooldownEnds(t *testing.T) {
	healthy := okServer(t)
	client := failoverClient(t, []string{refusedURL(t), healthy.URL})
	now := time.Unix(1_700_000_000, 0)
	client.health.now = func() time.Time { return now }

	if _, err := client.WithContext(t.Context()).postPrepared("/render", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	// The pool rotates its starting node; while node-1 cools down every
	// render goes straight to node-2 without a failover.
	for i := range 4 {
		ctx, trace := commandtrace.WithNewTrace(t.Context())
		if _, err := client.WithContext(ctx).postPrepared("/render", map[string]any{}); err != nil {
			t.Fatal(err)
		}
		if ops := traceOps(trace); ops["drawing.failover"] != 0 || ops["drawing.http"] != 1 {
			t.Fatalf("render %d during the cooldown: trace ops = %v", i, ops)
		}
	}
	if healthy.hits.Load() != 5 {
		t.Fatalf("healthy hits = %d, want 5", healthy.hits.Load())
	}

	now = now.Add(drawingNodeCooldown)
	if !client.health.healthy("node-1") {
		t.Fatal("node-1 still skipped after the cooldown")
	}
}

func TestAllNodesCoolingDownStillTriesANode(t *testing.T) {
	healthy := okServer(t)
	client := failoverClient(t, []string{healthy.URL, refusedURL(t)})
	client.health.markDown("node-1")
	client.health.markDown("node-2")

	data, err := client.WithContext(t.Context()).postPrepared("/render", map[string]any{})
	if err != nil || string(data) != "png-bytes" {
		t.Fatalf("data = %q, err = %v", data, err)
	}
	if !client.health.healthy("node-1") {
		t.Fatal("a node that answered stayed unhealthy")
	}
}

func TestFailoverHappensOnlyOnce(t *testing.T) {
	client := failoverClient(t, []string{refusedURL(t), refusedURL(t), refusedURL(t)})

	ctx, trace := commandtrace.WithNewTrace(t.Context())
	_, err := client.WithContext(ctx).postPrepared("/render", map[string]any{})
	if err == nil {
		t.Fatal("expected an error with every node refusing")
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("error = %v, want the connection refusal", err)
	}
	if ops := traceOps(trace); ops["drawing.failover"] != 1 || ops["drawing.http"] != 2 {
		t.Fatalf("trace ops = %v", ops)
	}
	if client.health.healthy("node-1") || client.health.healthy("node-2") || !client.health.healthy("node-3") {
		t.Fatal("only the two refusing nodes that were tried should be unhealthy")
	}
}

func TestSingleNodeIsNotRetried(t *testing.T) {
	client := failoverClient(t, []string{refusedURL(t)})
	ctx, trace := commandtrace.WithNewTrace(t.Context())
	if _, err := client.WithContext(ctx).postPrepared("/render", map[string]any{}); err == nil {
		t.Fatal("expected an error")
	}
	if ops := traceOps(trace); ops["drawing.failover"] != 0 || ops["drawing.http"] != 1 {
		t.Fatalf("trace ops = %v", ops)
	}
}

// Failures after the node may have received the request are never retried.
func TestProcessedRequestsAreNotRetried(t *testing.T) {
	cases := map[string]struct {
		handler func(t *testing.T) http.HandlerFunc
		options []ClientOption
	}{
		"5xx with a body": {handler: func(*testing.T) http.HandlerFunc {
			return func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"detail":"boom"}`))
			}
		}},
		"503 from an overloaded node": {handler: func(*testing.T) http.HandlerFunc {
			return func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"detail":"busy"}`))
			}
		}},
		"timeout after sending": {
			handler: func(t *testing.T) http.HandlerFunc {
				return func(http.ResponseWriter, *http.Request) {
					time.Sleep(400 * time.Millisecond)
				}
			},
			options: []ClientOption{WithTimeout(100 * time.Millisecond)},
		},
		"connection closed after the request was read": {handler: func(t *testing.T) http.HandlerFunc {
			return func(w http.ResponseWriter, _ *http.Request) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
			}
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			failing := newCountingServer(t, tc.handler(t))
			other := okServer(t)
			client := failoverClient(t, []string{failing.URL, other.URL}, tc.options...)

			ctx, trace := commandtrace.WithNewTrace(t.Context())
			if _, err := client.WithContext(ctx).postPrepared("/render", map[string]any{}); err == nil {
				t.Fatal("expected an error")
			}
			if failing.hits.Load() != 1 || other.hits.Load() != 0 {
				t.Fatalf("failing hits = %d, other hits = %d", failing.hits.Load(), other.hits.Load())
			}
			if ops := traceOps(trace); ops["drawing.failover"] != 0 {
				t.Fatalf("trace ops = %v", ops)
			}
			if !client.health.healthy("node-1") {
				t.Fatal("a node that took the request was marked unhealthy")
			}
		})
	}
}

func TestCancelledRequestIsNotRetried(t *testing.T) {
	other := okServer(t)
	client := failoverClient(t, []string{refusedURL(t), other.URL})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.WithContext(ctx).postPrepared("/render", map[string]any{}); err == nil {
		t.Fatal("expected an error")
	}
	if other.hits.Load() != 0 {
		t.Fatalf("other hits = %d", other.hits.Load())
	}
}

func TestStoreRefFailoverAdoptsTheObjectOnce(t *testing.T) {
	server := newStoreRefDrawingServer(t, storeRefServe)
	indexer := &fakeStoreRefIndexer{}
	client := failoverClient(t, []string{refusedURL(t), server.URL}, WithArtifactConfig(storeRefConfig(indexer, "api/pjsk/sk")))
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}})
	client.SetRenderCache(cache)
	t.Cleanup(func() { _ = cache.Close() })

	ctx, trace := commandtrace.WithNewTrace(t.Context())
	image, err := client.WithContext(ctx).GenerateSKSpeedImage(&SpeedRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if ref := image.Ref(); ref == nil || ref.CDNPath != storeRefPath {
		t.Fatalf("ref = %+v", ref)
	}
	if _, hits := server.seen("/api/pjsk/sk/speed"); hits != 1 {
		t.Fatalf("drawing hits = %d", hits)
	}
	if adopted := indexer.adopted(); len(adopted) != 1 {
		t.Fatalf("adopted = %+v", adopted)
	}
	if ops := traceOps(trace); ops["drawing.failover"] != 1 || ops["drawing.store_ref"] != 1 {
		t.Fatalf("trace ops = %v", ops)
	}
}

func TestFailoverSafeClassification(t *testing.T) {
	trace := func(gotConn, writeFailed, gotBytes bool) *connTrace {
		tr := &connTrace{}
		tr.gotConn.Store(gotConn)
		tr.writeFailed.Store(writeFailed)
		tr.gotFirstBytes.Store(gotBytes)
		return tr
	}
	reset := &net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.ECONNRESET)}
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	timeout := &net.OpError{Op: "write", Net: "tcp", Err: os.ErrDeadlineExceeded}
	cases := []struct {
		name  string
		err   error
		trace *connTrace
		want  bool
	}{
		{"refused before a connection", refused, trace(false, false, false), true},
		{"TLS handshake failure", errors.New("tls: handshake failure"), trace(false, false, false), true},
		{"reset while writing", reset, trace(true, true, false), true},
		{"timeout while writing", timeout, trace(true, true, false), false},
		{"reset after the request was written", reset, trace(true, false, false), false},
		{"error after response bytes", reset, trace(true, false, true), false},
		{"request deadline", fmt.Errorf("post: %w", context.DeadlineExceeded), trace(false, false, false), false},
		{"request cancelled", fmt.Errorf("post: %w", context.Canceled), trace(false, false, false), false},
		{"no error", nil, trace(false, false, false), false},
		{"no trace", refused, nil, false},
	}
	for _, tc := range cases {
		if got := failoverSafe(tc.err, tc.trace); got != tc.want {
			t.Errorf("%s: failoverSafe = %v, want %v", tc.name, got, tc.want)
		}
	}
}
