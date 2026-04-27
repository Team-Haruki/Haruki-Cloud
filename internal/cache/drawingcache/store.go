package drawingcache

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	sqliteTimeLayout         = time.RFC3339Nano
	infiniteTTLSeconds int64 = 0
)

var (
	ErrRecordNotFound = errors.New("cache record not found")
	sha256KeyPattern  = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	infiniteExpiresAt = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)
)

// CacheRecord is one row in the drawing cache index.
type CacheRecord struct {
	Sha256Key  string
	APIPath    string
	UserID     string
	FilePath   string
	CreatedAt  time.Time
	LastUsedAt time.Time
	TTLSeconds int64
	ExpiresAt  time.Time
}

// DAO wraps the image_cache_index SQLite table.
type DAO struct {
	db *sql.DB
}

func InitDB(dbPath string) (*sql.DB, error) {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return nil, errors.New("db path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir failed: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite failed: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite failed: %w", err)
	}

	const ddl = `
CREATE TABLE IF NOT EXISTS image_cache_index (
	sha256_key TEXT PRIMARY KEY,
	api_path   TEXT NOT NULL DEFAULT '',
	user_id    TEXT NOT NULL DEFAULT '',
	file_path  TEXT NOT NULL,
	created_at TEXT NOT NULL,
	last_used_at TEXT NOT NULL,
	ttl_seconds INTEGER NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_image_cache_expires_at ON image_cache_index(expires_at);
CREATE INDEX IF NOT EXISTS idx_image_cache_scope ON image_cache_index(api_path, user_id);
`
	if _, err := db.Exec(ddl); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create table image_cache_index failed: %w", err)
	}
	if err := ensureColumn(db, "image_cache_index", "api_path", "TEXT NOT NULL DEFAULT ''"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure column api_path failed: %w", err)
	}
	if err := ensureColumn(db, "image_cache_index", "user_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure column user_id failed: %w", err)
	}
	if err := ensureColumn(db, "image_cache_index", "last_used_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure column last_used_at failed: %w", err)
	}
	if err := ensureColumn(db, "image_cache_index", "ttl_seconds", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure column ttl_seconds failed: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_image_cache_scope ON image_cache_index(api_path, user_id)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create scope index failed: %w", err)
	}

	return db, nil
}

func ensureColumn(db *sql.DB, tableName, columnName, columnDDL string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, columnName) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDDL))
	return err
}

func NewDAO(db *sql.DB) *DAO {
	return &DAO{db: db}
}

func ValidateSHA256Key(key string) error {
	if !sha256KeyPattern.MatchString(strings.TrimSpace(key)) {
		return errors.New("invalid key: expected 64-char hex sha256")
	}
	return nil
}

func (d *DAO) GetRecord(key string) (*CacheRecord, error) {
	if d == nil || d.db == nil {
		return nil, errors.New("dao is not initialized")
	}
	if err := ValidateSHA256Key(key); err != nil {
		return nil, err
	}

	const query = `
SELECT sha256_key, api_path, user_id, file_path, created_at, last_used_at, ttl_seconds, expires_at
FROM image_cache_index
WHERE sha256_key = ?
`

	var (
		record        CacheRecord
		createdAtRaw  string
		lastUsedAtRaw string
		ttlSeconds    int64
		expiresAtRaw  string
	)
	err := d.db.QueryRow(query, key).Scan(
		&record.Sha256Key,
		&record.APIPath,
		&record.UserID,
		&record.FilePath,
		&createdAtRaw,
		&lastUsedAtRaw,
		&ttlSeconds,
		&expiresAtRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("query cache record failed: %w", err)
	}

	record.CreatedAt, err = time.Parse(sqliteTimeLayout, createdAtRaw)
	if err != nil {
		return nil, fmt.Errorf("parse created_at failed: %w", err)
	}
	if strings.TrimSpace(lastUsedAtRaw) == "" {
		record.LastUsedAt = record.CreatedAt
	} else {
		record.LastUsedAt, err = time.Parse(sqliteTimeLayout, lastUsedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse last_used_at failed: %w", err)
		}
	}
	record.ExpiresAt, err = time.Parse(sqliteTimeLayout, expiresAtRaw)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at failed: %w", err)
	}
	record.TTLSeconds = ttlSeconds
	if isInfiniteTTL(record.TTLSeconds) {
		record.TTLSeconds = infiniteTTLSeconds
		record.ExpiresAt = infiniteExpiresAt
	}

	return &record, nil
}

