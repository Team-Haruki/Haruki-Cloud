//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package drawing

import (
	"context"
	"errors"
	"haruki-cloud/utils/imagecache"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"
)

func TestRunSharedRenderFlightAcceptsNilParent(t *testing.T) {
	result := runSharedRenderFlight(nil, func(ctx context.Context) ([]byte, error) {
		if ctx == nil {
			t.Fatal("shared context is nil")
		}
		return []byte("shared"), nil
	})
	if result.err != nil || string(result.data) != "shared" {
		t.Fatalf("shared result = %q, %v", result.data, result.err)
	}
}

func TestRenderCacheFallbackContexts(t *testing.T) {
	var nilClient *RenderCacheClient
	called := false
	data, err := nilClient.RenderSharedContext(nil, "/api/pjsk/profile", nil, func(ctx context.Context) ([]byte, error) {
		called = true
		if ctx != nil {
			t.Fatal("nil client changed the supplied context")
		}
		return []byte("nil-client"), nil
	})
	if err != nil || !called || string(data) != "nil-client" {
		t.Fatalf("nil-client fallback = %q, called=%v, err=%v", data, called, err)
	}

	client := newIndexClient(t, &fakeRenderIndex{})
	data, err = client.RenderSharedContext(nil, "/api/pjsk/event/detail", nil, func(ctx context.Context) ([]byte, error) {
		if ctx == nil {
			t.Fatal("configured client did not supply a fallback context")
		}
		return []byte("disabled-policy"), nil
	})
	if err != nil || string(data) != "disabled-policy" {
		t.Fatalf("disabled-policy fallback = %q, %v", data, err)
	}
}

func TestWaitForRenderFlightResultBranches(t *testing.T) {
	sentinel := errors.New("flight failed")
	assertRenderFlightError(t, singleflight.Result{Err: sentinel}, sentinel)
	assertRenderFlightError(t, singleflight.Result{Val: "unexpected"}, nil)
	assertRenderFlightError(t, singleflight.Result{Val: renderFlightResult{err: sentinel}}, sentinel)

	token := new(renderFlightToken)
	result := make(chan singleflight.Result, 1)
	result <- singleflight.Result{Val: renderFlightResult{data: []byte("ok"), leader: token}}
	data, err := waitForRenderFlight(context.Background(), result, token, "test")
	if err != nil || string(data) != "ok" {
		t.Fatalf("successful flight = %q, %v", data, err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := waitForRenderFlight(canceled, make(chan singleflight.Result), token, "test"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled flight error = %v", err)
	}
}

func assertRenderFlightError(t *testing.T, completed singleflight.Result, expected error) {
	t.Helper()
	result := make(chan singleflight.Result, 1)
	result <- completed
	_, err := waitForRenderFlight(context.Background(), result, new(renderFlightToken), "test")
	if expected != nil {
		if !errors.Is(err, expected) {
			t.Fatalf("flight error = %v, want %v", err, expected)
		}
		return
	}
	if err == nil {
		t.Fatal("unexpected flight value was accepted")
	}
}

func TestRenderRemoteFlightWorkHitAndRenderFailure(t *testing.T) {
	index := &fakeRenderIndex{entries: map[string]imagecache.RenderIndexEntry{"hit": indexEntry("hit", time.Now().Add(time.Hour))}}
	client := newIndexClient(t, index)
	policy := renderCachePolicy{APIPath: "api/pjsk/profile", TTL: time.Minute}
	rendered := false
	data, err := client.renderRemoteFlightWork(context.Background(), "/api/pjsk/profile", "hit", policy, func(context.Context) ([]byte, error) {
		rendered = true
		return nil, nil
	})
	if err != nil || rendered || string(data) != "stored" {
		t.Fatalf("cache hit = %q, rendered=%v, err=%v", data, rendered, err)
	}

	sentinel := errors.New("render failed")
	if _, err := client.renderRemoteFlightWork(context.Background(), "/api/pjsk/profile", "miss", policy, func(context.Context) ([]byte, error) {
		return nil, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("render failure = %v", err)
	}
}
