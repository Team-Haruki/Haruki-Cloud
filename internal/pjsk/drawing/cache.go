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
)

type renderFlightResult struct {
	data       []byte
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

	entry := &localRenderEntry{
		data:      owned,
		permanent: permanent,
		size:      size,
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
	finishKey := commandtrace.MeasureOperation(ctx, "drawing.cache_key")
	policy, err := buildRenderCachePolicy(endpoint, request)
	if err != nil {
		finishKey()
		return render(ctx)
	}
	key, err := buildRenderCacheKey(policy)
	finishKey()
	if err != nil {
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
	select {
	case completed := <-result:
		finishWait()
		if completed.Err != nil {
			return nil, completed.Err
		}
		flightResult, ok := completed.Val.(renderFlightResult)
		if !ok {
			return nil, fmt.Errorf("local render cache returned unexpected type %T", completed.Val)
		}
		commandtrace.MergeOperations(ctx, flightResult.operations)
		if flightResult.leader != callerToken {
			commandtrace.RecordOperation(ctx, "drawing.cache_shared", 0)
		}
		if flightResult.err != nil {
			return nil, flightResult.err
		}
		cacheLogger.DebugContext(ctx, "drawing local cache miss rendered", "upstream_path", endpoint)
		return cloneRenderBytes(flightResult.data), nil
	case <-ctx.Done():
		finishWait()
		return nil, ctx.Err()
	}
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

	finishKey := commandtrace.MeasureOperation(ctx, "drawing.cache_key")
	policy, policyErr := buildRenderCachePolicy(endpoint, request)
	if policyErr == nil {
		key, keyErr := buildRenderCacheKey(policy)
		finishKey()
		if keyErr == nil {
			finishWait := commandtrace.MeasureOperation(ctx, "drawing.cache_wait")
			callerToken := new(renderFlightToken)
			result := c.flight.DoChan(key, func() (any, error) {
				flightResult := runSharedRenderFlight(ctx, func(sharedCtx context.Context) ([]byte, error) {
					tLookup := time.Now()
					finishLookup := commandtrace.MeasureOperation(sharedCtx, "drawing.cache_lookup")
					cached, hit := c.lookupContext(sharedCtx, key, policy.APIPath)
					finishLookup()
					if hit {
						commandtrace.RecordOperation(sharedCtx, drawingCacheHitTraceField, 0)
						cacheLogger.DebugContext(sharedCtx, "drawing remote cache hit",
							"upstream_path", endpoint,
							"cache_key", shortRenderCacheKey(key),
							"duration_ms", commandtrace.Milliseconds(time.Since(tLookup)),
						)
						return cached, nil
					}
					commandtrace.RecordOperation(sharedCtx, "drawing.cache_miss", 0)
					cacheLogger.DebugContext(sharedCtx, "drawing remote cache miss",
						"upstream_path", endpoint,
						"cache_key", shortRenderCacheKey(key),
						"duration_ms", commandtrace.Milliseconds(time.Since(tLookup)),
					)

					image, err := render(sharedCtx)
					if err != nil {
						return nil, err
					}

					ttl := policy.TTL
					if ttl <= 0 && !policy.Infinite {
						ttl = c.ttl
					}
					// Store write-behind: failures were already warn-only, so no
					// waiter depends on the store having completed. Returning the
					// rendered bytes first keeps ~3.6ms of store+index work off
					// every rendered command's critical path.
					c.storeAsync(sharedCtx, endpoint, key, policy.APIPath, policy.UserID, image, ttl, policy.Infinite)
					return image, nil
				})
				flightResult.leader = callerToken
				return flightResult, nil
			})
			select {
			case completed := <-result:
				finishWait()
				if completed.Err != nil {
					return nil, completed.Err
				}
				flightResult, ok := completed.Val.(renderFlightResult)
				if !ok {
					return nil, fmt.Errorf("remote render cache returned unexpected type %T", completed.Val)
				}
				commandtrace.MergeOperations(ctx, flightResult.operations)
				if flightResult.leader != callerToken {
					commandtrace.RecordOperation(ctx, "drawing.cache_shared", 0)
				}
				if flightResult.err != nil {
					return nil, flightResult.err
				}
				return cloneRenderBytes(flightResult.data), nil
			case <-ctx.Done():
				finishWait()
				return nil, ctx.Err()
			}
		}
	} else {
		finishKey()
	}

	return render(ctx)
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
		return nil, false
	}
	if resp.StatusCode() != http.StatusOK || strings.TrimSpace(record.FilePath) == "" {
		return nil, false
	}

	finishRead := commandtrace.MeasureOperation(ctx, "drawing.cache_read")
	body, err := c.readCacheFile(record.FilePath)
	finishRead()
	if err != nil {
		return nil, false
	}
	return body, true
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
		info, statErr := os.Lstat(current)
		switch {
		case statErr == nil:
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("render cache directory contains a non-directory component")
			}
		case os.IsNotExist(statErr):
			if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
				return err
			}
		case statErr != nil:
			return statErr
		}
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
