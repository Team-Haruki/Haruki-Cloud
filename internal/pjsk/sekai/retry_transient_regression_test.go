package sekai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/config"
)

// flakyServer answers the first `failures` requests with failStatus (or, when
// hang > 0, by stalling past the client timeout) and then 200 with body.
func flakyServer(t *testing.T, failures int32, failStatus int, hang time.Duration, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= failures {
			if hang > 0 {
				select {
				case <-time.After(hang):
				case <-r.Context().Done():
				}
				return
			}
			w.WriteHeader(failStatus)
			_, _ = w.Write([]byte(`{"result":"failed","message":"transient"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

const profileBody = `{"user":{"userId":1,"name":"x","rank":1}}`

// fastClientRetry keeps the default retry count and conditions with
// millisecond waits.
var fastClientRetry = config.UpstreamRetryConfig{Wait: time.Millisecond, MaxWait: time.Millisecond}

// Regression for 3.8.0: a single transient upstream failure on the profile
// GET was absorbed by the client's retries through 3.7.18 (4 retries on any
// 5xx or network error). 3.8.0 retried only 502/503 and resets, once, so
// these surfaced as profile command errors.
func TestSekaiProfileTransientFailureIsRetried(t *testing.T) {
	cases := []struct {
		name       string
		failures   int32
		failStatus int
	}{
		{name: "500 once", failures: 1, failStatus: http.StatusInternalServerError},
		{name: "504 once", failures: 1, failStatus: http.StatusGatewayTimeout},
		{name: "502 twice", failures: 2, failStatus: http.StatusBadGateway},
		{name: "503 twice", failures: 2, failStatus: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, calls := flakyServer(t, tc.failures, tc.failStatus, 0, profileBody)
			client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: srv.URL, Retry: fastClientRetry})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err := client.GetUserProfileContext(ctx, "cn", "1")
			if err != nil {
				t.Fatalf("profile fetch failed after %d upstream calls: %T %v", calls.Load(), err, err)
			}
		})
	}
}

// Same for the Toolbox private-data GET that every suite command depends on.
func TestToolboxTransientFailureIsRetried(t *testing.T) {
	cases := []struct {
		name       string
		failures   int32
		failStatus int
	}{
		{name: "500 once", failures: 1, failStatus: http.StatusInternalServerError},
		{name: "504 once", failures: 1, failStatus: http.StatusGatewayTimeout},
		{name: "502 twice", failures: 2, failStatus: http.StatusBadGateway},
		{name: "503 three times", failures: 3, failStatus: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, calls := flakyServer(t, tc.failures, tc.failStatus, 0, `{"upload_time":1}`)
			client := NewToolboxClient(&config.ToolboxConfig{BaseURL: srv.URL, APIToken: "t", Retry: fastClientRetry})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err := client.GetPrivateDataContext(ctx, "cn", ToolboxDataTypeSuite, 1, "qq", "1")
			if err != nil {
				t.Fatalf("toolbox fetch failed after %d upstream calls: %T %v", calls.Load(), err, err)
			}
		})
	}
}

// markFirstAttempt replaces the request context from OnBeforeRequest. The
// replacement must still carry the caller's cancellation to the wire.
func TestRetryContextKeepsCancellation(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: srv.URL})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	started := time.Now()
	_, err := client.GetUserProfileContext(ctx, "cn", "1")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancellation did not reach the in-flight request: returned after %s (%v)", elapsed, err)
	}
	if err != context.Canceled {
		t.Fatalf("err = %T %v, want context.Canceled", err, err)
	}
}
