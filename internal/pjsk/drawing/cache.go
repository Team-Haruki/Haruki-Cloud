package drawing

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

var cacheLogger = logger.NewLoggerFromGlobal("DrawingCache")

const (
	renderCachePublic              = "public"
	renderCacheKeyVersion          = 3
	renderCacheEventListKeyVersion = 5
	localRenderCacheMaxEntries     = 512
	localRenderCacheMaxBytes       = 256 << 20
	pendingRenderCacheMaxBytes     = 64 << 20
)

type renderFlightResult struct {
	data       []byte
	image      ImageResult
	err        error
	operations []commandtrace.Stats
	leader     *renderFlightToken
}

type renderFlightToken byte

func runSharedRenderFlight(parent context.Context, work func(context.Context) ([]byte, error)) renderFlightResult {
	if parent == nil {
		parent = context.Background()
	}
	timeout := config.Cfg.PJSKRender.DrawingTimeout
	if timeout <= 0 {
		timeout = config.HTTPClientTimeout
	}
	detached := displaytime.WithRequestTimeZone(
		logger.DetachedContext(parent),
		displaytime.RequestTimeZoneFromContext(parent),
	)
	// Artifact mode: the same window also covers Drawing's encode + upload.
	if mode := artifactModeFrom(parent); mode != nil {
		timeout += mode.artifactTimeout
		detached = context.WithValue(detached, artifactModeCtxKey{}, mode)
	}
	detached = logger.WithContextAttrs(detached, slog.Bool("shared_work", true))
	sharedBase, cancel := context.WithTimeout(detached, timeout)
	defer cancel()
	sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
	data, err := work(sharedCtx)
	return renderFlightResult{
		data:       data,
		err:        err,
		operations: trace.Snapshot().Operations,
	}
}

func newLocalRenderCache(ttl time.Duration) *localRenderCache {
	return newLocalRenderCacheWithLimits(ttl, localRenderCacheMaxEntries, localRenderCacheMaxBytes)
}

func newLocalRenderCacheWithLimits(ttl time.Duration, maxEntries int, maxBytes int64) *localRenderCache {
	if ttl <= 0 {
		ttl = config.LocalRenderCacheTTL
	}
	return &localRenderCache{
		entries:    make(map[string]*localRenderEntry),
		lru:        list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
	}
}

func (lc *localRenderCache) get(key string) ([]byte, bool) {
	data, ref, ok := lc.lookupEntry(key)
	if !ok || ref != nil {
		return nil, false
	}
	return data, true
}

// lookupEntry returns a live entry: cloned bytes, or the pending ref.
func (lc *localRenderCache) lookupEntry(key string) ([]byte, *ArtifactRef, bool) {
	if lc == nil {
		return nil, nil, false
	}
	now := time.Now()
	lc.mu.Lock()
	entry, ok := lc.entries[key]
	if !ok {
		lc.mu.Unlock()
		return nil, nil, false
	}
	if !entry.permanent && !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
		lc.removeEntryLocked(key, entry)
		lc.mu.Unlock()
		return nil, nil, false
	}
	if entry.element != nil {
		lc.lru.MoveToFront(entry.element)
	}
	data, ref := entry.data, entry.ref
	lc.mu.Unlock()
	if ref != nil {
		return nil, ref, true
	}
	return cloneRenderBytes(data), nil, true
}

func (lc *localRenderCache) set(key string, data []byte, ttl time.Duration, permanent bool) {
	if lc == nil {
		return
	}
	size := int64(len(data))
	var owned []byte
	if lc.maxEntries > 0 && lc.maxBytes > 0 && size <= lc.maxBytes {
		owned = cloneRenderBytes(data)
	}
	lc.store(key, owned, nil, size, ttl, permanent)
}

// setRef keeps a pending artifact ref, accounted at len(cdn_path)+128 bytes,
// and returns its generation (0 when it was not retained).
func (lc *localRenderCache) setRef(key string, ref *ArtifactRef, ttl time.Duration) uint64 {
	if lc == nil || ref == nil {
		return 0
	}
	return lc.store(key, nil, ref, int64(len(ref.CDNPath)+pendingRefBaseBytes), ttl, false)
}

