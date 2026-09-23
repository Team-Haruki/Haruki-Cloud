// Package storage is Haruki-Cloud's file read/write abstraction: one small
// Store interface over forward-slash object keys, with a local filesystem
// backend and (in internal/storage/s3) an S3-compatible backend. Keys name the
// same object on every backend, so moving a slot from local to s3 is a
// configuration change only.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"
)

// DefaultMaxObjectBytes bounds a single object when a backend is built with a
// non-positive limit. It matches the 64 MiB caps used by the HTTP clients.
const DefaultMaxObjectBytes int64 = 64 << 20

// Key is a forward-slash relative object key. A key accepted by CleanKey never
// starts with "/", never contains "." or ".." segments and has no empty
// segments.
type Key string

// Object describes a stored object.
type Object struct {
	Key     Key
	Size    int64
	ModTime time.Time
	ETag    string // "" on local
}

// DirEntry is one immediate child of a ListDir prefix: an object (Dir false,
// Object set) or a common sub-prefix (Dir true, Object zero).
type DirEntry struct {
	// Name is the child's last key segment, without a trailing "/".
	Name   string
	Dir    bool
	Object Object
}

// PutOptions carries per-write metadata.
type PutOptions struct {
	// ContentType is sent by the s3 backend; the local backend ignores it.
	ContentType string
	// CacheControl is honoured by the s3 backend only. No Cloud call site sets
	// it: node Caddy owns Cache-Control for the public buckets.
	CacheControl string
}

// Store is the whole storage interface. Implementations are safe for
// concurrent use. List visits objects whose key starts with prefix (plain
// string-prefix semantics, "" lists everything) in unspecified order; an error
// returned by fn aborts the listing and is returned unchanged.
type Store interface {
	Get(ctx context.Context, key Key) ([]byte, error)
	Put(ctx context.Context, key Key, data []byte, opts PutOptions) error
	Stat(ctx context.Context, key Key) (Object, error)
	// Delete removes key; a missing key is not an error.
	Delete(ctx context.Context, key Key) error
	List(ctx context.Context, prefix Key, fn func(Object) error) error
	// ListDir visits the immediate children of a directory prefix ("" is the
	// root; a missing trailing "/" is added): the objects directly under it
	// and, once each, the common sub-prefixes (S3 delimiter "/" semantics).
	// Order is unspecified; a missing directory visits nothing. An error
	// returned by fn aborts the listing and is returned unchanged.
	ListDir(ctx context.Context, prefix Key, fn func(DirEntry) error) error
}

// ErrNotExist is fs.ErrNotExist so existing errors.Is checks keep working.
var ErrNotExist = fs.ErrNotExist

// ErrNotConfigured is returned by Disabled stores. It wraps fs.ErrNotExist so a
// caller that only asks "does this exist?" degrades to "no".
var ErrNotConfigured = fmt.Errorf("storage: backend not configured: %w", fs.ErrNotExist)

// ErrTooLarge is returned when an object exceeds the backend's size limit.
var ErrTooLarge = errors.New("storage: object exceeds size limit")

// ErrInvalidKey is returned for keys rejected by CleanKey.
var ErrInvalidKey = errors.New("storage: invalid key")

type disabledStore struct{}

// Disabled returns a Store whose every method returns ErrNotConfigured.
func Disabled() Store { return disabledStore{} }

func (disabledStore) Get(context.Context, Key) ([]byte, error) { return nil, ErrNotConfigured }

func (disabledStore) Put(context.Context, Key, []byte, PutOptions) error { return ErrNotConfigured }

func (disabledStore) Stat(context.Context, Key) (Object, error) { return Object{}, ErrNotConfigured }

func (disabledStore) Delete(context.Context, Key) error { return ErrNotConfigured }

func (disabledStore) List(context.Context, Key, func(Object) error) error {
	return ErrNotConfigured
}

func (disabledStore) ListDir(context.Context, Key, func(DirEntry) error) error {
	return ErrNotConfigured
}

// NotExistError builds the error backends return for a missing object. It
// satisfies both errors.Is(err, ErrNotExist) and os.IsNotExist(err).
func NotExistError(op string, key Key) error {
	return &fs.PathError{Op: op, Path: string(key), Err: fs.ErrNotExist}
}

// TooLargeError builds the error backends return for an oversize object.
func TooLargeError(op string, key Key, size, limit int64) error {
	return fmt.Errorf("storage: %s %q: %d bytes over limit %d: %w", op, key, size, limit, ErrTooLarge)
}
