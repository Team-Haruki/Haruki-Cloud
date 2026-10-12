package app

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/logger"
)

const (
	// masterdataWarmConcurrency bounds how many regions load their bulk
	// indexes at once, across the startup warm-up and the reloads that
	// follow a masterdata reset, so a warm-up cannot saturate the database
	// host's CPU or connections.
	masterdataWarmConcurrency = 2
	masterdataWarmTimeout     = 2 * time.Minute
)

// masterdataWarmer is implemented by providers that can preload their bulk
// indexes (today the database provider).
type masterdataWarmer interface {
	WarmMasterdata(ctx context.Context) error
}

// masterdataWarmup serialises warm-ups through a bounded set of slots. App
// holds one by pointer (the App value is copied per request), created in
// New; App literals without one get a limiter per warm-up call.
type masterdataWarmup struct {
	slots chan struct{}
}

func newMasterdataWarmup() *masterdataWarmup {
	return &masterdataWarmup{slots: make(chan struct{}, masterdataWarmConcurrency)}
}

func (a *App) warmLimiter() *masterdataWarmup {
	if a != nil && a.masterdataWarm != nil {
		return a.masterdataWarm
	}
	return newMasterdataWarmup()
}

// run loads the region's indexes once a slot is free. It returns false when
// ctx ended before a slot was acquired or the warm-up failed.
func (w *masterdataWarmup) run(ctx context.Context, region renderregion.Value, warmer masterdataWarmer) bool {
	if w == nil || w.slots == nil || warmer == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case w.slots <- struct{}{}:
	}
	defer func() { <-w.slots }()
	warmCtx, cancel := context.WithTimeout(ctx, masterdataWarmTimeout)
	defer cancel()
	startedAt := time.Now()
	if err := warmer.WarmMasterdata(warmCtx); err != nil {
		logger.Warn("masterdata warm-up failed",
			"region", region,
			"elapsed_ms", time.Since(startedAt).Milliseconds(),
			"error_type", fmt.Sprintf("%T", err),
			"error", err,
		)
		return false
	}
	logger.Info("masterdata warm-up finished",
		"region", region,
		"elapsed_ms", time.Since(startedAt).Milliseconds(),
	)
	return true
}

// WarmMasterdataIndexes preloads every region's hot bulk indexes in the
// background and returns at once. It is meant to run once the HTTP server
// is listening: readiness never waits for it, and at most
// masterdataWarmConcurrency regions load at a time. The returned channel is
// closed when every region has been attempted.
func (a *App) WarmMasterdataIndexes(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if a == nil {
		close(done)
		return done
	}
	if ctx == nil {
		ctx = context.Background()
	}
	regions := make([]renderregion.Value, 0, len(a.Providers))
	for region, src := range a.Providers {
		if _, ok := src.(masterdataWarmer); ok {
			regions = append(regions, region)
		}
	}
	if len(regions) == 0 {
		close(done)
		return done
	}
	slices.Sort(regions)
	limiter := a.warmLimiter()
	go func() {
		defer close(done)
		startedAt := time.Now()
		var wg sync.WaitGroup
		for _, region := range regions {
			warmer, _ := a.Providers[region].(masterdataWarmer)
			wg.Add(1)
			go func() {
				defer wg.Done()
				limiter.run(ctx, region, warmer)
			}()
		}
		wg.Wait()
		logger.Info("masterdata warm-up completed",
			"regions", len(regions),
			"elapsed_ms", time.Since(startedAt).Milliseconds(),
		)
	}()
	return done
}

// warmRegionAsync reloads one region's indexes after a masterdata reset so
// the next command does not pay for the cold load. It shares the warm-up
// slots with the startup warm-up.
func (a *App) warmRegionAsync(ctx context.Context, region renderregion.Value) {
	if a == nil {
		return
	}
	warmer, ok := a.Providers[region].(masterdataWarmer)
	if !ok {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go a.warmLimiter().run(ctx, region, warmer)
}
