package drawing

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/imagecache"
)

const (
	// pendingRenderCacheTTLIndex bridges concurrent misses in index mode until
	// the render index row is visible (capped by the business TTL).
	pendingRenderCacheTTLIndex = 120 * time.Second
	// defaultRenderIndexTouchInterval is the per-key sliding-TTL throttle.
	defaultRenderIndexTouchInterval = 60 * time.Second
	// renderIndexBatchCap bounds the buffered touch and expire keys.
	renderIndexBatchCap = 4096
	// renderIndexFlushInterval is the batch flush period.
	renderIndexFlushInterval = time.Second
	// renderIndexFlushTimeout bounds one batched statement.
	renderIndexFlushTimeout = 5 * time.Second
	// renderIndexErrorLogInterval throttles lookup/flush error logs.
	renderIndexErrorLogInterval = time.Minute
	// pendingRefBaseBytes is the pending-cache accounting of one ref beyond
	// its cdn_path.
	pendingRefBaseBytes = 128
)

// RenderIndex is the consumer-side view of the PostgreSQL render index
// (*imagecache.PGStore implements it). Cloud never inserts rows: Drawing does.
type RenderIndex interface {
	LookupRender(ctx context.Context, requestKey string) (imagecache.RenderIndexEntry, bool, error)
	TouchRender(ctx context.Context, keys []string) (int64, error)
	DeleteRender(ctx context.Context, keys []string) (int64, error)
}

// usableRenderIndex rejects nil and typed-nil indexes.
func usableRenderIndex(index RenderIndex) RenderIndex {
	if index == nil {
		return nil
	}
	if value := reflect.ValueOf(index); value.Kind() == reflect.Pointer && value.IsNil() {
		return nil
	}
	return index
}

// keyBatch is an insertion-ordered key set that drops its oldest key when full.
type keyBatch struct {
	order []string
	set   map[string]struct{}
}

func (b *keyBatch) add(key string) {
	if b.set == nil {
		b.set = make(map[string]struct{})
	}
	if _, ok := b.set[key]; ok {
		return
	}
	if len(b.order) >= renderIndexBatchCap {
		delete(b.set, b.order[0])
		b.order = b.order[1:]
	}
	b.order = append(b.order, key)
	b.set[key] = struct{}{}
}

func (b *keyBatch) drain() []string {
	keys := b.order
	b.order, b.set = nil, nil
	return keys
}

// renderIndexWriter batches the only render index writes Cloud makes on the
// request path: sliding-TTL touches and deletes of expired rows. Its flush
// goroutine starts on first use and stops on close.
type renderIndexWriter struct {
	index         RenderIndex
	touchInterval time.Duration
	now           func() time.Time
	flushEvery    time.Duration

	mu         sync.Mutex
	lastTouch  map[string]time.Time
	touches    keyBatch
	expires    keyBatch
	started    bool
	closed     bool
	stop       chan struct{}
	done       chan struct{}
	lastErrLog atomic.Int64
}

func newRenderIndexWriter(index RenderIndex, touchInterval time.Duration) *renderIndexWriter {
	if touchInterval <= 0 {
		touchInterval = defaultRenderIndexTouchInterval
	}
	return &renderIndexWriter{
		index:         index,
		touchInterval: touchInterval,
		now:           time.Now,
		flushEvery:    renderIndexFlushInterval,
		lastTouch:     make(map[string]time.Time),
	}
}

// touch buffers a sliding-TTL update, at most once per key per touchInterval.
func (w *renderIndexWriter) touch(key string) {
	now := w.now()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if last, ok := w.lastTouch[key]; ok && now.Sub(last) < w.touchInterval {
		return
	}
	w.lastTouch[key] = now
	w.touches.add(key)
	w.startLocked()
}

// expire buffers a delete of an expired row (fire-and-forget).
func (w *renderIndexWriter) expire(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	delete(w.lastTouch, key)
	w.expires.add(key)
	w.startLocked()
}

func (w *renderIndexWriter) startLocked() {
	if w.started {
		return
	}
	w.started = true
	w.stop = make(chan struct{})
	w.done = make(chan struct{})
	go w.loop(w.stop, w.done)
}

func (w *renderIndexWriter) loop(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(w.flushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.flush()
		case <-stop:
			return
		}
	}
}

// flush sends the buffered touches and deletes, one statement each.
func (w *renderIndexWriter) flush() {
	now := w.now()
	w.mu.Lock()
	touches := w.touches.drain()
	expires := w.expires.drain()
	for key, last := range w.lastTouch {
		if now.Sub(last) >= w.touchInterval {
			delete(w.lastTouch, key)
		}
	}
	if len(w.lastTouch) > 4*renderIndexBatchCap {
		// A burst of distinct keys only loosens the throttle; never grow unbounded.
		w.lastTouch = make(map[string]time.Time)
	}
	w.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), renderIndexFlushTimeout)
	defer cancel()
	if len(touches) > 0 {
		if _, err := w.index.TouchRender(ctx, touches); err != nil {
			w.logError(ctx, "render index touch failed", len(touches), err)
		}
	}
	if len(expires) > 0 {
		if _, err := w.index.DeleteRender(ctx, expires); err != nil {
			w.logError(ctx, "render index expired row delete failed", len(expires), err)
		}
	}
}

