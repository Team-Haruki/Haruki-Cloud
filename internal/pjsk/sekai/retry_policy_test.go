package sekai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"haruki-cloud/config"

	"github.com/go-resty/resty/v2"
)

func retryServer(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit := int(hits.Add(1))
		status := http.StatusOK
		if hit <= len(statuses) {
			status = statuses[hit-1]
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

var fastRetry = config.UpstreamRetryConfig{Wait: time.Millisecond, MaxWait: time.Millisecond}

func TestRetryPolicyRetriesTransientGETOnce(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		statuses []int
		wantHits int32
		wantCode int
	}{
		{"503 then ok", http.MethodGet, []int{503}, 2, 200},
		{"502 then ok", http.MethodGet, []int{502}, 2, 200},
		{"one retry only", http.MethodGet, []int{503, 503, 503}, 2, 503},
		{"500 is not retried", http.MethodGet, []int{500}, 1, 500},
		{"504 is not retried", http.MethodGet, []int{504}, 1, 504},
		{"POST is not retried", http.MethodPost, []int{503}, 1, 503},
		{"PUT is not retried", http.MethodPut, []int{503}, 1, 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, hits := retryServer(t, tc.statuses...)
			resp, err := newRestyClient(fastRetry).R().Execute(tc.method, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if hits.Load() != tc.wantHits || resp.StatusCode() != tc.wantCode {
				t.Fatalf("hits=%d status=%d, want %d/%d", hits.Load(), resp.StatusCode(), tc.wantHits, tc.wantCode)
			}
		})
	}
}

func TestRetryPolicyDoesNotRetryClientTimeout(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-time.After(200 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	client := newRestyClient(fastRetry).SetTimeout(20 * time.Millisecond)
	if _, err := client.R().Get(server.URL); err == nil {
		t.Fatal("expected a client timeout")
	}
	if hits.Load() != 1 {
		t.Fatalf("a client timeout was retried: %d attempts", hits.Load())
	}
}

func TestRetryPolicyStopsAfterBudget(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(60 * time.Millisecond)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	cfg := fastRetry
	cfg.MaxRetries = 3
	cfg.Budget = 30 * time.Millisecond
	if _, err := newRestyClient(cfg).R().Get(server.URL); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("a slow failure past the budget was retried: %d attempts", hits.Load())
	}

	hits.Store(0)
	cfg.Budget = -1
	cfg.MaxRetries = 2
	if _, err := newRestyClient(cfg).R().Get(server.URL); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 3 {
		t.Fatalf("without a budget max_retries=2 gives 3 attempts, got %d", hits.Load())
	}
}

func TestRetryPolicyDisabled(t *testing.T) {
	server, hits := retryServer(t, 503)
	if _, err := newRestyClient(config.UpstreamRetryConfig{MaxRetries: -1}).R().Get(server.URL); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("max_retries < 0 must disable retries, got %d attempts", hits.Load())
	}
}

func TestRetryPolicyRetriesRefusedConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	var attempts atomic.Int32
	client := newRestyClient(fastRetry).OnBeforeRequest(func(*resty.Client, *resty.Request) error {
		attempts.Add(1)
		return nil
	})
	if _, err := client.R().Get("http://" + addr); err == nil {
		t.Fatal("expected a refused connection")
	}
	if attempts.Load() != 2 {
		t.Fatalf("a refused connection should be retried once, attempts = %d", attempts.Load())
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestIsRetryableTransportError(t *testing.T) {
	cases := map[error]bool{
		nil:                               false,
		context.Canceled:                  false,
		context.DeadlineExceeded:          false,
		timeoutError{}:                    false,
		syscall.ECONNREFUSED:              true,
		syscall.ECONNRESET:                true,
		io.ErrUnexpectedEOF:               true,
		fmt.Errorf("wrapped: %w", io.EOF): true,
		errors.New("read: connection reset by peer"): true,
		errors.New("tls: bad certificate"):           false,
	}
	for err, want := range cases {
		if got := isRetryableTransportError(err); got != want {
			t.Fatalf("isRetryableTransportError(%v) = %v, want %v", err, got, want)
		}
	}
	if (retryPolicy{}).shouldRetry(nil, nil) {
		t.Fatal("a missing response is not retried")
	}
}

func TestUpstreamRetryConfigWiring(t *testing.T) {
	cfg := config.UpstreamRetryConfig{}.WithDefaults()
	if cfg.MaxRetries != 1 || cfg.Budget != config.DefaultUpstreamRetryBudget || cfg.Wait != config.DefaultUpstreamRetryWait {
		t.Fatalf("defaults = %+v", cfg)
	}
	if got := (config.UpstreamRetryConfig{Wait: 2 * time.Second}).WithDefaults(); got.MaxWait != 2*time.Second {
		t.Fatalf("max_wait must not be below wait: %+v", got)
	}
	tracker := NewTrackerClient(&config.TrackerConfig{Retry: config.UpstreamRetryConfig{MaxRetries: 2}})
	if tracker.http.RetryCount != 2 {
		t.Fatalf("tracker retry count = %d", tracker.http.RetryCount)
	}
	if NewToolboxClient(&config.ToolboxConfig{}).http.RetryCount != 1 || NewSekaiAPIClient(nil).http.RetryCount != 1 {
		t.Fatal("clients default to one retry")
	}
	if NewSekaiAPIClient(&config.SekaiAPIConfig{Retry: config.UpstreamRetryConfig{MaxRetries: -1}}).http.RetryCount != 0 {
		t.Fatal("retries can be disabled per upstream")
	}
	if retryConfig(nil) != (config.UpstreamRetryConfig{}) || toolboxRetryConfig(nil) != (config.UpstreamRetryConfig{}) || trackerRetryConfig(nil) != (config.UpstreamRetryConfig{}) {
		t.Fatal("nil configs take the defaults")
	}
}