func (lc *localRenderCache) store(key string, owned []byte, ref *ArtifactRef, size int64, ttl time.Duration, permanent bool) uint64 {
	if !permanent && ttl <= 0 {
		ttl = lc.ttl
	}
	now := time.Now()

	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.ensureInitializedLocked()
	lc.sweepExpiredLocked(now)
	if existing := lc.entries[key]; existing != nil {
		lc.removeEntryLocked(key, existing)
	}
	if lc.maxEntries <= 0 || lc.maxBytes <= 0 || size > lc.maxBytes {
		return 0
	}

	lc.nextGeneration++
	entry := &localRenderEntry{
		generation: lc.nextGeneration,
		data:       owned,
		ref:        ref,
		permanent:  permanent,
		size:       size,
	}
	if !permanent {
		entry.expiresAt = now.Add(ttl)
	}
	entry.element = lc.lru.PushFront(key)
	lc.entries[key] = entry
	lc.totalBytes += size
	lc.evictLocked()
	if lc.entries[key] != entry {
		return 0
	}
	return entry.generation
}

func (lc *localRenderCache) ensureInitializedLocked() {
	if lc.entries == nil {
		lc.entries = make(map[string]*localRenderEntry)
	}
	if lc.lru == nil {
		lc.lru = list.New()
	}
}

func (lc *localRenderCache) sweepExpiredLocked(now time.Time) {
	lc.ensureInitializedLocked()
	for key, entry := range lc.entries {
		if entry == nil || (!entry.permanent && !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt)) {
			lc.removeEntryLocked(key, entry)
		}
	}
}

func (lc *localRenderCache) evictLocked() {
	for len(lc.entries) > lc.maxEntries || lc.totalBytes > lc.maxBytes {
		oldest := lc.lru.Back()
		if oldest == nil {
			break
		}
		key, _ := oldest.Value.(string)
		lc.removeEntryLocked(key, lc.entries[key])
	}
}

func (lc *localRenderCache) removeEntryLocked(key string, entry *localRenderEntry) {
	if entry == nil {
		delete(lc.entries, key)
		return
	}
	if current := lc.entries[key]; current != entry {
		return
	}
	delete(lc.entries, key)
	if entry.element != nil && lc.lru != nil {
		lc.lru.Remove(entry.element)
	}
	lc.totalBytes -= entry.size
	if lc.totalBytes < 0 {
		lc.totalBytes = 0
	}
}

func cloneRenderBytes(data []byte) []byte {
	if data == nil {
		return nil
	}
	return append([]byte(nil), data...)
}

// Render returns cached bytes for identical requests; otherwise calls render() and stores the result.
// Concurrent calls with the same key are deduplicated via singleflight.
func (lc *localRenderCache) Render(endpoint string, request any, render func() ([]byte, error)) ([]byte, error) {
	return lc.RenderContext(context.Background(), endpoint, request, render)
}

func (lc *localRenderCache) RenderContext(ctx context.Context, endpoint string, request any, render func() ([]byte, error)) ([]byte, error) {
	return lc.RenderSharedContext(ctx, endpoint, request, func(context.Context) ([]byte, error) {
		return render()
	})
}

func (lc *localRenderCache) RenderSharedContext(ctx context.Context, endpoint string, request any, render func(context.Context) ([]byte, error)) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	policy, key, ok := resolveRenderCachePolicyKey(ctx, endpoint, request)
	if !ok {
		return render(ctx)
	}
	ttl := policy.TTL
	if cached, ok := lc.get(key); ok {
		commandtrace.RecordOperation(ctx, drawingCacheHitTraceField, 0)
		cacheLogger.DebugContext(ctx, "drawing local cache hit", "upstream_path", endpoint)
		return cached, nil
	}
	commandtrace.RecordOperation(ctx, "drawing.cache_miss", 0)
	finishWait := commandtrace.MeasureOperation(ctx, "drawing.cache_wait")
	callerToken := new(renderFlightToken)
	result := lc.flight.DoChan(key, func() (any, error) {
		flightResult := runSharedRenderFlight(ctx, func(sharedCtx context.Context) ([]byte, error) {
			if cached, ok := lc.get(key); ok {
				commandtrace.RecordOperation(sharedCtx, drawingCacheHitTraceField, 0)
				return cached, nil
			}
			data, err := render(sharedCtx)
			if err != nil {
				return nil, err
			}
			lc.set(key, data, ttl, policy.Infinite)
			return data, nil
		})
		flightResult.leader = callerToken
		return flightResult, nil
	})
	data, err := waitForRenderFlight(ctx, result, callerToken, "local")
	finishWait()
	if err != nil {
		return nil, err
	}
	cacheLogger.DebugContext(ctx, "drawing local cache miss rendered", "upstream_path", endpoint)
	return data, nil
}

