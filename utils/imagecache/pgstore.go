package imagecache

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// initSQL creates the table with cdn_path (relative path only, no domain).
const initSQL = `
CREATE TABLE IF NOT EXISTS image_cache_entries (
	hash       TEXT PRIMARY KEY,
	group_name TEXT NOT NULL,
	cdn_path   TEXT NOT NULL,
	file_path  TEXT NOT NULL,
	size_bytes BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

// migrateSQL renames the old cdn_url column (which stored full URLs) to cdn_path,
// then strips the domain prefix from any existing rows so that only the relative
// path is stored going forward. This is idempotent — it is a no-op once migrated.
const migrateSQL = `
DO $$ BEGIN
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'image_cache_entries' AND column_name = 'cdn_url'
	) AND NOT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'image_cache_entries' AND column_name = 'cdn_path'
	) THEN
		ALTER TABLE image_cache_entries RENAME COLUMN cdn_url TO cdn_path;
		UPDATE image_cache_entries
			SET cdn_path = regexp_replace(cdn_path, '^https?://[^/]+/', '');
	END IF;
END $$`

// PGStore is a PostgreSQL-backed metadata store for the image cache.
// It enables deduplication across restarts and multi-instance deployments.
// A nil PGStore is safe to use — all methods become no-ops.
type PGStore struct {
	db *sql.DB
}

// NewPGStore opens a PostgreSQL connection pool using the given DSN.
func NewPGStore(dsn string) (*PGStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("imagecache pgstore: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("imagecache pgstore: ping: %w", err)
	}
	return &PGStore{db: db}, nil
}

// Init creates the image_cache_entries table if it does not exist, then runs
// any pending schema migrations (e.g. renaming cdn_url → cdn_path).
func (s *PGStore) Init(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, initSQL); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, migrateSQL)
	return err
}

// Lookup returns the stored relative CDN path and file path for a previously
// stored image hash. The caller is responsible for prepending the CDN base URI.
// Returns ("", "", false) on miss or any error.
func (s *PGStore) Lookup(ctx context.Context, hash string) (cdnPath, filePath string, ok bool) {
	if s == nil {
		return "", "", false
	}
	err := s.db.QueryRowContext(ctx,
		`SELECT cdn_path, file_path FROM image_cache_entries WHERE hash = $1`, hash).Scan(&cdnPath, &filePath)
	if err != nil {
		return "", "", false
	}
	return cdnPath, filePath, true
}

// Insert records a new image cache entry. cdnPath must be a relative path
// (no scheme or domain), e.g. "pjsk/profile/abc123.png".
// If the same hash already exists, the file_path, cdn_path and size_bytes are updated in place.
func (s *PGStore) Insert(ctx context.Context, hash, groupName, cdnPath, filePath string, sizeBytes int64) {
	if s == nil {
		return
	}
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO image_cache_entries (hash, group_name, cdn_path, file_path, size_bytes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (hash) DO UPDATE
			SET cdn_path   = EXCLUDED.cdn_path,
			    file_path  = EXCLUDED.file_path,
			    size_bytes = EXCLUDED.size_bytes`,
		hash, groupName, cdnPath, filePath, sizeBytes)
}

// Close releases the database connection pool.
func (s *PGStore) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}
