package sk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type predictRenderCache struct {
	mu      sync.Mutex
	entries map[string]*predictRenderCacheEntry
}

type predictRenderCacheEntry struct {
	data          []byte
	lastError     string
	nextRefreshAt time.Time
	expiresAt     time.Time
	running       bool
	readyCh       chan struct{}
}

type predictRenderCacheKey struct {
	Version          int    `json:"version"`
	Region           string `json:"region"`
	EventID          int    `json:"event_id"`
	WlCharacterID    int    `json:"wl_character_id,omitempty"`
	Full             bool   `json:"full,omitempty"`
	Ranks            []int  `json:"ranks,omitempty"`
	EventName        string `json:"event_name,omitempty"`
	EventStartAt     int64  `json:"event_start_at,omitempty"`
	EventAggregateAt int64  `json:"event_aggregate_at,omitempty"`
	BannerImgPath    string `json:"banner_img_path,omitempty"`
}

func newPredictRenderCache() *predictRenderCache {
	return &predictRenderCache{
		entries: make(map[string]*predictRenderCacheEntry),
	}
}

var (
	predictRenderRefreshInterval      = 5 * time.Minute
	predictRenderFailureRetryInterval = 10 * time.Second
	predictRenderFailureRetryLimit    = 5
	predictRenderCacheRetention       = 2 * time.Hour
)

func (c *predictRenderCache) StartManaged(key string, now time.Time, render func() ([]byte, error)) {
	if c == nil || render == nil {
		return
	}
	c.ensureWorker(key, now, render)
}

func (c *predictRenderCache) RenderManaged(key string, now time.Time, render func() ([]byte, error)) ([]byte, error) {
	if c == nil || render == nil {
		return render()
	}
	entry := c.ensureWorker(key, now, render)
	if entry == nil {
		return nil, errors.New("predict cache is not initialized")
	}

	if len(entry.data) > 0 {
		return cloneBytes(entry.data), nil
	}
	if entry.readyCh != nil {
		<-entry.readyCh
	}
	return c.result(key, time.Now())
}

func (c *predictRenderCache) ensureWorker(key string, now time.Time, render func() ([]byte, error)) *predictRenderCacheEntry {
	c.mu.Lock()
	entry := c.entries[key]
	startWorker := false
	if entry == nil || (now.After(entry.expiresAt) && !entry.running) {
		entry = &predictRenderCacheEntry{}
		c.entries[key] = entry
	}
	if !entry.running {
		entry.running = true
		if len(entry.data) == 0 && entry.readyCh == nil {
			entry.readyCh = make(chan struct{})
		}
		startWorker = true
	}
	clone := *entry
	c.mu.Unlock()

	if startWorker {
		go c.runWorker(key, render)
	}
	return &clone
}

func (c *predictRenderCache) runWorker(key string, render func() ([]byte, error)) {
	for {
		if wait := c.waitUntilNextRefresh(key, time.Now()); wait > 0 {
			time.Sleep(wait)
		}

		data, err := c.runRefreshCycle(render)
		now := time.Now()
		if err == nil {
			c.finishSuccess(key, now, data)
			continue
		}
		c.finishFailure(key, now, err)
	}
}

func (c *predictRenderCache) waitUntilNextRefresh(key string, now time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.entries[key]
	if entry == nil || entry.nextRefreshAt.IsZero() || !now.Before(entry.nextRefreshAt) {
		return 0
	}
	return entry.nextRefreshAt.Sub(now)
}

func (c *predictRenderCache) runRefreshCycle(render func() ([]byte, error)) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= predictRenderFailureRetryLimit; attempt++ {
		data, err := render()
		if err == nil && len(data) > 0 {
			return data, nil
		}
		if err == nil {
			err = errors.New("forecast render returned empty payload")
		}
		lastErr = err
		if attempt < predictRenderFailureRetryLimit {
			time.Sleep(predictRenderFailureRetryInterval)
		}
	}
	return nil, lastErr
}

func (c *predictRenderCache) finishSuccess(key string, now time.Time, data []byte) {
	c.mu.Lock()
	entry := c.entries[key]
	if entry == nil {
		entry = &predictRenderCacheEntry{}
		c.entries[key] = entry
	}
	waitCh := entry.readyCh
	entry.data = cloneBytes(data)
	entry.lastError = ""
	entry.nextRefreshAt = now.Add(predictRenderRefreshInterval)
	entry.expiresAt = now.Add(predictRenderCacheRetention)
	entry.running = true
	entry.readyCh = nil
	c.mu.Unlock()

	if waitCh != nil {
		close(waitCh)
	}
}

func (c *predictRenderCache) finishFailure(key string, now time.Time, err error) {
	c.mu.Lock()
	entry := c.entries[key]
	if entry == nil {
		entry = &predictRenderCacheEntry{}
		c.entries[key] = entry
	}
	waitCh := entry.readyCh
	entry.lastError = err.Error()
	entry.nextRefreshAt = now.Add(predictRenderRefreshInterval)
	entry.expiresAt = now.Add(predictRenderCacheRetention)
	entry.running = true
	entry.readyCh = nil
	c.mu.Unlock()

	if waitCh != nil {
		close(waitCh)
	}
}

func (c *predictRenderCache) result(key string, now time.Time) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.entries[key]
	if entry == nil {
		return nil, errors.New("predict cache entry not found")
	}
	if now.After(entry.expiresAt) && len(entry.data) == 0 {
		return nil, errors.New("predict cache entry expired")
	}
	if len(entry.data) > 0 {
		return cloneBytes(entry.data), nil
	}
	if entry.lastError != "" {
		return nil, errors.New(entry.lastError)
	}
	return nil, errors.New("predict cache entry is empty")
}

func buildPredictRenderCacheKey(req TrackerRankQuery, aggregateAt int64) (string, error) {
	ranks := append([]int(nil), req.Ranks...)
	sort.Ints(ranks)
	key := predictRenderCacheKey{
		Version:          4,
		Region:           strings.ToLower(strings.TrimSpace(req.Region)),
		EventID:          req.EventID,
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

func predictRenderCacheTTL() time.Duration {
	return predictRenderCacheRetention
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
