package imagecache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"

	"github.com/DATA-DOG/go-sqlmock"
)

var (
	adoptHash = strings.Repeat("a", 64)
	adoptPath = "pjsk/" + adoptHash + "-ABCDEFGHIJKLMNOPQRSTUVWXYZ.jpg"
	adoptNow  = time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
)

func adoptObject() AdoptedObject {
	return AdoptedObject{Hash: adoptHash, Group: "pjsk", CDNPath: adoptPath, MediaType: "image/jpeg", SizeBytes: 270000, WriterNode: "gw-1"}
}

func TestValidateAdoptedObject(t *testing.T) {
	if err := ValidateAdoptedObject(adoptObject()); err != nil {
		t.Fatalf("valid object rejected: %v", err)
	}
	for name, mutate := range map[string]func(*AdoptedObject){
		"other hash":       func(o *AdoptedObject) { o.Hash = strings.Repeat("b", 64) },
		"upper hash":       func(o *AdoptedObject) { o.Hash = strings.ToUpper(o.Hash) },
		"other group":      func(o *AdoptedObject) { o.Group = "chunithm" },
		"render key shape": func(o *AdoptedObject) { o.CDNPath = "pjsk/api/pjsk/sk/" + adoptHash + "-g.jpg" },
		"no generation":    func(o *AdoptedObject) { o.CDNPath = "pjsk/" + adoptHash + ".jpg" },
		"webp":             func(o *AdoptedObject) { o.CDNPath = "pjsk/" + adoptHash + "-G.webp" },
		"dot segment":      func(o *AdoptedObject) { o.CDNPath = "pjsk/../" + adoptHash + "-G.jpg"; o.Group = "pjsk/.." },
		"media mismatch":   func(o *AdoptedObject) { o.MediaType = "image/png" },
		"empty":            func(o *AdoptedObject) { o.SizeBytes = 0 },
	} {
		obj := adoptObject()
		mutate(&obj)
		if err := ValidateAdoptedObject(obj); err == nil {
			t.Errorf("%s: accepted %+v", name, obj)
		}
	}
}

func TestAdoptObjectWithoutIndexReturnsItsOwnLocation(t *testing.T) {
	var store *PGStore
	got, err := store.AdoptObject(t.Context(), adoptObject(), adoptNow)
	if err != nil || got != (AdoptedLocation{CDNPath: adoptPath, WriterNode: "gw-1"}) {
		t.Fatalf("AdoptObject() = %+v, %v", got, err)
	}
	if _, err := store.AdoptObject(t.Context(), AdoptedObject{Hash: adoptHash}, adoptNow); err == nil {
		t.Fatal("invalid object accepted without an index")
	}
}

func lockedMockPGStore(t *testing.T) (*PGStore, sqlmock.Sqlmock) {
	t.Helper()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	store.widened.Store(true)
	store.lifecycle.Store(true)
	return store, mock
}

