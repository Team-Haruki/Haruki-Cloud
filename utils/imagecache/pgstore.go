package imagecache

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

const initSQL = `
CREATE TABLE IF NOT EXISTS image_cache_entries (
	hash       TEXT PRIMARY KEY,
	group_name TEXT NOT NULL,
	cdn_url    TEXT NOT NULL,
	file_path  TEXT NOT NULL,
	size_bytes BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

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

// Init creates the image_cache_entries table if it does not exist.
func (s *PGStore) Init(ctx context.Context) error {
	if s == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, initSQL)
	return err
}

// Lookup returns the CDN URL and stored file path for a previously stored image hash.
// Returns ("", "", false) on miss or any error.
func (s *PGStore) Lookup(ctx context.Context, hash string) (cdnURL, filePath string, ok bool) {
	if s == nil {
		return "", "", false
	}
	err := s.db.QueryRowContext(ctx,
		`SELECT cdn_url, file_path FROM image_cache_entries WHERE hash = $1`, hash).Scan(&cdnURL, &filePath)
	if err != nil {
		return "", "", false
	}
	return cdnURL, filePath, true
}

// Insert records a new image cache entry.
// If the same hash already exists, the file_path, cdn_url and size_bytes are updated in place.
func (s *PGStore) Insert(ctx context.Context, hash, groupName, cdnURL, filePath string, sizeBytes int64) {
	if s == nil {
		return
	}
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO image_cache_entries (hash, group_name, cdn_url, file_path, size_bytes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (hash) DO UPDATE
			SET cdn_url    = EXCLUDED.cdn_url,
			    file_path  = EXCLUDED.file_path,
			    size_bytes = EXCLUDED.size_bytes`,
		hash, groupName, cdnURL, filePath, sizeBytes)
}

// Close releases the database connection pool.
func (s *PGStore) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}
