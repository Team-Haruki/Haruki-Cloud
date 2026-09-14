package assets

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/storage"
)

const (
	assetStatMemoTTL        = 60 * time.Second
	assetStatMemoMaxEntries = 4096
)

// AssetReader is the single place where Cloud turns a Drawing-style asset path
// into bytes. With a store it reads object keys (ObjectKey); without one (a nil
// or Disabled() store) it reproduces today's AssetHelper probing exactly
// (case-insensitive per-segment resolution, multi-root, legacy roots) followed
// by os.ReadFile.
type AssetReader struct {
	helper *AssetHelper
	store  storage.Store
	memo   *assetStatMemo
}

// NewAssetReader builds a reader over helper (legacy branch) and store.
func NewAssetReader(helper *AssetHelper, store storage.Store) *AssetReader {
	return &AssetReader{
		helper: helper,
		store:  store,
		memo:   newAssetStatMemo(assetStatMemoTTL, assetStatMemoMaxEntries),
	}
}

// usesStore reports whether reads go through the store rather than the helper.
func (r *AssetReader) usesStore() bool {
	return r != nil && r.store != nil && r.store != storage.Disabled()
}

// ReadFirst returns the bytes of the first existing candidate. resolved is the
// absolute path (legacy branch) or the object key (store branch) and is meant
// for tracing only. When every candidate is missing it returns
// (nil, "", storage.ErrNotExist); any other store error is returned as is.
func (r *AssetReader) ReadFirst(ctx context.Context, drawingPaths ...string) ([]byte, string, error) {
	ctx = readerContext(ctx)
	if !r.usesStore() {
		return r.readLegacy(ctx, drawingPaths)
	}
	for _, candidate := range drawingPaths {
		key, ok := candidateKey(candidate)
		if !ok {
			continue
		}
		data, err := r.store.Get(ctx, key)
		if err == nil {
			r.memo.store(key, true)
			return data, string(key), nil
		}
		if !errors.Is(err, storage.ErrNotExist) {
			return nil, "", err
		}
	}
	return nil, "", storage.ErrNotExist
}

func (r *AssetReader) readLegacy(ctx context.Context, drawingPaths []string) ([]byte, string, error) {
	resolved := r.firstExistingLegacy(ctx, drawingPaths)
	if resolved == "" {
		return nil, "", storage.ErrNotExist
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, "", err
	}
	return data, resolved, nil
}

func (r *AssetReader) firstExistingLegacy(ctx context.Context, drawingPaths []string) string {
	if r == nil || r.helper == nil {
		return ""
	}
	return r.helper.WithContext(ctx).FirstExisting(drawingPaths...)
}

// Stat answers "does this asset exist?" through the same seam as ReadFirst:
// store branch -> store.Stat(ObjectKey(path)) with a bounded positive+negative
// memo; legacy branch -> AssetHelper.FirstExisting + os.Stat. resolved is the
// object key or the absolute path of the first existing candidate.
func (r *AssetReader) Stat(ctx context.Context, drawingPaths ...string) (string, bool) {
	ctx = readerContext(ctx)
	if !r.usesStore() {
		resolved := r.firstExistingLegacy(ctx, drawingPaths)
		if resolved == "" {
			return "", false
		}
		if _, err := os.Stat(resolved); err != nil {
			return "", false
		}
		return resolved, true
	}
	for _, candidate := range drawingPaths {
		key, ok := candidateKey(candidate)
		if !ok {
			continue
		}
		if exists, cached := r.memo.lookup(key); cached {
			if exists {
				return string(key), true
			}
			continue
		}
		_, err := r.store.Stat(ctx, key)
		switch {
		case err == nil:
			r.memo.store(key, true)
			return string(key), true
		case errors.Is(err, storage.ErrNotExist):
			r.memo.store(key, false)
		}
	}
	return "", false
}

// Exists is a bool wrapper over Stat for one path.
func (r *AssetReader) Exists(ctx context.Context, drawingPath string) bool {
	_, ok := r.Stat(ctx, drawingPath)
	return ok
}

func candidateKey(candidate string) (storage.Key, bool) {
	if strings.TrimSpace(candidate) == "" {
		return "", false
	}
	key, err := ObjectKey(candidate)
	return key, err == nil
}

func readerContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

type assetStatEntry struct {
	exists    bool
	expiresAt time.Time
}

// assetStatMemo is a small process-local existence memo. A successful read
// always records a hit, so the memo never keeps reporting a miss for an
// object that has since been read.
type assetStatMemo struct {
	mu         sync.Mutex
	entries    map[storage.Key]assetStatEntry
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

func newAssetStatMemo(ttl time.Duration, maxEntries int) *assetStatMemo {
	return &assetStatMemo{
		entries:    make(map[storage.Key]assetStatEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

func (m *assetStatMemo) lookup(key storage.Key) (exists, cached bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[key]
	if !ok {
		return false, false
	}
	if !m.now().Before(entry.expiresAt) {
		delete(m.entries, key)
		return false, false
	}
	return entry.exists, true
}

func (m *assetStatMemo) store(key storage.Key, exists bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if _, ok := m.entries[key]; !ok && len(m.entries) >= m.maxEntries {
		for existing, entry := range m.entries {
			if !now.Before(entry.expiresAt) {
				delete(m.entries, existing)
			}
		}
		if len(m.entries) >= m.maxEntries {
			clear(m.entries)
		}
	}
	m.entries[key] = assetStatEntry{exists: exists, expiresAt: now.Add(m.ttl)}
}
