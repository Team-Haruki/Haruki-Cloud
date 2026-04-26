package sk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type predictRenderCache struct {
	mu      sync.RWMutex
	entries map[string]predictRenderCacheEntry
	flight  singleflight.Group
}

type predictRenderCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

type predictRenderCacheKey struct {
	Version          int    `json:"version"`
	Region           string `json:"region"`
	EventID          int    `json:"event_id"`
	WlCharacterID    int    `json:"wl_character_id,omitempty"`
	BucketStart      int64  `json:"bucket_start"`
	Full             bool   `json:"full,omitempty"`
	Ranks            []int  `json:"ranks,omitempty"`
	EventName        string `json:"event_name,omitempty"`
	EventStartAt     int64  `json:"event_start_at,omitempty"`
	EventAggregateAt int64  `json:"event_aggregate_at,omitempty"`
	BannerImgPath    string `json:"banner_img_path,omitempty"`
}

func newPredictRenderCache() *predictRenderCache {
	return &predictRenderCache{
		entries: make(map[string]predictRenderCacheEntry),
	}
}

const (
	predictRenderRefreshInterval = 30 * time.Minute
	predictFreezeBeforeEnd       = time.Hour
	predictRenderCacheRetention  = 2 * time.Hour
)

func (c *predictRenderCache) Render(key string, ttl time.Duration, render func() ([]byte, error)) ([]byte, error) {
	if c == nil || ttl <= 0 {
		return render()
	}
	if cached, ok := c.get(key); ok {
		return cached, nil
	}

	v, err, _ := c.flight.Do(key, func() (any, error) {
		if cached, ok := c.get(key); ok {
			return cached, nil
		}
		data, err := render()
		if err != nil {
			return nil, err
		}
		c.set(key, data, ttl)
		return cloneBytes(data), nil
	})
	if err != nil {
		return nil, err
	}
	return cloneBytes(v.([]byte)), nil
}

func (c *predictRenderCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return cloneBytes(entry.data), true
}

func (c *predictRenderCache) set(key string, data []byte, ttl time.Duration) {
	c.mu.Lock()
	c.entries[key] = predictRenderCacheEntry{
		data:      cloneBytes(data),
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

func buildPredictRenderCacheKey(req TrackerRankQuery, aggregateAt int64, now time.Time) (string, error) {
	ranks := append([]int(nil), req.Ranks...)
	sort.Ints(ranks)
	key := predictRenderCacheKey{
		Version:          1,
		Region:           strings.ToLower(strings.TrimSpace(req.Region)),
		EventID:          req.EventID,
		BucketStart:      predictRenderBucketStart(now, aggregateAt),
		Full:             req.Full,
		Ranks:            ranks,
		EventName:        strings.TrimSpace(stringValue(req.EventName)),
		EventStartAt:     int64Value(req.EventStartAt),
		EventAggregateAt: aggregateAt,
		BannerImgPath:    strings.TrimSpace(stringValue(req.BannerImgPath)),
	}
	if req.WlCharacterID != nil {
		key.WlCharacterID = *req.WlCharacterID
	}
	payload, err := json.Marshal(key)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func predictRenderBucketStart(now time.Time, aggregateAt int64) int64 {
	nowMs := now.UnixMilli()
	if aggregateAt > 0 {
		freezeStart := aggregateAt - int64(predictFreezeBeforeEnd/time.Millisecond)
		if nowMs >= freezeStart {
			return floorToBucketMillis(maxInt64(0, freezeStart-1), predictRenderRefreshInterval)
		}
	}
	return floorToBucketMillis(nowMs, predictRenderRefreshInterval)
}

func predictRenderCacheTTL() time.Duration {
	return predictRenderCacheRetention
}

func floorToBucketMillis(value int64, interval time.Duration) int64 {
	bucket := int64(interval / time.Millisecond)
	if bucket <= 0 {
		return value
	}
	return value / bucket * bucket
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	return append([]byte(nil), data...)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
