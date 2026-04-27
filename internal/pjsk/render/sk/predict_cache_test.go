package sk

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPredictRenderCacheTTLUsesLongRetention(t *testing.T) {
	if got := predictRenderCacheTTL(); got != 2*time.Hour {
		t.Fatalf("predict cache ttl = %v, want %v", got, 2*time.Hour)
	}
}

func TestPredictRenderCacheReturnsStaleDataWhileRefreshing(t *testing.T) {
	cache := newPredictRenderCache()
	key := "predict-key"
	now := time.Now().UTC()

	cache.mu.Lock()
	cache.entries[key] = &predictRenderCacheEntry{
		data:          []byte("old"),
		nextRefreshAt: now.Add(-time.Second),
		expiresAt:     now.Add(time.Hour),
	}
	cache.mu.Unlock()

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	render := func() ([]byte, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []byte("new"), nil
	}

	first, err := cache.RenderManaged(key, now, render)
	if err != nil {
		t.Fatalf("first render with stale cache: %v", err)
	}
	if string(first) != "old" {
		t.Fatalf("expected stale cache bytes first, got %q", string(first))
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async refresh to start")
	}

	second, err := cache.RenderManaged(key, now.Add(time.Second), render)
	if err != nil {
		t.Fatalf("second render with stale cache: %v", err)
	}
	if string(second) != "old" {
		t.Fatalf("expected stale cache bytes during background refresh, got %q", string(second))
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one async refresh attempt while cache stays stale, got %d", got)
	}

	close(release)

	deadline := time.Now().Add(time.Second)
	for {
		third, err := cache.RenderManaged(key, now.Add(2*time.Second), render)
		if err == nil && string(third) == "new" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for refreshed cache, last result=%q err=%v", string(third), err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPredictRenderCacheRetriesFailuresThenCoolsDown(t *testing.T) {
	cache := newPredictRenderCache()
	key := "predict-key"
	now := time.Now().UTC()

	prevRefresh := predictRenderRefreshInterval
	prevRetry := predictRenderFailureRetryInterval
	prevLimit := predictRenderFailureRetryLimit
	t.Cleanup(func() {
		predictRenderRefreshInterval = prevRefresh
		predictRenderFailureRetryInterval = prevRetry
		predictRenderFailureRetryLimit = prevLimit
	})
	predictRenderRefreshInterval = 20 * time.Millisecond
	predictRenderFailureRetryInterval = 5 * time.Millisecond
	predictRenderFailureRetryLimit = 3

	var calls atomic.Int32
	allowSuccess := atomic.Bool{}
	render := func() ([]byte, error) {
		calls.Add(1)
		if allowSuccess.Load() {
			return []byte("warm"), nil
		}
		return nil, errors.New("forecast source down")
	}

	if _, err := cache.RenderManaged(key, now, render); err == nil || err.Error() != "forecast source down" {
		t.Fatalf("expected first refresh cycle to fail, got %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected three retry attempts in one failed cycle, got %d", got)
	}

	if _, err := cache.RenderManaged(key, now.Add(10*time.Millisecond), render); err == nil || err.Error() != "forecast source down" {
		t.Fatalf("expected cooldown request to reuse cached failure, got %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected cooldown request to avoid new retry cycle, got %d attempts", got)
	}

	time.Sleep(25 * time.Millisecond)
	allowSuccess.Store(true)

	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		got, err := cache.RenderManaged(key, time.Now().UTC(), render)
		if err == nil && string(got) == "warm" {
			if calls.Load() < 4 {
				t.Fatalf("expected a new refresh cycle after cooldown, got only %d attempts", calls.Load())
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected refresh after cooldown to succeed, got result=%q err=%v calls=%d", string(got), err, calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
