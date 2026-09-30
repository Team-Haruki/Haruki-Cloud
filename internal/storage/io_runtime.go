package storage

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

// IOConfig bounds network attempts across all slots belonging to a Set.
// Background attempts count towards every limit, while their separate cap
// leaves capacity for commands even during a full asset scan.
type IOConfig struct {
	MaxConcurrent int `yaml:"max_concurrent"`
	MaxPerOrigin  int `yaml:"max_per_origin"`
	MaxBackground int `yaml:"max_background"`
}

func (c IOConfig) normalized() IOConfig {
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 16
	}
	if c.MaxPerOrigin <= 0 {
		c.MaxPerOrigin = 8
	}
	if c.MaxBackground <= 0 {
		c.MaxBackground = 2
	}
	c.MaxPerOrigin = min(c.MaxPerOrigin, c.MaxConcurrent)
	c.MaxBackground = min(c.MaxBackground, max(1, c.MaxConcurrent-1))
	return c
}

type backgroundIOKey struct{}

// WithBackgroundIO gives prewarming and maintenance the shared background
// budget. It does not detach cancellation or alter the context deadline.
func WithBackgroundIO(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundIOKey{}, true)
}

// IsBackgroundIO reports the I/O class when shared work needs to inherit it.
func IsBackgroundIO(ctx context.Context) bool {
	background, _ := ctx.Value(backgroundIOKey{}).(bool)
	return background
}

// IOMetric contains inclusive timings and counters. Concurrent durations may
// overlap; Value is bytes for bytes_* metrics, and zero for timing metrics.
// Names contain only fixed slot, operation and configured endpoint ordinals.
type IOMetric struct {
	Name     string        `json:"name"`
	Count    int64         `json:"count"`
	Duration time.Duration `json:"duration_ns"`
	Max      time.Duration `json:"max_ns"`
	Value    int64         `json:"value"`
}

type ioMetricKey struct {
	slot       Slot
	background bool
	endpoint   int
	op, event  string
}

// IORuntime is owned by a storage.Set, never a package singleton. The admission
// mutex reserves all required capacities at once so waiting on an origin does
// not consume a global permit and block unrelated origins.
type IORuntime struct {
	transportsMu sync.Mutex
	transports   map[string]*http.Transport
	cfg          IOConfig
	mu           sync.Mutex
	changed      chan struct{}
	active       int
	background   int
	origins      map[string]int
	metricsMu    sync.Mutex
	metrics      map[ioMetricKey]IOMetric
}

func NewIORuntime(cfg IOConfig) *IORuntime {
	return &IORuntime{cfg: cfg.normalized(), changed: make(chan struct{}), origins: make(map[string]int), metrics: make(map[ioMetricKey]IOMetric)}
}

// Acquire reserves one request, including response-body consumption. The
// origin must be a configured endpoint origin, not an object key or URL path.
func (r *IORuntime) Acquire(ctx context.Context, origin string) (func(), error) {
	if r == nil {
		return func() {}, ctx.Err()
	}
	background := IsBackgroundIO(ctx)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r.mu.Lock()
		if r.active < r.cfg.MaxConcurrent && r.origins[origin] < r.cfg.MaxPerOrigin && (!background || r.background < r.cfg.MaxBackground) {
			r.active++
			r.origins[origin]++
			if background {
				r.background++
			}
			r.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					r.mu.Lock()
					r.active--
					r.origins[origin]--
					if r.origins[origin] == 0 {
						delete(r.origins, origin)
					}
					if background {
						r.background--
					}
					close(r.changed)
					r.changed = make(chan struct{})
					r.mu.Unlock()
				})
			}, nil
		}
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// Record observes one fixed metric. It is also collected without a request
// trace, allowing maintenance and prewarm traffic to be inspected in Stats.
// Callers supply only fixed implementation metric names, never keys or URLs.
func (r *IORuntime) Record(ctx context.Context, slot Slot, endpoint int, op, event string, elapsed time.Duration, value int64) {
	if r == nil {
		return
	}
	switch slot {
	case SlotAssets, SlotCache, SlotImageCache, SlotStatic, SlotUserUpload:
	default:
		slot = "standalone"
	}
	key := ioMetricKey{slot: slot, background: IsBackgroundIO(ctx), endpoint: endpoint, op: op, event: event}
	if elapsed < 0 {
		elapsed = 0
	}
	r.metricsMu.Lock()
	metric := r.metrics[key]
	if metric.Name == "" {
		workload := "foreground"
		if key.background {
			workload = "background"
		}
		metric.Name = fmt.Sprintf("storage.%s.%s.%s.endpoint_%d.%s", slot, workload, op, endpoint, event)
	}
	metric.Count++
	metric.Duration += elapsed
	metric.Max = max(metric.Max, elapsed)
	metric.Value += value
	r.metrics[key] = metric
	r.metricsMu.Unlock()
	if commandtrace.FromContext(ctx) == nil {
		return
	}
	commandtrace.RecordOperation(ctx, "storage."+op+"."+event, elapsed)
	if value > 0 {
		// Count is a byte counter for these explicitly named trace entries.
		commandtrace.MergeOperations(ctx, []commandtrace.Stats{{Name: "storage." + op + "." + event + ".total", Count: int(value)}})
	}
}

// Stats returns cumulative metrics without credentials, bucket names, object
// paths or endpoint URLs. It is safe to expose from authenticated diagnostics.
func (r *IORuntime) Stats() []IOMetric {
	if r == nil {
		return nil
	}
	r.metricsMu.Lock()
	result := make([]IOMetric, 0, len(r.metrics))
	for _, metric := range r.metrics {
		result = append(result, metric)
	}
	r.metricsMu.Unlock()
	slices.SortFunc(result, func(a, b IOMetric) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return result
}

// SharedTransport reuses connections between slots with identical network
// options. key identifies those options, never credentials or object paths.
func (r *IORuntime) SharedTransport(key string, build func() *http.Transport) *http.Transport {
	if r == nil {
		return build()
	}
	r.transportsMu.Lock()
	defer r.transportsMu.Unlock()
	if r.transports == nil {
		r.transports = make(map[string]*http.Transport)
	}
	if transport := r.transports[key]; transport != nil {
		return transport
	}
	transport := build()
	r.transports[key] = transport
	return transport
}
