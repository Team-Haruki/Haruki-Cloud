package drawingcache

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"haruki-cloud/utils/logger"
)

const defaultGCBatchSize = 1000

var drawingCacheLogger = logger.NewLoggerFromGlobal("DrawingCache")

func drawingCacheErrorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}

func StartGCWorker(ctx context.Context, db *sql.DB, storageDir string, interval time.Duration) {
	if db == nil {
		drawingCacheLogger.Warn("cache GC worker not started", "reason", "db_unavailable")
		return
	}
	if interval <= 0 {
		drawingCacheLogger.Info("cache GC worker disabled")
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runCleanup := func() {
		totalCleaned := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			cleaned, err := cleanupExpiredBatch(db, storageDir, defaultGCBatchSize)
			if err != nil {
				drawingCacheLogger.WarnContext(ctx, "cache GC cleanup failed",
					"operation", "cleanup_expired_batch",
					"error_type", drawingCacheErrorType(err),
				)
				return
			}
			totalCleaned += cleaned
			if cleaned < defaultGCBatchSize {
				break
			}
		}
		if totalCleaned > 0 {
			drawingCacheLogger.InfoContext(ctx, "cache GC cleanup completed", "records", totalCleaned)
		}
	}

	go func() {
		runCleanup()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runCleanup()
			}
		}
	}()
}

func cleanupExpiredBatch(db *sql.DB, storageDir string, limit int) (int, error) {
	if limit <= 0 {
		limit = defaultGCBatchSize
	}

	nowText := time.Now().UTC().Format(sqliteTimeLayout)
	rows, err := db.Query(`
SELECT sha256_key, file_path
FROM image_cache_index
WHERE ttl_seconds > 0 AND expires_at < ?
ORDER BY expires_at ASC
LIMIT ?
`, nowText, limit)
	if err != nil {
		return 0, fmt.Errorf("query expired records failed: %w", err)
	}
	defer rows.Close()

	keys := make([]string, 0, limit)
	paths := make([]string, 0, limit)
	for rows.Next() {
		var key, path string
		if err := rows.Scan(&key, &path); err != nil {
			return 0, fmt.Errorf("scan expired record failed: %w", err)
		}
		keys = append(keys, key)
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate expired records failed: %w", err)
	}
	if len(keys) == 0 {
		return 0, nil
	}

	deletableKeys := make([]string, 0, len(keys))
	for i := range keys {
		shared, err := fileReferencedByLiveRecord(db, paths[i], keys[i])
		if err != nil {
			return 0, err
		}
		if !shared {
			err := removeFileAndPruneEmptyDirsSafe(paths[i], storageDir)
			if err != nil {
				drawingCacheLogger.Warn("cache GC file removal failed",
					"operation", "remove_file",
					"error_type", drawingCacheErrorType(err),
				)
				continue
			}
		}
		deletableKeys = append(deletableKeys, keys[i])
	}
	if len(deletableKeys) == 0 {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin expired record deletion failed: %w", err)
	}
	deletionCommitted := false
	defer func() {
		if !deletionCommitted {
			_ = tx.Rollback()
		}
	}()

	deleteStmt, err := tx.Prepare(`DELETE FROM image_cache_index WHERE sha256_key = ?`)
	if err != nil {
		return 0, fmt.Errorf("prepare expired record deletion failed: %w", err)
	}
	defer func() { _ = deleteStmt.Close() }()

	for _, key := range deletableKeys {
		if _, err := deleteStmt.Exec(key); err != nil {
			return 0, fmt.Errorf("delete expired record failed: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit expired record deletion failed: %w", err)
	}
	deletionCommitted = true
	return len(deletableKeys), nil
}

func fileReferencedByLiveRecord(db *sql.DB, filePath string, excludingKey string) (bool, error) {
	if db == nil {
		return false, nil
	}

	nowText := time.Now().UTC().Format(sqliteTimeLayout)
	var count int
	err := db.QueryRow(`
SELECT COUNT(1)
FROM image_cache_index
WHERE file_path = ?
  AND sha256_key <> ?
  AND (ttl_seconds <= 0 OR expires_at >= ?)
`, filePath, excludingKey, nowText).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("query shared cache file refs failed: %w", err)
	}
	return count > 0, nil
}
