package drawing

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/utils/logger"

	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/singleflight"
)

var cacheLogger = logger.NewLoggerFromGlobal("DrawingCache")

const (
	renderCachePublic              = "public"
	renderCacheKeyVersion          = 3
	renderCacheEventListKeyVersion = 5
	localRenderCacheMaxEntries     = 512
	localRenderCacheMaxBytes       = 256 << 20
	renderCacheAPIResponseMaxBytes = 1 << 20
	renderCacheStoreConcurrency    = 8
	pendingRenderCacheTTL          = 30 * time.Second
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
	if lc == nil {
		return nil, false
	}
	now := time.Now()
	lc.mu.Lock()
	entry, ok := lc.entries[key]
	if !ok {
		lc.mu.Unlock()
		return nil, false
	}
	if !entry.permanent && !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
		lc.removeEntryLocked(key, entry)
		lc.mu.Unlock()
		return nil, false
	}
	if entry.element != nil {
		lc.lru.MoveToFront(entry.element)
	}
	data := entry.data
	lc.mu.Unlock()
	return cloneRenderBytes(data), true
}

func (lc *localRenderCache) set(key string, data []byte, ttl time.Duration, permanent bool) {
	if lc == nil {
		return
	}
	if !permanent && ttl <= 0 {
		ttl = lc.ttl
	}
	now := time.Now()
	size := int64(len(data))
	var owned []byte
	if lc.maxEntries > 0 && lc.maxBytes > 0 && size <= lc.maxBytes {
		owned = cloneRenderBytes(data)
	}

	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.ensureInitializedLocked()
	lc.sweepExpiredLocked(now)
	if existing := lc.entries[key]; existing != nil {
		lc.removeEntryLocked(key, existing)
	}
	if lc.maxEntries <= 0 || lc.maxBytes <= 0 || size > lc.maxBytes {
		return
	}

	lc.nextGeneration++
	entry := &localRenderEntry{
		generation: lc.nextGeneration,
		data:       owned,
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

func NewRenderCacheClient(cfg RenderCacheConfig) *RenderCacheClient {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	storageDir := strings.TrimSpace(cfg.StorageDir)
	if baseURL == "" || storageDir == "" || cfg.TTL <= 0 {
		return nil
	}

	return &RenderCacheClient{
		http: resty.New().
			SetTransport(upstream.NewTunedTransport(upstream.TunedTransportConfig{})).
			SetResponseBodyLimit(renderCacheAPIResponseMaxBytes).
			SetTimeout(config.HTTPClientTimeout),
		baseURL:       baseURL,
		storageDir:    storageDir,
		ttl:           cfg.TTL,
		imageCacheDir: strings.TrimSpace(cfg.ImageCacheDir),
		imageStore:    cfg.ImageStore,
		storeSlots:    make(chan struct{}, renderCacheStoreConcurrency),
		pending:       newLocalRenderCacheWithLimits(pendingRenderCacheTTL, 128, pendingRenderCacheMaxBytes),
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
			operation := "drawing.cache_shared"
			if cacheName == "file" {
				operation = "drawing.cache_read_shared"
			}
			commandtrace.RecordOperation(ctx, operation, 0)
		}
		if flightResult.err != nil {
			return ImageResult{}, flightResult.err
		}
		if flightResult.image.filePath != "" {
			return flightResult.image, nil
		}
		return ImageBytes(cloneRenderBytes(flightResult.data)), nil
	case <-ctx.Done():
		return ImageResult{}, ctx.Err()
	}
}

func (c *RenderCacheClient) renderRemoteFlight(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) ([]byte, error) {
	image, err := c.renderRemoteImageFlight(ctx, endpoint, key, policy, render, false)
	if err != nil {
		return nil, err
	}
	return image.Bytes(ctx)
}

func (c *RenderCacheClient) renderRemoteImageFlight(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error), rebuild bool) (ImageResult, error) {
	finishWait := commandtrace.MeasureOperation(ctx, "drawing.cache_wait")
	defer finishWait()
	callerToken := new(renderFlightToken)
	flightKey := key
	if rebuild {
		flightKey += ":rebuild"
	}
	result := c.flight.DoChan(flightKey, func() (any, error) {
		var image ImageResult
		flightResult := runSharedRenderFlight(ctx, func(sharedCtx context.Context) ([]byte, error) {
			var err error
			if rebuild {
				image.data, err = c.renderRemoteMiss(sharedCtx, endpoint, key, policy, render)
			} else {
				image, err = c.renderRemoteImageWork(sharedCtx, endpoint, key, policy, render)
			}
			return image.data, err
		})
		flightResult.image = image
		flightResult.leader = callerToken
		return flightResult, nil
	})
	image, err := waitForImageFlight(ctx, result, callerToken, "remote")
	if err == nil && image.filePath != "" {
		// Retry only the render work, once, if the file disappears after lookup.
		image.fallback = func(retryCtx context.Context) ([]byte, error) {
			fresh, err := c.renderRemoteImageFlight(retryCtx, endpoint, key, policy, render, true)
			if err != nil {
				return nil, err
			}
			return fresh.Bytes(retryCtx)
		}
	}
	return image, err
}

