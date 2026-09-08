package snapshot

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

// The reference below preserves the previous request-cache copy behavior.
// This benchmark measures authorized warm payload reuse and version extraction;
// it excludes HTTP, snapshot construction, and drawing.
func BenchmarkSnapshotWarmPayload(b *testing.B) {
	for _, mib := range []int{1, 10} {
		data := []byte(`{"upload_time":100,"padding":"` + strings.Repeat("x", mib<<20) + `"}`)
		cache := NewPrivateDataCache()
		key := suiteKey()
		if _, _, err := cache.Fetch(key, func(int64) ([]byte, bool, error) { return data, false, nil }); err != nil {
			b.Fatal(err)
		}
		authorizedRead := func(int64) ([]byte, bool, error) { return nil, true, nil }
		b.Run(fmt.Sprintf("%dMiB/previous", mib), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ctx := withLegacyBenchmarkRequestCache(context.Background())
				payload, err, _ := legacyBenchmarkCachedPrivateData(ctx, legacyBenchmarkPrivateDataCacheKey{Server: key.Server, DataType: key.DataType, UserID: key.UID}, func() ([]byte, error) {
					data, _, err := cache.Fetch(key, authorizedRead)
					return data, err
				})
				if err != nil {
					b.Fatal(err)
				}
				version, err := parseTopLevelUploadTime(payload)
				if err != nil || version != 100 {
					b.Fatal("incorrect cached version")
				}
				runtime.KeepAlive(payload)
			}
		})
		b.Run(fmt.Sprintf("%dMiB/shared", mib), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ctx := WithRequestCache(context.Background())
				payload, err, _ := cachedPrivateData(ctx, privateDataCacheKey{Server: key.Server, DataType: key.DataType, UserID: key.UID}, func() (privateDataPayload, error) {
					data, _, err := cache.fetchPayload(key, authorizedRead)
					return data, err
				})
				if err != nil || payload.uploadTime != 100 {
					b.Fatal("incorrect cached version")
				}
				runtime.KeepAlive(payload)
			}
		})
	}
}

type legacyBenchmarkRequestCacheContextKey struct{}

type legacyBenchmarkRequestCache struct {
	mu          sync.Mutex
	privateData map[legacyBenchmarkPrivateDataCacheKey]*legacyBenchmarkPrivateDataCacheEntry
}

type legacyBenchmarkPrivateDataCacheKey struct {
	Server         string
	DataType       string
	UserID         int64
	Platform       string
	PlatformUserID string
}

type legacyBenchmarkPrivateDataCacheEntry struct {
	once sync.Once
	data []byte
	err  error
}

// withLegacyBenchmarkRequestCache attaches a per-command cache for live snapshot source data.
// It is intentionally context-scoped so independent bot commands cannot share
// private snapshot payloads.
func withLegacyBenchmarkRequestCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if legacyBenchmarkCacheFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, legacyBenchmarkRequestCacheContextKey{}, &legacyBenchmarkRequestCache{
		privateData: make(map[legacyBenchmarkPrivateDataCacheKey]*legacyBenchmarkPrivateDataCacheEntry),
	})
}

func legacyBenchmarkCacheFromContext(ctx context.Context) *legacyBenchmarkRequestCache {
	if ctx == nil {
		return nil
	}
	cache, _ := ctx.Value(legacyBenchmarkRequestCacheContextKey{}).(*legacyBenchmarkRequestCache)
	return cache
}

func legacyBenchmarkCachedPrivateData(ctx context.Context, key legacyBenchmarkPrivateDataCacheKey, fetch func() ([]byte, error)) ([]byte, error, bool) {
	cache := legacyBenchmarkCacheFromContext(ctx)
	if cache == nil {
		finishFetch := commandtrace.MeasureOperation(ctx, "snapshot.private_data")
		data, err := fetch()
		finishFetch()
		return data, err, false
	}

	cache.mu.Lock()
	entry, hit := cache.privateData[key]
	if entry == nil {
		entry = &legacyBenchmarkPrivateDataCacheEntry{}
		cache.privateData[key] = entry
	}
	cache.mu.Unlock()

	didFetch := false
	waitStartedAt := time.Now()
	entry.once.Do(func() {
		didFetch = true
		finishFetch := commandtrace.MeasureOperation(ctx, "snapshot.private_data")
		entry.data, entry.err = fetch()
		if entry.data != nil {
			entry.data = append([]byte(nil), entry.data...)
		}
		finishFetch()
	})
	if !didFetch {
		commandtrace.RecordOperation(ctx, "snapshot.cache_wait", time.Since(waitStartedAt))
	}
	if entry.data == nil {
		return nil, entry.err, hit
	}
	finishCopy := commandtrace.MeasureOperation(ctx, "snapshot.cache_copy")
	data := append([]byte(nil), entry.data...)
	finishCopy()
	return data, entry.err, hit
}
