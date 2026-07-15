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
	key      builtSnapshotKey
	snapshot Snapshot
	storedAt time.Time
}

const (
	// Built snapshots retain both the parsed model and the raw JSON, so they are
	// larger per entry than raw payloads; keep the population bound conservative
	// and rely on the raw-bytes cache + factory.Build for the long tail.
	defaultBuiltSnapshotCacheMaxEntries = 2048
	defaultBuiltSnapshotCacheTTL        = 30 * time.Minute
)

// NewBuiltSnapshotCache builds a cache with default bounds. Bounds govern only
// memory retention; correctness never depends on retention.
func NewBuiltSnapshotCache() *BuiltSnapshotCache {
	return &BuiltSnapshotCache{
		ll:         list.New(),
		items:      make(map[builtSnapshotKey]*list.Element),
		maxEntries: defaultBuiltSnapshotCacheMaxEntries,
		ttl:        defaultBuiltSnapshotCacheTTL,
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
		c.ll.Remove(el)
		delete(c.items, entry.key)
		return nil
	}
	c.ll.MoveToFront(el)
	return entry.snapshot
}

// Put stores a built snapshot under key. A nil receiver or snapshot is a no-op.
func (c *BuiltSnapshotCache) Put(key builtSnapshotKey, snapshot Snapshot) {
	if c == nil || snapshot == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value = &builtSnapshotEntry{key: key, snapshot: snapshot, storedAt: time.Now()}
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&builtSnapshotEntry{key: key, snapshot: snapshot, storedAt: time.Now()})
	c.items[key] = el
	for c.maxEntries > 0 && c.ll.Len() > c.maxEntries {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.ll.Remove(back)
		delete(c.items, back.Value.(*builtSnapshotEntry).key)
	}
}
