package snapshot

import (
	"context"
	"fmt"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

// Shared builds may outlive an individual waiter, but DB work must remain bounded.
const sharedSnapshotBuildTimeout = 30 * time.Second

type builtSnapshotFlightResult struct {
	snapshot   Snapshot
	cacheHit   bool
	operations []commandtrace.Stats
}

// getOrBuild is reached only after this caller's authorized payload reads.
// Authorization and private-data fetching must never be included in the flight.
func (c *BuiltSnapshotCache) getOrBuild(ctx context.Context, key builtSnapshotKey, payloadBytes int64, build func(context.Context) (Snapshot, error)) (Snapshot, bool, error) {
	if ctx == nil {
		ctx = context.TODO()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if c == nil {
		snapshot, err := build(ctx)
		return snapshot, false, err
	}
	if snapshot := c.Get(key); snapshot != nil {
		return snapshot, true, nil
	}
	flightKey := fmt.Sprintf("%s:%d:%d:%t:%d", key.Region, key.UID, key.SuiteUploadTime, key.NeedMySekai, key.MySekaiUploadTime)
	// Keep only logging attributes, not request caches or the initiating request's
	// cancellation. Every waiter still selects on its own context below.
	base := logger.DetachedContext(ctx)
	result := c.builds.DoChan(flightKey, func() (any, error) {
		if snapshot := c.Get(key); snapshot != nil {
			return builtSnapshotFlightResult{snapshot: snapshot, cacheHit: true}, nil
		}
		shared, cancel := context.WithTimeout(base, sharedSnapshotBuildTimeout)
		defer cancel()
		shared, trace := commandtrace.WithNewTrace(shared)
		snapshot, err := build(shared)
		if err == nil {
			err = shared.Err()
		}
		if err == nil {
			c.Put(key, snapshot, payloadBytes)
		}
		return builtSnapshotFlightResult{snapshot: snapshot, operations: trace.Snapshot().Operations}, err
	})
	finishWait := commandtrace.MeasureOperation(ctx, "snapshot.build_wait")
	defer finishWait()
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case result := <-result:
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		built := result.Val.(builtSnapshotFlightResult)
		commandtrace.MergeOperations(ctx, built.operations)
		if result.Err != nil {
			return nil, false, result.Err
		}
		return built.snapshot, built.cacheHit, nil
	}
}
