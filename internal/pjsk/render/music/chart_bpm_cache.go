package music

import (
	"container/list"
	"context"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"haruki-cloud/internal/observability/commandtrace"
)

const (
	// Keep the five regions' chart sets together instead of evicting one
	// region during the next region's scan. Hits and misses share this bound.
	chartBPMCacheEntries = 32768
	chartBPMCacheTTL     = 10 * time.Minute
	chartBPMCacheMissTTL = 30 * time.Second
)

// chartBPMCache is a bounded LRU of parsed chart BPM results and of charts
// known to be missing, so a store-backed asset slot is not re-read (or
// re-probed for every candidate) on each repeated lookup or BPM scan. Misses
// expire after missTTL so a newly published chart shows up quickly.
type chartBPMCache struct {
	flights singleflight.Group
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

type chartBPMReadResult struct {
	parsed     *parsedChartBPM
	found      bool
	operations []commandtrace.Stats
}

func (c *chartBPMCache) load(ctx context.Context, key string, read func(context.Context) (*parsedChartBPM, bool, error)) (*parsedChartBPM, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if c == nil {
		return read(ctx)
	}
	if parsed, ok := c.get(key); ok {
		commandtrace.RecordOperation(ctx, "music.chart_cache_hit", 0)
		return parsed, parsed != nil, nil
	}
	commandtrace.RecordOperation(ctx, "music.chart_cache_miss", 0)
	finish := commandtrace.MeasureOperation(ctx, "music.chart_fill_wait")
	defer finish()
	flight := c.flights.DoChan(key, func() (any, error) {
		if parsed, ok := c.get(key); ok {
			return chartBPMReadResult{parsed: parsed, found: parsed != nil}, nil
		}
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		shared, trace := commandtrace.WithNewTrace(shared)
		parsed, found, err := read(shared)
		if err == nil && shared.Err() == nil {
			if found {
				c.put(key, parsed)
			} else {
				c.putMiss(key)
			}
		}
		return chartBPMReadResult{parsed: parsed, found: found, operations: trace.Snapshot().Operations}, err
	})
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case result := <-flight:
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if result.Shared {
			commandtrace.RecordOperation(ctx, "music.chart_fill_shared", 0)
		}
		value, _ := result.Val.(chartBPMReadResult)
		commandtrace.MergeOperations(ctx, value.operations)
		if value.parsed != nil {
			value.parsed = cloneParsedChartBPM(value.parsed)
		}
		return value.parsed, value.found, result.Err
	}
}
