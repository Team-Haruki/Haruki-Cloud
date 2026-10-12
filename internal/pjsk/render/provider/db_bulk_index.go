package provider

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

// Bulk indexes hold one region's master table in memory. An index is loaded
// once per generation and served until the master data changes: a reset
// (ResetMasterdataCache, driven by the registry pointer and the ingest
// version) bumps the generation, drops the index and discards any load that
// started before it. There is no time-based expiry; the ingest version check
// in the registry poll loop is what tells a running process that the
// database was restored in place.
const dbBulkIndexLoadTimeout = 30 * time.Second

type dbBulkIndexFlightToken byte

type dbBulkIndexFlightResult struct {
	err        error
	operations []commandtrace.Stats
	leader     *dbBulkIndexFlightToken
}

func runDBBulkIndexFlight(leader *dbBulkIndexFlightToken, work func(context.Context) error) dbBulkIndexFlightResult {
	detached := context.Background()
	detached = logger.WithContextAttrs(detached, slog.Bool("shared_work", true))
	loadBase, cancel := context.WithTimeout(detached, dbBulkIndexLoadTimeout)
	defer cancel()
	loadCtx, trace := commandtrace.WithNewTrace(loadBase)
	err := work(loadCtx)
	return dbBulkIndexFlightResult{
		err:        err,
		operations: trace.Snapshot().Operations,
		leader:     leader,
	}
}

func waitDBBulkIndexFlight(
	ctx context.Context,
	result <-chan singleflight.Result,
	caller *dbBulkIndexFlightToken,
	waitOperation string,
	sharedOperation string,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	finishWait := commandtrace.MeasureOperation(ctx, waitOperation)
	select {
	case <-ctx.Done():
		finishWait()
		return ctx.Err()
	case outcome := <-result:
		finishWait()
		if outcome.Err != nil {
			return outcome.Err
		}
		completed, ok := outcome.Val.(dbBulkIndexFlightResult)
		if !ok {
			return fmt.Errorf("database bulk index returned unexpected shared result %T", outcome.Val)
		}
		commandtrace.MergeOperations(ctx, completed.operations)
		if completed.leader != caller {
			commandtrace.RecordOperation(ctx, sharedOperation, 0)
		}
		return completed.err
	}
}