func (d *DAO) SaveRecord(record *CacheRecord) error {
	if d == nil || d.db == nil {
		return errors.New("dao is not initialized")
	}
	if record == nil {
		return errors.New("record is nil")
	}
	if err := ValidateSHA256Key(record.Sha256Key); err != nil {
		return err
	}
	if strings.TrimSpace(record.FilePath) == "" {
		return errors.New("file path is empty")
	}
	if strings.TrimSpace(record.APIPath) == "" {
		return errors.New("api path is empty")
	}
	if strings.TrimSpace(record.UserID) == "" {
		return errors.New("user id is empty")
	}
	if record.CreatedAt.IsZero() {
		return errors.New("created_at must not be zero")
	}
	if record.LastUsedAt.IsZero() {
		record.LastUsedAt = record.CreatedAt
	}
	normalizeCacheRecordLifetime(record)
	if record.ExpiresAt.IsZero() {
		return errors.New("expires_at must not be zero")
	}

	const upsert = `
	INSERT INTO image_cache_index (sha256_key, api_path, user_id, file_path, created_at, last_used_at, ttl_seconds, expires_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(sha256_key) DO UPDATE SET
		api_path = excluded.api_path,
		user_id = excluded.user_id,
		file_path = excluded.file_path,
		created_at = excluded.created_at,
		last_used_at = excluded.last_used_at,
		ttl_seconds = excluded.ttl_seconds,
		expires_at = excluded.expires_at
	`

	_, err := d.db.Exec(
		upsert,
		record.Sha256Key,
		record.APIPath,
		record.UserID,
		record.FilePath,
		record.CreatedAt.UTC().Format(sqliteTimeLayout),
		record.LastUsedAt.UTC().Format(sqliteTimeLayout),
		record.TTLSeconds,
		record.ExpiresAt.UTC().Format(sqliteTimeLayout),
	)
	if err != nil {
		return fmt.Errorf("upsert cache record failed: %w", err)
	}
	return nil
}

func (d *DAO) TouchRecordOnHit(key string, lastUsedAt time.Time, ttlSeconds int64) error {
	if d == nil || d.db == nil {
		return errors.New("dao is not initialized")
	}
	if err := ValidateSHA256Key(key); err != nil {
		return err
	}
	if lastUsedAt.IsZero() {
		return errors.New("last_used_at must not be zero")
	}
	expiresAt := cacheExpiresAt(lastUsedAt.UTC(), ttlSeconds)
	const update = `
	UPDATE image_cache_index
	SET last_used_at = ?, ttl_seconds = ?, expires_at = ?
	WHERE sha256_key = ?
	`

	result, err := d.db.Exec(
		update,
		lastUsedAt.UTC().Format(sqliteTimeLayout),
		ttlSeconds,
		expiresAt.UTC().Format(sqliteTimeLayout),
		key,
	)
	if err != nil {
		return fmt.Errorf("touch cache record failed: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("touch cache record rows_affected failed: %w", err)
	}
	if affected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (d *DAO) DeleteRecord(key string) error {
	if d == nil || d.db == nil {
		return errors.New("dao is not initialized")
	}
	if err := ValidateSHA256Key(key); err != nil {
		return err
	}

	const del = `DELETE FROM image_cache_index WHERE sha256_key = ?`
	if _, err := d.db.Exec(del, key); err != nil {
		return fmt.Errorf("delete cache record failed: %w", err)
	}
	return nil
}

func isInfiniteTTL(ttlSeconds int64) bool {
	return ttlSeconds <= infiniteTTLSeconds
}

func cacheExpiresAt(base time.Time, ttlSeconds int64) time.Time {
	if isInfiniteTTL(ttlSeconds) {
		return infiniteExpiresAt
	}
	return base.UTC().Add(time.Duration(ttlSeconds) * time.Second)
}

func normalizeCacheRecordLifetime(record *CacheRecord) {
	if record == nil {
		return
	}
	if isInfiniteTTL(record.TTLSeconds) {
		record.TTLSeconds = infiniteTTLSeconds
		record.ExpiresAt = infiniteExpiresAt
		return
	}
	if record.ExpiresAt.IsZero() {
		record.ExpiresAt = cacheExpiresAt(record.CreatedAt, record.TTLSeconds)
	}
}

func formatCacheRecordExpiresAt(record *CacheRecord) string {
	if record == nil || isInfiniteTTL(record.TTLSeconds) {
		return ""
	}
	return record.ExpiresAt.UTC().Format(time.RFC3339)
}
