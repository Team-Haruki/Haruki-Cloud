package imagecache

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/storage/storagetest"

	"github.com/DATA-DOG/go-sqlmock"
)

// A Cloud dedup hit re-emits a garage URL; without a last_referenced_at bump
// GC would collect the row 30 days after insert even while it is in use.
func TestLookupIndexedGarageHitBumpsLastReferencedAtRateLimited(t *testing.T) {
	data := []byte("reused garage image")
	hash := contentName(data, "")
	index, mock := widenedMockStore(t)
	client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: storagetest.NewMemory(), Index: index})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	expectHit := func() {
		mock.ExpectQuery(regexp.QuoteMeta(lookupWidenedSQL)).WithArgs(hash).WillReturnRows(
			sqlmock.NewRows(widenedLookupColumns).AddRow("pjsk/"+hash+".png", "", int64(3), BackendGarage, "image/png", nil))
	}
	store := func() {
		t.Helper()
		if _, err := client.StoreAndGetURL(context.Background(), data, "pjsk"); err != nil {
			t.Fatal(err)
		}
	}

	expectHit()
	mock.ExpectExec(regexp.QuoteMeta(touchEntrySQL)).WithArgs(hash).WillReturnResult(sqlmock.NewResult(0, 1))
	store()
	// Inside the interval: lookup only.
	now = now.Add(entryTouchInterval / 2)
	expectHit()
	store()
	// A failed touch is logged and does not advance the memo.
	now = now.Add(entryTouchInterval)
	expectHit()
	mock.ExpectExec(regexp.QuoteMeta(touchEntrySQL)).WithArgs(hash).WillReturnError(errors.New("touch failed"))
	store()
	expectHit()
	mock.ExpectExec(regexp.QuoteMeta(touchEntrySQL)).WithArgs(hash).WillReturnResult(sqlmock.NewResult(0, 1))
	store()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTouchEntryMemoIsBounded(t *testing.T) {
	index, mock := widenedMockStore(t)
	client, err := NewClient(ClientConfig{Hosts: testHosts(t), Objects: storagetest.NewMemory(), Index: index})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	mock.MatchExpectationsInOrder(false)
	for i := 0; i < entryTouchMemoCap; i++ {
		client.lastTouch[strings.Repeat("x", i+1)] = now.Add(-2 * entryTouchInterval)
	}
	mock.ExpectExec(regexp.QuoteMeta(touchEntrySQL)).WithArgs("stale").WillReturnResult(sqlmock.NewResult(0, 1))
	client.touchEntry(context.Background(), "stale")
	if len(client.lastTouch) != 1 {
		t.Fatalf("stale memo entries not evicted: %d", len(client.lastTouch))
	}

	for i := 0; i < entryTouchMemoCap; i++ {
		client.lastTouch[strings.Repeat("y", i+1)] = now
	}
	mock.ExpectExec(regexp.QuoteMeta(touchEntrySQL)).WithArgs("fresh").WillReturnResult(sqlmock.NewResult(0, 1))
	client.touchEntry(context.Background(), "fresh")
	if len(client.lastTouch) != 1 {
		t.Fatalf("full fresh memo not reset: %d", len(client.lastTouch))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGStoreTouchEntry(t *testing.T) {
	ctx := context.Background()
	var nilStore *PGStore
	if err := nilStore.TouchEntry(ctx, "h"); err != nil {
		t.Fatalf("nil store = %v", err)
	}
	legacy, legacyMock := newMockPGStore(t)
	if err := legacy.TouchEntry(ctx, "h"); err != nil {
		t.Fatalf("un-widened store = %v", err)
	}
	if err := legacyMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	store, mock := widenedMockStore(t)
	if err := store.TouchEntry(ctx, ""); err != nil {
		t.Fatalf("empty hash = %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta(touchEntrySQL)).WithArgs("h").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.TouchEntry(ctx, "h"); err != nil {
		t.Fatal(err)
	}
	execErr := errors.New("exec failed")
	mock.ExpectExec(regexp.QuoteMeta(touchEntrySQL)).WillReturnError(execErr)
	if err := store.TouchEntry(ctx, "h"); !errors.Is(err, execErr) {
		t.Fatalf("exec error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// The widened upsert keeps a garage row's location but always refreshes
// last_referenced_at, like Drawing's UPSERT_CONTENT.
func TestInsertWidenedSQLAlwaysBumpsLastReferencedAt(t *testing.T) {
	conflict := insertWidenedSQL[strings.Index(insertWidenedSQL, "ON CONFLICT"):]
	if strings.Contains(conflict, "WHERE") {
		t.Fatalf("conflict update must not be skipped for garage rows: %s", conflict)
	}
	if !strings.Contains(conflict, "last_referenced_at = NOW()") {
		t.Fatalf("conflict update must bump last_referenced_at: %s", conflict)
	}
	for _, column := range []string{"cdn_path", "file_path", "size_bytes", "storage_backend", "media_type"} {
		want := "WHEN image_cache_entries.storage_backend = 'garage' THEN image_cache_entries." + column + " ELSE EXCLUDED." + column
		if !strings.Contains(conflict, want) {
			t.Fatalf("garage guard missing for %s", column)
		}
	}
}
