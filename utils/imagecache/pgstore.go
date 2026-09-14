package imagecache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"haruki-cloud/utils/logger"

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

// Storage backend values recorded in image_cache_entries.storage_backend.
const (
	BackendLegacyDisk = "legacy_disk"
	BackendGarage     = "garage"
)

// Pool defaults. Before these limits the pool was unbounded.
const (
	DefaultPGMaxOpen     = 8
	defaultPGMaxIdle     = 4
	defaultPGConnMaxLife = 30 * time.Minute
)

// widenedProbeSQL detects the widened image_cache_entries schema. The widening
// DDL ships in a later release, so the store reads and writes both shapes.
const widenedProbeSQL = `SELECT 1 FROM information_schema.columns WHERE table_name = 'image_cache_entries' AND column_name = 'storage_backend'`

const lookupSQL = `SELECT cdn_path, COALESCE(file_path, ''), size_bytes FROM image_cache_entries WHERE hash = $1`

const lookupWidenedSQL = `SELECT cdn_path, COALESCE(file_path, ''), size_bytes, storage_backend, media_type, expires_at FROM image_cache_entries WHERE hash = $1`

const insertSQL = `
		INSERT INTO image_cache_entries (hash, group_name, cdn_path, file_path, size_bytes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (hash) DO UPDATE
			SET cdn_path   = EXCLUDED.cdn_path,
			    file_path  = EXCLUDED.file_path,
			    size_bytes = EXCLUDED.size_bytes`

// insertWidenedSQL never rewrites a garage row: Drawing may already serve its
// recorded cdn_path, and a Cloud write must not move it.
const insertWidenedSQL = `
		INSERT INTO image_cache_entries (hash, group_name, cdn_path, file_path, size_bytes, storage_backend, media_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (hash) DO UPDATE
			SET cdn_path        = EXCLUDED.cdn_path,
			    file_path       = EXCLUDED.file_path,
			    size_bytes      = EXCLUDED.size_bytes,
			    storage_backend = EXCLUDED.storage_backend,
			    media_type      = EXCLUDED.media_type
			WHERE image_cache_entries.storage_backend IS DISTINCT FROM 'garage'`

// ImageEntry is one image_cache_entries row.
type ImageEntry struct {
	Hash      string
	GroupName string
	CDNPath   string
	// FilePath is "" when the column is NULL (garage rows).
	FilePath string
	// StorageBackend is BackendLegacyDisk or BackendGarage. On the un-widened
	// schema, or when the column is NULL, it is inferred from FilePath.
	StorageBackend string
	MediaType      string
	SizeBytes      int64
	// ExpiresAt is zero for "never". It is not a lifetime signal for garage rows.
	ExpiresAt time.Time
}

// PGStoreOptions tunes the connection pool.
type PGStoreOptions struct {
	// MaxOpen bounds open connections; <= 0 selects DefaultPGMaxOpen.
	MaxOpen int
}

// PGStore is a PostgreSQL-backed metadata store for the image cache.
// It enables deduplication across restarts and multi-instance deployments.
// A nil PGStore is safe to use — all methods become no-ops.
type PGStore struct {
	db      *sql.DB
	widened atomic.Bool
}

// NewPGStore opens a PostgreSQL connection pool using the given DSN with the
// default pool limits.
func NewPGStore(dsn string) (*PGStore, error) {
	return NewPGStoreWithOptions(dsn, PGStoreOptions{})
}

// NewPGStoreWithOptions opens a PostgreSQL connection pool using the given DSN.
func NewPGStoreWithOptions(dsn string, opts PGStoreOptions) (*PGStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("imagecache pgstore: open: %w", err)
	}
	store := NewPGStoreFromDB(db, opts)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("imagecache pgstore: ping: %w", err)
	}
	return store, nil
}

// NewPGStoreFromDB wraps an already opened pool (tests pass a sqlmock pool)
// and applies the pool limits. Init must still run before use.
func NewPGStoreFromDB(db *sql.DB, opts PGStoreOptions) *PGStore {
	configurePool(db, opts)
	return &PGStore{db: db}
}

