package handler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"haruki-cloud/config"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	json "github.com/bytedance/sonic"
	"golang.org/x/sync/singleflight"
)

type sekaiProfileCacheEntry struct {
	raw       []byte
	expiresAt time.Time
}

var (
	sekaiProfileCacheMu          sync.RWMutex
	sekaiProfileCache            = make(map[string]sekaiProfileCacheEntry)
	sekaiProfileCacheGroup       singleflight.Group
	sekaiProfileCacheNextCleanup atomic.Int64
)

func fetchCachedSekaiUserProfile(_ context.Context, app *renderapp.App, region, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if app == nil || app.SekaiAPI == nil {
		return nil, sekaiapi.ErrClientNotConfigured
	}

	region = strings.ToLower(strings.TrimSpace(region))
	userID = strings.TrimSpace(userID)
	if region == "" || userID == "" {
		return nil, fmt.Errorf("sekai api: profile selector is incomplete")
	}

	ttl := config.Cfg.Backend.APICacheTTL
	if ttl <= 0 {
		return app.SekaiAPI.GetUserProfile(region, userID)
	}

	key := region + ":" + userID
	if resp, ok, err := loadCachedSekaiUserProfile(key, time.Now()); err != nil {
		return nil, err
	} else if ok {
		return resp, nil
	}

	value, err, _ := sekaiProfileCacheGroup.Do(key, func() (any, error) {
		now := time.Now()
		if resp, ok, err := loadCachedSekaiUserProfile(key, now); err != nil {
			return nil, err
		} else if ok {
			return resp, nil
		}

		resp, fetchErr := app.SekaiAPI.GetUserProfile(region, userID)
		if fetchErr != nil {
			return nil, fetchErr
		}

		raw, marshalErr := json.Marshal(resp)
		if marshalErr != nil {
			return resp, nil
		}

		storeCachedSekaiUserProfile(key, raw, now.Add(ttl), now, ttl)
		return decodeSekaiUserProfile(raw)
	})
	if err != nil {
		return nil, err
	}

	switch resolved := value.(type) {
	case *sekaiapi.GetAnotherProfileResponse:
		return resolved, nil
	default:
		return nil, fmt.Errorf("sekai api: unexpected cached profile result type %T", value)
	}
}

func loadCachedSekaiUserProfile(key string, now time.Time) (*sekaiapi.GetAnotherProfileResponse, bool, error) {
	sekaiProfileCacheMu.RLock()
	entry, ok := sekaiProfileCache[key]
	sekaiProfileCacheMu.RUnlock()
	if !ok {
		return nil, false, nil
	}

	if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		sekaiProfileCacheMu.Lock()
		if current, ok := sekaiProfileCache[key]; ok && current.expiresAt.Equal(entry.expiresAt) {
			delete(sekaiProfileCache, key)
		}
		sekaiProfileCacheMu.Unlock()
		return nil, false, nil
	}

	resp, err := decodeSekaiUserProfile(entry.raw)
	if err != nil {
		sekaiProfileCacheMu.Lock()
		delete(sekaiProfileCache, key)
		sekaiProfileCacheMu.Unlock()
		return nil, false, nil
	}
	return resp, true, nil
}

func decodeSekaiUserProfile(raw []byte) (*sekaiapi.GetAnotherProfileResponse, error) {
	var resp sekaiapi.GetAnotherProfileResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("sekai api: failed to unmarshal cached profile response: %w", err)
	}
	return &resp, nil
}

func storeCachedSekaiUserProfile(key string, raw []byte, expiresAt, now time.Time, ttl time.Duration) {
	sekaiProfileCacheMu.Lock()
	sekaiProfileCache[key] = sekaiProfileCacheEntry{
		raw:       append([]byte(nil), raw...),
		expiresAt: expiresAt,
	}
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
			delete(sekaiProfileCache, key)
		}
	}
	sekaiProfileCacheMu.Unlock()
}
