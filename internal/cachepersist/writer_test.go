package cachepersist

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
)

func testOptions() Options {
	return Options{WriteTimeout: time.Second, RetryInterval: time.Hour, CoalesceWindow: time.Millisecond}
}

func TestWriterCoalescesAndDoesNotBlockSchedule(t *testing.T) {
	var latest atomic.Uint64
	var writes atomic.Int64
	entered, release := make(chan struct{}), make(chan struct{})
	latest.Store(1)
	writer := New("test", func(ctx context.Context) (uint64, error) {
		generation := latest.Load()
		if writes.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
		return generation, nil
	}, testOptions())
	defer writer.Close(t.Context())
	writer.Schedule(1)
	select {
	case <-entered:
	case <-t.Context().Done():
		t.Fatal("write did not start")
	}
	scheduled := make(chan struct{})
	go func() {
		for i := uint64(2); i <= 1000; i++ {
			latest.Store(i)
			writer.Schedule(i)
		}
		close(scheduled)
	}()
	select {
	case <-scheduled:
	case <-time.After(time.Second):
		t.Fatal("schedule blocked behind persistence")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := writer.Flush(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled flush = %v", err)
	}
	close(release)
	if err := writer.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := writes.Load(); got != 2 {
		t.Fatalf("writes=%d, want one initial and one coalesced", got)
	}
}

func TestWriterTimeoutFailureRemainsRetryableAndCloseFlushes(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	opts := testOptions()
	opts.WriteTimeout = 15 * time.Millisecond
	writer := New("test", func(ctx context.Context) (uint64, error) {
		if fail.Load() {
			<-ctx.Done()
			return 0, ctx.Err()
		}
		return 5, nil
	}, opts)
	writer.Schedule(5)
	if err := writer.Flush(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout=%v", err)
	}
	fail.Store(false)
	if err := writer.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-writer.done:
	default:
		t.Fatal("Close returned while worker is alive")
	}
	writer.Schedule(6)
	writer.mu.Lock()
	requested := writer.requested
	writer.mu.Unlock()
	if requested != 5 {
		t.Fatalf("closed writer accepted generation %d", requested)
	}
}

func TestWriterCloseCancellationStopsPendingIO(t *testing.T) {
	started := make(chan struct{})
	writer := New("test", func(ctx context.Context) (uint64, error) { close(started); <-ctx.Done(); return 0, ctx.Err() }, testOptions())
	writer.Schedule(1)
	<-started
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := writer.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("close=%v", err)
	}
	select {
	case <-writer.done:
	case <-time.After(time.Second):
		t.Fatal("worker ignored close cancellation")
	}
}

func TestNamespacedKey(t *testing.T) {
	for _, ns := range []string{"", " ", "../node", "a/b", "a\\b", ".", ".."} {
		if _, err := NamespacedKey("cache.json", ns); err == nil {
			t.Fatalf("accepted invalid namespace %q", ns)
		}
	}
	a, err := NamespacedKey("dir/cache.json", "cn08-main")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NamespacedKey("dir/cache.json", "cn08-secondary")
	if err != nil {
		t.Fatal(err)
	}
	if a == b || a != storage.Key("instances/cn08-main/dir/cache.json") {
		t.Fatalf("keys=%q,%q", a, b)
	}
}
