package imagecache

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// canonicalRenderIndexDDL is the addendum A5 listing, copied verbatim from the
// plan (and mirrored verbatim in Drawing's plan section 9.3).
const canonicalRenderIndexDDL = `ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS storage_backend TEXT;
ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS media_type TEXT;
ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL;
ALTER TABLE image_cache_entries ADD COLUMN IF NOT EXISTS last_referenced_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE image_cache_entries ALTER COLUMN file_path DROP NOT NULL;
UPDATE image_cache_entries SET storage_backend = 'legacy_disk' WHERE storage_backend IS NULL;
CREATE INDEX IF NOT EXISTS idx_ice_group_created ON image_cache_entries (group_name, created_at);
CREATE INDEX IF NOT EXISTS idx_ice_expires ON image_cache_entries (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ice_last_referenced ON image_cache_entries (last_referenced_at);
CREATE TABLE IF NOT EXISTS render_cache_index (
    request_key  TEXT PRIMARY KEY,
    content_hash TEXT NOT NULL REFERENCES image_cache_entries(hash),
    api_path     TEXT NOT NULL,
    user_id      TEXT NOT NULL DEFAULT 'public',
    group_name   TEXT NOT NULL DEFAULT 'pjsk',
    key_version  INT NOT NULL DEFAULT 3,
    ttl_seconds  BIGINT NOT NULL DEFAULT 0,
    expires_at   TIMESTAMPTZ NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_rci_expires ON render_cache_index (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_rci_api_path_user ON render_cache_index (api_path, user_id);
CREATE INDEX IF NOT EXISTS idx_rci_content_hash ON render_cache_index (content_hash);`

func TestRenderIndexDDLCanonicalText(t *testing.T) {
	if got := strings.Join(renderIndexDDL, ";\n") + ";"; got != canonicalRenderIndexDDL {
		t.Fatalf("renderIndexDDL drifted from the canonical text:\n%s", got)
	}
	if len(renderIndexDDL) != 13 {
		t.Fatalf("statement count = %d, want 13", len(renderIndexDDL))
	}
	all := strings.ToUpper(strings.Join(renderIndexDDL, "\n"))
	for _, forbidden := range []string{"SET DEFAULT", "ON DELETE CASCADE", "ON DELETE", "LAST_USED_AT)", "(STORAGE_BACKEND)"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("DDL contains forbidden %q", forbidden)
		}
	}
	if got := strings.Count(all, "CREATE INDEX IF NOT EXISTS"); got != 6 {
		t.Fatalf("index count = %d, want 6", got)
	}
	for _, stmt := range renderIndexDDL {
		upper := strings.ToUpper(stmt)
		idempotent := strings.Contains(upper, "IF NOT EXISTS") || strings.Contains(upper, "DROP NOT NULL") ||
			strings.Contains(upper, "WHERE STORAGE_BACKEND IS NULL")
		if !idempotent {
			t.Fatalf("statement is not re-runnable: %s", stmt)
		}
	}
	copied := RenderIndexDDL()
	copied[0] = "mutated"
	if renderIndexDDL[0] == "mutated" {
		t.Fatal("RenderIndexDDL returned the backing slice")
	}
}

func newEqualMockPGStore(t *testing.T, opts PGStoreOptions) (*PGStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	return NewPGStoreFromDB(db, opts), mock
}

