package assets

import (
	"context"
	"errors"
	"os"
	"strings"

	"haruki-cloud/internal/storage"
)

// AssetReader is the single place where Cloud turns a Drawing-style asset path
// into bytes. With a store it reads object keys (ObjectKey); without one (a nil
// or Disabled() store) it reproduces today's AssetHelper probing exactly
// (case-insensitive per-segment resolution, multi-root, legacy roots) followed
// by os.ReadFile. While the helper still has real local roots (a slot derived
// from asset_dirs before E1 blanks Primary), that probing runs first and the
// store is only consulted when it finds nothing, so an unchanged config reads
// exactly what it read before the store existed.
type AssetReader struct {
	helper *AssetHelper
	store  storage.Store
}

// NewAssetReader builds a reader over helper (legacy branch) and store.
func NewAssetReader(helper *AssetHelper, store storage.Store) *AssetReader {
	return &AssetReader{helper: helper, store: store}
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
	if r.hasLocalRoots() {
		// Local roots still exist (a legacy-derived slot before E1 blanks
		// Primary): keep today's multi-root, case-insensitive lookup first and
		// only fall back to the store when it finds nothing.
		data, resolved, err := r.readLegacy(ctx, drawingPaths)
		if !errors.Is(err, storage.ErrNotExist) {
			return data, resolved, err
		}
	}
	for _, candidate := range drawingPaths {
		key, ok := candidateKey(candidate)
		if !ok {
			continue
		}
		data, err := r.store.Get(ctx, key)
		if err == nil {
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

// hasLocalRoots reports whether the reader's helper probes a configured local
// root. A helper built without any root falls back to "." and does not count.
func (r *AssetReader) hasLocalRoots() bool {
	return r != nil && helperHasLocalRoots(r.helper)
}

// StoreOnly reports whether r answers exclusively from its store: a store is
// configured and no local asset root is left to probe first.
func (r *AssetReader) StoreOnly() bool {
	return r.usesStore() && !r.hasLocalRoots()
}

func helperHasLocalRoots(helper *AssetHelper) bool {
	if helper == nil || len(helper.roots) == 0 {
		return false
	}
	return len(helper.roots) > 1 || helper.roots[0] != "."
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
