package imagecache

import (
	"context"
	"fmt"
	"time"
)

const retireObjectSQL = `WITH retired AS (
 DELETE FROM image_cache_entries e WHERE e.hash = $1 AND e.cdn_path = $3
 AND e.storage_backend = 'garage' AND e.last_referenced_at < $2
 AND NOT EXISTS (SELECT 1 FROM render_cache_index r WHERE r.content_hash = e.hash)
 RETURNING hash, cdn_path
), queued AS (
 INSERT INTO image_cache_object_deletions (content_hash, cdn_path)
 SELECT hash, cdn_path FROM retired ON CONFLICT (content_hash, cdn_path) DO NOTHING
) SELECT count(*) FROM retired`
const pendingObjectsSQL = `SELECT content_hash, cdn_path FROM image_cache_object_deletions WHERE next_attempt_at <= $1 ORDER BY next_attempt_at, queued_at LIMIT $2`
const pendingObjectSQL = `SELECT 1 FROM image_cache_object_deletions WHERE content_hash = $1 AND cdn_path = $2 AND next_attempt_at <= clock_timestamp() FOR UPDATE`
const finishObjectDeleteSQL = `DELETE FROM image_cache_object_deletions WHERE content_hash = $1 AND cdn_path = $2`
const retryObjectDeleteSQL = `UPDATE image_cache_object_deletions SET attempts = attempts + 1, next_attempt_at = NOW() + LEAST(21600, 300 * power(2::numeric, LEAST(attempts, 7))) * INTERVAL '1 second' WHERE content_hash = $1 AND cdn_path = $2`
const countObjectDeletesSQL = `SELECT count(*) FROM image_cache_object_deletions`

func (s *PGStore) retireObject(ctx context.Context, entry ImageEntry, cutoff time.Time) (int64, error) {
	var n int64
	err := s.executor().QueryRowContext(ctx, retireObjectSQL, entry.Hash, cutoff, entry.CDNPath).Scan(&n)
	return n, err
}

func (s *PGStore) pendingObjects(ctx context.Context, now time.Time, limit int) ([]pendingObjectDelete, error) {
	rows, err := s.executor().QueryContext(ctx, pendingObjectsSQL, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var pending []pendingObjectDelete
	for rows.Next() {
		var item pendingObjectDelete
		if err := rows.Scan(&item.Hash, &item.Key); err != nil {
			return nil, err
		}
		pending = append(pending, item)
	}
	return pending, rows.Err()
}

const registerUploadSQL = `INSERT INTO image_cache_object_deletions (content_hash, cdn_path, next_attempt_at) VALUES ($1, $2, NOW() + INTERVAL '5 minutes') ON CONFLICT (content_hash, cdn_path) DO NOTHING`

func (s *PGStore) registerUpload(ctx context.Context, hash, key string) error {
	_, err := s.executor().ExecContext(ctx, registerUploadSQL, hash, key)
	return err
}

const verifyUploadSQL = `SELECT 1 FROM image_cache_object_deletions WHERE content_hash = $1 AND cdn_path = $2 AND attempts = 0 AND next_attempt_at > clock_timestamp() FOR UPDATE`

func (s *PGStore) verifyUpload(ctx context.Context, hash, key string) error {
	var one int
	if err := s.executor().QueryRowContext(ctx, verifyUploadSQL, hash, key).Scan(&one); err != nil {
		return fmt.Errorf("imagecache: upload intent unavailable or expired: %w", err)
	}
	return nil
}