func TestPGStoreInitRunsRenderIndexDDLInOrder(t *testing.T) {
	ctx := context.Background()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{RenderIndexDDL: true})
	mock.ExpectExec(initSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(migrateSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	for _, stmt := range renderIndexDDL {
		mock.ExpectExec(stmt).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery(widenedProbeSQL).WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !store.Widened() {
		t.Fatal("widened schema not detected after DDL")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGStoreInitSkipsRenderIndexDDLWhenDisabled(t *testing.T) {
	ctx := context.Background()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	mock.ExpectExec(initSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(migrateSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	// Any DDL exec here would be unexpected and fail Init.
	mock.ExpectQuery(widenedProbeSQL).WillReturnRows(sqlmock.NewRows([]string{"?column?"}))
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGStoreInitRenderIndexDDLFailure(t *testing.T) {
	ctx := context.Background()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{RenderIndexDDL: true})
	wantErr := errors.New("lock timeout")
	mock.ExpectExec(initSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(migrateSQL).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(renderIndexDDL[0]).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(renderIndexDDL[1]).WillReturnError(wantErr)
	err := store.Init(ctx)
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "step 2") {
		t.Fatalf("Init() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

var lookupRenderColumns = []string{
	"content_hash", "api_path", "user_id", "group_name", "key_version", "ttl_seconds", "expires_at", "last_used_at",
	"group_name", "cdn_path", "storage_backend", "media_type", "size_bytes", "file_path", "expires_at", "last_referenced_at",
}

func TestPGStoreLookupRender(t *testing.T) {
	ctx := context.Background()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	expires := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	used := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	referenced := time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC)

	mock.ExpectQuery(lookupRenderSQL).WithArgs("key-1").WillReturnRows(sqlmock.NewRows(lookupRenderColumns).AddRow(
		"h1", "api/event/list", "public", "pjsk", 5, int64(3600), expires, used,
		"pjsk", "pjsk/api/event/list/h1.png", BackendGarage, "image/png", int64(42), "", expires, referenced,
	))
	got, ok, err := store.LookupRender(ctx, "key-1")
	want := RenderIndexEntry{
		RequestKey: "key-1", ContentHash: "h1", APIPath: "api/event/list", UserID: "public", GroupName: "pjsk",
		KeyVersion: 5, TTLSeconds: 3600, ExpiresAt: expires, LastUsedAt: used,
		Entry: ImageEntry{
			Hash: "h1", GroupName: "pjsk", CDNPath: "pjsk/api/event/list/h1.png", StorageBackend: BackendGarage,
			MediaType: "image/png", SizeBytes: 42, ExpiresAt: expires, LastReferencedAt: referenced,
		},
	}
	if !ok || err != nil || got != want {
		t.Fatalf("LookupRender() = (%+v, %v, %v)", got, ok, err)
	}

	// Infinite TTL (NULL expires_at) and a NULL file_path/backend/media_type on a legacy row.
	mock.ExpectQuery(lookupRenderSQL).WithArgs("key-2").WillReturnRows(sqlmock.NewRows(lookupRenderColumns).AddRow(
		"h2", "api/card", "123", "pjsk", 3, int64(0), nil, used,
		"pjsk", "pjsk/h2.png", nil, nil, int64(1), "/cache/pjsk/h2.png", nil, referenced,
	))
	got, ok, err = store.LookupRender(ctx, "key-2")
	if !ok || err != nil || !got.ExpiresAt.IsZero() || !got.Entry.ExpiresAt.IsZero() ||
		got.Entry.StorageBackend != BackendLegacyDisk || got.Entry.MediaType != "" || got.Entry.FilePath != "/cache/pjsk/h2.png" {
		t.Fatalf("LookupRender(NULLs) = (%+v, %v, %v)", got, ok, err)
	}

	mock.ExpectQuery(lookupRenderSQL).WithArgs("key-3").WillReturnRows(sqlmock.NewRows(lookupRenderColumns).AddRow(
		"h3", "api/x", "public", "pjsk", 3, int64(0), nil, used,
		"pjsk", "pjsk/api/x/h3.png", nil, nil, int64(1), "", nil, referenced,
	))
	if got, ok, err = store.LookupRender(ctx, "key-3"); !ok || err != nil || got.Entry.StorageBackend != BackendGarage {
		t.Fatalf("LookupRender(NULL file_path) = (%+v, %v, %v)", got, ok, err)
	}

	mock.ExpectQuery(lookupRenderSQL).WithArgs("miss").WillReturnError(sql.ErrNoRows)
	if got, ok, err := store.LookupRender(ctx, "miss"); ok || err != nil || got != (RenderIndexEntry{}) {
		t.Fatalf("LookupRender(miss) = (%+v, %v, %v)", got, ok, err)
	}

	driverErr := errors.New("relation render_cache_index does not exist")
	mock.ExpectQuery(lookupRenderSQL).WithArgs("broken").WillReturnError(driverErr)
	if got, ok, err := store.LookupRender(ctx, "broken"); ok || !errors.Is(err, driverErr) || got != (RenderIndexEntry{}) {
		t.Fatalf("LookupRender(error) = (%+v, %v, %v)", got, ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRenderIndexQueryTexts(t *testing.T) {
	join := regexp.MustCompile(`JOIN image_cache_entries e ON e\.hash = r\.content_hash\s+WHERE r\.request_key = \$1`)
	if !join.MatchString(lookupRenderSQL) {
		t.Fatalf("lookupRenderSQL join = %s", lookupRenderSQL)
	}
	if !strings.Contains(touchRenderSQL, "THEN now() + (r.ttl_seconds || ' seconds')::interval") ||
		!strings.Contains(touchRenderSQL, "ELSE NULL END") || !strings.Contains(touchRenderSQL, "r.request_key = ANY($1)") {
		t.Fatalf("touchRenderSQL = %s", touchRenderSQL)
	}
	if !strings.Contains(expiredRenderKeysSQL, "expires_at IS NOT NULL AND expires_at < $1") ||
		!strings.Contains(expiredRenderKeysSQL, "ORDER BY expires_at LIMIT $2") {
		t.Fatalf("expiredRenderKeysSQL = %s", expiredRenderKeysSQL)
	}
	orphan := orphanGarageEntriesSQL
	for _, want := range []string{
		"e.storage_backend = 'garage'",
		"e.last_referenced_at < $1",
		"NOT EXISTS (SELECT 1 FROM render_cache_index r WHERE r.content_hash = e.hash)",
		"ORDER BY e.last_referenced_at LIMIT $2",
	} {
		if !strings.Contains(orphan, want) {
			t.Fatalf("orphanGarageEntriesSQL missing %q", want)
		}
	}
	// entries.expires_at is selected but never part of the predicate (addendum A6).
	where := orphan[strings.Index(orphan, "WHERE"):]
	if strings.Contains(where, "expires_at") || strings.Contains(where, "legacy_disk") {
		t.Fatalf("orphan predicate must key only on last_referenced_at and references: %s", where)
	}
	for _, text := range []string{lookupRenderSQL, touchRenderSQL, deleteRenderSQL, expiredRenderKeysSQL, orphanGarageEntriesSQL, deleteEntriesSQL} {
		if strings.Contains(strings.ToUpper(text), "INSERT INTO RENDER_CACHE_INDEX") {
			t.Fatalf("Cloud must not insert into render_cache_index: %s", text)
		}
	}
}

func TestPGStoreRenderKeyStatements(t *testing.T) {
	ctx := context.Background()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	cases := []struct {
		name  string
		query string
		call  func([]string) (int64, error)
	}{
		{"touch", touchRenderSQL, func(k []string) (int64, error) { return store.TouchRender(ctx, k) }},
		{"delete render", deleteRenderSQL, func(k []string) (int64, error) { return store.DeleteRender(ctx, k) }},
		{"delete entries", deleteEntriesSQL, func(k []string) (int64, error) { return store.DeleteEntries(ctx, k) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if n, err := tc.call(nil); n != 0 || err != nil {
				t.Fatalf("empty keys = (%d, %v)", n, err)
			}
			mock.ExpectExec(tc.query).WithArgs("{\"a\",\"b\"}").WillReturnResult(sqlmock.NewResult(0, 2))
			if n, err := tc.call([]string{"a", "b"}); n != 2 || err != nil {
				t.Fatalf("batch = (%d, %v)", n, err)
			}
			execErr := errors.New("exec failed")
			mock.ExpectExec(tc.query).WillReturnError(execErr)
			if n, err := tc.call([]string{"a"}); n != 0 || !errors.Is(err, execErr) {
				t.Fatalf("exec error = (%d, %v)", n, err)
			}
			rowsErr := errors.New("rows affected unsupported")
			mock.ExpectExec(tc.query).WillReturnResult(sqlmock.NewErrorResult(rowsErr))
			if n, err := tc.call([]string{"a"}); n != 0 || !errors.Is(err, rowsErr) {
				t.Fatalf("rows error = (%d, %v)", n, err)
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGStoreExpiredRenderKeys(t *testing.T) {
	ctx := context.Background()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	if keys, err := store.ExpiredRenderKeys(ctx, now, 0); keys != nil || err != nil {
		t.Fatalf("limit 0 = (%v, %v)", keys, err)
	}
	mock.ExpectQuery(expiredRenderKeysSQL).WithArgs(now, 2).
		WillReturnRows(sqlmock.NewRows([]string{"request_key"}).AddRow("old").AddRow("older"))
	if keys, err := store.ExpiredRenderKeys(ctx, now, 2); err != nil || strings.Join(keys, ",") != "old,older" {
		t.Fatalf("ExpiredRenderKeys() = (%v, %v)", keys, err)
	}

	queryErr := errors.New("query failed")
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnError(queryErr)
	if _, err := store.ExpiredRenderKeys(ctx, now, 2); !errors.Is(err, queryErr) {
		t.Fatalf("query error = %v", err)
	}

	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(sqlmock.NewRows([]string{"request_key"}).AddRow(nil))
	if _, err := store.ExpiredRenderKeys(ctx, now, 2); err == nil || !strings.Contains(err.Error(), "scan") {
		t.Fatalf("scan error = %v", err)
	}

	iterErr := errors.New("iteration failed")
	mock.ExpectQuery(expiredRenderKeysSQL).WillReturnRows(
		sqlmock.NewRows([]string{"request_key"}).AddRow("a").RowError(0, iterErr))
	if _, err := store.ExpiredRenderKeys(ctx, now, 2); !errors.Is(err, iterErr) {
		t.Fatalf("rows error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

var orphanColumns = []string{
	"hash", "group_name", "cdn_path", "file_path", "storage_backend", "media_type", "size_bytes", "expires_at", "last_referenced_at",
}

func TestPGStoreOrphanGarageEntries(t *testing.T) {
	ctx := context.Background()
	store, mock := newEqualMockPGStore(t, PGStoreOptions{})
	cutoff := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	referenced := cutoff.Add(-time.Hour)
	expires := cutoff.Add(24 * time.Hour)

	if entries, err := store.OrphanGarageEntries(ctx, cutoff, -1); entries != nil || err != nil {
		t.Fatalf("limit -1 = (%v, %v)", entries, err)
	}
	mock.ExpectQuery(orphanGarageEntriesSQL).WithArgs(cutoff, 10).WillReturnRows(sqlmock.NewRows(orphanColumns).
		AddRow("h1", "pjsk", "pjsk/api/a/h1.png", "", BackendGarage, "image/png", int64(5), expires, referenced).
		AddRow("h2", "pjsk", "pjsk/api/a/h2.png", "", BackendGarage, nil, int64(6), nil, referenced))
	entries, err := store.OrphanGarageEntries(ctx, cutoff, 10)
	if err != nil || len(entries) != 2 {
		t.Fatalf("OrphanGarageEntries() = (%+v, %v)", entries, err)
	}
	want := ImageEntry{Hash: "h1", GroupName: "pjsk", CDNPath: "pjsk/api/a/h1.png", StorageBackend: BackendGarage,
		MediaType: "image/png", SizeBytes: 5, ExpiresAt: expires, LastReferencedAt: referenced}
	if entries[0] != want || entries[1].MediaType != "" || !entries[1].ExpiresAt.IsZero() {
		t.Fatalf("entries = %+v", entries)
	}

	queryErr := errors.New("query failed")
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnError(queryErr)
	if _, err := store.OrphanGarageEntries(ctx, cutoff, 10); !errors.Is(err, queryErr) {
		t.Fatalf("query error = %v", err)
	}

	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(sqlmock.NewRows(orphanColumns).
		AddRow("h1", "pjsk", "p", "", BackendGarage, nil, "not-a-number", nil, referenced))
	if _, err := store.OrphanGarageEntries(ctx, cutoff, 10); err == nil || !strings.Contains(err.Error(), "scan") {
		t.Fatalf("scan error = %v", err)
	}

	iterErr := errors.New("iteration failed")
	mock.ExpectQuery(orphanGarageEntriesSQL).WillReturnRows(sqlmock.NewRows(orphanColumns).
		AddRow("h1", "pjsk", "p", "", BackendGarage, nil, int64(1), nil, referenced).RowError(0, iterErr))
	if _, err := store.OrphanGarageEntries(ctx, cutoff, 10); !errors.Is(err, iterErr) {
		t.Fatalf("rows error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGStoreRenderIndexNilReceiver(t *testing.T) {
	var store *PGStore
	ctx := context.Background()
	if got, ok, err := store.LookupRender(ctx, "k"); ok || err != nil || got != (RenderIndexEntry{}) {
		t.Fatalf("LookupRender() = (%+v, %v, %v)", got, ok, err)
	}
	for _, call := range []func() (int64, error){
		func() (int64, error) { return store.TouchRender(ctx, []string{"k"}) },
		func() (int64, error) { return store.DeleteRender(ctx, []string{"k"}) },
		func() (int64, error) { return store.DeleteEntries(ctx, []string{"k"}) },
	} {
		if n, err := call(); n != 0 || err != nil {
			t.Fatalf("nil store exec = (%d, %v)", n, err)
		}
	}
	if keys, err := store.ExpiredRenderKeys(ctx, time.Now(), 1); keys != nil || err != nil {
		t.Fatalf("ExpiredRenderKeys() = (%v, %v)", keys, err)
	}
	if entries, err := store.OrphanGarageEntries(ctx, time.Now(), 1); entries != nil || err != nil {
		t.Fatalf("OrphanGarageEntries() = (%v, %v)", entries, err)
	}
	if !errors.Is(ErrIndexMiss, ErrIndexMiss) || ErrIndexMiss.Error() == "" {
		t.Fatal("ErrIndexMiss")
	}
}
