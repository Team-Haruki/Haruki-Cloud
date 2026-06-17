package drawing

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDrawingSKLimiterLimitsDifferentCacheMisses(t *testing.T) {
	client := NewHarukiDrawingClient("http://drawing", WithLimiter(LimiterConfig{
		SKMaxConcurrency: 1,
		SKAcquireTimeout: time.Second,
	}))

	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var active atomic.Int32
	var maxActive atomic.Int32
	render := func(any) ([]byte, error) {
		current := active.Add(1)
		for {
			max := maxActive.Load()
			if current <= max || maxActive.CompareAndSwap(max, current) {
				break
			}
		}
		if current == 1 {
			startedOnce.Do(func() { close(started) })
		}
		<-release
		active.Add(-1)
		return []byte("image"), nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, rank := range []int{1, 2} {
		rank := rank
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.RenderWithCache("/api/pjsk/sk/query", map[string]any{
				"region": "CN",
				"rank":   rank,
			}, render)
			errs <- err
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first render did not start")
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("render failed: %v", err)
		}
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("expected SK limiter to cap active renders at 1, got %d", got)
	}
}

func TestDrawingSKLimiterDoesNotRunOnCacheHit(t *testing.T) {
	client := NewHarukiDrawingClient("http://drawing", WithLimiter(LimiterConfig{
		SKMaxConcurrency: 1,
		SKAcquireTimeout: time.Millisecond,
	}))
	req := map[string]any{"region": "CN", "rank": 100}
	var renders atomic.Int32
	first, err := client.RenderWithCache("/api/pjsk/sk/query", req, func(any) ([]byte, error) {
		renders.Add(1)
		return []byte("cached-image"), nil
	})
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}
	second, err := client.RenderWithCache("/api/pjsk/sk/query", req, func(any) ([]byte, error) {
		renders.Add(1)
		return nil, errors.New("cache hit should not render")
	})
	if err != nil {
		t.Fatalf("cached render failed: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("cache hit returned different image")
	}
	if got := renders.Load(); got != 1 {
		t.Fatalf("expected cache hit to avoid render, got %d renders", got)
	}
}

func TestDrawingSKLimiterRespectsContextTimeout(t *testing.T) {
	client := NewHarukiDrawingClient("http://drawing", WithLimiter(LimiterConfig{
		SKMaxConcurrency: 1,
		SKAcquireTimeout: time.Second,
	}))
	release := make(chan struct{})
	started := make(chan struct{})
	go func() {
		_, _ = client.RenderWithCache("/api/pjsk/sk/query", map[string]any{"region": "CN", "rank": 1}, func(any) ([]byte, error) {
			close(started)
			<-release
			return []byte("image"), nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first render did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := client.WithContext(ctx).RenderWithCache("/api/pjsk/sk/query", map[string]any{"region": "CN", "rank": 2}, func(any) ([]byte, error) {
		return []byte("unexpected"), nil
	})
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline while waiting for SK limiter, got %v", err)
	}
}
