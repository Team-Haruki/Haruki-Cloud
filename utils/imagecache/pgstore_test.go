package imagecache

import (
	"context"
	"errors"
	"regexp"
	"testing"

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
	if cdnPath, filePath, ok := store.Lookup(ctx, "hash"); ok || cdnPath != "" || filePath != "" {
		t.Fatalf("Lookup() = (%q, %q, %v)", cdnPath, filePath, ok)
	}
	store.Insert(ctx, "hash", "group", "cdn", "file", 1)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestPGStoreInit(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		store, mock := newMockPGStore(t)
		mock.ExpectExec(regexp.QuoteMeta(initSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(migrateSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
		if err := store.Init(ctx); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("SQL expectations: %v", err)
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

func TestPGStoreLookup(t *testing.T) {
	ctx := context.Background()
	store, mock := newMockPGStore(t)
	query := regexp.QuoteMeta(`SELECT cdn_path, file_path FROM image_cache_entries WHERE hash = $1`)
	mock.ExpectQuery(query).WithArgs("hit").WillReturnRows(
		sqlmock.NewRows([]string{"cdn_path", "file_path"}).AddRow("pjsk/a.png", "/cache/a.png"),
	)
	cdnPath, filePath, ok := store.Lookup(ctx, "hit")
	if !ok || cdnPath != "pjsk/a.png" || filePath != "/cache/a.png" {
		t.Fatalf("Lookup() = (%q, %q, %v)", cdnPath, filePath, ok)
	}

	mock.ExpectQuery(query).WithArgs("miss").WillReturnError(errors.New("missing"))
	if cdnPath, filePath, ok := store.Lookup(ctx, "miss"); ok || cdnPath != "" || filePath != "" {
		t.Fatalf("Lookup(miss) = (%q, %q, %v)", cdnPath, filePath, ok)
	}
}

func TestPGStoreInsertAndClose(t *testing.T) {
	store, mock := newMockPGStore(t)
	mock.ExpectExec("INSERT INTO image_cache_entries").
		WithArgs("hash", "profile", "pjsk/a.png", "/cache/a.png", int64(12)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	store.Insert(context.Background(), "hash", "profile", "pjsk/a.png", "/cache/a.png", 12)
	mock.ExpectClose()
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestNewPGStoreRejectsInvalidDSN(t *testing.T) {
	store, err := NewPGStore("://invalid")
	if err == nil || store != nil {
		t.Fatalf("NewPGStore() = (%v, %v), want error", store, err)
	}
}
