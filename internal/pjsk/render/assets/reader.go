package assets

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"haruki-cloud/internal/observability/commandtrace"
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
// by os.ReadFile. While the helper still has real local roots (a slot derived
// from asset_dirs before E1 blanks Primary), that probing runs first and the
// store is only consulted when it finds nothing, so an unchanged config reads
// exactly what it read before the store existed.
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
		if probe := r.sharedProbe(); probe != nil {
			if probe.metadata != nil {
				resolved, found, authoritative := probe.metadata.Lookup(key)
				if authoritative {
					if !found {
						continue
					}
					key = resolved
				}
			}
			if known, cached := probe.keys.lookup(string(key), probe.now()); cached && known.found {
				key = known.key
			}
		}
		generation := r.memo.currentGeneration()
		var probeGeneration uint64
		if probe := r.sharedProbe(); probe != nil {
			probeGeneration = probe.keys.currentGeneration()
		}
		finishGet := commandtrace.MeasureOperation(ctx, "asset.store_get")
		data, err := r.store.Get(ctx, key)
		finishGet()
		if err == nil {
			r.memo.storeGeneration(key, true, generation)
			if probe := r.sharedProbe(); probe != nil {
				probe.remember(key, storeProbeResult{key: key, found: true}, probeGeneration)
			}
			return data, string(key), nil
		}
		if !errors.Is(err, storage.ErrNotExist) {
			return nil, "", err
		}
		r.memo.storeGeneration(key, false, generation)
		if probe := r.sharedProbe(); probe != nil {
			probe.remember(key, storeProbeResult{}, probeGeneration)
		}
	}
	return nil, "", storage.ErrNotExist
}

func (r *AssetReader) readLegacy(ctx context.Context, drawingPaths []string) ([]byte, string, error) {
	resolved := r.firstExistingLegacy(ctx, drawingPaths)
	if resolved == "" {
		return nil, "", storage.ErrNotExist
	}
	finishRead := commandtrace.MeasureOperation(ctx, "asset.read_file")
	data, err := os.ReadFile(resolved)
	finishRead()
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

// Stat keeps the legacy bool API. A store-backed helper shares its published
// metadata, directory and key caches with the reader. A standalone reader uses
// a bounded HEAD memo. Call StatResult to distinguish unavailable storage from
// a confirmed absent asset.
func (r *AssetReader) Stat(ctx context.Context, drawingPaths ...string) (string, bool) {
	resolved, found, _ := r.StatResult(ctx, drawingPaths...)
	return resolved, found
}

func (r *AssetReader) sharedProbe() *storeProbe {
	if r == nil || r.helper == nil {
		return nil
	}
	return r.helper.store
}

// ClearMetadataCache is called when a resource publication replaces metadata.
// In-flight pre-publication HEAD/Get results cannot repopulate the new memo.
func (r *AssetReader) ClearMetadataCache() {
	if r == nil {
		return
	}
	r.memo.clear()
	r.helper.ClearResolutionCache()
}

// StatResult distinguishes a confirmed missing object from an unavailable
// metadata source. The first unknown candidate stops the ordered search.
func (r *AssetReader) StatResult(ctx context.Context, drawingPaths ...string) (string, bool, error) {
	ctx = readerContext(ctx)
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if !r.usesStore() || r.hasLocalRoots() {
		if resolved, ok := r.statLegacy(ctx, drawingPaths); ok || !r.usesStore() {
			return resolved, ok, nil
		}
	}
	for _, candidate := range drawingPaths {
		key, ok := candidateKey(candidate)
		if !ok {
			continue
		}
		if probe := r.sharedProbe(); probe != nil {
			resolved, found, err := probe.resolve(ctx, key)
			if err != nil {
				return "", false, err
			}
			if found {
				return string(resolved), true, nil
			}
			continue
		}
		exists, err := r.statKey(ctx, key)
		if err != nil {
			return "", false, err
		}
		if exists {
			return string(key), true, nil
		}
	}
	return "", false, nil
}

func (r *AssetReader) statKey(ctx context.Context, key storage.Key) (bool, error) {
	if exists, cached := r.memo.lookup(key); cached {
		return exists, nil
	}
	generation := r.memo.currentGeneration()
	results := r.memo.loads.DoChan(flightKey(string(key), generation), func() (any, error) {
		if exists, cached := r.memo.lookup(key); cached {
			return readerStatResult{exists: exists}, nil
		}
		sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultStoreProbeTimeout)
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedCtx)
		finish := commandtrace.MeasureOperation(sharedCtx, "asset.store_stat")
		_, err := r.store.Stat(sharedCtx, key)
		finish()
		result := readerStatResult{exists: err == nil, err: err, operations: &sharedAssetOperations{stats: trace.Snapshot().Operations}}
		if err == nil || errors.Is(err, storage.ErrNotExist) {
			r.memo.storeGeneration(key, err == nil, generation)
			result.err = nil
		}
		return result, nil
	})
	select {
	case flight := <-results:
		result := flight.Val.(readerStatResult)
		result.operations.merge(ctx)
		return result.exists, result.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

type readerStatResult struct {
	exists     bool
	err        error
	operations *sharedAssetOperations
}

func (r *AssetReader) statLegacy(ctx context.Context, drawingPaths []string) (string, bool) {
	resolved := r.firstExistingLegacy(ctx, drawingPaths)
	if resolved == "" {
		return "", false
	}
	finishStat := commandtrace.MeasureOperation(ctx, "asset.stat")
	_, err := os.Stat(resolved)
	finishStat()
	if err != nil {
		return "", false
	}
	return resolved, true
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
	loads      singleflight.Group
	generation uint64
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

func (m *assetStatMemo) currentGeneration() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generation
}

func (m *assetStatMemo) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.entries)
	m.generation++
}

func (m *assetStatMemo) store(key storage.Key, exists bool) {
	m.storeGeneration(key, exists, m.currentGeneration())
}

func (m *assetStatMemo) storeGeneration(key storage.Key, exists bool, generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != generation {
		return
	}
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
