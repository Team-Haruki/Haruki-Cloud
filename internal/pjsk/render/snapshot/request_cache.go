package snapshot

import (
	"context"
	"sync"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

type requestCacheContextKey struct{}

type requestCache struct {
	mu          sync.Mutex
	privateData map[privateDataCacheKey]*privateDataCacheEntry
}

type privateDataCacheKey struct {
	Server         string
	DataType       string
	UserID         int64
	Platform       string
	PlatformUserID string
	Projection     string
}

type privateDataCacheEntry struct {
	once sync.Once
	data privateDataPayload
	err  error
}

// WithRequestCache attaches a per-command cache for live snapshot source data.
// It is intentionally context-scoped so independent bot commands cannot share
// private snapshot payloads.
func WithRequestCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if cacheFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, requestCacheContextKey{}, &requestCache{
		privateData: make(map[privateDataCacheKey]*privateDataCacheEntry),
	})
}

func cacheFromContext(ctx context.Context) *requestCache {
	if ctx == nil {
		return nil
	}
	cache, _ := ctx.Value(requestCacheContextKey{}).(*requestCache)
	return cache
}

func cachedPrivateData(ctx context.Context, key privateDataCacheKey, fetch func() (privateDataPayload, error)) (privateDataPayload, error, bool) {
	return requestCachedPrivateData(ctx, key, false, fetch)
}

// refreshPrivateData replaces this command's entry for key with a new fetch.
// It is used when a versionOnly entry turns out not to be servable.
func refreshPrivateData(ctx context.Context, key privateDataCacheKey, fetch func() (privateDataPayload, error)) (privateDataPayload, error) {
	data, err, _ := requestCachedPrivateData(ctx, key, true, fetch)
	return data, err
}

func requestCachedPrivateData(ctx context.Context, key privateDataCacheKey, replace bool, fetch func() (privateDataPayload, error)) (privateDataPayload, error, bool) {
	cache := cacheFromContext(ctx)
	if cache == nil {
		commandtrace.RecordOperation(ctx, "snapshot.request_cache_bypass", 0)
		finishFetch := commandtrace.MeasureOperation(ctx, "snapshot.private_data")
		data, err := fetch()
		finishFetch()
		return data, err, false
	}

	cache.mu.Lock()
	entry, hit := cache.privateData[key]
	if replace {
		entry, hit = nil, false
	}
	if entry == nil {
		entry = &privateDataCacheEntry{}
		cache.privateData[key] = entry
	}
	cache.mu.Unlock()
	if hit {
		commandtrace.RecordOperation(ctx, "snapshot.request_cache_hit", 0)
	} else {
		commandtrace.RecordOperation(ctx, "snapshot.request_cache_miss", 0)
	}

	didFetch := false
	waitStartedAt := time.Now()
	entry.once.Do(func() {
		didFetch = true
		finishFetch := commandtrace.MeasureOperation(ctx, "snapshot.private_data")
		entry.data, entry.err = fetch()
		finishFetch()
	})
	if !didFetch {
		commandtrace.RecordOperation(ctx, "snapshot.cache_wait", time.Since(waitStartedAt))
		if ctx.Err() != nil {
			commandtrace.RecordOperation(ctx, "snapshot.cache_wait_canceled", 0)
		}
	}
	return entry.data, entry.err, hit
}
