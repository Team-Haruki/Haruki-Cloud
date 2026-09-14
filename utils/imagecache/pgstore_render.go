package imagecache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// ErrIndexMiss marks a render index miss for callers that fold the found flag
// into an error.
var ErrIndexMiss = errors.New("imagecache: render index miss")

// Render index queries (section 5.4). Cloud never INSERTs into
// render_cache_index: Drawing owns the writes. Cloud only touches rows on a
// hit and deletes them during GC.
const (
	lookupRenderSQL = `SELECT r.content_hash, r.api_path, r.user_id, r.group_name, r.key_version, r.ttl_seconds,
       r.expires_at, r.last_used_at,
       e.group_name, e.cdn_path, e.storage_backend, e.media_type, e.size_bytes, COALESCE(e.file_path, ''),
       e.expires_at, e.last_referenced_at
  FROM render_cache_index r JOIN image_cache_entries e ON e.hash = r.content_hash
 WHERE r.request_key = $1`

	// touchRenderSQL is the sliding TTL: a hit restarts the row's lifetime.
	touchRenderSQL = `UPDATE render_cache_index r
   SET last_used_at = now(),
       expires_at   = CASE WHEN r.ttl_seconds > 0
                           THEN now() + (r.ttl_seconds || ' seconds')::interval
                           ELSE NULL END
 WHERE r.request_key = ANY($1)`

	deleteRenderSQL = `DELETE FROM render_cache_index WHERE request_key = ANY($1)`

	expiredRenderKeysSQL = `SELECT request_key FROM render_cache_index
 WHERE expires_at IS NOT NULL AND expires_at < $1
 ORDER BY expires_at LIMIT $2`

	// orphanGarageEntriesSQL deliberately ignores e.expires_at: Drawing only
	// ever extends it, so the reference check plus the retention window is the
	// whole lifetime rule (addendum A6).
	orphanGarageEntriesSQL = `SELECT hash, group_name, cdn_path, COALESCE(file_path, ''), storage_backend, media_type,
       size_bytes, expires_at, last_referenced_at
  FROM image_cache_entries e
 WHERE e.storage_backend = 'garage'
   AND e.last_referenced_at < $1
   AND NOT EXISTS (SELECT 1 FROM render_cache_index r WHERE r.content_hash = e.hash)
 ORDER BY e.last_referenced_at LIMIT $2`

	deleteEntriesSQL = `DELETE FROM image_cache_entries WHERE hash = ANY($1)`
)

// RenderIndexEntry is one render_cache_index row joined with its
// image_cache_entries row.
type RenderIndexEntry struct {
	RequestKey  string
	ContentHash string
	APIPath     string
	UserID      string
	GroupName   string
	KeyVersion  int
	// TTLSeconds <= 0 means infinite.
	TTLSeconds int64
	// ExpiresAt is zero for "never".
	ExpiresAt  time.Time
	LastUsedAt time.Time
	// Entry is the joined image_cache_entries row.
	Entry ImageEntry
}

// LookupRender returns the render index row for requestKey. A missing row is
// (RenderIndexEntry{}, false, nil); any other failure is returned.
func (s *PGStore) LookupRender(ctx context.Context, requestKey string) (RenderIndexEntry, bool, error) {
	if s == nil {
		return RenderIndexEntry{}, false, nil
	}
	out := RenderIndexEntry{RequestKey: requestKey}
	var (
		expiresAt, entryExpiresAt sql.NullTime
		backend, mediaType        sql.NullString
	)
	err := s.db.QueryRowContext(ctx, lookupRenderSQL, requestKey).Scan(
		&out.ContentHash, &out.APIPath, &out.UserID, &out.GroupName, &out.KeyVersion, &out.TTLSeconds,
		&expiresAt, &out.LastUsedAt,
		&out.Entry.GroupName, &out.Entry.CDNPath, &backend, &mediaType, &out.Entry.SizeBytes, &out.Entry.FilePath,
		&entryExpiresAt, &out.Entry.LastReferencedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RenderIndexEntry{}, false, nil
	}
	if err != nil {
		return RenderIndexEntry{}, false, fmt.Errorf("imagecache pgstore: lookup render: %w", err)
	}
	out.ExpiresAt = nullTime(expiresAt)
	out.Entry.Hash = out.ContentHash
	out.Entry.StorageBackend = backend.String
	if out.Entry.StorageBackend == "" {
		out.Entry.StorageBackend = inferBackend(out.Entry.FilePath)
	}
	out.Entry.MediaType = mediaType.String
	out.Entry.ExpiresAt = nullTime(entryExpiresAt)
	return out, true, nil
}

// TouchRender slides the TTL of the given request keys and returns the number
// of rows updated. An empty key list issues no statement.
func (s *PGStore) TouchRender(ctx context.Context, keys []string) (int64, error) {
	return s.execKeys(ctx, "touch render", touchRenderSQL, keys)
}

// DeleteRender deletes the given render index rows.
func (s *PGStore) DeleteRender(ctx context.Context, keys []string) (int64, error) {
	return s.execKeys(ctx, "delete render", deleteRenderSQL, keys)
}

// DeleteEntries deletes image_cache_entries rows by hash. A row still
// referenced by render_cache_index makes the statement fail (no cascade).
func (s *PGStore) DeleteEntries(ctx context.Context, hashes []string) (int64, error) {
	return s.execKeys(ctx, "delete entries", deleteEntriesSQL, hashes)
}

func (s *PGStore) execKeys(ctx context.Context, op, query string, keys []string) (int64, error) {
	if s == nil || len(keys) == 0 {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx, query, pq.Array(keys))
	if err != nil {
		return 0, fmt.Errorf("imagecache pgstore: %s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("imagecache pgstore: %s rows: %w", op, err)
	}
	return n, nil
}

// ExpiredRenderKeys returns up to limit request keys whose finite expiry is
// before now, oldest first. Infinite rows (expires_at NULL) are never returned.
func (s *PGStore) ExpiredRenderKeys(ctx context.Context, now time.Time, limit int) ([]string, error) {
	if s == nil || limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, expiredRenderKeysSQL, now, limit)
	if err != nil {
		return nil, fmt.Errorf("imagecache pgstore: expired render keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("imagecache pgstore: expired render keys scan: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("imagecache pgstore: expired render keys: %w", err)
	}
	return keys, nil
}

// OrphanGarageEntries returns up to limit garage rows that no render index row
// references and that were last referenced before retentionCutoff, oldest
// first. legacy_disk rows are never returned.
func (s *PGStore) OrphanGarageEntries(ctx context.Context, retentionCutoff time.Time, limit int) ([]ImageEntry, error) {
	if s == nil || limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, orphanGarageEntriesSQL, retentionCutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("imagecache pgstore: orphan garage entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []ImageEntry
	for rows.Next() {
		var (
			entry              ImageEntry
			backend, mediaType sql.NullString
			expiresAt          sql.NullTime
		)
		if err := rows.Scan(&entry.Hash, &entry.GroupName, &entry.CDNPath, &entry.FilePath, &backend, &mediaType,
			&entry.SizeBytes, &expiresAt, &entry.LastReferencedAt); err != nil {
			return nil, fmt.Errorf("imagecache pgstore: orphan garage entries scan: %w", err)
		}
		entry.StorageBackend = backend.String
		entry.MediaType = mediaType.String
		entry.ExpiresAt = nullTime(expiresAt)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("imagecache pgstore: orphan garage entries: %w", err)
	}
	return entries, nil
}

func nullTime(value sql.NullTime) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}
