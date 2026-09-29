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
