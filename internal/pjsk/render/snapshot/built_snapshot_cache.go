package snapshot

import (
	"container/list"
	"sync"
	"time"
)

// BuiltSnapshotCache memoizes fully built Snapshots across commands so a warm
// render of unchanged data skips factory.Build entirely — the JSON normalize,
// the full suite unmarshal, the leader-image DB lookup, and the model
// transforms.
//
// Correctness comes from the key: a built snapshot is fully determined by its
// region, account, and the upload_time of each source payload, so an entry can
// only ever be reused for byte-identical inputs. Freshness is inherited from the
// private-data layer — the caller reaches this cache only after an authorized
// upload_time probe validated those same upload_times for this request, so a
// changed account produces a different key and a fresh build.
//
// The stored Snapshot is shared read-only across concurrent requests. This is
// safe because every Snapshot accessor returns freshly built/defensively copied
// values and no accessor mutates the Snapshot; the two non-copying accessors
// (RawData, MusicMetaView) expose data every caller treats as read-only.
type BuiltSnapshotCache struct {
	mu         sync.Mutex
	ll         *list.List
	items      map[builtSnapshotKey]*list.Element
	maxEntries int
	maxBytes   int64
	curBytes   int64
	ttl        time.Duration
}

// builtSnapshotKey fully determines a built snapshot. MySekaiUploadTime is 0
// when NeedMySekai is false (mysekai data is not part of the snapshot).
type builtSnapshotKey struct {
	Region            string
	UID               int64
	SuiteUploadTime   int64
	NeedMySekai       bool
	MySekaiUploadTime int64
}

type builtSnapshotEntry struct {
	key         builtSnapshotKey
	snapshot    Snapshot
	storedAt    time.Time
	approxBytes int64
}

const (
	// Built snapshots retain both the parsed model and the raw JSON, so they are
	// larger per entry than raw payloads; keep the population bound conservative
	// and rely on the raw-bytes cache + factory.Build for the long tail.
	defaultBuiltSnapshotCacheMaxEntries = 2048
	defaultBuiltSnapshotCacheTTL        = 30 * time.Minute
	// defaultBuiltSnapshotCacheMaxBytes bounds the estimated retained memory.
	// Keys embed upload_time, so once an account uploads new data the old
	// entry is never queried again and Get's lazy TTL check never fires for
	// it — without a byte bound and the Put-time tail sweep, 2048 multi-MB
	// churned entries could pin >20GiB.
	defaultBuiltSnapshotCacheMaxBytes = 1 << 30 // 1GiB estimated
	// builtSnapshotSizeFactor scales source payload size to an estimate of a
	// built entry's footprint: the raw JSON clone, the parsed model, and the
	// derived structures each retain roughly one payload's worth of data.
	builtSnapshotSizeFactor = 3
)

// NewBuiltSnapshotCache builds a cache with default bounds. Bounds govern only
// memory retention; correctness never depends on retention.
func NewBuiltSnapshotCache() *BuiltSnapshotCache {
	return NewBuiltSnapshotCacheWithLimits(
		defaultBuiltSnapshotCacheMaxEntries,
		defaultBuiltSnapshotCacheMaxBytes,
		defaultBuiltSnapshotCacheTTL,
	)
}

// NewBuiltSnapshotCacheWithLimits builds a cache with explicit bounds. A
// non-positive maxEntries or maxBytes disables that bound; a non-positive ttl
// disables expiry.
func NewBuiltSnapshotCacheWithLimits(maxEntries int, maxBytes int64, ttl time.Duration) *BuiltSnapshotCache {
	return &BuiltSnapshotCache{
		ll:         list.New(),
		items:      make(map[builtSnapshotKey]*list.Element),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
	}
}

// Get returns the memoized snapshot for key, or nil on miss / expiry. A nil
// receiver always misses, so the cache is an optional dependency.
func (c *BuiltSnapshotCache) Get(key builtSnapshotKey) Snapshot {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil
	}
	entry := el.Value.(*builtSnapshotEntry)
	if c.ttl > 0 && time.Since(entry.storedAt) > c.ttl {
		c.removeElementLocked(el)
		return nil
	}
	c.ll.MoveToFront(el)
	return entry.snapshot
}

// Put stores a built snapshot under key. payloadBytes is the total size of the
// source payloads the snapshot was built from (suite + mysekai JSON); the
// retained footprint is estimated as payloadBytes×builtSnapshotSizeFactor.
// A nil receiver or snapshot is a no-op.
func (c *BuiltSnapshotCache) Put(key builtSnapshotKey, snapshot Snapshot, payloadBytes int64) {
	if c == nil || snapshot == nil {
		return
	}
	approx := payloadBytes * builtSnapshotSizeFactor
	if approx < 0 {
		approx = 0
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweepExpiredTailLocked(now)
	if el, ok := c.items[key]; ok {
		c.curBytes += approx - el.Value.(*builtSnapshotEntry).approxBytes
		el.Value = &builtSnapshotEntry{key: key, snapshot: snapshot, storedAt: now, approxBytes: approx}
		c.ll.MoveToFront(el)
	} else {
		el = c.ll.PushFront(&builtSnapshotEntry{key: key, snapshot: snapshot, storedAt: now, approxBytes: approx})
		c.items[key] = el
		c.curBytes += approx
	}
	for (c.maxEntries > 0 && c.ll.Len() > c.maxEntries) || (c.maxBytes > 0 && c.curBytes > c.maxBytes) {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.removeElementLocked(back)
	}
}

// sweepExpiredTailLocked drops expired entries from the LRU tail. Entries
// whose key was churned by a new upload are never Get-ed again, so Get's lazy
// expiry cannot reclaim them; they sink to the tail as live entries stay hot,
// which makes a stop-at-first-fresh tail walk an amortized-O(1) reclaim.
func (c *BuiltSnapshotCache) sweepExpiredTailLocked(now time.Time) {
	if c.ttl <= 0 {
		return
	}
	for {
		back := c.ll.Back()
		if back == nil {
			return
		}
		entry := back.Value.(*builtSnapshotEntry)
		if now.Sub(entry.storedAt) <= c.ttl {
			return
		}
		c.removeElementLocked(back)
	}
}

func (c *BuiltSnapshotCache) removeElementLocked(el *list.Element) {
	entry := el.Value.(*builtSnapshotEntry)
	c.ll.Remove(el)
	delete(c.items, entry.key)
	c.curBytes -= entry.approxBytes
	if c.curBytes < 0 {
		c.curBytes = 0
	}
}
