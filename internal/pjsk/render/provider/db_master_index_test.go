package provider

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDBMasterIndexSharesLoadsAndAllowsCancellation(t *testing.T) {
	var index dbMasterIndex[int]
	entered, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	var calls atomic.Int32
	load := func(ctx context.Context) (int, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
			return 42, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	canceled := make(chan error, 1)
	go func() { _, err := index.get(ctx, "test.index", load); canceled <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("load did not start")
	}
	var waiters sync.WaitGroup
	for range 16 {
		waiters.Go(func() {
			value, err := index.get(t.Context(), "test.index", load)
			if err != nil || value != 42 {
				t.Errorf("follower=%d %v", value, err)
			}
		})
	}
	cancel()
	select {
	case err := <-canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation blocked")
	}
	unblock.Do(func() { close(release) })
	waiters.Wait()
	if calls.Load() != 1 {
		t.Fatalf("shared loads=%d", calls.Load())
	}
}

func TestDBMasterIndexDiscardsPreResetFlight(t *testing.T) {
	var index dbMasterIndex[int]
	entered, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	old := make(chan int, 1)
	go func() {
		value, err := index.get(t.Context(), "test.index", func(context.Context) (int, error) { close(entered); <-release; return 1, nil })
		if err != nil {
			t.Error(err)
		}
		old <- value
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("load did not start")
	}
	index.reset()
	value, err := index.get(t.Context(), "test.index", func(context.Context) (int, error) { return 2, nil })
	if err != nil || value != 2 {
		t.Fatalf("current generation=%d %v", value, err)
	}
	unblock.Do(func() { close(release) })
	select {
	case value := <-old:
		if value != 2 {
			t.Fatalf("pre-reset flight returned stale data %d", value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("old load did not finish")
	}
}

// waitForIndexRefresh polls until a background index refresh is visible.
func waitForIndexRefresh(t *testing.T, refreshed func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !refreshed() {
		if time.Now().After(deadline) {
			t.Fatal("background index refresh did not complete")
		}
		time.Sleep(time.Millisecond)
	}
}

func expireMasterIndex[T any](index *dbMasterIndex[T]) {
	index.mu.Lock()
	index.loadedAt = time.Now().Add(-dbBulkIndexTTL)
	index.mu.Unlock()
}

func TestDBMasterIndexServesStaleWhileOneFlightRefreshes(t *testing.T) {
	var index dbMasterIndex[int]
	if value, err := index.get(t.Context(), "test.index", func(context.Context) (int, error) { return 1, nil }); err != nil || value != 1 {
		t.Fatalf("initial load = %d, %v", value, err)
	}
	expireMasterIndex(&index)

	entered, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	var calls atomic.Int32
	refresh := func(context.Context) (int, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return 2, nil
	}
	// Readers get the previous index immediately while the reload is blocked.
	for range 8 {
		value, err := index.get(t.Context(), "test.index", refresh)
		if err != nil || value != 1 {
			t.Fatalf("stale read = %d, %v", value, err)
		}
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("background refresh did not start")
	}
	unblock.Do(func() { close(release) })
	waitForIndexRefresh(t, func() bool {
		value, err := index.get(t.Context(), "test.index", refresh)
		return err == nil && value == 2
	})
	if calls.Load() != 1 {
		t.Fatalf("refresh loads = %d, want 1", calls.Load())
	}
}

func TestDBMasterIndexKeepsStaleValueWhenRefreshFails(t *testing.T) {
	var index dbMasterIndex[int]
	if _, err := index.get(t.Context(), "test.index", func(context.Context) (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	expireMasterIndex(&index)
	var calls atomic.Int32
	failing := func(context.Context) (int, error) { calls.Add(1); return 0, errors.New("database unavailable") }
	if value, err := index.get(t.Context(), "test.index", failing); err != nil || value != 1 {
		t.Fatalf("stale read with failing refresh = %d, %v", value, err)
	}
	waitForIndexRefresh(t, func() bool { return calls.Load() >= 1 })
	// The failed refresh left the index stale, so the next reader retries it.
	waitForIndexRefresh(t, func() bool {
		value, err := index.get(t.Context(), "test.index", func(context.Context) (int, error) { return 3, nil })
		return err == nil && value == 3
	})
}

func TestDBMasterIndexResetMakesReadersWaitForNewData(t *testing.T) {
	var index dbMasterIndex[int]
	if _, err := index.get(t.Context(), "test.index", func(context.Context) (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	expireMasterIndex(&index)
	index.reset()
	// After a reset nothing stale may be served, even though the TTL passed.
	if value, err := index.get(t.Context(), "test.index", func(context.Context) (int, error) { return 2, nil }); err != nil || value != 2 {
		t.Fatalf("read after reset = %d, %v", value, err)
	}
}
