package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigureAppCachePersistenceRunsEveryCacheConcurrently(t *testing.T) {
	var inFlight, peak atomic.Int32
	var mu sync.Mutex
	namespaces := map[string]string{}
	configure := func(name string, err error) func(context.Context, string) error {
		return func(_ context.Context, namespace string) error {
			current := inFlight.Add(1)
			for {
				observed := peak.Load()
				if current <= observed || peak.CompareAndSwap(observed, current) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			inFlight.Add(-1)
			mu.Lock()
			namespaces[name] = namespace
			mu.Unlock()
			return err
		}
	}

	startedAt := time.Now()
	configureAppCachePersistence(context.Background(), "node-a", map[string]func(context.Context, string) error{
		"sk":      configure("sk", nil),
		"mysekai": configure("mysekai", errors.New("read down")),
	})
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("restores ran serially: %v", elapsed)
	}
	if peak.Load() != 2 {
		t.Fatalf("peak concurrency = %d, want 2", peak.Load())
	}
	if namespaces["sk"] != "node-a" || namespaces["mysekai"] != "node-a" {
		t.Fatalf("namespaces = %v", namespaces)
	}

	configureAppCachePersistence(context.Background(), "node-a", nil)
}