func (w *renderIndexWriter) logError(ctx context.Context, msg string, keys int, err error) {
	if !throttleLog(&w.lastErrLog, w.now()) {
		return
	}
	cacheLogger.ErrorContext(ctx, msg, "keys", keys, "error", err)
}

// close stops the flush goroutine after a final flush. It is idempotent.
func (w *renderIndexWriter) close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	started, stop, done := w.started, w.stop, w.done
	w.mu.Unlock()
	if started {
		close(stop)
		<-done
	}
	w.flush()
}

// throttleLog reports whether a throttled log may be written now.
func throttleLog(last *atomic.Int64, now time.Time) bool {
	for {
		previous := last.Load()
		if previous != 0 && now.Sub(time.Unix(0, previous)) < renderIndexErrorLogInterval {
			return false
		}
		if last.CompareAndSwap(previous, now.UnixNano()) {
			return true
		}
	}
}

// indexMode reports whether the render cache reads the PostgreSQL index.
func (c *RenderCacheClient) indexMode() bool { return c != nil && c.index != nil }

// legacyConfigured reports whether the legacy /cache API is still wired.
func (c *RenderCacheClient) legacyConfigured() bool {
	return c != nil && c.baseURL != "" && c.storageDir != ""
}

// lookupIndexContext serves a render from render_cache_index. Expiry is
// evaluated in Go; a hit slides the TTL through the batched, throttled touch.
func (c *RenderCacheClient) lookupIndexContext(ctx context.Context, key string) (ImageResult, bool) {
	finish := commandtrace.MeasureOperation(ctx, "drawing.cache_lookup_pg")
	defer finish()
	entry, ok, err := c.index.LookupRender(ctx, key)
	if err != nil {
		if throttleLog(&c.indexErrLog, time.Now()) {
			cacheLogger.ErrorContext(ctx, "render index lookup failed",
				"cache_key", shortRenderCacheKey(key), "error", err)
		}
		return ImageResult{}, false
	}
	if !ok {
		return ImageResult{}, false
	}
	if !entry.ExpiresAt.IsZero() && !entry.ExpiresAt.After(time.Now()) {
		c.indexWriter.expire(key)
		return ImageResult{}, false
	}
	ref, ok := refFromIndexEntry(entry)
	if !ok {
		cacheLogger.WarnContext(ctx, "render index row has an unusable cdn_path",
			"cache_key", shortRenderCacheKey(key), "content_hash", entry.ContentHash)
		return ImageResult{}, false
	}
	c.indexWriter.touch(key)
	return ImageResult{ref: ref, fetcher: c.fetcher}, true
}

// refFromIndexEntry builds a synthetic ref from a joined index row. It carries
// no node name: an indexed object is not fresh, so round-robin is correct.
func refFromIndexEntry(entry imagecache.RenderIndexEntry) (*ArtifactRef, bool) {
	if validateArtifactCDNPath(entry.Entry.CDNPath) != nil {
		return nil, false
	}
	ref := &ArtifactRef{
		Kind:           artifactRefKind,
		Hash:           entry.ContentHash,
		CDNPath:        entry.Entry.CDNPath,
		StorageBackend: entry.Entry.StorageBackend,
		ObjectKey:      entry.Entry.CDNPath,
		SizeBytes:      entry.Entry.SizeBytes,
		MediaType:      entry.Entry.MediaType,
		CacheKey:       entry.RequestKey,
		TTLSeconds:     entry.TTLSeconds,
		IndexWritten:   true,
	}
	if !entry.ExpiresAt.IsZero() {
		expires := entry.ExpiresAt.UTC().Format(time.RFC3339)
		ref.ExpiresAt = &expires
	}
	return ref, true
}

// pendingIndexTTL is the pending-entry lifetime in index mode.
func pendingIndexTTL(policy renderCachePolicy, ttl time.Duration) time.Duration {
	pendingTTL := pendingRenderCacheTTLIndex
	if !policy.Infinite && ttl > 0 && ttl < pendingTTL {
		pendingTTL = ttl
	}
	return pendingTTL
}

// Close stops the render index flush goroutine after a final flush.
func (c *RenderCacheClient) Close() error {
	if c == nil {
		return nil
	}
	c.indexWriter.close()
	return nil
}

// Close releases the render cache's background work (the render index flush
// goroutine). It is safe on a nil client or one without a render cache.
func (c *HarukiDrawingClient) Close() error {
	if c == nil {
		return nil
	}
	return c.cache.Close()
}
