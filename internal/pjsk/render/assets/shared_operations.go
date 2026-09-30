package assets

import (
	"context"
	"sync"

	"haruki-cloud/internal/observability/commandtrace"
)

// A shared directory flight can be consumed by several key flights in the
// same request. Keep its operation identity until the request receives it,
// otherwise merging intermediate aggregates would count one network call for
// every waiter. Batches live only as long as their flights, never in caches.
type sharedAssetOperations struct {
	stats    []commandtrace.Stats
	children []*sharedAssetOperations
	mu       sync.Mutex
	merged   map[*commandtrace.Trace]struct{}
}

type assetOperationCollector struct{ children []*sharedAssetOperations }
type assetOperationCollectorKey struct{}

func collectAssetOperations(ctx context.Context) (context.Context, *assetOperationCollector) {
	collector := &assetOperationCollector{}
	return context.WithValue(ctx, assetOperationCollectorKey{}, collector), collector
}

func (c *assetOperationCollector) finish(trace *commandtrace.Trace) *sharedAssetOperations {
	return &sharedAssetOperations{stats: trace.Snapshot().Operations, children: c.children}
}

func (batch *sharedAssetOperations) merge(ctx context.Context) {
	if batch == nil {
		return
	}
	if collector, ok := ctx.Value(assetOperationCollectorKey{}).(*assetOperationCollector); ok {
		collector.children = append(collector.children, batch)
		return
	}
	trace := commandtrace.FromContext(ctx)
	if trace == nil {
		return
	}
	batch.mergeTrace(ctx, trace)
}

func (batch *sharedAssetOperations) mergeTrace(ctx context.Context, trace *commandtrace.Trace) {
	batch.mu.Lock()
	if _, exists := batch.merged[trace]; exists {
		batch.mu.Unlock()
		return
	}
	if batch.merged == nil {
		batch.merged = make(map[*commandtrace.Trace]struct{})
	}
	batch.merged[trace] = struct{}{}
	batch.mu.Unlock()
	commandtrace.MergeOperations(ctx, batch.stats)
	for _, child := range batch.children {
		child.mergeTrace(ctx, trace)
	}
}
