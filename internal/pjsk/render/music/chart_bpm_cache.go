package music

import (
	"container/list"
	"slices"
	"sync"
	"time"
)

const (
	chartBPMCacheEntries = 64
	chartBPMCacheTTL     = 10 * time.Minute
)

// chartBPMCache is a small LRU of parsed chart BPM results, so a store-backed
// asset slot is not re-read for every repeated lookup of the same chart.
type chartBPMCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	max     int
	ttl     time.Duration
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
		now:     time.Now,
	}
}

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
	c.mu.Lock()
	defer c.mu.Unlock()
	parsed = cloneParsedChartBPM(parsed)
	expiresAt := c.now().Add(c.ttl)
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
