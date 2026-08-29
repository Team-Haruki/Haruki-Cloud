package drawingcache

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveFileAndPruneEmptyDirsSafeSupportsContainedRelativePaths(t *testing.T) {
	storageRoot := filepath.Join(t.TempDir(), "cache")
	target := filepath.Join(storageRoot, "api", "profile", "cached.png")
	writeTestCacheFile(t, target, []byte("cached"))

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	relativeRoot, err := filepath.Rel(workingDir, storageRoot)
	if err != nil {
		t.Fatalf("make storage root relative: %v", err)
	}
	relativeTarget, err := filepath.Rel(workingDir, target)
	if err != nil {
		t.Fatalf("make target relative: %v", err)
	}

	if err := removeFileAndPruneEmptyDirsSafe(relativeTarget, relativeRoot); err != nil {
		t.Fatalf("remove contained relative path: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target still exists after cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storageRoot, "api")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty parent directories were not pruned: %v", err)
	}
	if info, err := os.Stat(storageRoot); err != nil || !info.IsDir() {
		t.Fatalf("storage root must remain intact: info=%v err=%v", info, err)
	}
}

func TestRemoveFileAndPruneEmptyDirsSafeRejectsRelativeTraversal(t *testing.T) {
	baseDir := t.TempDir()
	storageRoot := filepath.Join(baseDir, "cache")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatalf("create storage root: %v", err)
	}
	outsideTarget := filepath.Join(baseDir, "outside.png")
	writeTestCacheFile(t, outsideTarget, []byte("outside"))

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	relativeRoot, err := filepath.Rel(workingDir, storageRoot)
	if err != nil {
		t.Fatalf("make storage root relative: %v", err)
	}
	relativeOutsideTarget := filepath.Join(relativeRoot, "..", filepath.Base(outsideTarget))

	if err := removeFileAndPruneEmptyDirsSafe(relativeOutsideTarget, relativeRoot); err == nil {
		t.Fatal("cleanup accepted a relative path traversing outside storage root")
	}
	if body, err := os.ReadFile(outsideTarget); err != nil || string(body) != "outside" {
		t.Fatalf("outside target was modified: body=%q err=%v", body, err)
	}
}

func TestRemoveFileAndPruneEmptyDirsSafeRejectsSymlinkEscapes(t *testing.T) {
	baseDir := t.TempDir()
	storageRoot := filepath.Join(baseDir, "cache")
	outsideDir := filepath.Join(baseDir, "outside")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatalf("create storage root: %v", err)
	}
	outsideTarget := filepath.Join(outsideDir, "outside.png")
	writeTestCacheFile(t, outsideTarget, []byte("outside"))

	t.Run("symlinked parent", func(t *testing.T) {
		linkDir := filepath.Join(storageRoot, "linked-dir")
		if err := os.Symlink(outsideDir, linkDir); err != nil {
			t.Fatalf("create directory symlink: %v", err)
		}
		if err := removeFileAndPruneEmptyDirsSafe(filepath.Join(linkDir, filepath.Base(outsideTarget)), storageRoot); err == nil {
			t.Fatal("cleanup accepted a target through a symlinked parent")
		}
		if body, err := os.ReadFile(outsideTarget); err != nil || string(body) != "outside" {
			t.Fatalf("outside target was modified: body=%q err=%v", body, err)
		}
	})

	t.Run("symlinked target", func(t *testing.T) {
		linkPath := filepath.Join(storageRoot, "linked-file.png")
		if err := os.Symlink(outsideTarget, linkPath); err != nil {
			t.Fatalf("create file symlink: %v", err)
		}
		if err := removeFileAndPruneEmptyDirsSafe(linkPath, storageRoot); err == nil {
			t.Fatal("cleanup accepted a symlink resolving outside storage root")
		}
		if _, err := os.Lstat(linkPath); err != nil {
			t.Fatalf("unsafe symlink was removed: %v", err)
		}
		if body, err := os.ReadFile(outsideTarget); err != nil || string(body) != "outside" {
			t.Fatalf("outside target was modified: body=%q err=%v", body, err)
		}
	})

	t.Run("prune through symlinked parent", func(t *testing.T) {
		outsideChild := filepath.Join(outsideDir, "empty-child")
		if err := os.Mkdir(outsideChild, 0o755); err != nil {
			t.Fatalf("create outside child directory: %v", err)
		}
		linkDir := filepath.Join(storageRoot, "prune-link")
		if err := os.Symlink(outsideDir, linkDir); err != nil {
			t.Fatalf("create prune directory symlink: %v", err)
		}
		if err := pruneEmptyDirsSafe(filepath.Join(linkDir, filepath.Base(outsideChild)), storageRoot); err == nil {
			t.Fatal("directory pruning accepted a path through a symlinked parent")
		}
		if info, err := os.Stat(outsideChild); err != nil || !info.IsDir() {
			t.Fatalf("outside directory was removed: info=%v err=%v", info, err)
		}
	})
}
