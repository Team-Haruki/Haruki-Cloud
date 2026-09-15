package music

import (
	"container/list"
	"slices"
	"sync"
	"time"
)

const (
	// chartBPMCacheEntries covers a whole FindMusicChartsByBPM scan (every
	// music x difficulty), hits and misses alike.
	chartBPMCacheEntries = 4096
	chartBPMCacheTTL     = 10 * time.Minute
	chartBPMCacheMissTTL = 30 * time.Second
)

// chartBPMCache is a bounded LRU of parsed chart BPM results and of charts
// known to be missing, so a store-backed asset slot is not re-read (or
// re-probed for every candidate) on each repeated lookup or BPM scan. Misses
// expire after missTTL so a newly published chart shows up quickly.
type chartBPMCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	max     int
	ttl     time.Duration
	missTTL time.Duration
	now     func() time.Time
}

type chartBPMCacheEntry struct {
	key       string
	parsed    *parsedChartBPM
	expiresAt time.Time
}

func newChartBPMCache(maxEntries int, ttl time.Duration) *chartBPMCache {
	return &chartBPMCache{
		entries: make(map[string]*list.Element, maxEntries),
		order:   list.New(),
		max:     maxEntries,
		ttl:     ttl,
		missTTL: min(ttl, chartBPMCacheMissTTL),
		now:     time.Now,
	}
}

// get returns the cached result for key. ok reports a live entry; a nil parsed
// with ok true is a cached miss.
func (c *chartBPMCache) get(key string) (*parsedChartBPM, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	entry := element.Value.(*chartBPMCacheEntry)
	if !c.now().Before(entry.expiresAt) {
		c.order.Remove(element)
		delete(c.entries, key)
		return nil, false
	}
	c.order.MoveToFront(element)
	if entry.parsed == nil {
		return nil, true
	}
	return cloneParsedChartBPM(entry.parsed), true
}

// cloneParsedChartBPM copies a parse so the cached value and the caller's value
// never share the Events backing array.
func cloneParsedChartBPM(parsed *parsedChartBPM) *parsedChartBPM {
	clone := *parsed
	clone.Events = slices.Clone(parsed.Events)
	return &clone
}

func (c *chartBPMCache) put(key string, parsed *parsedChartBPM) {
	if c == nil || parsed == nil {
		return
	}
	c.store(key, cloneParsedChartBPM(parsed), c.ttl)
}

// putMiss records that no candidate of key exists.
func (c *chartBPMCache) putMiss(key string) {
	if c == nil {
		return
	}
	c.store(key, nil, c.missTTL)
}

func (c *chartBPMCache) store(key string, parsed *parsedChartBPM, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	expiresAt := c.now().Add(ttl)
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*chartBPMCacheEntry)
		entry.parsed = parsed
		entry.expiresAt = expiresAt
		c.order.MoveToFront(element)
		return
	}
	c.entries[key] = c.order.PushFront(&chartBPMCacheEntry{key: key, parsed: parsed, expiresAt: expiresAt})
	for c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.entries, oldest.Value.(*chartBPMCacheEntry).key)
	}
}
