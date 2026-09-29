package provider

import (
	"context"
	"strconv"
	"sync"
	"time"

	"haruki-cloud/internal/observability/commandtrace"

	"golang.org/x/sync/singleflight"
)

// dbMasterIndex holds a small masterdata lookup table. Returned values remain
// immutable; callers clone the rows they expose outside the provider.
type dbMasterIndex[T any] struct {
	mu         sync.RWMutex
	loads      singleflight.Group
	value      T
	loaded     bool
	loadedAt   time.Time
	generation uint64
}

func (index *dbMasterIndex[T]) get(ctx context.Context, operation string, load func(context.Context) (T, error)) (T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			var zero T
			return zero, err
		}
		index.mu.RLock()
		value, fresh, generation := index.value, dbBulkIndexFresh(index.loaded, index.loadedAt), index.generation
		index.mu.RUnlock()
		if fresh {
			return value, nil
		}
		caller := new(dbBulkIndexFlightToken)
		result := index.loads.DoChan(strconv.FormatUint(generation, 10), func() (any, error) {
			completed := runDBBulkIndexFlight(caller, func(loadCtx context.Context) error {
				index.mu.RLock()
				fresh := dbBulkIndexFresh(index.loaded, index.loadedAt)
				current := index.generation
				index.mu.RUnlock()
				if fresh || current != generation {
					return nil
				}
				finish := commandtrace.MeasureOperation(loadCtx, operation)
				defer finish()
				value, err := load(loadCtx)
				if err != nil {
					return err
				}
				index.mu.Lock()
				if index.generation == generation {
					index.value, index.loaded, index.loadedAt = value, true, time.Now()
				}
				index.mu.Unlock()
				return nil
			})
			return completed, nil
		})
		if err := waitDBBulkIndexFlight(ctx, result, caller, operation+"_wait", operation+"_shared"); err != nil {
			var zero T
			return zero, err
		}
		// A reset may have discarded this flight. Read the current generation
		// rather than returning data that predates the masterdata update.
	}
}

func (index *dbMasterIndex[T]) reset() {
	index.mu.Lock()
	defer index.mu.Unlock()
	var zero T
	index.value, index.loaded, index.loadedAt = zero, false, time.Time{}
	index.generation++
}
