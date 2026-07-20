package commandtrace

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTraceAggregatesConcurrentOperations(t *testing.T) {
	ctx, trace := WithTrace(context.Background())
	const workers = 32
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			RecordOperation(ctx, "asset.stat", 250*time.Microsecond)
		}()
	}
	wg.Wait()

	snapshot := trace.Snapshot()
	if len(snapshot.Operations) != 1 {
		t.Fatalf("operations = %+v", snapshot.Operations)
	}
	got := snapshot.Operations[0]
	if got.Count != workers || got.Total != workers*250*time.Microsecond || got.Max != 250*time.Microsecond {
		t.Fatalf("unexpected aggregate: %+v", got)
	}
}

func TestTraceSeparatesPhasesAndNestedOperations(t *testing.T) {
	ctx, trace := WithTrace(context.Background())
	RecordPhase(ctx, "command_execute", 10*time.Millisecond)
	RecordOperation(ctx, "drawing.http", 4*time.Millisecond)

	snapshot := trace.Snapshot()
	if len(snapshot.Phases) != 1 || snapshot.Phases[0].Name != "command_execute" {
		t.Fatalf("phases = %+v", snapshot.Phases)
	}
	if len(snapshot.Operations) != 1 || snapshot.Operations[0].Name != "drawing.http" {
		t.Fatalf("operations = %+v", snapshot.Operations)
	}
	if got := snapshot.UnattributedDuration(12 * time.Millisecond); got != 2*time.Millisecond {
		t.Fatalf("unattributed = %s", got)
	}
	if got := snapshot.PhaseOverrunDuration(8 * time.Millisecond); got != 2*time.Millisecond {
		t.Fatalf("phase overrun = %s", got)
	}
}

func TestStatsLogValuePreservesSubMillisecondPrecision(t *testing.T) {
	value := statsLogValue([]Stats{{Name: "response_encode", Count: 1, Total: 375 * time.Microsecond, Max: 375 * time.Microsecond}})
	group := value.Group()
	if len(group) != 1 {
		t.Fatalf("group = %+v", group)
	}
	var total float64
	for _, attr := range group[0].Value.Group() {
		if attr.Key == "total_ms" {
			total = attr.Value.Float64()
		}
	}
	if total != 0.375 {
		t.Fatalf("total_ms = %v, want 0.375", total)
	}
}

func TestMergeOperationsPreservesAggregates(t *testing.T) {
	ctx, trace := WithTrace(context.Background())
	MergeOperations(ctx, []Stats{{
		Name:  "drawing.http",
		Count: 2,
		Total: 7 * time.Millisecond,
		Max:   5 * time.Millisecond,
	}})

	operations := trace.Snapshot().Operations
	if len(operations) != 1 {
		t.Fatalf("operations = %#v, want one", operations)
	}
	if got := operations[0]; got.Count != 2 || got.Total != 7*time.Millisecond || got.Max != 5*time.Millisecond {
		t.Fatalf("merged operation = %#v", got)
	}
}

func TestMeasureFinishIsIdempotent(t *testing.T) {
	ctx, trace := WithTrace(context.Background())
	finish := MeasurePhase(ctx, "request_decode")
	finish()
	finish()

	phases := trace.Snapshot().Phases
	if len(phases) != 1 || phases[0].Count != 1 {
		t.Fatalf("phases = %+v, want one completed measurement", phases)
	}
}

func TestSnapshotStatsAreSortedAndNegativeDurationsClampToZero(t *testing.T) {
	ctx, trace := WithTrace(context.Background())
	RecordOperation(ctx, "z.last", time.Millisecond)
	RecordOperation(ctx, "a.first", -time.Millisecond)

	operations := trace.Snapshot().Operations
	if len(operations) != 2 || operations[0].Name != "a.first" || operations[1].Name != "z.last" {
		t.Fatalf("operations are not deterministically sorted: %+v", operations)
	}
	if operations[0].Total != 0 || operations[0].Max != 0 {
		t.Fatalf("negative duration was not clamped: %+v", operations[0])
	}
}

func TestMeasureWithoutTraceIsSafe(t *testing.T) {
	finishPhase := MeasurePhase(context.Background(), "request_decode")
	var unsetContext context.Context
	finishOperation := MeasureOperation(unsetContext, "asset.stat")
	finishPhase()
	finishOperation()
}

func TestSetErrorTypeKeepsLatestBoundaryFailure(t *testing.T) {
	ctx, trace := WithTrace(context.Background())
	SetErrorType(ctx, "*fiber.Error")
	SetErrorType(ctx, "panic")

	if got := trace.Snapshot().ErrorType; got != "panic" {
		t.Fatalf("error type = %q, want panic", got)
	}
}
