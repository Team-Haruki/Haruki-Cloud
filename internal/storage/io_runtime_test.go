package storage

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestIOAdmissionSharesLimitsWithoutHoldingGlobalForBusyOrigin(t *testing.T) {
	runtime := NewIORuntime(IOConfig{MaxConcurrent: 3, MaxPerOrigin: 1, MaxBackground: 1})
	ctx := context.Background()
	a, err := runtime.Acquire(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	defer a()
	queued, cancel := context.WithCancel(ctx)
	failed := make(chan error, 1)
	go func() {
		release, err := runtime.Acquire(queued, "a")
		if release != nil {
			release()
		}
		failed <- err
	}()
	// A waiter for a full origin must not prevent the other origin entering.
	bctx, bcancel := context.WithTimeout(ctx, time.Second)
	defer bcancel()
	b, err := runtime.Acquire(bctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	b()
	cancel()
	if err := <-failed; !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
	runtime.mu.Lock()
	active := runtime.active
	runtime.mu.Unlock()
	if active != 1 {
		t.Fatalf("active = %d after waiter cancellation", active)
	}
}

func TestIOBackgroundReservesForegroundCapacityAndReleaseIsIdempotent(t *testing.T) {
	runtime := NewIORuntime(IOConfig{MaxConcurrent: 2, MaxPerOrigin: 2, MaxBackground: 1})
	background := WithBackgroundIO(context.Background())
	if !IsBackgroundIO(background) {
		t.Fatal("background marker lost")
	}
	release, err := runtime.Acquire(background, "a")
	if err != nil {
		t.Fatal(err)
	}
	queued, cancel := context.WithCancel(background)
	failed := make(chan error, 1)
	go func() {
		r, err := runtime.Acquire(queued, "b")
		if r != nil {
			r()
		}
		failed <- err
	}()
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	foreground, err := runtime.Acquire(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	foreground()
	cancel()
	if err := <-failed; !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
	release()
	release()
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.active != 0 || runtime.background != 0 || len(runtime.origins) != 0 {
		t.Fatalf("permits leaked: active=%d background=%d origins=%v", runtime.active, runtime.background, runtime.origins)
	}
}

func TestIORuntimeSharesOnlyMatchingTransportOptions(t *testing.T) {
	runtime := NewIORuntime(IOConfig{})
	var builds atomic.Int32
	build := func() *http.Transport { builds.Add(1); return &http.Transport{} }
	a := runtime.SharedTransport("one", build)
	if runtime.SharedTransport("one", build) != a {
		t.Fatal("same options did not share")
	}
	if runtime.SharedTransport("two", build) == a || builds.Load() != 2 {
		t.Fatal("different options shared")
	}
}

func BenchmarkIOAdmission(b *testing.B) {
	runtime := NewIORuntime(IOConfig{})
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		release, err := runtime.Acquire(ctx, "origin")
		if err != nil {
			b.Fatal(err)
		}
		release()
	}
}

func BenchmarkIOMetricNoTrace(b *testing.B) {
	runtime := NewIORuntime(IOConfig{})
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		runtime.Record(ctx, SlotAssets, 1, "listdir", "attempt", time.Millisecond, 0)
	}
}
