// Package cachefill coordinates the fills of in-memory master data caches
// that read from the database with an optional local fallback.
//
// A Group gives each cache key one fill at a time: concurrent callers share
// the in-flight query instead of each starting their own, the query runs
// detached from the request context (a client that disconnects mid-fill
// cannot abort it for the others, and it is bounded by Timeout), and a
// failed fill is not retried until Backoff has elapsed so a database outage
// does not pile up queries. The Group tracks failures only; whether a
// result is cached stays with the caller, so an error is never recorded as
// a loaded cache.
package cachefill

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// DefaultTimeout bounds one detached fill.
	DefaultTimeout = 30 * time.Second
	// DefaultBackoff is how long a failed fill blocks retries of its key.
	DefaultBackoff = 5 * time.Second
)

// ErrBackoff reports a fill that was skipped because the previous fill of
// the same key failed less than Backoff ago. It wraps that failure.
var ErrBackoff = errors.New("cache fill skipped: previous fill failed recently")

// Group coordinates fills per key. The zero value is ready to use.
//
// Reset starts a new generation: a fill that began before it records
// neither its failure nor clears one, so a stale outcome cannot block or
// unblock the key. Callers that cache results keep their own generation the
// same way and discard a result whose fill straddled a reset.
type Group struct {
	// Timeout bounds one fill; zero means DefaultTimeout.
	Timeout time.Duration
	// Backoff is the retry hold-off after a failed fill; zero means
	// DefaultBackoff.
	Backoff time.Duration
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	flights    singleflight.Group
	mu         sync.Mutex
	failed     map[string]failure
	generation uint64
}

type failure struct {
	at  time.Time
	err error
}

// Do runs fill for key unless a fill is already in flight, in which case
// the caller waits for that one and shares its outcome. fill receives a
// context detached from ctx's cancellation (its values are kept) with the
// group's Timeout. While key is backing off, Do returns ErrBackoff wrapping
// the previous error without calling fill.
func (g *Group) Do(ctx context.Context, key string, fill func(ctx context.Context) error) error {
	if g == nil {
		return errors.New("cachefill: nil group")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := g.backoffError(key); err != nil {
		return err
	}
	_, err, _ := g.flights.Do(key, func() (any, error) {
		g.mu.Lock()
		generation := g.generation
		g.mu.Unlock()
		fillCtx, cancel := g.Context(ctx)
		defer cancel()
		err := fill(fillCtx)
		g.record(key, generation, err)
		return nil, err
	})
	return err
}

// Context derives the detached, bounded context a fill runs on.
func (g *Group) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := DefaultTimeout
	if g != nil && g.Timeout > 0 {
		timeout = g.Timeout
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

// Failing reports whether key is currently backing off.
func (g *Group) Failing(key string) bool {
	return g != nil && g.backoffError(key) != nil
}

// Reset forgets every failure so the next call of each key fills again and
// starts a new generation, so fills already in flight record nothing when
// they finish; callers use it when the underlying cache is reset.
func (g *Group) Reset() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.failed = nil
	g.generation++
	g.mu.Unlock()
}

func (g *Group) backoffError(key string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	last, ok := g.failed[key]
	if !ok {
		return nil
	}
	if g.now().Sub(last.at) >= g.backoff() {
		delete(g.failed, key)
		return nil
	}
	return fmt.Errorf("%w: %w", ErrBackoff, last.err)
}

func (g *Group) record(key string, generation uint64, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if generation != g.generation {
		return
	}
	if err == nil {
		delete(g.failed, key)
		return
	}
	if g.failed == nil {
		g.failed = make(map[string]failure)
	}
	g.failed[key] = failure{at: g.now(), err: err}
}

func (g *Group) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *Group) backoff() time.Duration {
	if g.Backoff > 0 {
		return g.Backoff
	}
	return DefaultBackoff
}