func (c *RenderCacheClient) renderRemoteFlightWork(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) ([]byte, error) {
	image, err := c.renderRemoteImageWork(ctx, endpoint, key, policy, render)
	if err != nil {
		return nil, err
	}
	return image.Bytes(ctx)
}

func (c *RenderCacheClient) renderRemoteImageWork(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) (ImageResult, error) {
	if data, ok := c.pending.get(key); ok {
		commandtrace.RecordOperation(ctx, "drawing.cache_pending_hit", 0)
		commandtrace.RecordOperation(ctx, drawingCacheHitTraceField, 0)
		return ImageBytes(data), nil
	}
	lookupStarted := time.Now()
	finishLookup := commandtrace.MeasureOperation(ctx, "drawing.cache_lookup")
	cached, hit := c.lookupImageContext(ctx, key, policy.APIPath)
	finishLookup()
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
	data, err := c.renderRemoteMiss(ctx, endpoint, key, policy, render)
	return ImageBytes(data), err
}

func (c *RenderCacheClient) renderRemoteMiss(ctx context.Context, endpoint, key string, policy renderCachePolicy, render func(context.Context) ([]byte, error)) ([]byte, error) {
	image, err := render(ctx)
	if err != nil {
		return nil, err
	}
	ttl := policy.TTL
	if ttl <= 0 && !policy.Infinite {
		ttl = c.ttl
	}
	// Store write-behind: failures were already warn-only, so no waiter
	// depends on the store having completed.
	pendingTTL := pendingRenderCacheTTL
	if !policy.Infinite && ttl > 0 && ttl < pendingTTL {
		pendingTTL = ttl
	}
	c.pending.set(key, image, pendingTTL, false)
	c.storeAsync(ctx, endpoint, key, policy.APIPath, policy.UserID, image, ttl, policy.Infinite)
	return image, nil
}

func shortRenderCacheKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 12 {
		return key
	}
	return key[:12]
}

func (c *RenderCacheClient) lookup(key string, apiPath string) ([]byte, bool) {
	return c.lookupContext(context.Background(), key, apiPath)
}

func (c *RenderCacheClient) lookupContext(ctx context.Context, key string, apiPath string) ([]byte, bool) {
	image, hit := c.lookupImageContext(ctx, key, apiPath)
	if !hit {
		return nil, false
	}
	data, err := image.Bytes(ctx)
	return data, err == nil
}

func (c *RenderCacheClient) lookupImageContext(ctx context.Context, key string, apiPath string) (ImageResult, bool) {
	var record renderCacheRecord
	var apiErr renderCacheAPIError

	request := c.http.R().
		SetContext(ctx).
		SetQueryParam("key", key).
		SetResult(&record).
		SetError(&apiErr)
	if strings.TrimSpace(apiPath) != "" {
		request.SetQueryParam("api_path", apiPath)
	}
	finishHTTP := commandtrace.MeasureOperation(ctx, "drawing.cache_lookup_http")
	resp, err := request.Get(c.baseURL + "/cache")
	finishHTTP()
	if err != nil {
		return ImageResult{}, false
	}
	if resp.StatusCode() != http.StatusOK || strings.TrimSpace(record.FilePath) == "" {
		return ImageResult{}, false
	}

	image, err := c.cachedFile(record.FilePath)
	if err != nil {
		return ImageResult{}, false
	}
	return image, true
}

func (c *RenderCacheClient) store(key string, apiPath string, userID string, image []byte, ttl time.Duration, infinite bool) error {
	return c.storeContext(context.Background(), key, apiPath, userID, image, ttl, infinite)
}

