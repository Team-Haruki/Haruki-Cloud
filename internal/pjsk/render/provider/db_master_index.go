package provider

import (
	"context"
	"strconv"
	"sync"

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
	generation uint64
}

// get returns the index, loading it on first use or after a reset. A loaded
// index is served until the next reset (masterdata changed), which drops it
// and makes readers wait for the reload.
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
		value, loaded, generation := index.value, index.loaded, index.generation
		index.mu.RUnlock()
		if loaded {
			return value, nil
		}
		caller := new(dbBulkIndexFlightToken)
		result := index.startLoad(generation, caller, operation, load)
		if err := waitDBBulkIndexFlight(ctx, result, caller, operation+"_wait", operation+"_shared"); err != nil {
			var zero T
			return zero, err
		}
		// A reset may have discarded this flight. Read the current generation
		// rather than returning data that predates the masterdata update.
	}
}

// startLoad joins or starts the shared load for generation. The flight runs
// detached from the caller so a cancelled leader does not fail the others.
func (index *dbMasterIndex[T]) startLoad(generation uint64, caller *dbBulkIndexFlightToken, operation string, load func(context.Context) (T, error)) <-chan singleflight.Result {
	return index.loads.DoChan(strconv.FormatUint(generation, 10), func() (any, error) {
		completed := runDBBulkIndexFlight(caller, func(loadCtx context.Context) error {
			index.mu.RLock()
			loaded := index.loaded
			current := index.generation
			index.mu.RUnlock()
			if loaded || current != generation {
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
				index.value, index.loaded = value, true
			}
			index.mu.Unlock()
			return nil
		})
		return completed, nil
	})
}

// isLoaded reports whether the current generation holds a value.
func (index *dbMasterIndex[T]) isLoaded() bool {
	index.mu.RLock()
	defer index.mu.RUnlock()
	return index.loaded
}

func (index *dbMasterIndex[T]) reset() {
	index.mu.Lock()
	defer index.mu.Unlock()
	var zero T
	index.value, index.loaded = zero, false
	index.generation++
}
