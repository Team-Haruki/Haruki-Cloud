package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
	json "haruki-cloud/internal/jsonutil"
)

type sekaiProfileCacheEntry struct {
	raw       []byte
	expiresAt time.Time
	sequence  uint64
}

const (
	sekaiProfileCacheMaxItemBytes  = 16 << 20
	sekaiProfileCacheMaxEntries    = 256
	sekaiProfileCacheMaxTotalBytes = 128 << 20
)

type sekaiProfileFlightResult struct {
	raw           []byte
	err           error
	httpElapsed   time.Duration
	encodeElapsed time.Duration
	operations    []commandtrace.Stats
	leader        *sekaiProfileFlightToken
}

type sekaiProfileFlightToken byte

var (
	sekaiProfileCacheMu          sync.RWMutex
	sekaiProfileCache            = make(map[string]sekaiProfileCacheEntry)
	sekaiProfileCacheGroup       singleflight.Group
	sekaiProfileCacheNextCleanup atomic.Int64
	sekaiProfileCacheBytes       int64
	sekaiProfileCacheSequence    uint64
)

func fetchCachedSekaiUserProfile(ctx context.Context, app *renderapp.App, region, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if app == nil || app.SekaiAPI == nil {
		return nil, sekaiapi.ErrClientNotConfigured
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	region = strings.ToLower(strings.TrimSpace(region))
	userID = strings.TrimSpace(userID)
	if region == "" || userID == "" {
		return nil, fmt.Errorf("sekai api: profile selector is incomplete")
	}

	ttl := config.Cfg.Backend.APICacheTTL
	if ttl <= 0 {
		return app.SekaiAPI.WithContext(ctx).GetUserProfile(region, userID)
	}

	key := region + ":" + userID
	finishLookup := commandtrace.MeasureOperation(ctx, "sekai.profile_cache_lookup")
	resp, ok, err := loadCachedSekaiUserProfile(key, time.Now())
	finishLookup()
	if err != nil {
		return nil, err
	} else if ok {
		return resp, nil
	}

	callerToken := new(sekaiProfileFlightToken)
	resultCh := sekaiProfileCacheGroup.DoChan(key, func() (any, error) {
		detached := context.Background()
		detached = logger.WithContextAttrs(detached, slog.Bool("shared_work", true))
		sharedBase, cancel := context.WithTimeout(detached, config.HTTPClientTimeout)
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
		complete := func(result sekaiProfileFlightResult) sekaiProfileFlightResult {
			result.operations = trace.Snapshot().Operations
			return result
		}
		client := app.SekaiAPI.WithContext(sharedCtx)
		now := time.Now()
		if raw, ok := loadCachedSekaiUserProfileRaw(key, now); ok {
			return complete(sekaiProfileFlightResult{raw: raw, leader: callerToken}), nil
		}

		startedAt := time.Now()
		resp, fetchErr := client.GetUserProfile(region, userID)
		flightResult := sekaiProfileFlightResult{httpElapsed: time.Since(startedAt), leader: callerToken}
		if fetchErr != nil {
			flightResult.err = fetchErr
			return complete(flightResult), nil
		}

		startedAt = time.Now()
		raw, marshalErr := json.Marshal(resp)
		flightResult.encodeElapsed = time.Since(startedAt)
		if marshalErr != nil {
			flightResult.err = fmt.Errorf("sekai api: failed to encode profile response for cache: %w", marshalErr)
			return complete(flightResult), nil
		}

		storeCachedSekaiUserProfile(key, raw, now.Add(ttl), now, ttl)
		flightResult.raw = raw
		return complete(flightResult), nil
	})
	finishWait := commandtrace.MeasureOperation(ctx, "sekai.profile_cache_wait")
	var result singleflight.Result
	select {
	case <-ctx.Done():
		finishWait()
		return nil, ctx.Err()
	case result = <-resultCh:
		finishWait()
	}
	if result.Err != nil {
		return nil, result.Err
	}
	resolved, ok := result.Val.(sekaiProfileFlightResult)
	if !ok {
		return nil, fmt.Errorf("sekai api: unexpected cached profile result type %T", result.Val)
	}
	commandtrace.MergeOperations(ctx, resolved.operations)
	if resolved.httpElapsed > 0 {
		commandtrace.RecordOperation(ctx, "sekai.profile_fetch", resolved.httpElapsed)
	}
	if resolved.encodeElapsed > 0 {
		commandtrace.RecordOperation(ctx, "sekai.profile_encode", resolved.encodeElapsed)
	}
	if resolved.leader != callerToken {
		commandtrace.RecordOperation(ctx, "sekai.profile_cache_shared", 0)
	}
	if resolved.err != nil {
		return nil, resolved.err
	}
	startedAt := time.Now()
	response, decodeErr := decodeSekaiUserProfile(resolved.raw)
	commandtrace.RecordOperation(ctx, "sekai.profile_decode", time.Since(startedAt))
	return response, decodeErr
}

func loadCachedSekaiUserProfile(key string, now time.Time) (*sekaiapi.GetAnotherProfileResponse, bool, error) {
	raw, ok := loadCachedSekaiUserProfileRaw(key, now)
	if !ok {
		return nil, false, nil
	}

	resp, err := decodeSekaiUserProfile(raw)
	if err != nil {
		sekaiProfileCacheMu.Lock()
		deleteSekaiUserProfileCacheEntryLocked(key)
		sekaiProfileCacheMu.Unlock()
		return nil, false, nil
	}
	return resp, true, nil
}

func loadCachedSekaiUserProfileRaw(key string, now time.Time) ([]byte, bool) {
	sekaiProfileCacheMu.RLock()
	entry, ok := sekaiProfileCache[key]
	sekaiProfileCacheMu.RUnlock()
	if !ok {
		return nil, false
	}

	if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		sekaiProfileCacheMu.Lock()
		if current, ok := sekaiProfileCache[key]; ok && current.sequence == entry.sequence {
			deleteSekaiUserProfileCacheEntryLocked(key)
		}
		sekaiProfileCacheMu.Unlock()
		return nil, false
	}
	return append([]byte(nil), entry.raw...), true
}

