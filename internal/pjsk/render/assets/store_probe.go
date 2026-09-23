package assets

import (
	"container/list"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"
)

// StoreProbeConfig tunes the store-backed path probe (pjsk_render.asset_probe).
// Non-positive fields take the defaults.
type StoreProbeConfig struct {
	// PositiveTTL is how long a resolved key and a directory listing stay
	// cached.
	PositiveTTL time.Duration
	// NegativeTTL is how long a miss stays cached.
	NegativeTTL time.Duration
	// Timeout bounds one store call: a Stat or one directory listing. The
	// backend's own stat_timeout / request_timeout still apply per attempt.
	Timeout time.Duration
}

// Defaults for StoreProbeConfig.
const (
	DefaultStoreProbePositiveTTL = 6 * time.Hour
	DefaultStoreProbeNegativeTTL = 10 * time.Minute
	DefaultStoreProbeTimeout     = 5 * time.Second
)

const (
	// storeProbeErrorTTL bounds how often a failing store is retried per key.
	storeProbeErrorTTL = 30 * time.Second
	// storeProbeMaxDirNames caps the children indexed per directory; a larger
	// directory is listed once, remembered as too large and never
	// case-corrected. The biggest production directories (card thumbnails,
	// member images) hold a few thousand children.
	storeProbeMaxDirNames = 20_000
	storeProbeLogInterval = time.Minute
)

func (c StoreProbeConfig) withDefaults() StoreProbeConfig {
	if c.PositiveTTL <= 0 {
		c.PositiveTTL = DefaultStoreProbePositiveTTL
	}
	if c.NegativeTTL <= 0 {
		c.NegativeTTL = DefaultStoreProbeNegativeTTL
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultStoreProbeTimeout
	}
	return c
}

// storeProbe answers "which of these keys exists, and with which exact
// casing?" against the assets store. Every outcome is cached in a bounded LRU
// (positive, negative and error entries with their own TTLs), concurrent
// probes of one key share a single store round trip, and each store call is
// bounded by cfg.Timeout on a detached context so a cancelled request never
// aborts the shared flight.
type storeProbe struct {
	store       storage.Store
	cfg         StoreProbeConfig
	log         *logger.Logger
	now         func() time.Time
	maxDirNames int

	keys       *probeCache[storeProbeResult]
	dirs       *probeCache[*storeDirIndex]
	keyFlights singleflight.Group
	dirFlights singleflight.Group

	lastLogNano atomic.Int64
	suppressed  atomic.Uint64
}

type storeProbeResult struct {
	key        storage.Key
	found      bool
	err        error
	operations []commandtrace.Stats
}

func newStoreProbe(store storage.Store, cfg StoreProbeConfig, log *logger.Logger) *storeProbe {
	return &storeProbe{
		store:       store,
		cfg:         cfg.withDefaults(),
		log:         log,
		now:         time.Now,
		maxDirNames: storeProbeMaxDirNames,
		keys:        newProbeCache[storeProbeResult](assetResolutionMaxEntries, 0),
		dirs:        newProbeCache[*storeDirIndex](assetDirectoryMaxEntries, assetDirectoryMaxNames),
	}
}

// resolve returns the key that answers key: key itself when it exists, or the
// case-corrected key when only a differently cased object does. found is
// false for a definite miss; err reports a store failure (also cached
// briefly), which callers treat as "unknown" and fall back on.
func (p *storeProbe) resolve(ctx context.Context, key storage.Key) (resolved storage.Key, found bool, err error) {
	if p == nil || key == "" {
		return "", false, nil
	}
	if cached, ok := p.keys.lookup(string(key), p.now()); ok {
		commandtrace.RecordOperation(ctx, "asset.store_probe_cache_hit", 0)
		return cached.key, cached.found, cached.err
	}
	commandtrace.RecordOperation(ctx, "asset.store_probe_cache_miss", 0)
	finishWait := commandtrace.MeasureOperation(ctx, "asset.store_probe_wait")
	defer finishWait()
	results := p.keyFlights.DoChan(string(key), func() (any, error) {
		return p.resolveUncached(key), nil
	})
	select {
	case flight := <-results:
		result, _ := flight.Val.(storeProbeResult)
		if flight.Shared {
			commandtrace.RecordOperation(ctx, "asset.store_probe_shared", 0)
		}
		commandtrace.MergeOperations(ctx, result.operations)
		return result.key, result.found, result.err
	case <-ctx.Done():
		return "", false, ctx.Err()
	}
}

