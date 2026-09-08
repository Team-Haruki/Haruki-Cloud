package deck

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

const (
	remoteUserdataCacheEntries = 256
	remoteUserdataCacheTTL     = 10 * time.Minute
	remoteUserdataMaxHashBytes = 4096
)

// Owned by one target state. Only digests and remote handles are retained,
// never the user's payload or the compressed upload.
type remoteUserdataCache struct {
	mu      sync.Mutex
	items   map[[32]byte]*list.Element
	lru     list.List
	uploads singleflight.Group
}

type remoteUserdataEntry struct {
	key      [32]byte
	hash     string
	storedAt time.Time
}

func (c *remoteUserdataCache) get(key [32]byte) *remoteUserdataEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	el := c.items[key]
	if el == nil {
		return nil
	}
	entry := el.Value.(*remoteUserdataEntry)
	if time.Since(entry.storedAt) >= remoteUserdataCacheTTL {
		c.remove(el)
		return nil
	}
	c.lru.MoveToFront(el)
	return entry
}

func (c *remoteUserdataCache) put(key [32]byte, hash string) *remoteUserdataEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = make(map[[32]byte]*list.Element)
	}
	if old := c.items[key]; old != nil {
		c.remove(old)
	}
	entry := &remoteUserdataEntry{key: key, hash: hash, storedAt: time.Now()}
	c.items[key] = c.lru.PushFront(entry)
	for len(c.items) > remoteUserdataCacheEntries {
		c.remove(c.lru.Back())
	}
	return entry
}

func (c *remoteUserdataCache) invalidate(key [32]byte, failed *remoteUserdataEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el := c.items[key]; el != nil && el.Value.(*remoteUserdataEntry) == failed {
		// A delayed failure must not erase a newer upload, even if the service
		// returns the same deterministic hash after restarting.
		c.remove(el)
	}
}

func (c *remoteUserdataCache) remove(el *list.Element) {
	delete(c.items, el.Value.(*remoteUserdataEntry).key)
	c.lru.Remove(el)
}

func remoteUserdataDigest(ctx context.Context, data []byte) [32]byte {
	finish := commandtrace.MeasureOperation(ctx, "deck.userdata_hash")
	defer finish()
	return sha256.Sum256(data)
}

type remoteUserdataFlight struct {
	retryAfterLeaderCancel bool
	entry                  *remoteUserdataEntry
	operations             []commandtrace.Stats
}

func (r *RemoteDeckRecommender) cachedUserdata(ctx context.Context, state *remoteTargetState, key [32]byte, data []byte) (*remoteUserdataEntry, error) {
	cache := &state.userdata
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry := cache.get(key); entry != nil {
			commandtrace.RecordOperation(ctx, "deck.userdata_cache_hit", 0)
			return entry, nil
		}
		base := logger.DetachedContext(ctx)
		var leader atomic.Bool
		result := cache.uploads.DoChan(hex.EncodeToString(key[:]), func() (any, error) {
			leader.Store(true)
			if err := ctx.Err(); err != nil {
				return remoteUserdataFlight{retryAfterLeaderCancel: true}, err
			}
			if entry := cache.get(key); entry != nil {
				return remoteUserdataFlight{entry: entry}, nil
			}
			// Only the leader owns an upload copy. Its cancellation path drains
			// this flight before releasing request buffers and the upstream lease.
			owned := slices.Clone(data)
			shared, cancel := context.WithTimeout(base, r.readySharedTimeout())
			defer cancel()
			// Upload remains tied to the initiating request's upstream lease.
			// If it cancels, surviving waiters retry under their own request.
			stop := context.AfterFunc(ctx, cancel)
			defer stop()
			if ctx.Err() != nil {
				cancel()
			}
			shared, trace := commandtrace.WithNewTrace(shared)
			finish := commandtrace.MeasureOperation(shared, "deck.userdata_upload")
			payload := buildMultipartPayload(shared, owned)
			var response remoteUserDataCacheResponse
			err := r.postBinary(shared, &remoteExecution{state: state}, "/cache_userdata", payload, &response)
			if err == nil && (strings.TrimSpace(response.UserdataHash) == "" || len(response.UserdataHash) > remoteUserdataMaxHashBytes) {
				err = fmt.Errorf("deck-service cache_userdata returned empty userdata_hash or oversized hash")
			}
			if err == nil {
				err = shared.Err()
			}
			var entry *remoteUserdataEntry
			if err == nil {
				entry = cache.put(key, response.UserdataHash)
			}
			finish()
			return remoteUserdataFlight{entry: entry, operations: trace.Snapshot().Operations, retryAfterLeaderCancel: ctx.Err() != nil}, err
		})
		finishWait := commandtrace.MeasureOperation(ctx, "deck.userdata_wait")
		select {
		case <-ctx.Done():
			if leader.Load() {
				// Keep the initiating lease until its canceled HTTP operation
				// has stopped. Other waiters can cancel without waiting.
				outcome := <-result
				completed := outcome.Val.(remoteUserdataFlight)
				commandtrace.MergeOperations(ctx, completed.operations)
			}
			finishWait()
			return nil, ctx.Err()
		case outcome := <-result:
			finishWait()
			completed := outcome.Val.(remoteUserdataFlight)
			commandtrace.MergeOperations(ctx, completed.operations)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if outcome.Err != nil && completed.retryAfterLeaderCancel {
				continue
			}
			return completed.entry, outcome.Err
		}
	}
}
