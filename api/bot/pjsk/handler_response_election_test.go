package pjsk

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	commandhandler "haruki-cloud/internal/pjsk/handler"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestDisabledResponseElectionFallsBackToRequestGuard(t *testing.T) {
	t.Parallel()

	t.Run("selected request executes and completes guard", func(t *testing.T) {
		guard := &birthdayMonitorTestGuard{allow: true}
		coordinator := &requestGuardResponseElection{guard: guard}
		executions := 0
		decision := coordinator.Coordinate(
			context.Background(),
			responseElectionRequest{Request: responseElectionIdentityTestRequest(), BotID: "bot-a"},
			func(context.Context) sharedCommandResult {
				executions++
				return responseElectionTestResult("guard-result", "bot-a", false)
			},
		)
		if !decision.visible || decision.reason != "guard_selected" {
			t.Fatalf("decision = %+v, want visible guard selection", decision)
		}
		if executions != 1 || guard.acquired != 1 || guard.completed != 1 {
			t.Fatalf("executions/acquired/completed = %d/%d/%d, want 1/1/1", executions, guard.acquired, guard.completed)
		}
	})

	t.Run("rejected duplicate does not execute or complete", func(t *testing.T) {
		guard := &birthdayMonitorTestGuard{}
		coordinator := &requestGuardResponseElection{guard: guard}
		executions := 0
		decision := coordinator.Coordinate(
			context.Background(),
			responseElectionRequest{Request: responseElectionIdentityTestRequest(), BotID: "bot-b"},
			func(context.Context) sharedCommandResult {
				executions++
				return sharedCommandResult{}
			},
		)
		if decision.visible || decision.reason != "guard_rejected" {
			t.Fatalf("decision = %+v, want invisible guard rejection", decision)
		}
		if executions != 0 || guard.acquired != 1 || guard.completed != 0 {
			t.Fatalf("executions/acquired/completed = %d/%d/%d, want 0/1/0", executions, guard.acquired, guard.completed)
		}
	})
}

func TestCommandResponseDependsOnExecutor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resolved *commandhandler.CommandRequest
		want     bool
	}{
		{name: "nil request"},
		{
			name:     "non CN MySekai",
			resolved: &commandhandler.CommandRequest{Region: "jp", Mode: "mysekai-resource"},
		},
		{
			name:     "CN profile MySekai lookup",
			resolved: &commandhandler.CommandRequest{Region: "cn", Mode: "mysekai"},
			want:     true,
		},
		{
			name:     "CN MySekai render",
			resolved: &commandhandler.CommandRequest{Region: "CN", Mode: "mysekai-resource"},
			want:     true,
		},
		{
			name:     "CN housing ranking exception",
			resolved: &commandhandler.CommandRequest{Region: "cn", Mode: "mysekai-housing-sk"},
		},
		{
			name:     "CN MySekai deck exception",
			resolved: &commandhandler.CommandRequest{Region: "cn", Mode: "deck-mysekai"},
		},
		{
			name:     "unrelated CN command",
			resolved: &commandhandler.CommandRequest{Region: "cn", Mode: "card-list"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := commandResponseDependsOnExecutor(test.resolved); got != test.want {
				t.Fatalf("commandResponseDependsOnExecutor() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSynchronousResponseElectionPathsIsolateCommandPhases(t *testing.T) {
	t.Run("direct", func(t *testing.T) {
		assertSynchronousResponseElectionTraceIsolation(t, func(
			ctx context.Context,
			execute func(context.Context) sharedCommandResult,
		) responseElectionDecision {
			return coordinateCommandResponse(ctx, nil, responseElectionRequest{}, execute)
		}, "direct")
	})

	t.Run("legacy guard", func(t *testing.T) {
		coordinator := &requestGuardResponseElection{guard: &birthdayMonitorTestGuard{allow: true}}
		assertSynchronousResponseElectionTraceIsolation(t, func(
			ctx context.Context,
			execute func(context.Context) sharedCommandResult,
		) responseElectionDecision {
			return coordinator.Coordinate(ctx, responseElectionRequest{Request: responseElectionIdentityTestRequest()}, execute)
		}, "guard_selected")
	})

	t.Run("Redis admission fail-open for direct messages", func(t *testing.T) {
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{
			Addr:         server.Addr(),
			DialTimeout:  10 * time.Millisecond,
			ReadTimeout:  10 * time.Millisecond,
			WriteTimeout: 10 * time.Millisecond,
			MaxRetries:   -1,
		})
		coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
		t.Cleanup(func() {
			coordinator.Close()
			_ = client.Close()
		})
		server.Close()

		request := responseElectionIdentityTestRequest()
		request.PlatformGroupID = ""
		assertSynchronousResponseElectionTraceIsolation(t, func(
			ctx context.Context,
			execute func(context.Context) sharedCommandResult,
		) responseElectionDecision {
			return coordinator.Coordinate(ctx, responseElectionRequest{
				Request: request,
				BotID:   "bot-a",
			}, execute)
		}, "admission_fail_open")
	})

	t.Run("Redis admission fails closed for group messages", func(t *testing.T) {
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{
			Addr:         server.Addr(),
			DialTimeout:  10 * time.Millisecond,
			ReadTimeout:  10 * time.Millisecond,
			WriteTimeout: 10 * time.Millisecond,
			MaxRetries:   -1,
		})
		coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
		t.Cleanup(func() {
			coordinator.Close()
			_ = client.Close()
		})
		server.Close()

		executed := false
		decision := coordinator.Coordinate(
			context.Background(),
			responseElectionRequest{Request: responseElectionIdentityTestRequest(), BotID: "bot-a"},
			func(context.Context) sharedCommandResult {
				executed = true
				return responseElectionTestResult("unexpected", "bot-a", false)
			},
		)
		if decision.visible || decision.reason != "admission_fail_closed" || executed {
			t.Fatalf("decision = %+v, executed=%v; want invisible fail-closed", decision, executed)
		}
	})
}

func assertSynchronousResponseElectionTraceIsolation(
	t *testing.T,
	coordinate func(context.Context, func(context.Context) sharedCommandResult) responseElectionDecision,
	wantReason string,
) {
	t.Helper()
	ctx, trace := commandtrace.WithTrace(context.Background())
	finishElection := commandtrace.MeasurePhase(ctx, "response_election")
	decision := coordinate(ctx, func(executionCtx context.Context) sharedCommandResult {
		finishExecute := commandtrace.MeasurePhase(executionCtx, "command_execute")
		finishNested := commandtrace.MeasureOperation(executionCtx, "nested_operation")
		finishNested()
		finishExecute()
		return responseElectionTestResult("result", "bot-a", false)
	})
	finishElection()
	mergeSharedCommandOperations(ctx, decision.result.Operations)

	if !decision.visible || decision.reason != wantReason {
		t.Fatalf("decision = %+v, want visible reason %q", decision, wantReason)
	}
	snapshot := trace.Snapshot()
	for _, phase := range snapshot.Phases {
		if phase.Name == "command_execute" {
			t.Fatalf("command_execute leaked into top-level phases: %+v", snapshot.Phases)
		}
	}
	assertResponseElectionTraceOperation(t, snapshot, "command.shared.phase.command_execute")
	assertResponseElectionTraceOperation(t, snapshot, "command.shared.operation.nested_operation")
}
