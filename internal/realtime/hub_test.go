package realtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
)

func TestHubEvictsOldestAndCapsGlobally(t *testing.T) {
	hub := NewHub(3, 2)
	key := Key{SubscriptionID: 1, Version: "v1"}
	first, _ := hub.Register(key)
	second, _ := hub.Register(key)
	third, err := hub.Register(key)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	select {
	case <-first.closed:
	default:
		t.Fatal("oldest stream not evicted")
	}
	if first.closeReason() != CloseReasonEvicted || hub.Count() != 2 {
		t.Fatalf("reason %q, count %d", first.closeReason(), hub.Count())
	}
	hub.Unregister(first) // already gone: no double count
	if hub.Count() != 2 {
		t.Fatalf("count after stale unregister = %d", hub.Count())
	}
	other, err := hub.Register(Key{SubscriptionID: 2, Version: "v1"})
	if err != nil {
		t.Fatalf("register other: %v", err)
	}
	if _, err := hub.Register(Key{SubscriptionID: 3, Version: "v1"}); !errors.Is(err, ErrTooManyConnections) {
		t.Fatalf("over the global cap = %v", err)
	}
	// At the global cap a new stream for a full key still fits by eviction.
	if _, err := hub.Register(key); err != nil {
		t.Fatalf("evicting register at cap: %v", err)
	}
	if !hub.Online(key) || hub.Online(Key{SubscriptionID: 9}) {
		t.Fatal("online")
	}
	if got := len(hub.Keys()); got != 2 {
		t.Fatalf("keys = %d", got)
	}
	if closed := hub.CloseAll(CloseReasonShutdown); closed != 3 || hub.Count() != 0 {
		t.Fatalf("close all = %d, count %d", closed, hub.Count())
	}
	if second.closeReason() != CloseReasonEvicted || third.closeReason() != CloseReasonShutdown || other.closeReason() != CloseReasonShutdown {
		t.Fatalf("reasons %q %q %q", second.closeReason(), third.closeReason(), other.closeReason())
	}
	hub.Unregister(nil)
}

func TestHubDeliverDropsWhenQueueIsFull(t *testing.T) {
	hub := NewHub(0, 0)
	key := Key{SubscriptionID: 1, Version: "v1"}
	if _, err := hub.Register(key); err != nil {
		t.Fatal(err)
	}
	event := Event{ID: 1, SubscriptionID: 1, SubscriptionVersion: "v1"}
	for range connQueueSize {
		if hub.Deliver(event, false) != 1 {
			t.Fatal("delivery refused before the queue filled")
		}
	}
	if hub.Deliver(event, false) != 0 {
		t.Fatal("full queue accepted a delivery")
	}
	if hub.Close(key, CloseReasonRequested) != 1 || hub.Close(key, CloseReasonRequested) != 0 {
		t.Fatal("close counts")
	}
}

func TestHTTPCloser(t *testing.T) {
	var gotPath, gotQuery, gotAuth, gotAgent string
	var status atomic.Int32
	status.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		gotAuth, gotAgent = r.Header.Get("Authorization"), r.Header.Get("User-Agent")
		w.WriteHeader(int(status.Load()))
	}))
	defer server.Close()

	if NewHTTPCloser(" ", "a", "b") != nil {
		t.Fatal("empty base URL should give a nil closer")
	}
	var nilCloser *HTTPCloser
	if err := nilCloser.CloseStreams(context.Background(), 1, "v1"); err != nil {
		t.Fatalf("nil closer: %v", err)
	}
	closer := NewHTTPCloser(server.URL+"/", "Bearer shared", "")
	if err := closer.CloseStreams(context.Background(), 0, "v1"); err != nil || gotPath != "" {
		t.Fatalf("invalid id should be a no-op: %v", err)
	}
	if err := closer.CloseStreams(context.Background(), 7, " v 1 "); err != nil {
		t.Fatalf("close: %v", err)
	}
	if gotPath != "/internal/subscriptions/7/close" || gotQuery != "subscription_version=v+1" || gotAuth != "Bearer shared" || gotAgent != "Haruki-Cloud" {
		t.Fatalf("request = %s ? %s auth=%q ua=%q", gotPath, gotQuery, gotAuth, gotAgent)
	}
	status.Store(http.StatusUnauthorized)
	closer.UserAgent = "Custom"
	closer.Client = nil
	if err := closer.CloseStreams(context.Background(), 7, "v1"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("status error = %v", err)
	}
	if gotAgent != "Custom" {
		t.Fatalf("user agent = %q", gotAgent)
	}
	broken := &HTTPCloser{BaseURL: "http://[::1", Client: http.DefaultClient}
	if err := broken.CloseStreams(context.Background(), 1, "v1"); err == nil {
		t.Fatal("bad URL should fail")
	}
	server.Close()
	if err := closer.CloseStreams(context.Background(), 1, "v1"); err == nil {
		t.Fatal("closed server should fail")
	}
}

