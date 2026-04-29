package snapshot

import (
	"context"
	"sync"
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
}

type privateDataCacheEntry struct {
	once sync.Once
	data []byte
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

func cachedPrivateData(ctx context.Context, key privateDataCacheKey, fetch func() ([]byte, error)) ([]byte, error, bool) {
	cache := cacheFromContext(ctx)
	if cache == nil {
		data, err := fetch()
		return data, err, false
	}

	cache.mu.Lock()
	entry, hit := cache.privateData[key]
	if entry == nil {
		entry = &privateDataCacheEntry{}
		cache.privateData[key] = entry
	}
	cache.mu.Unlock()

	entry.once.Do(func() {
		entry.data, entry.err = fetch()
		if entry.data != nil {
			entry.data = append([]byte(nil), entry.data...)
		}
	})
	if entry.data == nil {
		return nil, entry.err, hit
	}
	return append([]byte(nil), entry.data...), entry.err, hit
}
