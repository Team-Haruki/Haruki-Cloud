package drawingcache

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureImageCacheColumnUsesFixedWhitelist(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE image_cache_index (sha256_key TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create legacy cache table: %v", err)
	}
	for _, column := range []string{"api_path", "user_id", "last_used_at", "ttl_seconds"} {
		if err := ensureImageCacheColumn(db, column); err != nil {
			t.Fatalf("ensure fixed column %q: %v", column, err)
		}
		if err := ensureImageCacheColumn(db, column); err != nil {
			t.Fatalf("ensure existing column %q: %v", column, err)
		}
	}
	if err := ensureImageCacheColumn(db, `unsafe TEXT; DROP TABLE image_cache_index; --`); err == nil {
		t.Fatal("unlisted schema identifier should be rejected")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('image_cache_index') WHERE name IN ('api_path', 'user_id', 'last_used_at', 'ttl_seconds')`).Scan(&count); err != nil {
		t.Fatalf("inspect migrated columns: %v", err)
	}
	if count != 4 {
		t.Fatalf("migrated column count = %d, want 4", count)
	}
}

func TestInitDBMigratesLegacyFileBeforeCreatingScopeIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-cache.sqlite")
	legacyDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy SQLite database: %v", err)
	}
	if _, err := legacyDB.Exec(`
CREATE TABLE image_cache_index (
	sha256_key TEXT PRIMARY KEY,
	file_path TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
)
`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("create legacy cache table: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy SQLite database: %v", err)
	}

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("migrate legacy cache database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var columnCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('image_cache_index') WHERE name IN ('api_path', 'user_id', 'last_used_at', 'ttl_seconds')`).Scan(&columnCount); err != nil {
		t.Fatalf("inspect migrated columns: %v", err)
	}
	if columnCount != 4 {
		t.Fatalf("migrated column count = %d, want 4", columnCount)
	}

	var indexCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'index' AND name = 'idx_image_cache_scope'`).Scan(&indexCount); err != nil {
		t.Fatalf("inspect scope index: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("scope index count = %d, want 1", indexCount)
	}
}

func TestCleanupExpiredBatchDeletesMultipleRecordsWithFixedStatement(t *testing.T) {
	db := openTestDB(t)
	dao := NewDAO(db)
	storageDir := t.TempDir()
	now := time.Now().UTC()

	keys := []string{strings.Repeat("1", 64), strings.Repeat("2", 64)}
	for _, key := range keys {
		filePath := filepath.Join(storageDir, key+".png")
		writeTestCacheFile(t, filePath, []byte("expired"))
		if err := dao.SaveRecord(&CacheRecord{
			Sha256Key:  key,
			APIPath:    "api/pjsk/profile",
			UserID:     "public",
			FilePath:   filePath,
			CreatedAt:  now.Add(-2 * time.Hour),
			LastUsedAt: now.Add(-2 * time.Hour),
			TTLSeconds: 60,
			ExpiresAt:  now.Add(-time.Hour),
		}); err != nil {
			t.Fatalf("save expired record %q: %v", key, err)
		}
	}

	cleaned, err := cleanupExpiredBatch(db, storageDir, len(keys))
	if err != nil {
		t.Fatalf("cleanup expired records: %v", err)
	}
	if cleaned != len(keys) {
		t.Fatalf("cleaned records = %d, want %d", cleaned, len(keys))
	}
	for _, key := range keys {
		if _, err := dao.GetRecord(key); !errors.Is(err, ErrRecordNotFound) {
			t.Errorf("expired record %q remains: %v", key, err)
		}
	}
}