func TestServiceCloseStreamsAndAccessors(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 1 })
	var closer Closer = env.svc
	if err := closer.CloseStreams(context.Background(), sub.ID, " v1 "); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !stream.waitClosed(3 * time.Second) {
		t.Fatal("stream not closed in process")
	}
	if cfg := env.svc.Config(); cfg.ReplayPolicy != config.EventsReplayLatest || cfg.MaxConnsPerSubscription != config.DefaultEventsMaxConnsPerSubscription {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestNewServiceDefaults(t *testing.T) {
	svc := NewService(nil, config.EventsConfig{IngestTokenSHA256: "zz"}, Options{})
	if svc.readOnly() || svc.now().IsZero() || svc.log == nil || len(svc.ingestHash) != 0 {
		t.Fatal("defaults not applied")
	}
	if tokenVersion("no-dot") != "" || tokenVersion(" v1.secret ") != "v1" {
		t.Fatal("tokenVersion")
	}
	if _, err := svc.Authenticate(context.Background(), "1", "v1", "v1.x"); errors.Is(err, ErrInvalidSubscription) || err == nil {
		t.Fatalf("store failure must be transient, got %v", err)
	}
}

func TestDeliverLiveLogsClaimFailure(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	env.connect(streamPath(sub, "v1"), nil)
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 1 })
	_ = env.db.Close()
	if n := env.svc.deliverLive(context.Background(), Event{ID: 1, SubscriptionID: sub.ID, SubscriptionVersion: "v1"}, false); n != 0 {
		t.Fatalf("delivered = %d", n)
	}
	env.svc.Sweep(context.Background())
	env.svc.collectGarbage(context.Background())
	logs := env.logs.String()
	for _, want := range []string{"realtime_claim_failed", "realtime_sweep_failed", "realtime_gc_failed"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("missing %s in logs", want)
		}
	}
}

func TestForwarderDropsWhenSaturated(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-release
	}))
	defer server.Close()
	logs := &logBuffer{}
	f := newForwarder(server.URL, "", harukiLogger.NewLogger("Events", "DEBUG", logs))
	for range forwardConcurrency + 1 {
		f.forward([]byte(`{}`))
	}
	if !strings.Contains(logs.String(), "realtime_forward_dropped") {
		t.Fatal("saturated forward was not dropped")
	}
	close(release)
	f.wait(3 * time.Second)
	f.wait(time.Millisecond)

	stuck := newForwarder(server.URL, "", harukiLogger.NewLogger("Events", "DEBUG", logs))
	stuck.inflight.Add(1)
	start := time.Now()
	stuck.wait(20 * time.Millisecond)
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("wait returned before the timeout")
	}
	stuck.inflight.Done()
}

func TestRegisterWithoutCloseAuth(t *testing.T) {
	cfg := testConfig()
	svc := NewService(openTestDB(t), cfg, Options{Logger: harukiLogger.NewLogger("Events", "DEBUG", &logBuffer{})})
	app := fiber.New()
	svc.Register(app, nil)
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/internal/subscriptions/1/close?subscription_version=v1", nil))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("unguarded close = %v, %v", resp, err)
	}
}
