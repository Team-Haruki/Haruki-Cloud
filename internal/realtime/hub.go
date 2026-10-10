package realtime

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// connQueueSize bounds the events buffered for one stream. A full queue
// drops the delivery; the redelivery sweep sends it again.
const connQueueSize = 16

// ErrTooManyConnections is returned when the global stream cap is reached.
var ErrTooManyConnections = errors.New("realtime: too many connections")

// Close reasons reported by a stream that the hub ended.
const (
	CloseReasonRequested = "close_requested"
	CloseReasonStale     = "subscription_stale"
	CloseReasonEvicted   = "evicted"
	CloseReasonShutdown  = "shutdown"
)

type delivery struct {
	event      Event
	redelivery bool
}

// conn is one live SSE stream in the hub.
type conn struct {
	id          uint64
	key         Key
	connectedAt time.Time
	queue       chan delivery
	closed      chan struct{}
	closeOnce   sync.Once
	reasonMu    sync.Mutex
	reason      string
}

func (c *conn) close(reason string) {
	c.closeOnce.Do(func() {
		c.reasonMu.Lock()
		c.reason = reason
		c.reasonMu.Unlock()
		close(c.closed)
	})
}

func (c *conn) closeReason() string {
	c.reasonMu.Lock()
	defer c.reasonMu.Unlock()
	return c.reason
}

// Hub tracks the live streams of this process, keyed by subscription
// version. It is the in-process fan-out; persistence is the Store's job.
type Hub struct {
	mu        sync.Mutex
	streams   map[Key]map[uint64]*conn
	total     int
	nextID    uint64
	maxConns  int
	maxPerKey int
	now       func() time.Time
}

// NewHub returns a hub with a global cap of maxConns streams and at most
// maxPerKey streams per subscription version.
func NewHub(maxConns, maxPerKey int) *Hub {
	return &Hub{
		streams:   make(map[Key]map[uint64]*conn),
		maxConns:  maxConns,
		maxPerKey: maxPerKey,
		now:       time.Now,
	}
}

// Register adds a stream for key. Over the per-key cap it closes the oldest
// streams of key (a reconnecting Client can leave half-open streams behind
// that heartbeats only notice much later). Over the global cap it fails.
func (h *Hub) Register(key Key) (*conn, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	streams := h.streams[key]
	evictable := 0
	if h.maxPerKey > 0 && len(streams) >= h.maxPerKey {
		evictable = len(streams) - h.maxPerKey + 1
	}
	if h.maxConns > 0 && h.total-evictable >= h.maxConns {
		return nil, ErrTooManyConnections
	}
	if evictable > 0 {
		for _, old := range oldestFirst(streams)[:evictable] {
			h.removeLocked(old)
			old.close(CloseReasonEvicted)
		}
	}
	h.nextID++
	c := &conn{
		id:          h.nextID,
		key:         key,
		connectedAt: h.now(),
		queue:       make(chan delivery, connQueueSize),
		closed:      make(chan struct{}),
	}
	if h.streams[key] == nil {
		h.streams[key] = make(map[uint64]*conn)
	}
	h.streams[key][c.id] = c
	h.total++
	return c, nil
}

func oldestFirst(streams map[uint64]*conn) []*conn {
	list := make([]*conn, 0, len(streams))
	for _, c := range streams {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id < list[j].id })
	return list
}

// Unregister removes the stream; it is a no-op for a stream already removed.
func (h *Hub) Unregister(c *conn) {
	if c == nil {
		return
	}
	h.mu.Lock()
	h.removeLocked(c)
	h.mu.Unlock()
}

func (h *Hub) removeLocked(c *conn) {
	streams := h.streams[c.key]
	if _, ok := streams[c.id]; !ok {
		return
	}
	delete(streams, c.id)
	h.total--
	if len(streams) == 0 {
		delete(h.streams, c.key)
	}
}

// Online reports whether key has at least one live stream.
func (h *Hub) Online(key Key) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.streams[key]) > 0
}

// Deliver queues the event on every stream of its key without blocking and
// returns how many streams accepted it.
func (h *Hub) Deliver(event Event, redelivery bool) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	accepted := 0
	for _, c := range h.streams[event.Key()] {
		select {
		case c.queue <- delivery{event: event, redelivery: redelivery}:
			accepted++
		default:
		}
	}
	return accepted
}

// Keys returns the keys that currently have live streams.
func (h *Hub) Keys() []Key {
	h.mu.Lock()
	defer h.mu.Unlock()
	keys := make([]Key, 0, len(h.streams))
	for key := range h.streams {
		keys = append(keys, key)
	}
	return keys
}

// Close ends every stream of key and returns how many it closed.
func (h *Hub) Close(key Key, reason string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	streams := h.streams[key]
	count := len(streams)
	for _, c := range streams {
		h.removeLocked(c)
		c.close(reason)
	}
	return count
}

// CloseAll ends every stream.
func (h *Hub) CloseAll(reason string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for _, streams := range h.streams {
		for _, c := range streams {
			h.removeLocked(c)
			c.close(reason)
			count++
		}
	}
	return count
}

// Count returns the number of live streams.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.total
}
