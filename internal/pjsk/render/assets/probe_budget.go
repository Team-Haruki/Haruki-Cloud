package assets

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Per-request probe budget defaults (StoreProbeConfig.Request*). A cold
// assets store once cost one event/list request 1,305 HEADs and 28 s; the
// budget lets such a request fall back to first candidates instead, while
// the flights it already started finish in the background and warm the
// caches for the next request.
const (
	DefaultStoreProbeRequestBudget        = 5 * time.Second
	DefaultStoreProbeRequestMaxStoreCalls = 128
	DefaultStoreProbeRequestConcurrency   = 8
)

var errStoreProbeBudget = errors.New("assets: request probe budget exhausted")

type probeBudgetKey struct{}

// probeBudget is one request's allowance for uncached store probes: at most
// concurrency waits at a time, store calls up to maxCalls, and no waiting
// after budget has elapsed since the request's first uncached probe. Limits
// come from the probe's config on first use, so callers only need to attach
// an empty budget to the request context.
type probeBudget struct {
	once     sync.Once
	sem      chan struct{}
	budget   time.Duration
	maxCalls int64
	deadline atomic.Int64
	calls    atomic.Int64
}

// WithProbeBudget attaches a fresh store-probe budget to ctx unless it
// already carries one. Every asset helper copy bound to the returned context
// (AssetHelper.WithContext) then shares one budget.
func WithProbeBudget(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(probeBudgetKey{}).(*probeBudget); ok {
		return ctx
	}
	return context.WithValue(ctx, probeBudgetKey{}, &probeBudget{})
}

// budgetFor returns ctx's budget initialised from cfg, or nil when ctx has
// none or every limit is disabled.
func (p *storeProbe) budgetFor(ctx context.Context) *probeBudget {
	budget, ok := ctx.Value(probeBudgetKey{}).(*probeBudget)
	if !ok || budget == nil {
		return nil
	}
	budget.once.Do(func() {
		budget.budget = p.cfg.RequestBudget
		budget.maxCalls = int64(p.cfg.RequestMaxStoreCalls)
		if p.cfg.RequestConcurrency > 0 {
			budget.sem = make(chan struct{}, p.cfg.RequestConcurrency)
		}
	})
	if budget.budget <= 0 && budget.maxCalls <= 0 && budget.sem == nil {
		return nil
	}
	return budget
}

// remaining is the wait time left; the clock starts at the first call.
func (b *probeBudget) remaining(now time.Time) (time.Duration, bool) {
	if b.budget <= 0 {
		return 0, false
	}
	deadline := b.deadline.Load()
	if deadline == 0 {
		b.deadline.CompareAndSwap(0, now.Add(b.budget).UnixNano())
		deadline = b.deadline.Load()
	}
	return time.Unix(0, deadline).Sub(now), true
}

// acquire admits one uncached probe, waiting for a concurrency slot no
// longer than the remaining budget. The returned release must be called.
func (b *probeBudget) acquire(ctx context.Context, now time.Time) (func(), error) {
	if b.maxCalls > 0 && b.calls.Load() >= b.maxCalls {
		return nil, errStoreProbeBudget
	}
	left, timed := b.remaining(now)
	if timed && left <= 0 {
		return nil, errStoreProbeBudget
	}
	if b.sem == nil {
		return func() {}, nil
	}
	release := func() { <-b.sem }
	select {
	case b.sem <- struct{}{}:
		return release, nil
	default:
	}
	var expired <-chan time.Time
	if timed {
		timer := time.NewTimer(left)
		defer timer.Stop()
		expired = timer.C
	}
	select {
	case b.sem <- struct{}{}:
		return release, nil
	case <-expired:
		return nil, errStoreProbeBudget
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// expiry returns a channel that fires when the wait budget runs out (nil when
// unlimited) and its stop function.
func (b *probeBudget) expiry(now time.Time) (<-chan time.Time, func() bool) {
	left, timed := b.remaining(now)
	if !timed {
		return nil, func() bool { return false }
	}
	timer := time.NewTimer(max(left, 0))
	return timer.C, timer.Stop
}

func (b *probeBudget) charge(calls int64) {
	if calls > 0 {
		b.calls.Add(calls)
	}
}

// storeCallCounter counts the store round trips one key flight made,
// including the directory listings it led; the waiting requests charge them
// to their budgets.
type storeCallCounter struct{ n atomic.Int64 }

type storeCallCounterKey struct{}

func withStoreCallCounter(ctx context.Context) (context.Context, *storeCallCounter) {
	counter := &storeCallCounter{}
	return context.WithValue(ctx, storeCallCounterKey{}, counter), counter
}

func storeCallCounterFrom(ctx context.Context) *storeCallCounter {
	counter, _ := ctx.Value(storeCallCounterKey{}).(*storeCallCounter)
	return counter
}

// inheritStoreCallCounter carries from's counter (if any) onto ctx, for
// flights that run on a detached context.
func inheritStoreCallCounter(ctx, from context.Context) context.Context {
	if counter := storeCallCounterFrom(from); counter != nil {
		return context.WithValue(ctx, storeCallCounterKey{}, counter)
	}
	return ctx
}

// warmPrefixesNone in warm_prefixes disables warm-up.
const warmPrefixesNone = "none"

// DefaultWarmPrefixes are the directories whose cold listing set the route
// tails: card thumbnails (card lists, event lists) and the event banner
// sources event/list resolves for every event, for each region.
func DefaultWarmPrefixes() []string {
	regions := []string{"jp", "en", "tw", "kr", "cn"}
	dirs := []string{"startapp/thumbnail/chara", "startapp/home/banner", "ondemand/event"}
	prefixes := make([]string, 0, len(regions)*len(dirs))
	for _, region := range regions {
		for _, dir := range dirs {
			prefixes = append(prefixes, region+"-assets/"+dir)
		}
	}
	return prefixes
}

// ResolveWarmPrefixes applies the warm_prefixes defaults: an empty list
// selects DefaultWarmPrefixes and ["none"] disables warm-up.
func ResolveWarmPrefixes(configured []string) []string {
	if len(configured) == 0 {
		return DefaultWarmPrefixes()
	}
	if len(configured) == 1 && configured[0] == warmPrefixesNone {
		return nil
	}
	return configured
}