// storeAsync persists a rendered image to the remote cache in the background
// so flight waiters receive the bytes immediately. context.WithoutCancel keeps
// the request's log/trace attrs while detaching from its cancellation.
// The slot is acquired BEFORE spawning: when every slot is busy (e.g. the
// remote cache API is stalling on its 10s timeout) the store is dropped with a
// warn instead of queueing — store failures were already warn-only, so a
// dependency stall must degrade to cache misses, never to an unbounded goroutine
// backlog each pinning a full image clone.
func (c *RenderCacheClient) storeAsync(ctx context.Context, endpoint string, key string, apiPath string, userID string, image []byte, ttl time.Duration, infinite bool) {
	storeCtx := context.WithoutCancel(ctx)
	if c.storeSlots != nil {
		select {
		case c.storeSlots <- struct{}{}:
		default:
			cacheLogger.WarnContext(storeCtx, "drawing remote cache store dropped",
				"upstream_path", endpoint,
				"cache_key", shortRenderCacheKey(key),
				"reason", "store slots saturated",
			)
			return
		}
	}
	// The flight result retains the original slice and hands clones to
	// waiters; clone here too (after slot acquisition, so dropped stores never
	// copy) so the background store never races a future owner mutation.
	pendingGeneration := c.pending.peekGeneration(key)
	owned := cloneRenderBytes(image)
	c.storeWG.Add(1)
	go func() {
		defer c.storeWG.Done()
		defer func() {
			if c.storeSlots != nil {
				<-c.storeSlots
			}
		}()
		startedAt := time.Now()
		storeErr := c.storeContext(storeCtx, key, apiPath, userID, owned, ttl, infinite)
		if storeErr != nil {
			cacheLogger.WarnContext(storeCtx, "drawing remote cache store failed",
				"upstream_path", endpoint,
				"cache_key", shortRenderCacheKey(key),
				"duration_ms", commandtrace.Milliseconds(time.Since(startedAt)),
				"error_type", fmt.Sprintf("%T", storeErr),
			)
			return
		}
		c.pending.deleteGeneration(key, pendingGeneration)
		cacheLogger.DebugContext(storeCtx, "drawing remote cache stored",
			"upstream_path", endpoint,
			"cache_key", shortRenderCacheKey(key),
			"duration_ms", commandtrace.Milliseconds(time.Since(startedAt)),
		)
	}()
}

// waitForPendingStores blocks until all write-behind stores have drained.
// Test-only today; wire into a shutdown hook if graceful drain is ever needed.
func (c *RenderCacheClient) waitForPendingStores() {
	c.storeWG.Wait()
}

func (c *RenderCacheClient) storeContext(ctx context.Context, key string, apiPath string, userID string, image []byte, ttl time.Duration, infinite bool) error {
	if len(image) > drawingMaxResponseBytes {
		return fmt.Errorf("render cache image exceeds %d bytes", drawingMaxResponseBytes)
	}
	if ttl <= 0 && !infinite {
		ttl = c.ttl
	}
	finishHash := commandtrace.MeasureOperation(ctx, "drawing.cache_hash")
	contentHash, targetPath := c.contentFilePath(apiPath, userID, key, image)
	finishHash()
	finishWrite := commandtrace.MeasureOperation(ctx, "drawing.cache_write")
	targetPath, err := c.prepareCacheTarget(targetPath)
	if err != nil {
		finishWrite()
		return err
	}
	existingInfo, statErr := os.Stat(targetPath)
	fileAlreadyExisted := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		finishWrite()
		return statErr
	}
	if fileAlreadyExisted && (!existingInfo.Mode().IsRegular() || existingInfo.Size() < 0 || existingInfo.Size() > drawingMaxResponseBytes) {
		finishWrite()
		return fmt.Errorf("existing render cache file is invalid")
	}
	if !fileAlreadyExisted {
		if err := writeRenderCacheFileAtomic(targetPath, image); err != nil {
			finishWrite()
			return err
		}
	}
	finishWrite()

	var apiErr renderCacheAPIError
	ttlSeconds := "0"
	if !infinite {
		ttlSeconds = strconv.Itoa(int(math.Ceil(ttl.Seconds())))
	}

	finishHTTP := commandtrace.MeasureOperation(ctx, "drawing.cache_store_http")
	resp, err := c.http.R().
		SetContext(ctx).
		SetFormData(map[string]string{
			"key":       key,
			"ttl":       ttlSeconds,
			"api_path":  apiPath,
			"user_id":   userID,
			"ext":       strings.TrimPrefix(filepath.Ext(targetPath), "."),
			"file_path": targetPath,
		}).
		SetError(&apiErr).
		Post(c.baseURL + "/cache")
	finishHTTP()
	if err != nil || resp.StatusCode() != http.StatusOK {
		if !fileAlreadyExisted {
			_ = os.Remove(targetPath)
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("cache register failed with status: %d", resp.StatusCode())
	}
	c.registerImageCachePath(ctx, contentHash, targetPath, image)
	return nil
}

