package accountdata

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"haruki-cloud/utils/logger"
)

var profileBGCleanupLogger = logger.NewLoggerFromGlobal("ProfileBGCleanup")

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
			profileBGCleanupLogger.WarnContext(ctx, "profile background cleanup entry skipped",
				"operation", "walk",
				"error_type", fmt.Sprintf("%T", walkErr),
			)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(s.rootDir, path)
		if err != nil {
			profileBGCleanupLogger.WarnContext(ctx, "profile background cleanup entry skipped",
				"operation", "relative_path",
				"error_type", fmt.Sprintf("%T", err),
			)
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		if activePaths[relPath] {
			return nil
		}

		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			profileBGCleanupLogger.WarnContext(ctx, "profile background cleanup entry skipped",
				"operation", "remove",
				"error_type", fmt.Sprintf("%T", rmErr),
			)
			return nil
		}
		deleted++
		return nil
	})
	if err != nil {
		return deleted, fmt.Errorf("profile bg cleanup: walk dir: %w", err)
	}
	if deleted > 0 {
		profileBGCleanupLogger.InfoContext(ctx, "profile background cleanup completed", "deleted", deleted)
	}
	return deleted, nil
}
