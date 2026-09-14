package imagecache

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func newMockPGStore(t *testing.T) (*PGStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	return &PGStore{db: db}, mock
}

func TestPGStoreNilReceiver(t *testing.T) {
	var store *PGStore
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if entry, ok, err := store.Lookup(ctx, "hash"); ok || err != nil || entry != (ImageEntry{}) {
		t.Fatalf("Lookup() = (%+v, %v, %v)", entry, ok, err)
	}
	if err := store.InsertEntry(ctx, ImageEntry{Hash: "hash"}); err != nil {
		t.Fatalf("InsertEntry() error = %v", err)
	}
	store.Insert(ctx, "hash", "group", "cdn", "file", 1)
	if store.Widened() {
		t.Fatal("nil store reports a widened schema")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestPGStoreInit(t *testing.T) {
	ctx := context.Background()
	probe := regexp.QuoteMeta(widenedProbeSQL)
	t.Run("probe present", func(t *testing.T) {
		store, mock := newMockPGStore(t)
		mock.ExpectExec(regexp.QuoteMeta(initSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(migrateSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(probe).WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
		if err := store.Init(ctx); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		if !store.Widened() {
			t.Fatal("widened schema not detected")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("SQL expectations: %v", err)
		}
	})

	t.Run("probe absent", func(t *testing.T) {
		store, mock := newMockPGStore(t)
		store.widened.Store(true)
		mock.ExpectExec(regexp.QuoteMeta(initSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(migrateSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(probe).WillReturnRows(sqlmock.NewRows([]string{"?column?"}))
		if err := store.Init(ctx); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		if store.Widened() {
			t.Fatal("un-widened schema reported as widened")
		}
	})

	t.Run("probe failure", func(t *testing.T) {
		store, mock := newMockPGStore(t)
		wantErr := errors.New("probe failed")
		mock.ExpectExec(regexp.QuoteMeta(initSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(migrateSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(probe).WillReturnError(wantErr)
		if err := store.Init(ctx); !errors.Is(err, wantErr) {
			t.Fatalf("Init() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("create failure", func(t *testing.T) {
		store, mock := newMockPGStore(t)
		wantErr := errors.New("create failed")
		mock.ExpectExec(regexp.QuoteMeta(initSQL)).WillReturnError(wantErr)
		if err := store.Init(ctx); !errors.Is(err, wantErr) {
			t.Fatalf("Init() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("migration failure", func(t *testing.T) {
		store, mock := newMockPGStore(t)
		wantErr := errors.New("migration failed")
		mock.ExpectExec(regexp.QuoteMeta(initSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(migrateSQL)).WillReturnError(wantErr)
		if err := store.Init(ctx); !errors.Is(err, wantErr) {
			t.Fatalf("Init() error = %v, want %v", err, wantErr)
		}
	})
}

func TestPGStoreLookupUnwidened(t *testing.T) {
	ctx := context.Background()
	store, mock := newMockPGStore(t)
	query := regexp.QuoteMeta(lookupSQL)
	columns := []string{"cdn_path", "file_path", "size_bytes"}
	mock.ExpectQuery(query).WithArgs("hit").WillReturnRows(
		sqlmock.NewRows(columns).AddRow("pjsk/a.png", "/cache/a.png", int64(7)),
	)
	entry, ok, err := store.Lookup(ctx, "hit")
	want := ImageEntry{Hash: "hit", CDNPath: "pjsk/a.png", FilePath: "/cache/a.png", StorageBackend: BackendLegacyDisk, SizeBytes: 7}
	if !ok || err != nil || entry != want {
		t.Fatalf("Lookup() = (%+v, %v, %v)", entry, ok, err)
	}

	// COALESCE turns a NULL file_path into ""; such a row is a garage row.
	mock.ExpectQuery(query).WithArgs("null-file").WillReturnRows(
		sqlmock.NewRows(columns).AddRow("pjsk/b.png", "", int64(8)),
	)
	entry, ok, err = store.Lookup(ctx, "null-file")
	if !ok || err != nil || entry.FilePath != "" || entry.StorageBackend != BackendGarage {
		t.Fatalf("Lookup(NULL file_path) = (%+v, %v, %v)", entry, ok, err)
	}

	mock.ExpectQuery(query).WithArgs("miss").WillReturnError(sql.ErrNoRows)
	if entry, ok, err := store.Lookup(ctx, "miss"); ok || err != nil || entry != (ImageEntry{}) {
		t.Fatalf("Lookup(miss) = (%+v, %v, %v)", entry, ok, err)
	}

	driverErr := errors.New("connection reset")
	mock.ExpectQuery(query).WithArgs("broken").WillReturnError(driverErr)
	if entry, ok, err := store.Lookup(ctx, "broken"); ok || !errors.Is(err, driverErr) || entry != (ImageEntry{}) {
		t.Fatalf("Lookup(driver error) = (%+v, %v, %v)", entry, ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGStoreLookupWidened(t *testing.T) {
	ctx := context.Background()
	store, mock := newMockPGStore(t)
	store.widened.Store(true)
	query := regexp.QuoteMeta(lookupWidenedSQL)
	expires := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(query).WithArgs("garage").WillReturnRows(
		sqlmock.NewRows(widenedLookupColumns).AddRow("pjsk/api/a.png", "", int64(9), BackendGarage, "image/png", expires),
	)
	entry, ok, err := store.Lookup(ctx, "garage")
	want := ImageEntry{Hash: "garage", CDNPath: "pjsk/api/a.png", StorageBackend: BackendGarage, MediaType: "image/png", SizeBytes: 9, ExpiresAt: expires}
	if !ok || err != nil || entry != want {
		t.Fatalf("Lookup() = (%+v, %v, %v)", entry, ok, err)
	}

	// Before the backfill runs, storage_backend is NULL and is inferred.
	mock.ExpectQuery(query).WithArgs("pre-backfill").WillReturnRows(
		sqlmock.NewRows(widenedLookupColumns).AddRow("pjsk/b.png", "/cache/b.png", int64(1), nil, nil, nil),
	)
	entry, ok, err = store.Lookup(ctx, "pre-backfill")
	if !ok || err != nil || entry.StorageBackend != BackendLegacyDisk || !entry.ExpiresAt.IsZero() || entry.MediaType != "" {
		t.Fatalf("Lookup(NULL columns) = (%+v, %v, %v)", entry, ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGStoreInsertTexts(t *testing.T) {
	ctx := context.Background()
	t.Run("unwidened", func(t *testing.T) {
		store, mock := newMockPGStore(t)
		mock.ExpectExec(regexp.QuoteMeta(insertSQL)).
			WithArgs("hash", "pjsk", "pjsk/a.png", "", int64(12)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		if err := store.InsertEntry(ctx, ImageEntry{Hash: "hash", GroupName: "pjsk", CDNPath: "pjsk/a.png", SizeBytes: 12}); err != nil {
			t.Fatal(err)
		}
		insertErr := errors.New("insert failed")
		mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WillReturnError(insertErr)
		if err := store.InsertEntry(ctx, ImageEntry{Hash: "hash"}); !errors.Is(err, insertErr) {
			t.Fatalf("InsertEntry() error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("widened", func(t *testing.T) {
		store, mock := newMockPGStore(t)
		store.widened.Store(true)
		mock.ExpectExec(regexp.QuoteMeta(insertWidenedSQL)).
			WithArgs("hash", "pjsk", "pjsk/a.png", sql.NullString{String: "/cache/pjsk/a.png", Valid: true}, int64(12), BackendLegacyDisk, sql.NullString{}).
			WillReturnResult(sqlmock.NewResult(1, 1))
		if err := store.InsertEntry(ctx, ImageEntry{Hash: "hash", GroupName: "pjsk", CDNPath: "pjsk/a.png", FilePath: "/cache/pjsk/a.png", SizeBytes: 12}); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPGStoreInsertWrapperAndClose(t *testing.T) {
	store, mock := newMockPGStore(t)
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).
		WithArgs("hash", "profile", "pjsk/a.png", "/cache/a.png", int64(12)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	store.Insert(context.Background(), "hash", "profile", "pjsk/a.png", "/cache/a.png", 12)
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WillReturnError(errors.New("logged, not returned"))
	store.Insert(context.Background(), "hash", "profile", "pjsk/a.png", "/cache/a.png", 12)
	mock.ExpectClose()
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestConfigurePoolLimits(t *testing.T) {
	for _, tc := range []struct{ maxOpen, want int }{{0, DefaultPGMaxOpen}, {-1, DefaultPGMaxOpen}, {2, 2}, {20, 20}} {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		configurePool(db, PGStoreOptions{MaxOpen: tc.maxOpen})
		if got := db.Stats().MaxOpenConnections; got != tc.want {
			t.Fatalf("MaxOpen(%d) = %d, want %d", tc.maxOpen, got, tc.want)
		}
		_ = db.Close()
	}
}

func TestNewPGStoreRejectsInvalidDSN(t *testing.T) {
	store, err := NewPGStore("://invalid")
	if err == nil || store != nil {
		t.Fatalf("NewPGStore() = (%v, %v), want error", store, err)
	}
	store, err = NewPGStoreWithOptions("postgres://127.0.0.1:1/none?sslmode=disable&connect_timeout=1", PGStoreOptions{MaxOpen: 2})
	if err == nil || store != nil {
		t.Fatalf("NewPGStoreWithOptions(unreachable) = (%v, %v), want error", store, err)
	}
}