func (c *RenderCacheClient) defaultFilePath(apiPath string, userID string, key string) string {
	return filepath.Join(
		c.storageDir,
		filepath.FromSlash(normalizeRenderCacheAPIPath(apiPath)),
		normalizeRenderCacheUserID(userID),
		key+".png",
	)
}

func (c *RenderCacheClient) readCacheFile(candidate string) ([]byte, error) {
	resolved, err := resolveContainedCacheFile(c.storageDir, candidate)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("render cache path is not a regular file")
	}
	if info.Size() < 0 || info.Size() > drawingMaxResponseBytes {
		return nil, fmt.Errorf("render cache file exceeds %d bytes", drawingMaxResponseBytes)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, drawingMaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > drawingMaxResponseBytes {
		return nil, fmt.Errorf("render cache file exceeds %d bytes", drawingMaxResponseBytes)
	}
	return body, nil
}

func (c *RenderCacheClient) prepareCacheTarget(candidate string) (string, error) {
	root, target, err := absoluteContainedCachePath(c.storageDir, candidate)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	targetDir := filepath.Dir(target)
	if err := ensureRenderCacheDirectory(root, targetDir); err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realDir, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		return "", err
	}
	if !renderCachePathWithin(realRoot, realDir) {
		return "", fmt.Errorf("render cache target escapes storage directory")
	}
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", fmt.Errorf("render cache target is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return target, nil
}

func ensureRenderCacheDirectory(root, targetDir string) error {
	relative, err := filepath.Rel(root, targetDir)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("render cache directory escapes storage directory")
	}
	current := root
	if relative == "." {
		return nil
	}
	for _, segment := range strings.Split(relative, string(os.PathSeparator)) {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		if err := ensureRenderCacheDirectoryComponent(current); err != nil {
			return err
		}
	}
	return nil
}

func ensureRenderCacheDirectoryComponent(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("render cache directory contains a non-directory component")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(path, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}

func resolveContainedCacheFile(storageDir, candidate string) (string, error) {
	root, target, err := absoluteContainedCachePath(storageDir, candidate)
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	if !renderCachePathWithin(realRoot, realTarget) {
		return "", fmt.Errorf("render cache file escapes storage directory")
	}
	return realTarget, nil
}

func absoluteContainedCachePath(storageDir, candidate string) (string, string, error) {
	root, err := filepath.Abs(filepath.Clean(storageDir))
	if err != nil {
		return "", "", err
	}
	target, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", "", err
	}
	if !renderCachePathWithin(root, target) {
		return "", "", fmt.Errorf("render cache path escapes storage directory")
	}
	return root, target, nil
}

func renderCachePathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || filepath.IsAbs(relative) {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func writeRenderCacheFileAtomic(target string, data []byte) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".render-cache-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(0o644); err != nil {
		return err
	}
	if _, err = temporary.Write(data); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporaryPath, target); err != nil {
		return err
	}
	return nil
}

func (c *RenderCacheClient) contentFilePath(apiPath string, userID string, key string, image []byte) (string, string) {
	digest := sha256.Sum256(image)
	contentHash := hex.EncodeToString(digest[:])
	dir := strings.TrimSuffix(c.defaultFilePath(apiPath, userID, key), ".png")
	return contentHash, filepath.Join(dir, contentHash+renderCacheFileExtFromData(image))
}

func (c *RenderCacheClient) registerImageCachePath(ctx context.Context, contentHash string, targetPath string, image []byte) {
	if c == nil || c.imageStore == nil {
		return
	}
	rel, ok := c.imageCacheRelativePath(targetPath)
	if !ok {
		return
	}
	finishIndex := commandtrace.MeasureOperation(ctx, "drawing.cache_index")
	c.imageStore.Insert(ctx, contentHash, "pjsk", rel, targetPath, int64(len(image)))
	finishIndex()
}

func (c *RenderCacheClient) imageCacheRelativePath(targetPath string) (string, bool) {
	if c == nil {
		return "", false
	}
	imageCacheDir := strings.TrimSpace(c.imageCacheDir)
	if imageCacheDir == "" {
		return "", false
	}
	rel, err := filepath.Rel(imageCacheDir, targetPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func renderCacheFileExtFromData(data []byte) string {
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	switch http.DetectContentType(sniff) {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
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
