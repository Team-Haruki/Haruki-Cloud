package accountdata

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// CleanupOrphanedFiles walks the profile background directory and removes any
// files whose relative paths are not present in activePaths.
// activePaths values must use forward-slash separators, matching the format
// stored in ProfileBgSettings.ImgPath (e.g. "user_upload/profile_bg/jp/uid_12345678901234_abc12345.jpg").
// Returns the number of deleted files.
func (s *LocalProfileBGStore) CleanupOrphanedFiles(ctx context.Context, activePaths map[string]bool) (int, error) {
	if s == nil {
		return 0, nil
	}

	bgRootAbs := filepath.Join(s.rootDir, s.relativeDir)
	if _, err := os.Stat(bgRootAbs); os.IsNotExist(err) {
		return 0, nil // directory doesn't exist yet — nothing to clean
	}

	var deleted int
	err := filepath.WalkDir(bgRootAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			slog.WarnContext(ctx, "profile bg cleanup: walk error", "path", path, "err", walkErr)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(s.rootDir, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		if activePaths[relPath] {
			return nil
		}

		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			slog.WarnContext(ctx, "profile bg cleanup: remove failed", "path", relPath, "err", rmErr)
			return nil
		}
		deleted++
		slog.InfoContext(ctx, "profile bg cleanup: removed orphaned file", "path", relPath)
		return nil
	})
	if err != nil {
		return deleted, fmt.Errorf("profile bg cleanup: walk dir: %w", err)
	}
	return deleted, nil
}
