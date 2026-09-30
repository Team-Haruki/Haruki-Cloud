package sk

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type forecastBlockingPutStore struct {
	storage.Store
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (s *forecastBlockingPutStore) Put(ctx context.Context, key storage.Key, data []byte, opts storage.PutOptions) error {
	if s.calls.Add(1) == 1 {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.Store.Put(ctx, key, data, opts)
}

func TestForecastRefreshDoesNotWaitForPersistenceAndDoesNotRetainTrace(t *testing.T) {
	memory := storagetest.NewMemory()
	store := &forecastBlockingPutStore{Store: memory, entered: make(chan struct{}), release: make(chan struct{})}
	cache := newForecastDataCacheWithStore(&keyedForecastProvider{}, store, "forecast.json")
	if err := cache.configurePersistence(t.Context(), "node-a"); err != nil {
		t.Fatal(err)
	}
	traceCtx, trace := commandtrace.WithTrace(t.Context())
	ctx, cancel := context.WithCancel(traceCtx)
	if err := cache.RefreshNow(ctx, "jp", 1); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("background persistence did not start")
	}
	before := trace.Snapshot()
	second := make(chan error, 1)
	go func() { second <- cache.RefreshNow(t.Context(), "jp", 2) }()
	select {
	case err := <-second:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh blocked on an unrelated PUT")
	}
	close(store.release)
	if err := cache.closePersistence(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, trace.Snapshot()) {
		t.Fatal("background persistence modified canceled request trace")
	}
	data, err := memory.Get(t.Context(), cache.storeKey)
	if err != nil {
		t.Fatal(err)
	}
	var persisted persistedForecastDataCache
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Entries) != 2 {
		t.Fatalf("entries=%d", len(persisted.Entries))
	}
	if _, err := memory.Get(t.Context(), "forecast.json"); err == nil {
		t.Fatal("wrote shared legacy key")
	}
}

func TestForecastNamespaceMigrationAndUnconfiguredReadOnly(t *testing.T) {
	memory := storagetest.NewMemory()
	source := newForecastDataCache(&keyedForecastProvider{})
	if err := source.RefreshNow(t.Context(), "jp", 8); err != nil {
		t.Fatal(err)
	}
	source.mu.Lock()
	payload, err := json.Marshal(source.snapshotForPersistenceLocked())
	source.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	memory.Seed(map[string][]byte{"forecast.json": payload})
	cache := newForecastDataCacheWithStore(&keyedForecastProvider{}, memory, "forecast.json")
	if _, err := cache.CachedBySource("jp", 8, []int{100}); err != nil {
		t.Fatal(err)
	}
	cache.persistLatest(t.Context(), 1)
	if cache.persistence != nil {
		t.Fatal("unconfigured shared store started a writer")
	}
	if err := cache.configurePersistence(t.Context(), ""); err == nil {
		t.Fatal("accepted empty namespace")
	}
	if err := cache.configurePersistence(t.Context(), "node-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.CachedBySource("jp", 8, []int{100}); err != nil {
		t.Fatal("missing namespace did not keep legacy snapshot", err)
	}
	if err := cache.RefreshNow(t.Context(), "jp", 9); err != nil {
		t.Fatal(err)
	}
	if err := cache.closePersistence(t.Context()); err != nil {
		t.Fatal(err)
	}
	reloaded := newForecastDataCacheWithStore(nil, memory, "forecast.json")
	if err := reloaded.configurePersistence(t.Context(), "node-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := reloaded.CachedBySource("jp", 9, []int{100}); err != nil {
		t.Fatal("namespace snapshot did not take precedence", err)
	}
	other := newForecastDataCacheWithStore(nil, memory, "forecast.json")
	if err := other.configurePersistence(t.Context(), "node-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := other.CachedBySource("jp", 9, []int{100}); err == nil {
		t.Fatal("different instance read private namespace")
	}
}

type lifecycleForecastProvider struct {
	entered chan struct{}
	calls   atomic.Int32
}

func (p *lifecycleForecastProvider) Fetch(ctx context.Context, _ string, _ int, _ []int) (map[int]ForecastScore, error) {
	p.calls.Add(1)
	close(p.entered)
	<-ctx.Done()
	// A late provider response must not replace good cache data during shutdown.
	return map[int]ForecastScore{100: {Score: 999}}, nil
}

func TestForecastLifecycleShutdownDrainsAndKeepsPreviousData(t *testing.T) {
	life, cancel := context.WithCancel(t.Context())
	cache := newForecastDataCache(&keyedForecastProvider{})
	if err := cache.configurePersistence(life, "node-a"); err != nil {
		t.Fatal(err)
	}
	if err := cache.RefreshNow(t.Context(), "jp", 1); err != nil {
		t.Fatal(err)
	}
	before, err := cache.CachedBySource("jp", 1, []int{100})
	if err != nil {
		t.Fatal(err)
	}
	provider := &lifecycleForecastProvider{entered: make(chan struct{})}
	cache.mu.Lock()
	cache.provider = provider
	cache.mu.Unlock()
	cache.StartRefresh("jp", 1)
	select {
	case <-provider.entered:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	if err := cache.closePersistence(t.Context()); err != nil {
		t.Fatal(err)
	}
	after, err := cache.CachedBySource("jp", 1, []int{100})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("canceled refresh overwrote previous data")
	}
	if err := cache.RefreshNow(t.Context(), "jp", 2); err == nil {
		t.Fatal("refresh accepted after shutdown")
	}
	if provider.calls.Load() != 1 {
		t.Fatal("shutdown allowed another upstream fetch")
	}
}
