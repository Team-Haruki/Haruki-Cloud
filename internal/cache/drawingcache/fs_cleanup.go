package drawingcache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

var fileOpMu sync.Mutex

func removeFileAndPruneEmptyDirsSafe(path, storageRoot string) error {
	fileOpMu.Lock()
	defer fileOpMu.Unlock()

	root, relativeTarget, err := openCleanupTarget(path, storageRoot, false)
	if err != nil {
		return err
	}
	defer root.Close()

	if err := validateCleanupTarget(root, relativeTarget); err != nil {
		return err
	}
	err = root.Remove(relativeTarget)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return pruneEmptyDirsLocked(root, filepath.Dir(relativeTarget))
}

func pruneEmptyDirsSafe(path, storageRoot string) error {
	fileOpMu.Lock()
	defer fileOpMu.Unlock()

	root, relativePath, err := openCleanupTarget(path, storageRoot, true)
	if err != nil {
		return err
	}
	defer root.Close()
	return pruneEmptyDirsLocked(root, relativePath)
}

func pruneEmptyDirsLocked(root *os.Root, relativePath string) error {
	current := filepath.Clean(relativePath)
	for current != "." {
		if !isSafeRelativeCleanupPath(current) {
			return fmt.Errorf("cache cleanup path %q is outside storage root", relativePath)
		}
		if err := validateCleanupTarget(root, current); err != nil {
			return err
		}
		err := root.Remove(current)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			current = filepath.Dir(current)
			continue
		}
		if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
			return nil
		}
		return err
	}
	return nil
}

func openCleanupTarget(path, storageRoot string, allowRoot bool) (*os.Root, string, error) {
	absRoot, err := absoluteCleanupPath(storageRoot)
	if err != nil {
		return nil, "", fmt.Errorf("invalid cache storage root: %w", err)
	}
	absPath, err := absoluteCleanupPath(path)
	if err != nil {
		return nil, "", fmt.Errorf("invalid cache cleanup path: %w", err)
	}
	relativePath, err := filepath.Rel(absRoot, absPath)
	if err != nil || !isSafeRelativeCleanupPath(relativePath) {
		return nil, "", fmt.Errorf("cache cleanup path %q is outside storage root %q", absPath, absRoot)
	}
	if relativePath == "." && !allowRoot {
		return nil, "", fmt.Errorf("cache cleanup path must not be the storage root %q", absRoot)
	}
	// Keep traversal and deletion relative to one open directory handle so a
	// parent symlink replacement cannot redirect the operation outside the root.
	root, err := os.OpenRoot(absRoot)
	if err != nil {
		return nil, "", fmt.Errorf("open cache storage root: %w", err)
	}
	return root, relativePath, nil
}

func absoluteCleanupPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is empty")
	}
	return filepath.Abs(filepath.Clean(path))
}

func isSafeRelativeCleanupPath(path string) bool {
	if path == "" || filepath.IsAbs(path) || path == ".." {
		return false
	}
	return path == "." || !strings.HasPrefix(path, ".."+string(os.PathSeparator))
}

func validateCleanupTarget(root *os.Root, relativePath string) error {
	_, err := root.Stat(relativePath)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("cache cleanup path %q is not safely contained in storage root: %w", relativePath, err)
}
