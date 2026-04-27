package drawingcache

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

var fileOpMu sync.Mutex

func removeFileAndPruneEmptyDirsSafe(path, storageRoot string) error {
	fileOpMu.Lock()
	defer fileOpMu.Unlock()

	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return pruneEmptyDirsLocked(filepath.Dir(path), storageRoot)
}

func pruneEmptyDirsSafe(path, storageRoot string) error {
	fileOpMu.Lock()
	defer fileOpMu.Unlock()

	return pruneEmptyDirsLocked(path, storageRoot)
}

func pruneEmptyDirsLocked(path, storageRoot string) error {
	if storageRoot == "" || path == "" {
		return nil
	}

	absRoot, err := filepath.Abs(storageRoot)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !isPathUnderBase(absRoot, absPath) {
		return nil
	}

	current := absPath
	for current != absRoot {
		err := os.Remove(current)
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
