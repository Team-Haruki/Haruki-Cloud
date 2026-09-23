package assets

import (
	"container/list"
	"context"
	"errors"
	"strconv"
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
// Non-positive durations take the defaults.
type StoreProbeConfig struct {
	// PositiveTTL is how long a resolved key stays cached.
	PositiveTTL time.Duration
	// ListingTTL is how long a directory listing stays cached.
	ListingTTL time.Duration
	// NegativeTTL is how long a miss stays cached, and how old a listing may
	// be before a miss in it re-lists the directory once.
	NegativeTTL time.Duration
	// Timeout bounds one store call: a HEAD or one directory listing. The
	// backend's own stat_timeout / request_timeout still apply per attempt.
	Timeout time.Duration
	// WarmPrefixes are directories (bucket keys, e.g.
	// "jp-assets/startapp/thumbnail/chara") listed in the background at
	// startup so the first renders find them cached.
	WarmPrefixes []string
}

// Defaults for StoreProbeConfig.
const (
	DefaultStoreProbePositiveTTL = 6 * time.Hour
	DefaultStoreProbeListingTTL  = 30 * time.Minute
	DefaultStoreProbeNegativeTTL = 5 * time.Minute
	DefaultStoreProbeTimeout     = 3 * time.Second
)

const (
	// storeProbeErrorTTL bounds how often a key whose HEAD failed is retried.
	storeProbeErrorTTL = 30 * time.Second
	// storeProbeMaxListEntries caps one directory listing at about 20
	// ListObjectsV2 pages (max-keys 1000). A larger directory is remembered
	// as unavailable for NegativeTTL and answered by HEAD instead.
	storeProbeMaxListEntries = 20_000
	// storeProbeBulkMinChildren is the sub-directory count from which a
	// directory is listed recursively once, in the background and under its
	// own timeout (storeProbeBulkMaxObjects, about 10 pages), to index every
	// child at once, e.g. music/jacket/<n>/<n>.png.
	storeProbeBulkMinChildren = 64
	storeProbeBulkMaxObjects  = 10_000
	storeProbeBulkTimeout     = 10 * time.Second
	// storeProbeMaxDirEntries bounds cached directory indexes by count; the
	// name budget (assetDirectoryMaxNames) bounds their memory.
	storeProbeMaxDirEntries = 65_536
	// The circuit breaker opens after storeProbeBreakerThreshold consecutive
	// store failures and lets one probe through after
	// storeProbeBreakerCooldown.
	storeProbeBreakerThreshold = 3
	storeProbeBreakerCooldown  = 30 * time.Second
	storeProbeLogInterval      = time.Minute
)

var (
	errStoreProbeOpen  = errors.New("assets: store probe circuit open")
	errStoreListCapped = errors.New("assets: listing capped")
)

func (c StoreProbeConfig) withDefaults() StoreProbeConfig {
	if c.PositiveTTL <= 0 {
		c.PositiveTTL = DefaultStoreProbePositiveTTL
	}
	if c.ListingTTL <= 0 {
		c.ListingTTL = DefaultStoreProbeListingTTL
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
// casing?" against the assets store. Existence is read from cached directory
// listings (one ListDir per directory answers every sibling, exact or
// case-folded), so a cold render costs one listing per directory level rather
// than one HEAD per asset; HEAD is only the fallback for directories that
// cannot be listed. Outcomes live in bounded LRUs, concurrent probes of one
// key or one directory share a flight on a detached context, every store call
// is bounded by cfg.Timeout, and a circuit breaker skips the store while it
// keeps failing.
type storeProbe struct {
	store           storage.Store
	cfg             StoreProbeConfig
	log             *logger.Logger
	now             func() time.Time
	maxListEntries  int
	bulkMinChildren int
	bulkMaxObjects  int

	keys       *probeCache[storeProbeResult]
	dirs       *probeCache[*storeDirIndex]
	keyFlights singleflight.Group
	dirFlights singleflight.Group
	breaker    storeBreaker

	bulkMu      sync.Mutex
	bulkRunning map[string]struct{}
	bulkWait    sync.WaitGroup

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
		store:           store,
		cfg:             cfg.withDefaults(),
		log:             log,
		now:             time.Now,
		maxListEntries:  storeProbeMaxListEntries,
		bulkMinChildren: storeProbeBulkMinChildren,
		bulkMaxObjects:  storeProbeBulkMaxObjects,
		keys:            newProbeCache[storeProbeResult](assetResolutionMaxEntries, 0),
		dirs:            newProbeCache[*storeDirIndex](storeProbeMaxDirEntries, assetDirectoryMaxNames),
		breaker:         storeBreaker{threshold: storeProbeBreakerThreshold, cooldown: storeProbeBreakerCooldown},
		bulkRunning:     make(map[string]struct{}),
	}
}

// resolve returns the key that answers key: key itself when it exists, or the
// case-corrected key when only a differently cased object does. found is
// false for a definite miss; err reports a store failure or an open circuit,
// which callers treat as "unknown" and fall back on.
func (p *storeProbe) resolve(ctx context.Context, key storage.Key) (resolved storage.Key, found bool, err error) {
	if p == nil || key == "" {
		return "", false, nil
	}
	now := p.now()
	if cached, ok := p.keys.lookup(string(key), now); ok {
		commandtrace.RecordOperation(ctx, "asset.store_probe_cache_hit", 0)
		return cached.key, cached.found, cached.err
	}
	if p.breaker.blocked(now) {
		commandtrace.RecordOperation(ctx, "asset.store_probe_circuit_open", 0)
		return "", false, errStoreProbeOpen
	}
	commandtrace.RecordOperation(ctx, "asset.store_probe_cache_miss", 0)
	finishWait := commandtrace.MeasureOperation(ctx, "asset.store_probe_wait")
	defer finishWait()
	generation := p.keys.currentGeneration()
	results := p.keyFlights.DoChan(flightKey(string(key), generation), func() (any, error) {
		return p.resolveUncached(key, generation), nil
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

func flightKey(name string, generation uint64) string {
	return name + "\x00" + strconv.FormatUint(generation, 10)
}

func (p *storeProbe) resolveUncached(key storage.Key, generation uint64) storeProbeResult {
	if cached, ok := p.keys.lookup(string(key), p.now()); ok {
		return cached
	}
	sharedCtx, trace := commandtrace.WithNewTrace(context.Background())
	result := p.probe(sharedCtx, key)
	result.operations = trace.Snapshot().Operations
	if !errors.Is(result.err, errStoreProbeOpen) {
		p.remember(key, result, generation)
	}
	if result.err != nil {
		p.logError(key, result.err)
	}
	return result
}

// probe walks key one segment at a time through cached directory listings,
// substituting the stored spelling of each segment. A directory that cannot
// be listed hands the rest of the path to HEAD.
func (p *storeProbe) probe(ctx context.Context, key storage.Key) storeProbeResult {
	segments := strings.Split(string(key), "/")
	parent := ""
	for idx, segment := range segments {
		last := idx == len(segments)-1
		index, err := p.dirIndex(ctx, parent, time.Time{})
		if err != nil {
			return storeProbeResult{err: err}
		}
		if index.unavailable {
			return p.stat(ctx, storage.Key(parent+strings.Join(segments[idx:], "/")))
		}
		name, ok := index.match(segment, last)
		if !ok && p.now().Sub(index.listedAt) > p.cfg.NegativeTTL {
			// The listing predates the negative TTL: the asset may have been
			// published since. Re-list once, then trust the answer.
			if index, err = p.dirIndex(ctx, parent, index.listedAt); err != nil {
				return storeProbeResult{err: err}
			}
			if index.unavailable {
				return p.stat(ctx, storage.Key(parent+strings.Join(segments[idx:], "/")))
			}
			name, ok = index.match(segment, last)
		}
		if !ok {
			return storeProbeResult{}
		}
		if last {
			return storeProbeResult{key: storage.Key(parent + name), found: true}
		}
		parent += name + "/"
	}
	return storeProbeResult{}
}

func (p *storeProbe) stat(ctx context.Context, key storage.Key) storeProbeResult {
	err := p.call(ctx, "asset.store_stat", func(ctx context.Context) error {
		_, err := p.store.Stat(ctx, key)
		return err
	})
	switch {
	case err == nil:
		return storeProbeResult{key: key, found: true}
	case errors.Is(err, storage.ErrNotExist):
		return storeProbeResult{}
	default:
		return storeProbeResult{err: err}
	}
}

// call runs one store round trip under the circuit breaker and cfg.Timeout.
// A missing object or a capped listing is a healthy answer; a call cut short
// because the caller's own context ended (shutdown, warm-up cancelled) says
// nothing about the store and is not counted.
func (p *storeProbe) call(ctx context.Context, op string, fn func(context.Context) error) error {
	if !p.breaker.allow(p.now()) {
		return errStoreProbeOpen
	}
	callCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	startedAt := time.Now()
	err := fn(callCtx)
	commandtrace.RecordOperation(ctx, op, time.Since(startedAt))
	if err == nil || errors.Is(err, storage.ErrNotExist) || errors.Is(err, errStoreListCapped) {
		p.breaker.success()
		return err
	}
	if ctx.Err() != nil {
		p.breaker.release()
		return err
	}
	if p.breaker.failure(p.now()) {
		p.log.Warn("asset store probe circuit opened; skipping the store",
			"cooldown", p.breaker.cooldown.String(), "error", err)
	}
	return err
}

// dirIndex returns the cached listing of parent ("" is the root, otherwise a
// key with a trailing "/"). A cached listing no newer than staleBefore is
// replaced, so a caller that saw a miss in an old listing can ask for a fresh
// one; concurrent callers share the flight.
func (p *storeProbe) dirIndex(ctx context.Context, parent string, staleBefore time.Time) (*storeDirIndex, error) {
	if cached, ok := p.dirs.lookup(parent, p.now()); ok && cached.listedAt.After(staleBefore) {
		return cached, nil
	}
	generation := p.dirs.currentGeneration()
	value, err, _ := p.dirFlights.Do(flightKey(parent, generation), func() (any, error) {
		if cached, ok := p.dirs.lookup(parent, p.now()); ok && cached.listedAt.After(staleBefore) {
			return cached, nil
		}
		if staleBefore.IsZero() {
			p.startBulkList(parent, generation)
		}
		index := p.listDir(ctx, parent)
		if index.err != nil {
			return nil, index.err
		}
		p.rememberDir(parent, index, generation)
		return index, nil
	})
	if err != nil {
		return nil, err
	}
	index, _ := value.(*storeDirIndex)
	return index, nil
}

func (p *storeProbe) rememberDir(parent string, index *storeDirIndex, generation uint64) {
	ttl := p.cfg.ListingTTL
	if index.unavailable {
		ttl = p.cfg.NegativeTTL
	}
	p.dirs.store(parent, index, index.weight(), index.listedAt.Add(ttl), generation)
}

// listDir lists one directory. A capped or failed listing yields an
// unavailable index (remembered for NegativeTTL) so the probe falls back to
// HEAD there; only an open circuit is returned as an error.
func (p *storeProbe) listDir(ctx context.Context, parent string) *storeDirIndex {
	index := &storeDirIndex{}
	err := p.call(ctx, "asset.store_list", func(ctx context.Context) error {
		return p.store.ListDir(ctx, storage.Key(parent), func(entry storage.DirEntry) error {
			if index.count >= p.maxListEntries {
				return errStoreListCapped
			}
			index.add(entry)
			return nil
		})
	})
	index.listedAt = p.now()
	switch {
	case err == nil || errors.Is(err, storage.ErrNotExist):
		return index
	case errors.Is(err, errStoreProbeOpen):
		return &storeDirIndex{err: err}
	case errors.Is(err, errStoreListCapped):
		return &storeDirIndex{listedAt: index.listedAt, unavailable: true}
	default:
		p.logError(storage.Key(parent), err)
		return &storeDirIndex{listedAt: index.listedAt, unavailable: true}
	}
}

func bulkMarker(grand string) string {
	return "bulk\x00" + grand
}

// bulkCandidate returns the directory containing dir when it is wide
// (bulkMinChildren sub-directories), already listed, not yet bulk-listed
// and the breaker is closed. The root is never listed recursively.
func (p *storeProbe) bulkCandidate(dir string) (string, bool) {
	grand, ok := parentDir(dir)
	if !ok || grand == "" {
		return "", false
	}
	now := p.now()
	if p.breaker.blocked(now) {
		return "", false
	}
	grandIndex, ok := p.dirs.lookup(grand, now)
	if !ok || grandIndex.unavailable || len(grandIndex.dirs.exact) < p.bulkMinChildren {
		return "", false
	}
	if _, done := p.dirs.lookup(bulkMarker(grand), now); done {
		return "", false
	}
	return grand, true
}

// startBulkList indexes every child directory of parent's parent in the
// background when that directory is wide (music/jacket/<name>/<name>.png
// has hundreds of one-file children): one bounded recursive List replaces
// one ListDir per child for every later sibling, while the request that
// noticed the directory proceeds with its own listing. At most one bulk
// listing per directory runs at a time.
func (p *storeProbe) startBulkList(parent string, generation uint64) {
	grand, ok := p.bulkCandidate(parent)
	if !ok {
		return
	}
	p.bulkWait.Add(1)
	go func() {
		defer p.bulkWait.Done()
		p.runBulkList(context.Background(), grand, generation)
	}()
}

func (p *storeProbe) runBulkList(ctx context.Context, grand string, generation uint64) {
	p.bulkMu.Lock()
	if _, running := p.bulkRunning[grand]; running {
		p.bulkMu.Unlock()
		return
	}
	p.bulkRunning[grand] = struct{}{}
	p.bulkMu.Unlock()
	defer func() {
		p.bulkMu.Lock()
		delete(p.bulkRunning, grand)
		p.bulkMu.Unlock()
	}()
	if _, done := p.dirs.lookup(bulkMarker(grand), p.now()); done {
		return
	}
	grandIndex, ok := p.dirs.lookup(grand, p.now())
	if !ok {
		return
	}
	children, complete, err := p.listRecursive(ctx, grand)
	if err != nil && ctx.Err() != nil {
		return
	}
	listedAt := p.now()
	if complete {
		for name := range grandIndex.dirs.exact {
			if _, ok := children[name]; !ok {
				children[name] = &storeDirIndex{}
			}
		}
		for name, index := range children {
			index.listedAt = listedAt
			p.rememberDir(grand+name+"/", index, generation)
		}
	}
	// A subtree over the object cap stays over it: do not try again before
	// PositiveTTL. A failed listing is retried after NegativeTTL.
	ttl := p.cfg.ListingTTL
	switch {
	case err != nil:
		ttl = p.cfg.NegativeTTL
	case !complete:
		ttl = p.cfg.PositiveTTL
	}
	p.dirs.store(bulkMarker(grand), &storeDirIndex{listedAt: listedAt, unavailable: !complete}, 1, listedAt.Add(ttl), generation)
}

// listRecursive lists every object under grand (a directory prefix) and
// groups them into per-child indexes. It runs under storeProbeBulkTimeout and
// outside the circuit breaker: a slow or failed bulk listing costs nothing
// but itself. complete is false when the object cap was hit.
func (p *storeProbe) listRecursive(ctx context.Context, grand string) (map[string]*storeDirIndex, bool, error) {
	children := make(map[string]*storeDirIndex)
	objects := 0
	callCtx, cancel := context.WithTimeout(ctx, storeProbeBulkTimeout)
	defer cancel()
	startedAt := time.Now()
	err := p.store.List(callCtx, storage.Key(grand), func(object storage.Object) error {
		if objects >= p.bulkMaxObjects {
			return errStoreListCapped
		}
		objects++
		rel := strings.TrimPrefix(string(object.Key), grand)
		child, rest, nested := strings.Cut(rel, "/")
		if !nested || child == "" || rest == "" {
			return nil
		}
		index := children[child]
		if index == nil {
			index = &storeDirIndex{}
			children[child] = index
		}
		if name, _, deeper := strings.Cut(rest, "/"); deeper {
			index.add(storage.DirEntry{Name: name, Dir: true})
		} else {
			index.add(storage.DirEntry{Name: rest})
		}
		return nil
	})
	commandtrace.RecordOperation(ctx, "asset.store_list_bulk", time.Since(startedAt))
	switch {
	case err == nil:
		return children, true, nil
	case errors.Is(err, errStoreListCapped):
		return nil, false, nil
	default:
		if ctx.Err() == nil {
			p.logError(storage.Key(grand), err)
		}
		return nil, false, err
	}
}

// parentDir returns the directory containing dir ("a/b/c/" -> "a/b/",
// "a/" -> ""); the root has no parent.
func parentDir(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	trimmed := strings.TrimSuffix(dir, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[:idx+1], true
	}
	return "", true
}

// warm lists cfg.WarmPrefixes and the directories above them, and runs the
// bulk listing of a wide prefix synchronously, so the first renders after a
// restart find their listings cached.
func (p *storeProbe) warm(ctx context.Context) {
	for _, raw := range p.cfg.WarmPrefixes {
		prefix, err := storage.CleanDirPrefix(raw)
		if err != nil || ctx.Err() != nil {
			continue
		}
		parent := ""
		if prefix != "" {
			for _, segment := range strings.Split(strings.TrimSuffix(string(prefix), "/"), "/") {
				if _, err := p.dirIndex(ctx, parent, time.Time{}); err != nil {
					break
				}
				parent += segment + "/"
			}
		}
		index, err := p.dirIndex(ctx, parent, time.Time{})
		if err != nil || index.unavailable {
			continue
		}
		for name := range index.dirs.exact {
			if grand, ok := p.bulkCandidate(parent + name + "/"); ok {
				p.runBulkList(ctx, grand, p.dirs.currentGeneration())
			}
			break
		}
	}
}

func (p *storeProbe) remember(key storage.Key, result storeProbeResult, generation uint64) {
	ttl := p.cfg.NegativeTTL
	switch {
	case result.err != nil:
		ttl = storeProbeErrorTTL
	case result.found:
		ttl = p.cfg.PositiveTTL
	}
	stored := result
	stored.operations = nil
	p.keys.store(string(key), stored, 1, p.now().Add(ttl), generation)
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

// clear drops every cached outcome. Flights started before the clear cannot
// repopulate the caches: they carry the previous generation.
func (p *storeProbe) clear() {
	if p == nil {
		return
	}
	p.keys.clear()
	p.dirs.clear()
}

// storeBreaker is a small circuit breaker: threshold consecutive failures
// open it for cooldown, after which exactly one probe is let through
// (half-open); its success closes the breaker, its failure re-opens it.
type storeBreaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	failures  int
	openUntil time.Time
	probing   bool
}

// blocked reports whether a probe would be refused right now, without
// claiming the half-open slot.
func (b *storeBreaker) blocked(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() {
		return false
	}
	return now.Before(b.openUntil) || b.probing
}

func (b *storeBreaker) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() {
		return true
	}
	if now.Before(b.openUntil) || b.probing {
		return false
	}
	b.probing = true
	return true
}

// release gives back the half-open slot after a call that ended for a
// reason unrelated to the store.
func (b *storeBreaker) release() {
	b.mu.Lock()
	b.probing = false
	b.mu.Unlock()
}

func (b *storeBreaker) success() {
	b.mu.Lock()
	b.failures = 0
	b.openUntil = time.Time{}
	b.probing = false
	b.mu.Unlock()
}

// failure records one failed call and reports whether it opened the breaker.
func (b *storeBreaker) failure(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	b.probing = false
	if b.failures < b.threshold {
		return false
	}
	wasOpen := !b.openUntil.IsZero() && now.Before(b.openUntil)
	b.openUntil = now.Add(b.cooldown)
	return !wasOpen
}

// storeDirNames indexes one directory's children by exact and case-folded
// name. An exact match always wins; among case-fold collisions the first
// listed name is kept, as newAssetDirectoryIndex does on disk.
type storeDirNames struct {
	exact  map[string]struct{}
	folded map[string]string
}

func (n *storeDirNames) add(name string) bool {
	if n.exact == nil {
		n.exact = make(map[string]struct{})
		n.folded = make(map[string]string)
	}
	if _, exists := n.exact[name]; exists {
		return false
	}
	n.exact[name] = struct{}{}
	folded := foldAssetName(name)
	if _, exists := n.folded[folded]; !exists {
		n.folded[folded] = name
	}
	return true
}

func (n *storeDirNames) match(segment string) (string, bool) {
	if _, ok := n.exact[segment]; ok {
		return segment, true
	}
	name, ok := n.folded[foldAssetName(segment)]
	return name, ok
}

// storeDirIndex is one directory's listing. unavailable marks a directory
// that could not be listed (capped or failed), which the probe answers by
// HEAD instead.
type storeDirIndex struct {
	objects     storeDirNames
	dirs        storeDirNames
	count       int
	listedAt    time.Time
	unavailable bool
	err         error
}

func (i *storeDirIndex) add(entry storage.DirEntry) {
	if entry.Name == "" {
		return
	}
	names := &i.objects
	if entry.Dir {
		names = &i.dirs
	}
	if names.add(entry.Name) {
		i.count++
	}
}

func (i *storeDirIndex) match(segment string, object bool) (string, bool) {
	if object {
		return i.objects.match(segment)
	}
	return i.dirs.match(segment)
}

// weight is the index's share of the name budget; an empty or unavailable
// listing still costs one.
func (i *storeDirIndex) weight() int {
	if i.count < 1 {
		return 1
	}
	return i.count
}

// probeCache is a bounded LRU with per-entry expiry, an optional weight
// budget (used to cap the total number of indexed directory names) and a
// generation counter so results computed before a clear are not stored.
type probeCache[V any] struct {
	mu         sync.Mutex
	entries    map[string]*probeCacheEntry[V]
	recent     list.List
	maxEntries int
	maxWeight  int
	weight     int
	generation uint64
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

// store adds value unless the cache was cleared since generation was read.
func (c *probeCache[V]) store(key string, value V, weight int, expiresAt time.Time, generation uint64) bool {
	if c.maxWeight > 0 && weight > c.maxWeight {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return false
	}
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
	return true
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

func (c *probeCache[V]) currentGeneration() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

func (c *probeCache[V]) clear() {
	c.mu.Lock()
	clear(c.entries)
	c.recent.Init()
	c.weight = 0
	c.generation++
	c.mu.Unlock()
}

func (c *probeCache[V]) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
