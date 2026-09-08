package snapshot

import (
	"container/list"
	"fmt"
	"slices"
	"sync"
	"time"

	json "haruki-cloud/internal/jsonutil"
)

// PrivateDataCache is a process-wide cache of Toolbox private-data payloads
// (suite / mysekai), shared across bot commands and keyed by game account
// (server, data type, uid) rather than by requester identity.
//
// Freshness and authorization are both enforced per request by the caller's
// own upstream read (see Fetch): a cached payload is only served after the
// caller's authorized request confirms the stored upload_time is still
// current. Because that read carries the same platform / platform_user_id
// authorization as a full fetch, an unauthorized caller fails it and never
// reaches cached data, so sharing by game account does not leak private
// payloads.
//
// Entries are immutable once stored: their data slice and uploadTime are never
// mutated in place, so a reader may share an entry's data after releasing the
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

// privateDataPayload is immutable after creation. Internal consumers may read
// data directly; a caller that exposes mutable bytes must use cloneBytes.
type privateDataPayload struct {
	data       []byte
	uploadTime int64
}

func newPrivateDataPayload(data []byte) privateDataPayload {
	uploadTime, _ := parseTopLevelUploadTime(data)
	return privateDataPayload{data: slices.Clone(data), uploadTime: uploadTime}
}

func (p privateDataPayload) cloneBytes() []byte {
	return slices.Clone(p.data)
}

type privateDataStoreEntry struct {
	privateDataPayload
	key      PrivateDataKey
	storedAt time.Time
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

// Fetch returns the private-data payload for key, serving the cached copy when
// upstream confirms the stored payload's upload_time is still current.
//
// fetch performs one authorized upstream read. It receives the cached entry's
// upload_time (0 when nothing usable is cached) and reports either the full
// payload or notModified=true when upstream validated that timestamp without
// resending the body. Authorization stays per request: the closure carries the
// caller's own platform identity, and a cached payload is only served after
// that caller's authorized request confirms the timestamp — the cache never
// substitutes one caller's authorization for another's. The returned bool
// reports whether the payload was served from the cross-request cache.
//
// Semantics:
//   - cold (no entry): fetch(0); parse and store the payload's upload_time.
//   - warm, unchanged: fetch(ts) reports notModified; serve the cached payload.
//   - warm, changed:   fetch(ts) returns the new payload; refresh the entry.
//   - any fetch error is returned as-is and the cached payload is never
//     served: the error may be an authorization revocation, not just a blip.
//
// A nil receiver bypasses caching entirely and always calls fetch(0), so the
// cache is an optional dependency.
func (c *PrivateDataCache) Fetch(
	key PrivateDataKey,
	fetch func(knownUploadTime int64) (data []byte, notModified bool, err error),
) ([]byte, bool, error) {
	var fetched []byte
	payload, hit, err := c.fetchPayload(key, func(known int64) ([]byte, bool, error) {
		data, notModified, err := fetch(known)
		fetched = data
		return data, notModified, err
	})
	if err != nil {
		return nil, false, err
	}
	if !hit {
		return fetched, false, nil
	}
	return payload.cloneBytes(), hit, err
}

// fetchPayload keeps the validated payload and its parsed version together,
// avoiding full-body copies and timestamp scans on the internal warm path.
func (c *PrivateDataCache) fetchPayload(
	key PrivateDataKey,
	fetch func(knownUploadTime int64) (data []byte, notModified bool, err error),
) (privateDataPayload, bool, error) {
	var cached *privateDataStoreEntry
	known := int64(0)
	if c != nil {
		if cached = c.load(key); cached != nil {
			known = cached.uploadTime
		}
	}

	data, notModified, err := fetch(known)
	if err != nil {
		return privateDataPayload{}, false, err
	}
	if notModified {
		if cached == nil {
			// Upstream cannot validate a timestamp this request never sent.
			return privateDataPayload{}, false, fmt.Errorf("snapshot: upstream reported not-modified without a cached payload")
		}
		return cached.privateDataPayload, true, nil
	}
	payload := newPrivateDataPayload(data)
	if c != nil && payload.uploadTime > 0 {
		c.storePayload(key, payload)
	}
	return payload, false, nil
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

func (c *PrivateDataCache) storePayload(key PrivateDataKey, payload privateDataPayload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		old := el.Value.(*privateDataStoreEntry)
		c.curBytes -= int64(len(old.data))
		// Replace with a fresh entry rather than mutating the existing one so a
		// concurrent reader that already captured the old entry keeps reading
		// immutable data.
		el.Value = &privateDataStoreEntry{key: key, privateDataPayload: payload, storedAt: time.Now()}
		c.curBytes += int64(len(payload.data))
		c.ll.MoveToFront(el)
	} else {
		el := c.ll.PushFront(&privateDataStoreEntry{key: key, privateDataPayload: payload, storedAt: time.Now()})
		c.items[key] = el
		c.curBytes += int64(len(payload.data))
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
	var envelope struct {
		UploadTime *int64 `json:"upload_time"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return 0, err
	}
	if envelope.UploadTime == nil {
		return 0, fmt.Errorf("upload_time is missing")
	}
	return *envelope.UploadTime, nil
}
