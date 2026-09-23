package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	localDirMode  fs.FileMode = 0o755
	localFileMode fs.FileMode = 0o644
)

type localStore struct {
	root           string
	maxObjectBytes int64
}

// NewLocal returns a Store rooted at root, which must be a non-empty absolute
// path. Objects live at <root>/<key>; writes are atomic (temp file in the
// target directory, chmod 0644, rename) and create missing directories 0755.
// maxObjectBytes <= 0 selects DefaultMaxObjectBytes.
func NewLocal(root string, maxObjectBytes int64) (Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("storage: local root is empty")
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("storage: local root %q is not absolute", root)
	}
	if maxObjectBytes <= 0 {
		maxObjectBytes = DefaultMaxObjectBytes
	}
	return &localStore{root: filepath.Clean(root), maxObjectBytes: maxObjectBytes}, nil
}

// NewLocalAt is NewLocal for a directory that may be relative: a relative
// path resolves against the working directory, exactly as the os-based
// consumers it replaces resolved it.
func NewLocalAt(dir string, maxObjectBytes int64) (Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("storage: local root is empty")
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return NewLocal(absolute, maxObjectBytes)
}

func (s *localStore) resolve(key Key) (Key, string, error) {
	cleaned, err := CleanKey(string(key))
	if err != nil {
		return "", "", err
	}
	return cleaned, filepath.Join(s.root, filepath.FromSlash(string(cleaned))), nil
}

func (s *localStore) Get(ctx context.Context, key Key) ([]byte, error) {
	cleaned, full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(full)
	if err != nil {
		return nil, mapLocalError("get", cleaned, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, NotExistError("get", cleaned)
	}
	if info.Size() > s.maxObjectBytes {
		return nil, TooLargeError("get", cleaned, info.Size(), s.maxObjectBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, s.maxObjectBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > s.maxObjectBytes {
		return nil, TooLargeError("get", cleaned, int64(len(data)), s.maxObjectBytes)
	}
	return data, nil
}

func (s *localStore) Put(ctx context.Context, key Key, data []byte, _ PutOptions) error {
	cleaned, full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if int64(len(data)) > s.maxObjectBytes {
		return TooLargeError("put", cleaned, int64(len(data)), s.maxObjectBytes)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, localDirMode); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(full)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()
	if err := writeTemp(ctx, temp, data); err != nil {
		return err
	}
	if err := os.Rename(tempPath, full); err != nil {
		return err
	}
	committed = true
	return nil
}

func writeTemp(ctx context.Context, temp *os.File, data []byte) error {
	_, writeErr := temp.Write(data)
	if writeErr == nil {
		writeErr = temp.Chmod(localFileMode)
	}
	closeErr := temp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return ctx.Err()
}

func (s *localStore) Stat(ctx context.Context, key Key) (Object, error) {
	cleaned, full, err := s.resolve(key)
	if err != nil {
		return Object{}, err
	}
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return Object{}, mapLocalError("stat", cleaned, err)
	}
	if !info.Mode().IsRegular() {
		return Object{}, NotExistError("stat", cleaned)
	}
	return Object{Key: cleaned, Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (s *localStore) Delete(ctx context.Context, key Key) error {
	_, full, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if err != nil {
		if isLocalNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if err := os.Remove(full); err != nil && !isLocalNotExist(err) {
		return err
	}
	return nil
}

func (s *localStore) List(ctx context.Context, prefix Key, fn func(Object) error) error {
	cleaned, err := CleanPrefix(string(prefix))
	if err != nil {
		return err
	}
	base := s.root
	if dir := listBaseDir(cleaned); dir != "" {
		base = filepath.Join(s.root, filepath.FromSlash(dir))
	}
	walkErr := filepath.WalkDir(base, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			if isLocalNotExist(err) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Type().IsRegular() || isLocalTempName(entry.Name()) {
			return nil
		}
		object, ok, err := s.listObject(current, entry, cleaned)
		if err != nil || !ok {
			return err
		}
		return fn(object)
	})
	return walkErr
}

func (s *localStore) ListDir(ctx context.Context, prefix Key, fn func(DirEntry) error) error {
	dir, err := CleanDirPrefix(string(prefix))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	base := s.root
	if dir != "" {
		base = filepath.Join(s.root, filepath.FromSlash(strings.TrimSuffix(string(dir), "/")))
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if isLocalNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if isLocalTempName(name) {
			continue
		}
		mode := entry.Type()
		switch {
		case mode.IsDir():
			if err := fn(DirEntry{Name: name, Dir: true}); err != nil {
				return err
			}
		case mode.IsRegular():
			info, err := entry.Info()
			if err != nil {
				if isLocalNotExist(err) {
					continue
				}
				return err
			}
			object := Object{Key: dir + Key(name), Size: info.Size(), ModTime: info.ModTime()}
			if err := fn(DirEntry{Name: name, Object: object}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *localStore) listObject(current string, entry fs.DirEntry, prefix Key) (Object, bool, error) {
	rel, err := filepath.Rel(s.root, current)
	if err != nil {
		return Object{}, false, err
	}
	key := Key(filepath.ToSlash(rel))
	if !strings.HasPrefix(string(key), string(prefix)) {
		return Object{}, false, nil
	}
	info, err := entry.Info()
	if err != nil {
		if isLocalNotExist(err) {
			return Object{}, false, nil
		}
		return Object{}, false, err
	}
	return Object{Key: key, Size: info.Size(), ModTime: info.ModTime()}, true, nil
}

// listBaseDir returns the deepest directory that can contain keys starting
// with prefix, or "" for the root.
func listBaseDir(prefix Key) string {
	value := string(prefix)
	if strings.HasSuffix(value, "/") {
		return strings.TrimSuffix(value, "/")
	}
	if dir := path.Dir(value); dir != "." {
		return dir
	}
	return ""
}

func isLocalTempName(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, ".tmp-")
}

func isLocalNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

func mapLocalError(op string, key Key, err error) error {
	if isLocalNotExist(err) {
		return NotExistError(op, key)
	}
	return err
}