func decodeSekaiUserProfile(raw []byte) (*sekaiapi.GetAnotherProfileResponse, error) {
	var resp sekaiapi.GetAnotherProfileResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal cached profile response: %w", err)
	}
	return &resp, nil
}

func storeCachedSekaiUserProfile(key string, raw []byte, expiresAt, now time.Time, ttl time.Duration) {
	if len(raw) > sekaiProfileCacheMaxItemBytes {
		return
	}
	storedRaw := append([]byte(nil), raw...)

	sekaiProfileCacheMu.Lock()
	deleteSekaiUserProfileCacheEntryLocked(key)
	sekaiProfileCacheSequence++
	sekaiProfileCache[key] = sekaiProfileCacheEntry{
		raw:       storedRaw,
		expiresAt: expiresAt,
		sequence:  sekaiProfileCacheSequence,
	}
	sekaiProfileCacheBytes += int64(len(storedRaw))
	evictSekaiUserProfileCacheLocked(sekaiProfileCacheMaxEntries, sekaiProfileCacheMaxTotalBytes)
	sekaiProfileCacheMu.Unlock()

	maybeCleanupSekaiUserProfileCache(now, ttl)
}

func maybeCleanupSekaiUserProfileCache(now time.Time, ttl time.Duration) {
	interval := ttl
	if interval < time.Minute {
		interval = time.Minute
	}

	oldNext := sekaiProfileCacheNextCleanup.Load()
	if oldNext != 0 && now.UnixNano() < oldNext {
		return
	}
	newNext := now.Add(interval).UnixNano()
	if !sekaiProfileCacheNextCleanup.CompareAndSwap(oldNext, newNext) {
		return
	}

	sekaiProfileCacheMu.Lock()
	for key, entry := range sekaiProfileCache {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			deleteSekaiUserProfileCacheEntryLocked(key)
		}
	}
	sekaiProfileCacheMu.Unlock()
}

func evictSekaiUserProfileCacheLocked(maxEntries int, maxBytes int64) {
	for len(sekaiProfileCache) > maxEntries || sekaiProfileCacheBytes > maxBytes {
		oldestKey := ""
		oldestSequence := ^uint64(0)
		for key, entry := range sekaiProfileCache {
			if entry.sequence < oldestSequence {
				oldestKey = key
				oldestSequence = entry.sequence
			}
		}
		if oldestKey == "" {
			sekaiProfileCacheBytes = 0
			return
		}
		deleteSekaiUserProfileCacheEntryLocked(oldestKey)
	}
}

func deleteSekaiUserProfileCacheEntryLocked(key string) {
	entry, ok := sekaiProfileCache[key]
	if !ok {
		return
	}
	delete(sekaiProfileCache, key)
	sekaiProfileCacheBytes -= int64(len(entry.raw))
	if sekaiProfileCacheBytes < 0 {
		sekaiProfileCacheBytes = 0
	}
}
