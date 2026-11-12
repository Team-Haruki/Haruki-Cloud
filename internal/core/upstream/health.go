package upstream

import (
	"context"
	"slices"
	"sync"
	"time"
)

// HealthChange is called once when a target goes down (down=true) and once
// when a success brings a down target back (down=false). It runs outside
// the Health lock.
type HealthChange func(name string, down bool)

// Health is passive per-target health keyed by target name: a target is
// down for window after MarkDown and eligible again once the window has
// passed. MarkUp (a success) clears the down state at once. A nil *Health
// reports every target healthy.
type Health struct {
	mu       sync.Mutex
	window   time.Duration
	now      func() time.Time
	onChange HealthChange
	downTill map[string]time.Time
}

// NewHealth returns a Health with the given down window. now defaults to
// time.Now; onChange may be nil.
func NewHealth(window time.Duration, now func() time.Time, onChange HealthChange) *Health {
	if now == nil {
		now = time.Now
	}
	return &Health{window: window, now: now, onChange: onChange, downTill: make(map[string]time.Time)}
}

// Healthy reports that name is not inside a down window.
func (h *Health) Healthy(name string) bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	until, ok := h.downTill[name]
	return !ok || !h.now().Before(until)
}

// MarkDown starts (or extends) name's down window. onChange fires only when
// the target was not already inside a window.
func (h *Health) MarkDown(name string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	now := h.now()
	until, ok := h.downTill[name]
	wasUp := !ok || !now.Before(until)
	h.downTill[name] = now.Add(h.window)
	h.mu.Unlock()
	if wasUp && h.onChange != nil {
		h.onChange(name, true)
	}
}

// MarkUp clears name's down state. onChange fires only when the target had
// been marked down (its window may already have passed).
func (h *Health) MarkUp(name string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	_, wasDown := h.downTill[name]
	delete(h.downTill, name)
	h.mu.Unlock()
	if wasDown && h.onChange != nil {
		h.onChange(name, false)
	}
}

// AcquireHealthy leases a target whose name is not in exclude, in tiers:
// healthy non-backup targets, then healthy backups, then down non-backup
// targets, then down backups. Within a tier the pick is the least-pending
// target, ties broken by a rotation over that tier, so idle traffic
// round-robins among the healthy non-backup targets only. A down target is
// still used when nothing better is left, so down state alone never fails a
// call. It returns ErrNoAvailableTargets once every target is excluded.
//
// A caller may wait for a slot of a concurrency-limited target. If that
// target went down meanwhile and a healthy target that is not excluded
// exists, the lease is released and that target is leased instead. This
// reselection happens at most once per call, before anything is sent.
func (p *Pool) AcquireHealthy(ctx context.Context, health *Health, exclude []string) (*Lease, error) {
	if p == nil || len(p.targets) == 0 {
		return nil, ErrNoTargetsConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	slot := p.pickHealthySlot(health, exclude)
	if slot == nil {
		return nil, ErrNoAvailableTargets
	}
	lease, err := slot.acquire(ctx, false)
	if err != nil || health.Healthy(slot.target.Name) {
		return lease, err
	}
	alternative := p.pickHealthySlot(health, append(slices.Clip(exclude), slot.target.Name))
	if alternative == nil || !health.Healthy(alternative.target.Name) {
		return lease, nil
	}
	lease.Release()
	return alternative.acquire(ctx, false)
}

func (p *Pool) pickHealthySlot(health *Health, exclude []string) *targetSlot {
	healthy := make([]bool, len(p.targets))
	for i, slot := range p.targets {
		healthy[i] = health.Healthy(slot.target.Name)
	}
	candidates := make([]*targetSlot, 0, len(p.targets))
	for _, tier := range [...]struct{ healthy, backup bool }{{true, false}, {true, true}, {false, false}, {false, true}} {
		candidates = candidates[:0]
		for i, slot := range p.targets {
			if healthy[i] == tier.healthy && slot.target.Backup == tier.backup && !slices.Contains(exclude, slot.target.Name) {
				candidates = append(candidates, slot)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		start := int((p.next.Add(1) - 1) % uint64(len(candidates)))
		var best *targetSlot
		var bestPending int64
		for offset := range candidates {
			slot := candidates[(start+offset)%len(candidates)]
			if pending := slot.resource.pending.Load(); best == nil || pending < bestPending {
				best, bestPending = slot, pending
			}
		}
		return best
	}
	return nil
}
