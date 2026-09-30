package mysekai

import (
	"context"
	"haruki-cloud/config"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

type housingBlockingPutStore struct {
	storage.Store
	entered, release chan struct{}
	calls            atomic.Int32
}

func (s *housingBlockingPutStore) Put(ctx context.Context, key storage.Key, data []byte, opts storage.PutOptions) error {
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

func TestHousingRefreshReturnsWhilePreviousPersistenceIsBlocked(t *testing.T) {
	memory := storagetest.NewMemory()
	store := &housingBlockingPutStore{Store: memory, entered: make(chan struct{}), release: make(chan struct{})}
	cache := newHousingCompetitionStatsCacheWithStore(store, "housing.json", time.Minute)
	if err := cache.configurePersistence(t.Context(), "node-a"); err != nil {
		t.Fatal(err)
	}
	api := &concurrentHousingCompetitionClient{lotteryAt: time.Now().UnixMilli()}
	if _, _, _, err := cache.Refresh(t.Context(), api, "jp", 1, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("persistence did not start")
	}
	second := make(chan error, 1)
	go func() { _, _, _, err := cache.Refresh(t.Context(), api, "jp", 2, 1); second <- err }()
	select {
	case err := <-second:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh blocked on previous PUT")
	}
	close(store.release)
	if err := cache.closePersistence(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := memory.Get(t.Context(), cache.storeKey)
	if err != nil {
		t.Fatal(err)
	}
	var persisted persistedHousingCompetitionStatsCache
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Buckets) != 2 {
		t.Fatalf("buckets=%d", len(persisted.Buckets))
	}
	if _, err := memory.Get(t.Context(), "housing.json"); err == nil {
		t.Fatal("wrote shared legacy key")
	}
}

func TestHousingNamespaceMigrationAndUnconfiguredReadOnly(t *testing.T) {
	memory := storagetest.NewMemory()
	source := newHousingCompetitionStatsCache("", time.Minute)
	seedHousingStatsForPersist(source)
	payload, err := json.Marshal(source.snapshotForPersistenceLocked())
	if err != nil {
		t.Fatal(err)
	}
	memory.Seed(map[string][]byte{"housing.json": payload})
	cache := newHousingCompetitionStatsCacheWithStore(memory, "housing.json", time.Minute)
	if len(cache.buckets) != 1 {
		t.Fatal("legacy snapshot not loaded")
	}
	cache.persistLatest(t.Context(), 1)
	if cache.persistence != nil {
		t.Fatal("unconfigured shared store started writer")
	}
	if err := cache.configurePersistence(t.Context(), ""); err == nil {
		t.Fatal("accepted empty namespace")
	}
	if err := cache.configurePersistence(t.Context(), "node-a"); err != nil {
		t.Fatal(err)
	}
	if len(cache.buckets) != 1 {
		t.Fatal("missing namespace discarded legacy snapshot")
	}
	cache.generation = 1
	cache.persistLatest(t.Context(), 1)
	if err := cache.closePersistence(t.Context()); err != nil {
		t.Fatal(err)
	}
	reloaded := newHousingCompetitionStatsCacheWithStore(memory, "housing.json", time.Minute)
	if err := reloaded.configurePersistence(t.Context(), "node-a"); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.buckets) != 1 {
		t.Fatal("namespace snapshot was not loaded")
	}
}

func TestHousingLifecycleShutdownCancelsUpstreamAndDrains(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer server.Close()
	api := sekaiapi.NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL})
	life, cancel := context.WithCancel(t.Context())
	defer cancel()
	cache := newHousingCompetitionStatsCache("", time.Minute)
	if err := cache.configurePersistence(life, "node-a"); err != nil {
		t.Fatal(err)
	}
	seedHousingStatsForPersist(cache)
	result := make(chan error, 1)
	go func() { _, _, _, err := cache.Refresh(t.Context(), api, "jp", 3, 1); result <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("upstream did not start")
	}
	cancel()
	if err := cache.closePersistence(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("refresh should be canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("refresh did not drain")
	}
	key := housingCompetitionStatsCacheKey{Region: "jp", HousingID: 3}
	cache.mu.Lock()
	bucket := cache.buckets[key]
	generation := cache.generation
	cache.mu.Unlock()
	if bucket == nil || bucket.entries["a"].ReviewCount != 9 || generation != 1 {
		t.Fatal("shutdown discarded previous cache data")
	}
	if _, _, _, err := cache.Refresh(t.Context(), api, "jp", 3, 1); err == nil {
		t.Fatal("refresh accepted after shutdown")
	}
}
