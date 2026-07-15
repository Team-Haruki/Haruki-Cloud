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

const dbBulkIndexLoadTimeout = 30 * time.Second

// Database masterdata is restored in place while the service keeps running.
// Bulk negative indexes therefore need a bounded lifetime instead of treating
// one successful load as immutable for the whole process.
const dbBulkIndexTTL = 5 * time.Minute

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

func dbBulkIndexFresh(loaded bool, loadedAt time.Time) bool {
	return loaded && !loadedAt.IsZero() && time.Since(loadedAt) < dbBulkIndexTTL
}