// NewRenderCacheClient returns the persistent render cache, which serves hits
// from render_cache_index only. Without a TTL or a usable index it returns nil
// and renders go straight to Drawing (or the in-process cache).
func NewRenderCacheClient(cfg RenderCacheConfig) *RenderCacheClient {
	index := usableRenderIndex(cfg.Index)
	if cfg.TTL <= 0 || index == nil {
		return nil
	}
	return &RenderCacheClient{
		ttl:         cfg.TTL,
		index:       index,
		indexWriter: newRenderIndexWriter(index, cfg.TouchInterval),
		fetcher:     newArtifactFetcher(cfg.Artifacts, cfg.Hosts, cfg.FetchTimeout),
		pending:     newLocalRenderCacheWithLimits(pendingRenderCacheTTLIndex, 128, pendingRenderCacheMaxBytes),
	}
}

func (c *RenderCacheClient) Render(endpoint string, request any, render func() ([]byte, error)) ([]byte, error) {
	return c.RenderContext(context.Background(), endpoint, request, render)
}

func (c *RenderCacheClient) RenderContext(ctx context.Context, endpoint string, request any, render func() ([]byte, error)) ([]byte, error) {
	return c.RenderSharedContext(ctx, endpoint, request, func(context.Context) ([]byte, error) {
		return render()
	})
}

