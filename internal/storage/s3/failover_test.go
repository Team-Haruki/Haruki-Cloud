package s3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
)

type countingServer struct {
	*httptest.Server
	calls  atomic.Int32
	status atomic.Int32
	delay  atomic.Int64
}

func newCountingServer(t *testing.T) *countingServer {
	t.Helper()
	s := &countingServer{}
	s.status.Store(http.StatusOK)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		if d := time.Duration(s.delay.Load()); d > 0 {
			select {
			case <-time.After(d):
			case <-r.Context().Done():
				return
			}
		}
		status := int(s.status.Load())
		if status != http.StatusOK {
			xmlError(w, status, "Injected")
			return
		}
		_, _ = w.Write([]byte(s.URL))
	}))
	t.Cleanup(s.Close)
	return s
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func closedURL(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	return url
}

func getFrom(t *testing.T, c *client) string {
	t.Helper()
	data, err := c.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	return string(data)
}

func TestFailoverClosedEndpoint(t *testing.T) {
	second := newCountingServer(t)
	third := newCountingServer(t)
	c := mustClient(t, testConfig(closedURL(t), second.URL, third.URL))
	if got := getFrom(t, c); got != second.URL {
		t.Fatalf("served by %s, want second", got)
	}
	if c.endpoints[0].consecutiveFail.Load() != 1 || third.calls.Load() != 0 {
		t.Fatal("closed endpoint must be marked failed and third untouched")
	}
}

func TestFailoverOn5xx(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable} {
		first, second := newCountingServer(t), newCountingServer(t)
		first.status.Store(int32(status))
		c := mustClient(t, testConfig(first.URL, second.URL))
		if got := getFrom(t, c); got != second.URL {
			t.Fatalf("%d: served by %s", status, got)
		}
		if first.calls.Load() != 1 || second.calls.Load() != 1 {
			t.Fatalf("%d: calls first=%d second=%d", status, first.calls.Load(), second.calls.Load())
		}
	}
}

func TestNoFailoverOn4xx(t *testing.T) {
	first, second := newCountingServer(t), newCountingServer(t)
	first.status.Store(http.StatusForbidden)
	c := mustClient(t, testConfig(first.URL, second.URL))
	var respErr *ResponseError
	if _, err := c.Get(context.Background(), "k"); !errors.As(err, &respErr) || respErr.StatusCode != http.StatusForbidden {
		t.Fatalf("Get error = %v", err)
	}
	if second.calls.Load() != 0 {
		t.Fatalf("second endpoint received %d requests after a 403", second.calls.Load())
	}
	if c.endpoints[0].consecutiveFail.Load() != 0 {
		t.Fatal("a 403 must not mark the endpoint unhealthy")
	}
	first.status.Store(http.StatusNotFound)
	c = mustClient(t, testConfig(first.URL, second.URL))
	if _, err := c.Get(context.Background(), "k"); !errors.Is(err, storage.ErrNotExist) || second.calls.Load() != 0 {
		t.Fatalf("404 error = %v, second calls = %d", err, second.calls.Load())
	}
}

func TestFailoverPerAttemptTimeout(t *testing.T) {
	first, second := newCountingServer(t), newCountingServer(t)
	first.delay.Store(int64(2 * time.Second))
	cfg := testConfig(first.URL, second.URL)
	cfg.RequestTimeout = 100 * time.Millisecond
	c := mustClient(t, cfg)
	start := time.Now()
	if got := getFrom(t, c); got != second.URL {
		t.Fatalf("served by %s", got)
	}
	if time.Since(start) > time.Second {
		t.Fatal("per-attempt timeout did not bound the slow endpoint")
	}
}

func TestFailoverCooldownSkipAndReadmission(t *testing.T) {
	first, second, third := newCountingServer(t), newCountingServer(t), newCountingServer(t)
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	cfg := testConfig(first.URL, second.URL, third.URL)
	cfg.FailoverCooldown = time.Minute
	c := mustClient(t, cfg)
	c.now = clock.Now

	first.status.Store(http.StatusInternalServerError)
	if got := getFrom(t, c); got != second.URL { // rotation slot 0: first fails, second serves
		t.Fatalf("op1 served by %s", got)
	}
	first.status.Store(http.StatusOK)
	for range 6 { // slots 1..6: first is cooling down and must be skipped
		getFrom(t, c)
	}
	if first.calls.Load() != 1 {
		t.Fatalf("cooling endpoint received %d requests, want 1", first.calls.Load())
	}
	clock.Advance(2 * time.Minute)
	for range 3 { // slots 7, 8, 9: the rotation reaches first again
		getFrom(t, c)
	}
	if first.calls.Load() != 2 || c.endpoints[0].consecutiveFail.Load() != 0 {
		t.Fatalf("first calls=%d fail=%d after cooldown", first.calls.Load(), c.endpoints[0].consecutiveFail.Load())
	}
}

func TestFailoverCoolingEndpointsAreLastResort(t *testing.T) {
	first, second := newCountingServer(t), newCountingServer(t)
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	c := mustClient(t, testConfig(first.URL, second.URL))
	c.now = clock.Now
	c.markFailure(c.endpoints[0])
	c.markFailure(c.endpoints[1])
	order := c.order()
	if len(order) != 2 {
		t.Fatalf("order length = %d", len(order))
	}
	c.endpoints[1].consecutiveFail.Store(0)
	order = c.order()
	if order[0] != c.endpoints[1] || order[1] != c.endpoints[0] {
		t.Fatal("healthy endpoint must precede the cooling one")
	}
}

func TestFailoverAllDown(t *testing.T) {
	first, second, third := newCountingServer(t), newCountingServer(t), newCountingServer(t)
	for _, s := range []*countingServer{first, second, third} {
		s.status.Store(http.StatusBadGateway)
	}
	c := mustClient(t, testConfig(first.URL, second.URL, third.URL))
	_, err := c.Get(context.Background(), "k")
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") || !strings.Contains(err.Error(), "last endpoint "+third.URL) {
		t.Fatalf("all-down error = %v", err)
	}
	var respErr *ResponseError
	if !errors.As(err, &respErr) || respErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("all-down error must wrap the last response: %v", err)
	}
	// Every endpoint is now cooling down; the next call still tries all of them.
	if _, err := c.Get(context.Background(), "k"); err == nil {
		t.Fatal("expected failure")
	}
	if first.calls.Load() != 2 || second.calls.Load() != 2 || third.calls.Load() != 2 {
		t.Fatalf("calls = %d %d %d", first.calls.Load(), second.calls.Load(), third.calls.Load())
	}
}

func TestMaxAttemptsCyclesEndpoints(t *testing.T) {
	only := newCountingServer(t)
	only.status.Store(http.StatusInternalServerError)
	cfg := testConfig(only.URL)
	cfg.MaxAttempts = 3
	c := mustClient(t, cfg)
	if err := c.Delete(context.Background(), "k"); err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("Delete error = %v", err)
	}
	if only.calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", only.calls.Load())
	}
}
