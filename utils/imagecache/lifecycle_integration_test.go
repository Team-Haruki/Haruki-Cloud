package imagecache

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

// Opt-in tests require a disposable local PostgreSQL; remote DSNs are rejected.
func lifecyclePG(t *testing.T) *PGStore {
	t.Helper()
	dsn := os.Getenv("HARUKI_IMAGECACHE_TEST_DSN")
	if dsn == "" {
		t.Skip("HARUKI_IMAGECACHE_TEST_DSN not set")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
		t.Fatal("lifecycle integration requires a loopback PostgreSQL DSN")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "lifecycle_" + strings.ToLower(rand.Text())
	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := NewPGStoreWithOptions(u.String(), PGStoreOptions{RenderIndexDDL: true, MaxOpen: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		_ = admin.Close()
	})
	if err := store.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	return store
}
func lifecycleClient(t *testing.T, store *PGStore, objects storage.Store) *Client {
	t.Helper()
	c, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: objects, Index: store})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func lifecycleCount(t *testing.T, store *PGStore, table string) int {
	t.Helper()
	var n int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func lifecycleOldEntry(t *testing.T, store *PGStore, client *Client, data []byte) ImageEntry {
	t.Helper()
	if _, err := client.StoreAndGetURL(t.Context(), data, "pjsk"); err != nil {
		t.Fatal(err)
	}
	hash := contentName(data, "")
	if _, err := store.db.ExecContext(t.Context(), `UPDATE image_cache_entries SET last_referenced_at=NOW()-INTERVAL '31 days' WHERE hash=$1`, hash); err != nil {
		t.Fatal(err)
	}
	entry, found, err := store.Lookup(t.Context(), hash)
	if err != nil || !found {
		t.Fatalf("lookup %v %v", found, err)
	}
	return entry
}

func TestLifecyclePGWriterWaitsForGCThenUsesNewGeneration(t *testing.T) {
	store := lifecyclePG(t)
	memory := storagetest.NewMemory()
	client := lifecycleClient(t, store, memory)
	data := []byte("same content")
	old := lifecycleOldEntry(t, store, client, data)
	entered, release := make(chan struct{}), make(chan struct{})
	memory.FailDelete = func(storage.Key) error { close(entered); <-release; return nil }
	gc := NewGC(store, memory, GCConfig{Enabled: true, ObjectDeleteEnabled: true}, nil)
	gcDone := make(chan error, 1)
	go func() { gcDone <- gc.collectEntry(t.Context(), old, time.Now().Add(-30*24*time.Hour), &GCReport{}) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("GC did not reach Delete")
	}
	writerDone := make(chan error, 1)
	go func() { _, err := client.StoreAndGetURL(t.Context(), data, "pjsk"); writerDone <- err }()
	deadline := time.Now().Add(5 * time.Second)
	waiting := false
	for !waiting && time.Now().Before(deadline) {
		var n int
		if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		waiting = n > 0
		if !waiting {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if !waiting {
		close(release)
		t.Fatal("writer did not wait for shared PostgreSQL lock")
	}
	select {
	case err := <-writerDone:
		close(release)
		t.Fatalf("writer returned before Delete finished: %v", err)
	default:
	}
	close(release)
	if err := <-gcDone; err != nil {
		t.Fatal(err)
	}
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	fresh, ok, err := store.Lookup(t.Context(), old.Hash)
	if err != nil || !ok || fresh.CDNPath == old.CDNPath {
		t.Fatalf("generation reused: %+v %v", fresh, err)
	}
	if got, err := memory.Get(t.Context(), storage.Key(fresh.CDNPath)); err != nil || string(got) != string(data) {
		t.Fatalf("fresh object missing: %v", err)
	}
	if n := lifecycleCount(t, store, "image_cache_object_deletions"); n != 0 {
		t.Fatalf("completed intents=%d", n)
	}
}

type lateDeleteStore struct {
	storage.Store
	delayed       atomic.Bool
	release, done chan struct{}
}

func (s *lateDeleteStore) Delete(ctx context.Context, key storage.Key) error {
	if s.delayed.CompareAndSwap(false, true) {
		go func() { <-s.release; _ = s.Store.Delete(context.Background(), key); close(s.done) }()
		return context.DeadlineExceeded
	}
	return s.Store.Delete(ctx, key)
}
func TestLifecyclePGLateDeleteAndFailedUploadRecovery(t *testing.T) {
	store := lifecyclePG(t)
	memory := storagetest.NewMemory()
	client := lifecycleClient(t, store, memory)
	data := []byte("late delete")
	old := lifecycleOldEntry(t, store, client, data)
	late := &lateDeleteStore{Store: memory, release: make(chan struct{}), done: make(chan struct{})}
	gc := NewGC(store, late, GCConfig{Enabled: true, ObjectDeleteEnabled: true}, nil)
	report := GCReport{}
	if err := gc.collectEntry(t.Context(), old, time.Now().Add(-30*24*time.Hour), &report); err != nil || report.ObjectLeaks != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if _, err := client.StoreAndGetURL(t.Context(), data, "pjsk"); err != nil {
		t.Fatal(err)
	}
	fresh, _, err := store.Lookup(t.Context(), old.Hash)
	if err != nil {
		t.Fatal(err)
	}
	close(late.release)
	<-late.done
	if _, err := memory.Get(t.Context(), storage.Key(fresh.CDNPath)); err != nil {
		t.Fatalf("late Delete removed replacement: %v", err)
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE image_cache_object_deletions SET next_attempt_at=NOW()-INTERVAL '1 second'`); err != nil {
		t.Fatal(err)
	}
	if err := gc.retryPending(t.Context(), &report); err != nil {
		t.Fatal(err)
	}
	if n := lifecycleCount(t, store, "image_cache_object_deletions"); n != 0 {
		t.Fatalf("retry left %d intents", n)
	}
	// A failed upload leaves one durable intent even with a one-connection pool.
	store.db.SetMaxOpenConns(1)
	memory.FailPut = func(storage.Key) error { return errors.New("upload failed") }
	if _, err := client.StoreAndGetURL(t.Context(), []byte("failed"), "pjsk"); err == nil {
		t.Fatal("failed upload accepted")
	}
	if n := lifecycleCount(t, store, "image_cache_object_deletions"); n != 1 {
		t.Fatalf("failed upload intents=%d", n)
	}
	if err := gc.retryPending(t.Context(), &report); err != nil {
		t.Fatal(err)
	}
	if n := lifecycleCount(t, store, "image_cache_object_deletions"); n != 1 {
		t.Fatal("live lease consumed early")
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE image_cache_object_deletions SET next_attempt_at=NOW()-INTERVAL '1 second'`); err != nil {
		t.Fatal(err)
	}
	if err := gc.retryPending(t.Context(), &report); err != nil {
		t.Fatal(err)
	}
	if n := lifecycleCount(t, store, "image_cache_object_deletions"); n != 0 {
		t.Fatal("failed upload not recovered")
	}
}

func TestLifecyclePGReconcileAndExpiryCAS(t *testing.T) {
	store := lifecyclePG(t)
	memory := storagetest.NewMemory()
	client := lifecycleClient(t, store, memory)
	data := []byte("missing")
	entry := lifecycleOldEntry(t, store, client, data)
	if _, err := store.db.ExecContext(t.Context(), `INSERT INTO render_cache_index(request_key,content_hash,api_path,ttl_seconds,expires_at) VALUES('request',$1,'test',3600,NOW()-INTERVAL '1 second')`, entry.Hash); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now()
	if _, err := store.TouchRender(t.Context(), []string{"request"}); err != nil {
		t.Fatal(err)
	}
	if n, err := store.DeleteExpiredRender(t.Context(), []string{"request"}, cutoff); err != nil || n != 0 {
		t.Fatalf("CAS n=%d err=%v", n, err)
	}
	if err := memory.Delete(t.Context(), storage.Key(entry.CDNPath)); err != nil {
		t.Fatal(err)
	}
	if missing, err := store.ReconcileObject(t.Context(), memory, entry.Hash, false); err != nil || !missing {
		t.Fatalf("dry-run %v %v", missing, err)
	}
	if n := lifecycleCount(t, store, "render_cache_index"); n != 1 {
		t.Fatal("dry-run modified index")
	}
	if missing, err := store.ReconcileObject(t.Context(), memory, entry.Hash, true); err != nil || !missing {
		t.Fatalf("repair %v %v", missing, err)
	}
	if n := lifecycleCount(t, store, "render_cache_index"); n != 0 {
		t.Fatal("repair left stale render")
	}
	if _, err := client.StoreAndGetURL(t.Context(), data, "pjsk"); err != nil {
		t.Fatal(err)
	}
	fresh, _, err := store.Lookup(t.Context(), entry.Hash)
	if err != nil || fresh.CDNPath == entry.CDNPath {
		t.Fatalf("rebuild=%+v %v", fresh, err)
	}
}

func TestLifecyclePGLegacyWidenedSchema(t *testing.T) {
	store := lifecyclePG(t)
	if _, err := store.db.ExecContext(t.Context(), `DROP TABLE image_cache_object_deletions; ALTER TABLE image_cache_entries DROP COLUMN writer_node, DROP COLUMN written_at`); err != nil {
		t.Fatal(err)
	}
	if err := store.InspectSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	if store.lifecycle.Load() || store.metadata.Load() {
		t.Fatal("legacy schema marked lifecycle capable")
	}
	if err := store.InsertEntry(t.Context(), ImageEntry{Hash: "old", CDNPath: "old.png", StorageBackend: BackendGarage}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Lookup(t.Context(), "old"); err != nil || !ok {
		t.Fatalf("legacy lookup %v %v", ok, err)
	}
	if _, err := store.db.ExecContext(t.Context(), `INSERT INTO render_cache_index(request_key,content_hash,api_path) VALUES('old','old','test')`); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.LookupRender(t.Context(), "old"); err != nil || !ok {
		t.Fatalf("legacy render lookup %v %v", ok, err)
	}
	if _, err := store.ReconcileObject(t.Context(), storagetest.NewMemory(), "old", true); err == nil {
		t.Fatal("legacy repair bypassed protocol gate")
	}
}

func TestLifecyclePGExpiredIntentCannotPublish(t *testing.T) {
	store := lifecyclePG(t)
	memory := storagetest.NewMemory()
	client := lifecycleClient(t, store, memory)
	data := []byte("expired lease")
	hash := contentName(data, "")
	key := "pjsk/expired-generation.png"
	if err := store.registerUpload(t.Context(), hash, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE image_cache_object_deletions SET next_attempt_at=NOW()-INTERVAL '1 second'`); err != nil {
		t.Fatal(err)
	}
	err := store.WithContentLock(t.Context(), hash, func(ctx context.Context, index *PGStore) error {
		_, err := client.storeHashedLocked(ctx, data, hash, "pjsk", key, index)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "intent unavailable or expired") {
		t.Fatalf("expired lease allowed upload: %v", err)
	}
	if len(memory.Calls()) != 0 || lifecycleCount(t, store, "image_cache_entries") != 0 {
		t.Fatal("expired lease published content")
	}
	gc := NewGC(store, memory, GCConfig{Enabled: true, ObjectDeleteEnabled: true}, nil)
	if err := gc.retryPending(t.Context(), &GCReport{}); err != nil {
		t.Fatal(err)
	}
	err = store.WithContentLock(t.Context(), hash, func(ctx context.Context, index *PGStore) error {
		_, err := client.storeHashedLocked(ctx, data, hash, "pjsk", key, index)
		return err
	})
	if err == nil {
		t.Fatal("collected lease allowed upload")
	}
}

func TestLifecyclePGRetryRechecksDueAndBoundsBatch(t *testing.T) {
	store := lifecyclePG(t)
	memory := storagetest.NewMemory()
	gc := NewGC(store, memory, GCConfig{Enabled: true, ObjectDeleteEnabled: true, Batch: 2}, nil)
	for _, key := range []string{"a.png", "b.png", "c.png"} {
		if err := store.registerUpload(t.Context(), key, key); err != nil {
			t.Fatal(err)
		}
	}
	// Even a stale scanner's candidate must not consume a renewed/not-yet-due lease.
	if err := gc.deletePending(t.Context(), pendingObjectDelete{Hash: "a.png", Key: "a.png"}, &GCReport{}, true); err != nil {
		t.Fatal(err)
	}
	if len(memory.Calls()) != 0 || lifecycleCount(t, store, "image_cache_object_deletions") != 3 {
		t.Fatal("future lease deleted")
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE image_cache_object_deletions SET next_attempt_at=NOW()-INTERVAL '1 second'`); err != nil {
		t.Fatal(err)
	}
	report := GCReport{}
	if err := gc.retryPending(t.Context(), &report); err != nil {
		t.Fatal(err)
	}
	if report.RetriedObjectDeletes != 2 || lifecycleCount(t, store, "image_cache_object_deletions") != 1 {
		t.Fatalf("unbounded retry %+v", report)
	}
}

type receiptStore struct {
	storage.Store
	node    string
	written time.Time
}

func (s *receiptStore) Put(ctx context.Context, key storage.Key, data []byte, opts storage.PutOptions) error {
	if err := s.Store.Put(ctx, key, data, opts); err != nil {
		return err
	}
	storage.ReportWriteReceipt(ctx, storage.WriteReceipt{WriterNode: s.node, WrittenAt: s.written})
	return nil
}
func TestLifecyclePGWriteReceiptSurvivesDifferentCloudClient(t *testing.T) {
	store := lifecyclePG(t)
	memory := storagetest.NewMemory()
	hosts, err := urlhost.New(map[string]string{"cn01": "https://one.example", "cn09": "https://nine.example"}, urlhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	written := time.Now().UTC().Truncate(time.Microsecond)
	objects := &receiptStore{Store: memory, node: "cn09", written: written}
	client, err := NewClient(ClientConfig{Hosts: hosts, Objects: objects, Index: store})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("fresh writer")
	first, err := client.StoreAndGetURL(t.Context(), data, "pjsk")
	if err != nil || !strings.HasPrefix(first, "https://nine.example/") {
		t.Fatalf("cold node hint %q %v", first, err)
	}
	entry, ok, err := store.Lookup(t.Context(), contentName(data, ""))
	if err != nil || !ok || entry.WriterNode != "cn09" || !entry.WrittenAt.Equal(written) {
		t.Fatalf("persisted receipt %+v %v", entry, err)
	}
	another, err := NewClient(ClientConfig{Hosts: hosts, Objects: memory, Index: store})
	if err != nil {
		t.Fatal(err)
	}
	second, err := another.StoreAndGetURL(t.Context(), data, "pjsk")
	if err != nil || second != first {
		t.Fatalf("PG hit lost node hint %q %v", second, err)
	}
	if _, err := store.db.ExecContext(t.Context(), `INSERT INTO render_cache_index(request_key,content_hash,api_path) VALUES('fresh',$1,'test')`, entry.Hash); err != nil {
		t.Fatal(err)
	}
	render, found, err := store.LookupRender(t.Context(), "fresh")
	if err != nil || !found || render.Entry.FreshWriterNode(time.Now()) != "cn09" {
		t.Fatalf("render writer hint %+v %v", render, err)
	}
}
