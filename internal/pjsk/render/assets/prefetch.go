package assets

import (
	"sync"

	"haruki-cloud/internal/observability/commandtrace"
)

const assetPrefetchWorkers = 8

// Prefetch resolves independent assets concurrently, with a fixed request-local
// worker budget. Tasks must only access immutable metadata and asset helpers;
// database adapters and request builders are intentionally kept sequential.
// The helper retains each asset's ordered candidate search and shared caches.
func (h *AssetHelper) Prefetch(tasks []func(*AssetHelper)) error {
	if h == nil || len(tasks) == 0 {
		return nil
	}
	ctx := readerContext(h.ctx)
	if !h.ProbesStore() || ctx.Err() != nil {
		return ctx.Err()
	}
	finish := commandtrace.MeasureOperation(ctx, "asset.prefetch")
	defer finish()
	jobs := make(chan func(*AssetHelper))
	var workers sync.WaitGroup
	for range min(assetPrefetchWorkers, len(tasks)) {
		workers.Go(func() {
			for task := range jobs {
				if ctx.Err() != nil {
					continue
				}
				task(h)
			}
		})
	}
	for _, task := range tasks {
		select {
		case jobs <- task:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	workers.Wait()
	return ctx.Err()
}

// Close cancels shared probes and speculative warm-up work. It is safe to call
// repeatedly and should run when the application using the helper shuts down.
func (h *AssetHelper) Close() {
	if h != nil && h.store != nil {
		h.store.cancel()
	}
}