func (p *storeProbe) resolveUncached(key storage.Key) storeProbeResult {
	if cached, ok := p.keys.lookup(string(key), p.now()); ok {
		return cached
	}
	sharedCtx, trace := commandtrace.WithNewTrace(context.Background())
	result := p.probe(sharedCtx, key)
	result.operations = trace.Snapshot().Operations
	p.remember(key, result)
	if result.err != nil {
		p.logError(key, result.err)
	}
	return result
}

func (p *storeProbe) probe(ctx context.Context, key storage.Key) storeProbeResult {
	err := p.stat(ctx, key)
	switch {
	case err == nil:
		return storeProbeResult{key: key, found: true}
	case !errors.Is(err, storage.ErrNotExist):
		return storeProbeResult{err: err}
	}
	corrected, found, err := p.correctCase(ctx, key)
	if err != nil {
		return storeProbeResult{err: err}
	}
	return storeProbeResult{key: corrected, found: found}
}

func (p *storeProbe) stat(ctx context.Context, key storage.Key) error {
	callCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	startedAt := time.Now()
	_, err := p.store.Stat(callCtx, key)
	commandtrace.RecordOperation(ctx, "asset.store_stat", time.Since(startedAt))
	return err
}

// correctCase walks key one segment at a time through cached directory
// listings, substituting the stored spelling of each segment that only
// differs in case. It mirrors AssetHelper.resolveCaseInsensitivePath.
func (p *storeProbe) correctCase(ctx context.Context, key storage.Key) (storage.Key, bool, error) {
	segments := strings.Split(string(key), "/")
	parent := ""
	for idx, segment := range segments {
		index, err := p.dirIndex(ctx, parent)
		if err != nil {
			return "", false, err
		}
		if index.truncated {
			return "", false, nil
		}
		last := idx == len(segments)-1
		names := &index.dirs
		if last {
			names = &index.objects
		}
		name, ok := names.match(segment)
		if !ok {
			return "", false, nil
		}
		if last {
			return storage.Key(parent + name), true, nil
		}
		parent += name + "/"
	}
	return "", false, nil
}

func (p *storeProbe) dirIndex(ctx context.Context, parent string) (*storeDirIndex, error) {
	if cached, ok := p.dirs.lookup(parent, p.now()); ok {
		return cached, nil
	}
	value, err, _ := p.dirFlights.Do(parent, func() (any, error) {
		if cached, ok := p.dirs.lookup(parent, p.now()); ok {
			return cached, nil
		}
		index, err := p.listDir(ctx, parent)
		if err != nil {
			return nil, err
		}
		p.dirs.store(parent, index, index.count, p.now().Add(p.cfg.PositiveTTL))
		return index, nil
	})
	if err != nil {
		return nil, err
	}
	index, _ := value.(*storeDirIndex)
	return index, nil
}

var errStoreDirTruncated = errors.New("assets: directory listing truncated")

func (p *storeProbe) listDir(ctx context.Context, parent string) (*storeDirIndex, error) {
	callCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	index := newStoreDirIndex()
	startedAt := time.Now()
	err := p.store.ListDir(callCtx, storage.Key(parent), func(entry storage.DirEntry) error {
		if index.count >= p.maxDirNames {
			index.truncated = true
			return errStoreDirTruncated
		}
		index.add(entry)
		return nil
	})
	commandtrace.RecordOperation(ctx, "asset.store_list", time.Since(startedAt))
	if err != nil && !errors.Is(err, errStoreDirTruncated) && !errors.Is(err, storage.ErrNotExist) {
		return nil, err
	}
	if index.truncated {
		index.objects, index.dirs = storeDirNames{}, storeDirNames{}
	}
	return index, nil
}

func (p *storeProbe) remember(key storage.Key, result storeProbeResult) {
	ttl := p.cfg.NegativeTTL
	switch {
	case result.err != nil:
		ttl = storeProbeErrorTTL
	case result.found:
		ttl = p.cfg.PositiveTTL
	}
	stored := result
	stored.operations = nil
	p.keys.store(string(key), stored, 1, p.now().Add(ttl))
}

