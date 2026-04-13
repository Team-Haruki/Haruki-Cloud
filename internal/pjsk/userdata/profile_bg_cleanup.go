package userdata

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userbinding"
)

// CleanupOrphanedFiles walks the profile background directory and removes any
// files whose relative paths are not present in activePaths.
// activePaths values must use forward-slash separators, matching the format
// stored in ProfileBgSettings.ImgPath (e.g. "user_upload/profile_bg/jp/binding_3_abc12345.jpg").
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

// ProfileBGCleaner queries the database for all active background image paths
// and removes any orphaned files from the profile background directory.
// It is stateless — call Run on whatever schedule suits the deployment
// (e.g. time.Ticker, cron, startup hook).
type ProfileBGCleaner struct {
	store *LocalProfileBGStore
	db    *pjskdb.Client
}

// NewProfileBGCleaner returns a new ProfileBGCleaner.
// Returns nil if either argument is nil.
func NewProfileBGCleaner(store *LocalProfileBGStore, db *pjskdb.Client) *ProfileBGCleaner {
	if store == nil || db == nil {
		return nil
	}
	return &ProfileBGCleaner{store: store, db: db}
}

// Run queries all active background paths from the database, then deletes any
// files in the background directory that are no longer referenced.
// Returns the number of deleted files and any non-fatal walk error.
func (c *ProfileBGCleaner) Run(ctx context.Context) (int, error) {
	if c == nil {
		return 0, nil
	}

	bindings, err := c.db.UserBinding.Query().
		Where(userbinding.BgNotNil()).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("profile bg cleanup: query bindings: %w", err)
	}

	active := make(map[string]bool, len(bindings))
	for _, b := range bindings {
		if b.Bg != nil && b.Bg.ImgPath != nil {
			p := filepath.ToSlash(strings.TrimSpace(*b.Bg.ImgPath))
			if p != "" {
				active[p] = true
			}
		}
	}

	return c.store.CleanupOrphanedFiles(ctx, active)
}