func TestAdoptObjectInsertsTheRowUnderTheContentLock(t *testing.T) {
	store, mock := lockedMockPGStore(t)
	expectContentLock(mock, adoptHash)
	mock.ExpectQuery(lookupWidenedSQL).WithArgs(adoptHash).WillReturnRows(sqlmock.NewRows(widenedLookupColumns))
	mock.ExpectExec(insertWidenedWithWriterSQL).
		WithArgs(adoptHash, "pjsk", adoptPath, nil, int64(270000), BackendGarage, "image/jpeg", "gw-1", adoptNow).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ctx, trace := commandtrace.WithNewTrace(t.Context())
	got, err := store.AdoptObject(ctx, adoptObject(), adoptNow)
	if err != nil || got != (AdoptedLocation{CDNPath: adoptPath, WriterNode: "gw-1"}) {
		t.Fatalf("AdoptObject() = %+v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	ops := map[string]int{}
	for _, op := range trace.Snapshot().Operations {
		ops[op.Name] += op.Count
	}
	for _, name := range []string{"image.ref_index", "image.content_lock", "image.lookup", "image.index"} {
		if ops[name] != 1 {
			t.Fatalf("trace ops = %v, want one %s", ops, name)
		}
	}
}

func TestAdoptObjectKeepsAnExistingRowAndQueuesTheDuplicate(t *testing.T) {
	store, mock := lockedMockPGStore(t)
	existing := "pjsk/" + adoptHash + "-ZYXWVUTSRQPONMLKJIHGFEDCBA.jpg"
	expectContentLock(mock, adoptHash)
	mock.ExpectQuery(lookupWidenedSQL).WithArgs(adoptHash).WillReturnRows(sqlmock.NewRows(widenedLookupColumns).
		AddRow(existing, "", 1, BackendGarage, "image/jpeg", nil, "gw-2", adoptNow.Add(-time.Minute)))
	mock.ExpectExec(touchEntrySQL).WithArgs(adoptHash).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(registerUploadSQL).WithArgs(adoptHash, adoptPath).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	got, err := store.AdoptObject(t.Context(), adoptObject(), adoptNow)
	if err != nil || got != (AdoptedLocation{CDNPath: existing, WriterNode: "gw-2", Duplicate: true}) {
		t.Fatalf("AdoptObject() = %+v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptObjectRecordingTheSamePathAgainIsAnUpsert(t *testing.T) {
	store, mock := lockedMockPGStore(t)
	expectContentLock(mock, adoptHash)
	mock.ExpectQuery(lookupWidenedSQL).WithArgs(adoptHash).WillReturnRows(sqlmock.NewRows(widenedLookupColumns).
		AddRow(adoptPath, "", 270000, BackendGarage, "image/jpeg", nil, "gw-1", adoptNow))
	mock.ExpectExec(insertWidenedWithWriterSQL).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if got, err := store.AdoptObject(t.Context(), adoptObject(), adoptNow); err != nil || got.Duplicate || got.CDNPath != adoptPath {
		t.Fatalf("AdoptObject() = %+v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptObjectFailureRollsBackAndReturnsItsOwnLocation(t *testing.T) {
	store, mock := lockedMockPGStore(t)
	expectContentLock(mock, adoptHash)
	mock.ExpectQuery(lookupWidenedSQL).WithArgs(adoptHash).WillReturnError(errors.New("connection reset"))
	mock.ExpectRollback()
	got, err := store.AdoptObject(t.Context(), adoptObject(), adoptNow)
	if err == nil || got.CDNPath != adoptPath {
		t.Fatalf("AdoptObject() = %+v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptObjectStaleWriterIsNotPreferred(t *testing.T) {
	var store *PGStore
	obj := adoptObject()
	got, err := store.AdoptObject(t.Context(), obj, adoptNow)
	if err != nil || got.WriterNode != "gw-1" {
		t.Fatalf("nil store keeps the writer: %+v %v", got, err)
	}
	entry := ImageEntry{WriterNode: "gw-1", WrittenAt: adoptNow}
	if entry.FreshWriterNode(adoptNow.Add(3*time.Minute)) != "" {
		t.Fatal("writer preference outlived the replication window")
	}
}

// The adopted row is a first-class image cache row: GC retires it once its
// retention passes, and a queued duplicate is deleted while the recorded
// object survives.
func TestLifecyclePGAdoptedObjectsFollowGC(t *testing.T) {
	store := lifecyclePG(t)
	memory := storagetest.NewMemory()
	ctx := context.Background()
	obj := adoptObject()
	duplicate := obj
	duplicate.CDNPath = "pjsk/" + adoptHash + "-ZYXWVUTSRQPONMLKJIHGFEDCBA.jpg"
	for _, o := range []AdoptedObject{obj, duplicate} {
		if err := memory.Put(ctx, storage.Key(o.CDNPath), []byte("jpeg"), storage.PutOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := store.AdoptObject(ctx, obj, time.Now()); err != nil || got.Duplicate || got.WriterNode != "gw-1" {
		t.Fatalf("first adopt = %+v, %v", got, err)
	}
	got, err := store.AdoptObject(ctx, duplicate, time.Now())
	if err != nil || !got.Duplicate || got.CDNPath != obj.CDNPath {
		t.Fatalf("duplicate adopt = %+v, %v", got, err)
	}
	entry, ok, err := store.Lookup(ctx, adoptHash)
	if err != nil || !ok || entry.CDNPath != obj.CDNPath || entry.WriterNode != "gw-1" || entry.StorageBackend != BackendGarage {
		t.Fatalf("row = %+v, %v, %v", entry, ok, err)
	}
	// Make the duplicate's intent due, then run object GC.
	if _, err := store.db.ExecContext(ctx, `UPDATE image_cache_object_deletions SET next_attempt_at = NOW() - INTERVAL '1 second'`); err != nil {
		t.Fatal(err)
	}
	gc := NewGC(store, memory, GCConfig{Enabled: true, ObjectDeleteEnabled: true}, nil)
	if _, err := gc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Stat(ctx, storage.Key(duplicate.CDNPath)); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("duplicate survived GC: %v", err)
	}
	if _, err := memory.Stat(ctx, storage.Key(obj.CDNPath)); err != nil {
		t.Fatalf("recorded object deleted: %v", err)
	}
	// Past retention, the adopted row is retired and its object deleted like Cloud's own.
	if _, err := store.db.ExecContext(ctx, `UPDATE image_cache_entries SET last_referenced_at = NOW() - INTERVAL '60 days'`); err != nil {
		t.Fatal(err)
	}
	if _, err := gc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE image_cache_object_deletions SET next_attempt_at = NOW() - INTERVAL '1 second'`); err != nil {
		t.Fatal(err)
	}
	if _, err := gc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Lookup(ctx, adoptHash); ok {
		t.Fatal("adopted row survived retention")
	}
	if _, err := memory.Stat(ctx, storage.Key(obj.CDNPath)); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("retired adopted object survived GC: %v", err)
	}
}