// logError warns at most once per storeProbeLogInterval and folds the
// suppressed count into the next line.
func (p *storeProbe) logError(key storage.Key, err error) {
	now := p.now().UnixNano()
	last := p.lastLogNano.Load()
	if now-last < int64(storeProbeLogInterval) || !p.lastLogNano.CompareAndSwap(last, now) {
		p.suppressed.Add(1)
		return
	}
	suppressed := p.suppressed.Swap(0)
	p.log.Warn("asset store probe failed; falling back to the first candidate path",
		"key", string(key), "error", err, "suppressed", suppressed)
}

func (p *storeProbe) clear() {
	if p == nil {
		return
	}
	p.keys.clear()
	p.dirs.clear()
}

// storeDirNames indexes one directory's children by exact and case-folded
// name. An exact match always wins; among case-fold collisions the first
// listed name is kept, as newAssetDirectoryIndex does on disk.
type storeDirNames struct {
	exact  map[string]struct{}
	folded map[string]string
}

func (n *storeDirNames) add(name string) {
	if n.exact == nil {
		n.exact = make(map[string]struct{})
		n.folded = make(map[string]string)
	}
	n.exact[name] = struct{}{}
	folded := foldAssetName(name)
	if _, exists := n.folded[folded]; !exists {
		n.folded[folded] = name
	}
}

func (n *storeDirNames) match(segment string) (string, bool) {
	if _, ok := n.exact[segment]; ok {
		return segment, true
	}
	name, ok := n.folded[foldAssetName(segment)]
	return name, ok
}

type storeDirIndex struct {
	objects   storeDirNames
	dirs      storeDirNames
	count     int
	truncated bool
}

func newStoreDirIndex() *storeDirIndex {
	return &storeDirIndex{}
}

func (i *storeDirIndex) add(entry storage.DirEntry) {
	if entry.Name == "" {
		return
	}
	if entry.Dir {
		i.dirs.add(entry.Name)
	} else {
		i.objects.add(entry.Name)
	}
	i.count++
}

// probeCache is a bounded LRU with per-entry expiry and an optional weight
// budget (used to cap the total number of indexed directory names).
type probeCache[V any] struct {
	mu         sync.Mutex
	entries    map[string]*probeCacheEntry[V]
	recent     list.List
	maxEntries int
	maxWeight  int
	weight     int
}

type probeCacheEntry[V any] struct {
	value     V
	weight    int
	expiresAt time.Time
	element   *list.Element
}

func newProbeCache[V any](maxEntries, maxWeight int) *probeCache[V] {
	return &probeCache[V]{
		entries:    make(map[string]*probeCacheEntry[V]),
		maxEntries: maxEntries,
		maxWeight:  maxWeight,
	}
}

func (c *probeCache[V]) lookup(key string, now time.Time) (V, bool) {
	var zero V
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return zero, false
	}
	if !now.Before(entry.expiresAt) {
		c.removeLocked(key, entry)
		return zero, false
	}
	c.recent.MoveToFront(entry.element)
	return entry.value, true
}

func (c *probeCache[V]) store(key string, value V, weight int, expiresAt time.Time) {
	if c.maxWeight > 0 && weight > c.maxWeight {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if current, ok := c.entries[key]; ok {
		c.removeLocked(key, current)
	}
	entry := &probeCacheEntry[V]{value: value, weight: weight, expiresAt: expiresAt}
	entry.element = c.recent.PushFront(key)
	c.entries[key] = entry
	c.weight += weight
	for len(c.entries) > c.maxEntries || (c.maxWeight > 0 && c.weight > c.maxWeight) {
		oldest := c.recent.Back()
		if oldest == nil {
			break
		}
		oldestKey, _ := oldest.Value.(string)
		c.removeLocked(oldestKey, c.entries[oldestKey])
	}
}

func (c *probeCache[V]) removeLocked(key string, entry *probeCacheEntry[V]) {
	delete(c.entries, key)
	if entry == nil {
		return
	}
	c.weight -= entry.weight
	if c.weight < 0 {
		c.weight = 0
	}
	if entry.element != nil {
		c.recent.Remove(entry.element)
		entry.element = nil
	}
}

func (c *probeCache[V]) clear() {
	c.mu.Lock()
	clear(c.entries)
	c.recent.Init()
	c.weight = 0
	c.mu.Unlock()
}

func (c *probeCache[V]) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