func (c *RenderCacheClient) RenderSharedContext(ctx context.Context, endpoint string, request any, render func(context.Context) ([]byte, error)) ([]byte, error) {
	if c == nil {
		return render(ctx)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	policy, key, ok := resolveRenderCachePolicyKey(ctx, endpoint, request)
	if !ok {
		return render(ctx)
	}
	return c.renderRemoteFlight(ctx, endpoint, key, policy, render)
}

func resolveRenderCachePolicyKey(ctx context.Context, endpoint string, request any) (renderCachePolicy, string, bool) {
	finishKey := commandtrace.MeasureOperation(ctx, "drawing.cache_key")
	defer finishKey()
	policy, err := buildRenderCachePolicy(endpoint, request)
	if err != nil {
		return renderCachePolicy{}, "", false
	}
	key, err := buildRenderCacheKey(policy)
	return policy, key, err == nil
}

func waitForRenderFlight(ctx context.Context, result <-chan singleflight.Result, callerToken *renderFlightToken, cacheName string) ([]byte, error) {
	image, err := waitForImageFlight(ctx, result, callerToken, cacheName)
	if err != nil {
		return nil, err
	}
	return image.Bytes(ctx)
}

func waitForImageFlight(ctx context.Context, result <-chan singleflight.Result, callerToken *renderFlightToken, cacheName string) (ImageResult, error) {
	select {
	case completed := <-result:
		if completed.Err != nil {
			return ImageResult{}, completed.Err
		}
		flightResult, ok := completed.Val.(renderFlightResult)
		if !ok {
			return ImageResult{}, fmt.Errorf("%s render cache returned unexpected type %T", cacheName, completed.Val)
		}
		commandtrace.MergeOperations(ctx, flightResult.operations)
		if flightResult.leader != callerToken {
			commandtrace.RecordOperation(ctx, "drawing.cache_shared", 0)
		}
		if flightResult.err != nil {
			return ImageResult{}, flightResult.err
		}
		if flightResult.image.ref != nil {
			return flightResult.image, nil
		}
		return ImageBytes(cloneRenderBytes(flightResult.data)), nil
	case <-ctx.Done():
		return ImageResult{}, ctx.Err()
	}
}

func (c *RenderCacheClient) renderRemoteFlight(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) ([]byte, error) {
	image, err := c.renderRemoteImageFlight(ctx, endpoint, key, policy, render)
	if err != nil {
		return nil, err
	}
	return image.Bytes(ctx)
}

func (c *RenderCacheClient) renderRemoteImageFlight(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) (ImageResult, error) {
	finishWait := commandtrace.MeasureOperation(ctx, "drawing.cache_wait")
	defer finishWait()
	callerToken := new(renderFlightToken)
	result := c.flight.DoChan(key, func() (any, error) {
		var image ImageResult
		flightResult := runSharedRenderFlight(ctx, func(sharedCtx context.Context) ([]byte, error) {
			var err error
			image, err = c.renderRemoteImageWork(sharedCtx, endpoint, key, policy, render)
			return image.data, err
		})
		flightResult.image = image
		flightResult.leader = callerToken
		return flightResult, nil
	})
	return waitForImageFlight(ctx, result, callerToken, "remote")
}

func (c *RenderCacheClient) renderRemoteFlightWork(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) ([]byte, error) {
	image, err := c.renderRemoteImageWork(ctx, endpoint, key, policy, render)
	if err != nil {
		return nil, err
	}
	return image.Bytes(ctx)
}

func (c *RenderCacheClient) renderRemoteImageWork(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) (ImageResult, error) {
	if data, ref, ok := c.pending.lookupEntry(key); ok {
		commandtrace.RecordOperation(ctx, "drawing.cache_pending_hit", 0)
		commandtrace.RecordOperation(ctx, drawingCacheHitTraceField, 0)
		if ref != nil {
			return ImageResult{ref: ref, fetcher: c.fetcher}, nil
		}
		return ImageBytes(data), nil
	}
	lookupStarted := time.Now()
	cached, hit := c.lookupIndexContext(ctx, key)
	if hit {
		commandtrace.RecordOperation(ctx, drawingCacheHitTraceField, 0)
		cacheLogger.DebugContext(ctx, "drawing remote cache hit",
			"upstream_path", endpoint,
			"cache_key", shortRenderCacheKey(key),
			"duration_ms", commandtrace.Milliseconds(time.Since(lookupStarted)),
		)
		return cached, nil
	}
	commandtrace.RecordOperation(ctx, "drawing.cache_miss", 0)
	cacheLogger.DebugContext(ctx, "drawing remote cache miss",
		"upstream_path", endpoint,
		"cache_key", shortRenderCacheKey(key),
		"duration_ms", commandtrace.Milliseconds(time.Since(lookupStarted)),
	)
	return c.renderRemoteMiss(ctx, endpoint, key, policy, render)
}

func (c *RenderCacheClient) renderRemoteMiss(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) (ImageResult, error) {
	ttl := policy.TTL
	if ttl <= 0 && !policy.Infinite {
		ttl = c.ttl
	}
	// Attach point A: allow-listed cached renders carry the full directive.
	renderCtx := ctx
	mode := artifactModeFrom(ctx)
	var directive *renderDirective
	if mode != nil && mode.allow.has(policy.APIPath) {
		directive = newRenderDirective(key, policy, ttl, true)
		renderCtx = withDirective(ctx, directive)
	}
	image, err := render(renderCtx)
	if err != nil {
		return ImageResult{}, err
	}
	if directive != nil && directive.outcome.Ref != nil {
		return c.pendingRef(key, policy, ttl, directive.outcome.Ref), nil
	}
	// Bytes (degraded write, Cache-Store: 0, an old Drawing, a non-allow-listed
	// endpoint): no index row will exist, so the pending entry stays until it
	// expires. Cloud no longer persists rendered bytes itself.
	c.pending.set(key, image, pendingIndexTTL(policy, ttl), false)
	return ImageBytes(image), nil
}

// pendingRef keeps a ref Drawing returned until its index row is visible:
// it is dropped at once when Drawing reported index_written.
func (c *RenderCacheClient) pendingRef(key string, policy renderCachePolicy, ttl time.Duration, ref *ArtifactRef) ImageResult {
	generation := c.pending.setRef(key, ref, pendingIndexTTL(policy, ttl))
	if ref.IndexWritten && generation != 0 {
		c.pending.deleteGeneration(key, generation)
	}
	return ImageResult{ref: ref, fetcher: c.fetcher}
}

func shortRenderCacheKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 12 {
		return key
	}
	return key[:12]
}

// The store completion retains only a generation, so evicted entries can
// release their image bytes while the asynchronous store is still running.
func (lc *localRenderCache) deleteGeneration(key string, generation uint64) {
	if lc == nil {
		return
	}
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if entry := lc.entries[key]; entry != nil && entry.generation == generation {
		lc.removeEntryLocked(key, entry)
	}
}

func (lc *localRenderCache) peekGeneration(key string) uint64 {
	if lc == nil {
		return 0
	}
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if entry := lc.entries[key]; entry != nil {
		return entry.generation
	}
	return 0
}
