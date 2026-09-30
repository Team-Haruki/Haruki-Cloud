package imagecache

import (
	"context"
	"errors"

	"haruki-cloud/internal/storage"
)

// ReconcileObject inspects one indexed object outside request handling. Missing
// objects invalidate their references when repair is true, permitting the next
// render to rebuild them. Errors other than NotExist never erase rows. Repair
// requires the same all-writer protocol as object GC.
func (s *PGStore) ReconcileObject(ctx context.Context, objects storage.Store, hash string, repair bool) (missing bool, err error) {
	if s == nil || objects == nil || objects == storage.Disabled() {
		return false, errors.New("imagecache: reconciliation requires index and objects")
	}
	if repair && !s.lifecycle.Load() {
		return false, errors.New("imagecache: content lock protocol unavailable")
	}
	ctx = storage.WithBackgroundIO(ctx)
	err = s.WithContentLock(ctx, hash, func(ctx context.Context, index *PGStore) error {
		entry, found, err := index.Lookup(ctx, hash)
		if err != nil || !found {
			return err
		}
		if entry.StorageBackend != BackendGarage {
			return nil
		}
		key, err := storage.CleanKey(entry.CDNPath)
		if err != nil {
			return err
		}
		if _, err = objects.Stat(ctx, key); err == nil {
			return nil
		}
		if errors.Is(err, storage.ErrNotConfigured) || !errors.Is(err, storage.ErrNotExist) {
			return err
		}
		missing = true
		if !repair {
			return nil
		}
		if _, err = index.executor().ExecContext(ctx, `DELETE FROM render_cache_index WHERE content_hash = $1`, hash); err != nil {
			return err
		}
		_, err = index.executor().ExecContext(ctx, `DELETE FROM image_cache_entries WHERE hash = $1 AND cdn_path = $2`, hash, entry.CDNPath)
		return err
	})
	return missing, err
}

// ReconcileCandidates uses a bounded keyset cursor for an offline scan.
func (s *PGStore) ReconcileCandidates(ctx context.Context, after string, limit int) ([]string, error) {
	if s == nil || limit <= 0 {
		return nil, nil
	}
	limit = min(limit, 1000)
	rows, err := s.executor().QueryContext(ctx, `SELECT hash FROM image_cache_entries WHERE storage_backend = 'garage' AND hash > $1 ORDER BY hash LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		hashes = append(hashes, hash)
	}
	return hashes, rows.Err()
}
