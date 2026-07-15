package snapshot

import (
	"container/list"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

// PrivateDataCache is a process-wide cache of Toolbox private-data payloads
// (suite / mysekai), shared across bot commands and keyed by game account
// (server, data type, uid) rather than by requester identity.
//
// Freshness and authorization are both enforced per request by the caller's
// own upload_time probe (see Fetch): a cached payload is only served after the
// caller independently obtains a matching, authorized upload_time. Because the
// probe carries the same platform / platform_user_id authorization as a full
// fetch, an unauthorized caller fails the probe and never reaches cached data,
// so sharing by game account does not leak private payloads.
//
// Entries are immutable once stored: their data slice and uploadTime are never
// mutated in place, so a reader may copy an entry's data after releasing the
// lock without racing a concurrent store or eviction.
type PrivateDataCache struct {
	mu         sync.Mutex
	ll         *list.List
	items      map[PrivateDataKey]*list.Element
	curBytes   int64
	maxBytes   int64
	maxEntries int
	ttl        time.Duration
}

// PrivateDataKey identifies a private-data payload by game account. It
// deliberately excludes the requester identity so authorized callers share one
// cached payload per account.
type PrivateDataKey struct {
	Server   string
	DataType string
	UID      int64
}

type privateDataStoreEntry struct {
	key        PrivateDataKey
	data       []byte
	uploadTime int64
	storedAt   time.Time
}

const (
	defaultPrivateDataCacheMaxBytes   = 256 << 20 // 256 MiB
	defaultPrivateDataCacheMaxEntries = 20_000
	defaultPrivateDataCacheTTL        = 30 * time.Minute
)

// NewPrivateDataCache builds a cache with default bounds. The bounds only
// govern memory retention; correctness is guaranteed by per-request upload_time
// validation regardless of eviction.
func NewPrivateDataCache() *PrivateDataCache {
	return &PrivateDataCache{
		ll:         list.New(),
		items:      make(map[PrivateDataKey]*list.Element),
		maxBytes:   defaultPrivateDataCacheMaxBytes,
		maxEntries: defaultPrivateDataCacheMaxEntries,
		ttl:        defaultPrivateDataCacheTTL,
	}
}

// Fetch returns the private-data payload for key, using the cache when a stored
// payload's upload_time still matches the account's current upload_time.
//
// getUploadTime is a cheap, authorized probe of the account's current
// upload_time. fullFetch retrieves the complete payload (also authorized).
// Both are supplied by the caller so each request performs its own upstream
// authorization — the cache never substitutes one caller's authorization for
// another's. The returned bool reports whether the payload was served from the
// cross-request cache.
//
// Semantics:
//   - cold (no entry): fullFetch only; parse and store the payload's upload_time.
//   - warm, unchanged: one getUploadTime probe, serve the cached payload.
//   - warm, changed:   getUploadTime probe then fullFetch; refresh the entry.
//   - any probe error (auth / not-found / transient) is returned as-is and the
//     cached payload is never served.
//
// A nil receiver bypasses caching entirely and just calls fullFetch, so the
// cache is an optional dependency.
func (c *PrivateDataCache) Fetch(
	key PrivateDataKey,
	getUploadTime func() (int64, error),
	fullFetch func() ([]byte, error),
) ([]byte, bool, error) {
	if c == nil {
		data, err := fullFetch()
		return data, false, err
	}

	if entry := c.load(key); entry != nil {
		uploadTime, err := getUploadTime()
		if err != nil {
			// Never serve cached private data when the authorized probe fails:
			// the error may be an authorization revocation, not just a blip.
			return nil, false, err
		}
		if uploadTime == entry.uploadTime {
			return append([]byte(nil), entry.data...), true, nil
		}
	}

	data, err := fullFetch()
	if err != nil {
		return nil, false, err
	}
	if uploadTime, perr := parseTopLevelUploadTime(data); perr == nil && uploadTime > 0 {
		c.store(key, data, uploadTime)
	}
	return data, false, nil
}

func (c *PrivateDataCache) load(key PrivateDataKey) *privateDataStoreEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil
	}
	entry := el.Value.(*privateDataStoreEntry)
	if c.ttl > 0 && time.Since(entry.storedAt) > c.ttl {
		c.removeElementLocked(el)
		return nil
	}
	c.ll.MoveToFront(el)
	return entry
}

func (c *PrivateDataCache) store(key PrivateDataKey, data []byte, uploadTime int64) {
	dataCopy := append([]byte(nil), data...)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		old := el.Value.(*privateDataStoreEntry)
		c.curBytes -= int64(len(old.data))
		// Replace with a fresh entry rather than mutating the existing one so a
		// concurrent reader that already captured the old entry keeps reading
		// immutable data.
		el.Value = &privateDataStoreEntry{key: key, data: dataCopy, uploadTime: uploadTime, storedAt: time.Now()}
		c.curBytes += int64(len(dataCopy))
		c.ll.MoveToFront(el)
	} else {
		el := c.ll.PushFront(&privateDataStoreEntry{key: key, data: dataCopy, uploadTime: uploadTime, storedAt: time.Now()})
		c.items[key] = el
		c.curBytes += int64(len(dataCopy))
	}
	c.evictLocked()
}

func (c *PrivateDataCache) evictLocked() {
	for (c.maxEntries > 0 && c.ll.Len() > c.maxEntries) || (c.maxBytes > 0 && c.curBytes > c.maxBytes) {
		el := c.ll.Back()
		if el == nil {
			return
		}
		c.removeElementLocked(el)
	}
}

func (c *PrivateDataCache) removeElementLocked(el *list.Element) {
	entry := el.Value.(*privateDataStoreEntry)
	c.ll.Remove(el)
	delete(c.items, entry.key)
	c.curBytes -= int64(len(entry.data))
}

// parseTopLevelUploadTime extracts the top-level upload_time (seconds) embedded
// in a full private-data payload, matching the value the key=upload_time probe
// returns. Returns an error when the field is absent or unparseable, in which
// case the caller skips caching.
func parseTopLevelUploadTime(payload []byte) (int64, error) {
	node, err := sonic.Get(payload, "upload_time")
	if err != nil {
		return 0, err
	}
	return node.Int64()
}
