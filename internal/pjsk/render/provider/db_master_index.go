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

// get returns the index, loading it on first use or after a reset. Once the
// TTL passes, the previous index keeps being served while one shared flight
// reloads it in the background, so no request waits for a routine refresh.
// A reset (masterdata changed) drops the index and makes readers wait.
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
		fresh := dbBulkIndexFresh(index.loaded, index.loadedAt)
		index.mu.RUnlock()
		if fresh {
			return value, nil
		}
		caller := new(dbBulkIndexFlightToken)
		result := index.startLoad(generation, caller, operation, load)
		if loaded {
			commandtrace.RecordOperation(ctx, operation+"_stale", 0)
			return value, nil
		}
		if err := waitDBBulkIndexFlight(ctx, result, caller, operation+"_wait", operation+"_shared"); err != nil {
			var zero T
			return zero, err
		}
		// A reset may have discarded this flight. Read the current generation
		// rather than returning data that predates the masterdata update.
	}
}

// startLoad joins or starts the shared load for generation. The flight runs
// detached from the caller, so a stale reader may return without waiting.
func (index *dbMasterIndex[T]) startLoad(generation uint64, caller *dbBulkIndexFlightToken, operation string, load func(context.Context) (T, error)) <-chan singleflight.Result {
	return index.loads.DoChan(strconv.FormatUint(generation, 10), func() (any, error) {
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
}

func (index *dbMasterIndex[T]) reset() {
	index.mu.Lock()
	defer index.mu.Unlock()
	var zero T
	index.value, index.loaded, index.loadedAt = zero, false, time.Time{}
	index.generation++
}
