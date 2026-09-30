package assetindex

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func fixture(t *testing.T) (context.Context, *storagetest.Memory, Manifest) {
	t.Helper()
	ctx := context.Background()
	s := storagetest.NewMemory()
	for key, data := range map[string]string{"jp-assets/startapp/home/A/banner.PNG": "a", "jp-assets/ondemand/music/jacket/1.png": "b"} {
		if err := s.Put(ctx, storage.Key(key), []byte(data), storage.PutOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Prepare(ctx, s, "jp")
	if err != nil {
		t.Fatal(err)
	}
	if err = Commit(ctx, s, m); err != nil {
		t.Fatal(err)
	}
	return ctx, s, m
}
func TestCompleteIndexAndScopedRevision(t *testing.T) {
	ctx, s, _ := fixture(t)
	changed := 0
	m := New(s, Config{Enabled: true}, func() { changed++ })
	if err := m.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	got, found, known := m.Lookup("jp-assets/startapp/HOME/a/banner.png")
	if !known || !found || got != "jp-assets/startapp/home/A/banner.PNG" {
		t.Fatalf("lookup: %q %v %v", got, found, known)
	}
	if _, found, known = m.Lookup("jp-assets/startapp/missing.png"); found || !known {
		t.Fatal("complete index must know missing")
	}
	if _, _, known = m.Lookup("kr-assets/startapp/missing.png"); known {
		t.Fatal("unloaded region is not authoritative")
	}
	payload := map[string]any{"image": "jp-assets/startapp/home/A/banner.PNG"}
	old := m.RevisionForPayload(payload)
	global := m.Revision()
	if err := s.Put(ctx, "jp-assets/ondemand/music/jacket/2.png", []byte("new"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	next, err := Prepare(ctx, s, "jp")
	if err != nil {
		t.Fatal(err)
	}
	if err = Commit(ctx, s, next); err != nil {
		t.Fatal(err)
	}
	if err = m.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if old == "" || m.RevisionForPayload(payload) != old || m.Revision() == global {
		t.Fatal("unrelated shard must preserve scoped revision and update global revision")
	}
	if changed != 2 {
		t.Fatalf("changes %d", changed)
	}
	if err = m.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if changed != 2 {
		t.Fatal("unchanged pointer should not clear caches")
	}
}
func TestPartialOrCorruptUpdateRetainsLastCompleteIndex(t *testing.T) {
	ctx, s, manifest := fixture(t)
	m := New(s, Config{Enabled: true}, nil)
	if err := m.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	old := m.Revision()
	manifest.Revision = "1111111111111111111111111111111111111111111111111111111111111111"
	data, _ := json.Marshal(manifest)
	if err := s.Put(ctx, PointerKey("jp"), data, storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(ctx); err == nil {
		t.Fatal("wrong revision must fail")
	}
	if m.Revision() != old {
		t.Fatal("failed update replaced working version")
	}
	if _, ok, known := m.Lookup("jp-assets/startapp/home/A/banner.PNG"); !ok || !known {
		t.Fatal("working index lost")
	}
}
func TestPublisherRejectsMutationAndIncompleteMetadata(t *testing.T) {
	ctx, s, manifest := fixture(t)
	if err := s.Put(ctx, "jp-assets/startapp/new.png", []byte("new"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Commit(ctx, s, manifest); err == nil {
		t.Fatal("concurrent mutation must stop pointer advance")
	}
	if _, err := Prepare(ctx, s, "zz"); err == nil {
		t.Fatal("invalid region accepted")
	}
	if _, err := Prepare(ctx, s, "kr"); err == nil {
		t.Fatal("empty region accepted")
	}
	manifest.Complete = false
	if err := Commit(ctx, s, manifest); err == nil {
		t.Fatal("incomplete manifest accepted")
	}
}
func TestStaleIndexStopsClaimingMissing(t *testing.T) {
	ctx, s, _ := fixture(t)
	m := New(s, Config{Enabled: true, MaxStale: time.Minute}, nil)
	now := time.Now()
	m.now = func() time.Time { return now }
	if err := m.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, _, known := m.Lookup("jp-assets/startapp/not-here"); known {
		t.Fatal("stale index claimed missing")
	}
	if err := m.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, known := m.Lookup("jp-assets/startapp/not-here"); !known {
		t.Fatal("pointer refresh did not renew freshness")
	}
}
func TestCanceledRefreshAndNilManager(t *testing.T) {
	ctx, s, _ := fixture(t)
	m := New(s, Config{Enabled: true}, nil)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.Refresh(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v", err)
	}
	var empty *Manager
	empty.Start(ctx)
	empty.Close()
	if empty.Revision() != "" {
		t.Fatal("nil revision")
	}
	if got := New(s, Config{}, nil); got != nil {
		t.Fatal("disabled manager")
	}
}

func TestCommitRejectsOmittedShardEvenWhenSourceDidNotChange(t *testing.T) {
	ctx, store, manifest := fixture(t)
	previous, err := store.Get(ctx, PointerKey("jp"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Shards = manifest.Shards[:1]
	manifest.Revision = manifestRevision(manifest.Shards)
	if err := Commit(ctx, store, manifest); err == nil {
		t.Fatal("published a partial inventory as complete")
	}
	current, err := store.Get(ctx, PointerKey("jp"))
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(previous) {
		t.Fatal("failed publication advanced pointer")
	}
}

func TestCaseFoldRespectsFileDirectoryKindsAndScopedRevision(t *testing.T) {
	ctx, store, _ := fixture(t)
	for _, key := range []storage.Key{"jp-assets/root.txt", "jp-assets/startapp/HOME"} {
		if err := store.Put(ctx, key, []byte("x"), storage.PutOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := Prepare(ctx, store, "jp")
	if err != nil {
		t.Fatal(err)
	}
	if err := Commit(ctx, store, manifest); err != nil {
		t.Fatal(err)
	}
	manager := New(store, Config{Enabled: true}, nil)
	defer manager.Close()
	if err := manager.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if key, found, known := manager.Lookup("jp-assets/startapp/HOME/a/banner.png"); key != "jp-assets/startapp/home/A/banner.PNG" || !found || !known {
		t.Fatalf("directory collision: %q %v %v", key, found, known)
	}
	if key, found, known := manager.Lookup("jp-assets/startapp/home"); key != "jp-assets/startapp/HOME" || !found || !known {
		t.Fatalf("file collision: %q %v %v", key, found, known)
	}
	payload := map[string]any{"path": "asset/jp-assets/startapp/HOME/a/banner.png"}
	previous := manager.RevisionForPayload(payload)
	if err := store.Put(ctx, "jp-assets/startapp/home/A/banner.PNG", []byte("changed"), storage.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	manifest, err = Prepare(ctx, store, "jp")
	if err != nil {
		t.Fatal(err)
	}
	if err := Commit(ctx, store, manifest); err != nil {
		t.Fatal(err)
	}
	if err := manager.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if manager.RevisionForPayload(payload) == previous {
		t.Fatal("case-folded resource incorrectly used unchanged root shard revision")
	}
}

func TestRustPublishedManifestAndBPMDescriptorLoadVerbatim(t *testing.T) {
	root, err := filepath.Abs("testdata/rust-publisher")
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewLocal(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	manager := New(store, Config{Enabled: true}, nil)
	defer manager.Close()
	if err := manager.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if key, found, known := manager.Lookup("jp-assets/ondemand/music/music_score/0002_01/master.txt"); !known || !found || key != "jp-assets/ondemand/music/music_score/0002_01/MASTER.TXT" {
		t.Fatalf("Rust lookup = %q %v %v", key, found, known)
	}
	if _, found, known := manager.Lookup("jp-assets/startapp/thumbnail/chara/missing.png"); found || !known {
		t.Fatal("Rust complete manifest did not report definite missing")
	}
	revision, bpmKey, complete := manager.BPMIndex("jp")
	if !complete || revision != "177d76a07c9e7efb0c6e11b7e4d33882412946f98c86583eb43345b1a1e48b86" || bpmKey != "indexes/bpm/v1/jp/8863d273f9776b7ce256d042f59ee671d4ab81877455b93c2acb9cab57f1766d.json" {
		t.Fatalf("Rust BPM descriptor = %s %s %v", revision, bpmKey, complete)
	}
}

func TestCloseBeforeStartCannotLeakRefreshLoop(t *testing.T) {
	_, store, _ := fixture(t)
	manager := New(store, Config{Enabled: true}, nil)
	manager.Close()
	manager.Start(t.Context())
	if err := manager.Refresh(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed refresh = %v", err)
	}
}

type blockingPointerStore struct {
	storage.Store
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (s *blockingPointerStore) Get(ctx context.Context, key storage.Key) ([]byte, error) {
	if key == PointerKey("jp") {
		if s.calls.Add(1) == 1 {
			close(s.entered)
		}
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.Store.Get(ctx, key)
}

func TestRefreshSingleflightOutlivesCanceledWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, memory, _ := fixture(t)
		store := &blockingPointerStore{Store: memory, entered: make(chan struct{}), release: make(chan struct{})}
		manager := New(store, Config{Enabled: true}, nil)
		defer manager.Close()
		firstCtx, cancel := context.WithCancel(t.Context())
		first := make(chan error, 1)
		go func() { first <- manager.Refresh(firstCtx) }()
		<-store.entered
		second := make(chan error, 1)
		go func() { second <- manager.Refresh(t.Context()) }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if err := <-first; !errors.Is(err, context.Canceled) {
			t.Fatalf("first waiter = %v", err)
		}
		select {
		case err := <-second:
			t.Fatalf("canceled caller interrupted shared refresh: %v", err)
		default:
		}
		close(store.release)
		synctest.Wait()
		if err := <-second; err != nil {
			t.Fatal(err)
		}
		if store.calls.Load() != 1 {
			t.Fatalf("shared pointer reads = %d", store.calls.Load())
		}
		if _, found, known := manager.Lookup("jp-assets/startapp/home/A/banner.PNG"); !found || !known {
			t.Fatal("shared refresh did not install the inventory")
		}
	})
}

type cancellationCleanupStore struct {
	storage.Store
	entered  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (s *cancellationCleanupStore) Get(ctx context.Context, key storage.Key) ([]byte, error) {
	if key == PointerKey("jp") {
		close(s.entered)
		<-ctx.Done()
		close(s.canceled)
		<-s.release
		return nil, ctx.Err()
	}
	return s.Store.Get(ctx, key)
}
func TestCloseDrainsSharedRefreshAfterCancellation(t *testing.T) {
	store := &cancellationCleanupStore{Store: storagetest.NewMemory(), entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	manager := New(store, Config{Enabled: true}, nil)
	refreshed := make(chan error, 1)
	go func() { refreshed <- manager.Refresh(t.Context()) }()
	<-store.entered
	closed := make(chan struct{})
	go func() { manager.Close(); close(closed) }()
	<-store.canceled
	select {
	case <-closed:
		close(store.release)
		t.Fatal("Close returned before shared storage cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	close(store.release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after shared storage cleanup")
	}
	if err := <-refreshed; !errors.Is(err, context.Canceled) {
		t.Fatalf("refresh error = %v", err)
	}
}