func configurePool(db *sql.DB, opts PGStoreOptions) {
	maxOpen := opts.MaxOpen
	if maxOpen <= 0 {
		maxOpen = DefaultPGMaxOpen
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(min(defaultPGMaxIdle, maxOpen))
	db.SetConnMaxLifetime(defaultPGConnMaxLife)
}

// Init creates the image_cache_entries table if it does not exist, runs any
// pending schema migrations (e.g. renaming cdn_url → cdn_path), then probes
// whether the widened columns exist.
func (s *PGStore) Init(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, initSQL); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, migrateSQL); err != nil {
		return err
	}
	return s.probeSchema(ctx)
}

func (s *PGStore) probeSchema(ctx context.Context) error {
	var one int
	err := s.db.QueryRowContext(ctx, widenedProbeSQL).Scan(&one)
	switch {
	case err == nil:
		s.widened.Store(true)
	case errors.Is(err, sql.ErrNoRows):
		s.widened.Store(false)
	default:
		return fmt.Errorf("imagecache pgstore: schema probe: %w", err)
	}
	return nil
}

// Widened reports whether Init found the widened schema.
func (s *PGStore) Widened() bool {
	return s != nil && s.widened.Load()
}

// Lookup returns the stored row for a previously stored image hash. A missing
// row is (ImageEntry{}, false, nil); any other failure is returned so the
// caller can log it instead of treating it as a silent miss. CDNPath is
// relative: the caller prepends a public base URL.
func (s *PGStore) Lookup(ctx context.Context, hash string) (ImageEntry, bool, error) {
	if s == nil {
		return ImageEntry{}, false, nil
	}
	entry := ImageEntry{Hash: hash}
	var err error
	if s.Widened() {
		var backend, mediaType sql.NullString
		var expiresAt sql.NullTime
		err = s.db.QueryRowContext(ctx, lookupWidenedSQL, hash).
			Scan(&entry.CDNPath, &entry.FilePath, &entry.SizeBytes, &backend, &mediaType, &expiresAt)
		entry.StorageBackend = backend.String
		entry.MediaType = mediaType.String
		if expiresAt.Valid {
			entry.ExpiresAt = expiresAt.Time
		}
	} else {
		err = s.db.QueryRowContext(ctx, lookupSQL, hash).Scan(&entry.CDNPath, &entry.FilePath, &entry.SizeBytes)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ImageEntry{}, false, nil
	}
	if err != nil {
		return ImageEntry{}, false, fmt.Errorf("imagecache pgstore: lookup: %w", err)
	}
	if entry.StorageBackend == "" {
		entry.StorageBackend = inferBackend(entry.FilePath)
	}
	return entry, true, nil
}

func inferBackend(filePath string) string {
	if filePath == "" {
		return BackendGarage
	}
	return BackendLegacyDisk
}

// InsertEntry records an image cache row. CDNPath must be a relative path (no
// scheme or domain), e.g. "pjsk/abc123.png". An existing row for the same hash
// is updated in place (on the widened schema, never a garage row).
func (s *PGStore) InsertEntry(ctx context.Context, e ImageEntry) error {
	if s == nil {
		return nil
	}
	var err error
	if s.Widened() {
		backend := e.StorageBackend
		if backend == "" {
			backend = inferBackend(e.FilePath)
		}
		_, err = s.db.ExecContext(ctx, insertWidenedSQL,
			e.Hash, e.GroupName, e.CDNPath, nullIfEmpty(e.FilePath), e.SizeBytes, backend, nullIfEmpty(e.MediaType))
	} else {
		_, err = s.db.ExecContext(ctx, insertSQL, e.Hash, e.GroupName, e.CDNPath, e.FilePath, e.SizeBytes)
	}
	if err != nil {
		return fmt.Errorf("imagecache pgstore: insert: %w", err)
	}
	return nil
}

func nullIfEmpty(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

// Insert records a legacy disk row and logs a failure at ERROR. It is kept
// for the drawing render cache's legacy write side.
func (s *PGStore) Insert(ctx context.Context, hash, groupName, cdnPath, filePath string, sizeBytes int64) {
	if s == nil {
		return
	}
	err := s.InsertEntry(ctx, ImageEntry{
		Hash: hash, GroupName: groupName, CDNPath: cdnPath, FilePath: filePath,
		StorageBackend: BackendLegacyDisk, MediaType: mediaTypeFromPath(cdnPath), SizeBytes: sizeBytes,
	})
	if err != nil {
		logger.ErrorContext(ctx, "image cache index insert failed", "error", err)
	}
}

// Close releases the database connection pool.
func (s *PGStore) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}
