package upstream

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	ErrNoTargetsConfigured = errors.New("upstream: no targets configured")
	ErrNoAvailableTargets  = errors.New("upstream: no available targets")
)

type TargetConfig struct {
	Name        string `yaml:"name"`
	BaseURL     string `yaml:"base_url"`
	Concurrency int    `yaml:"concurrency"`
}

type Lease struct {
	Target  TargetConfig
	release func()
}

func (l *Lease) Release() {
	if l == nil || l.release == nil {
		return
	}
	l.release()
	l.release = nil
}

type targetSlot struct {
	target      TargetConfig
	resource    *resourceSlot
	resourceKey string
}

type Pool struct {
	targets []*targetSlot
	next    atomic.Uint64
}

type TargetPredicate func(TargetConfig) bool

type resourceSlot struct {
	sem     chan struct{}
	pending atomic.Int64
}

type SharedResources struct {
	mu        sync.Mutex
	resources map[string]*resourceSlot
}

func ResolveTargets(legacyBaseURL string, targets []TargetConfig, defaultNamePrefix string) []TargetConfig {
	resolved := make([]TargetConfig, 0, len(targets)+1)
	for index, target := range targets {
		baseURL := normalizeBaseURL(target.BaseURL)
		if baseURL == "" {
			continue
		}
		target.BaseURL = baseURL
		target.Name = strings.TrimSpace(target.Name)
		if target.Name == "" {
			target.Name = defaultTargetName(defaultNamePrefix, index+1)
		}
		if target.Concurrency < 0 {
			target.Concurrency = 0
		}
		resolved = append(resolved, target)
	}
	if len(resolved) > 0 {
		return resolved
	}

	legacyBaseURL = normalizeBaseURL(legacyBaseURL)
	if legacyBaseURL == "" {
		return nil
	}
	return []TargetConfig{{
		Name:    defaultTargetName(defaultNamePrefix, 1),
		BaseURL: legacyBaseURL,
	}}
}

func NewPool(targets []TargetConfig) *Pool {
	return NewPoolWithResources(targets, nil)
}

func NewPoolWithResources(targets []TargetConfig, shared *SharedResources) *Pool {
	slots := make([]*targetSlot, 0, len(targets))
	for index, target := range targets {
		baseURL := normalizeBaseURL(target.BaseURL)
		if baseURL == "" {
			continue
		}
		concurrency := target.Concurrency
		if concurrency < 0 {
			concurrency = 0
		}
		name := strings.TrimSpace(target.Name)
		if name == "" {
			name = defaultTargetName("target", index+1)
		}
		resourceKey := defaultResourceKey(name, baseURL)
		slots = append(slots, &targetSlot{
			target: TargetConfig{
				Name:        name,
				BaseURL:     baseURL,
				Concurrency: concurrency,
			},
			resourceKey: resourceKey,
			resource:    shared.resolve(resourceKey, concurrency),
		})
	}
	if len(slots) == 0 {
		return nil
	}
	return &Pool{targets: slots}
}

func (p *Pool) Enabled() bool {
	return p != nil && len(p.targets) > 0
}

func (p *Pool) Acquire(ctx context.Context) (*Lease, error) {
	return p.AcquireFunc(ctx, nil)
}

func (p *Pool) AcquireFunc(ctx context.Context, accept TargetPredicate) (*Lease, error) {
	if p == nil || len(p.targets) == 0 {
		return nil, ErrNoTargetsConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}

	slot := p.pickSlotFunc(accept)
	if slot == nil {
		return nil, ErrNoAvailableTargets
	}
	slot.resource.pending.Add(1)
	if slot.resource.sem == nil {
		return &Lease{
			Target: slot.target,
			release: func() {
				slot.resource.pending.Add(-1)
			},
		}, nil
	}
	select {
	case slot.resource.sem <- struct{}{}:
		return &Lease{
			Target: slot.target,
			release: func() {
				<-slot.resource.sem
				slot.resource.pending.Add(-1)
			},
		}, nil
	case <-ctx.Done():
		slot.resource.pending.Add(-1)
		return nil, ctx.Err()
	}
}

func (p *Pool) pickSlotFunc(accept TargetPredicate) *targetSlot {
	if len(p.targets) == 0 {
		return nil
	}
	start := int(p.next.Add(1)-1) % len(p.targets)
	var best *targetSlot
	var bestPending int64
	for offset := 0; offset < len(p.targets); offset++ {
		slot := p.targets[(start+offset)%len(p.targets)]
		if accept != nil && !accept(slot.target) {
			continue
		}
		pending := slot.resource.pending.Load()
		if best == nil || pending < bestPending {
			best = slot
			bestPending = pending
		}
	}
	return best
}

func normalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func defaultTargetName(prefix string, index int) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "target"
	}
	return prefix + "-" + strconvItoa(index)
}

func defaultResourceKey(name string, baseURL string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	return baseURL
}

func (s *SharedResources) resolve(key string, concurrency int) *resourceSlot {
	if s == nil {
		return newResourceSlot(concurrency)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resources == nil {
		s.resources = make(map[string]*resourceSlot)
	}
	if slot, ok := s.resources[key]; ok {
		return slot
	}
	slot := newResourceSlot(concurrency)
	s.resources[key] = slot
	return slot
}

func newResourceSlot(concurrency int) *resourceSlot {
	slot := &resourceSlot{}
	if concurrency > 0 {
		slot.sem = make(chan struct{}, concurrency)
	}
	return slot
}

func strconvItoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	index := len(buf)
	for value > 0 {
		index--
		buf[index] = byte('0' + (value % 10))
		value /= 10
	}
	return string(buf[index:])
}
