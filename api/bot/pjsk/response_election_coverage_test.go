//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package pjsk

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type responseElectionCoverageStub struct {
	decision responseElectionDecision
	called   int
	closed   int
}

func (s *responseElectionCoverageStub) Coordinate(context.Context, responseElectionRequest, func(context.Context) sharedCommandResult) responseElectionDecision {
	s.called++
	return s.decision
}

func (s *responseElectionCoverageStub) Close() { s.closed++ }

func TestResponseElectionDirectGuardAndConstructorBranches(t *testing.T) {
	request := responseElectionRequest{Request: responseElectionTestRequest("coverage"), BotID: "bot"}
	execute := func(context.Context) sharedCommandResult { return responseElectionTestResult("result", "bot", false) }

	guard := &birthdayMonitorTestGuard{allow: false}
	legacy := &requestGuardResponseElection{guard: guard}
	if decision := legacy.Coordinate(context.Background(), request, nil); decision.reason != "missing_executor" {
		t.Fatalf("missing legacy executor decision = %+v", decision)
	}
	if decision := legacy.Coordinate(context.Background(), request, execute); decision.reason != "guard_rejected" || decision.visible {
		t.Fatalf("rejected legacy decision = %+v", decision)
	}
	guard.allow = true
	if decision := legacy.Coordinate(context.Background(), request, execute); decision.reason != "guard_selected" || !decision.visible || string(decision.result.Response.JSONBody) != "result" {
		t.Fatalf("selected legacy decision = %+v", decision)
	}
	legacy.Close()

	if decision := coordinateCommandResponse(context.Background(), nil, request, nil); decision.reason != "missing_executor" {
		t.Fatalf("missing direct executor decision = %+v", decision)
	}
	if decision := coordinateCommandResponse(context.Background(), nil, request, execute); decision.reason != "direct" || !decision.visible {
		t.Fatalf("direct decision = %+v", decision)
	}
	stub := &responseElectionCoverageStub{decision: responseElectionDecision{reason: "stub"}}
	if decision := coordinateCommandResponse(context.Background(), stub, request, execute); decision.reason != "stub" || stub.called != 1 {
		t.Fatalf("stub decision = %+v, calls=%d", decision, stub.called)
	}
	stub.Close()
	if stub.closed != 1 {
		t.Fatalf("stub close count = %d", stub.closed)
	}

	if NewResponseElectionCoordinator(context.Background(), nil, time.Second) != nil {
		t.Fatal("nil Redis client produced a coordinator")
	}
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	if NewResponseElectionCoordinator(context.Background(), client, -time.Second) != nil {
		t.Fatal("negative election window produced a coordinator")
	}
	defaulted := NewResponseElectionCoordinator(nil, client, 0)
	if defaulted == nil || defaulted.window != defaultResponseElectionWindow {
		t.Fatalf("default coordinator window = %+v", defaulted)
	}
	defaulted.Close()
	clamped := NewResponseElectionCoordinator(context.Background(), client, time.Nanosecond)
	if clamped.window != time.Millisecond || (*ResponseElectionCoordinator)(nil).WithRoster(nil) != nil || clamped.WithRoster(nil) != clamped {
		t.Fatalf("clamped/roster coordinator = %+v", clamped)
	}
	if decision := clamped.Coordinate(context.Background(), request, nil); decision.reason != "missing_executor" {
		t.Fatalf("missing coordinator executor decision = %+v", decision)
	}
	if decision := clamped.Coordinate(nil, responseElectionRequest{Request: request.Request}, execute); decision.reason != "direct" || !decision.visible {
		t.Fatalf("empty bot direct decision = %+v", decision)
	}
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if decision := clamped.Coordinate(canceledCtx, request, execute); decision.reason != "canceled" {
		t.Fatalf("canceled request decision = %+v", decision)
	}
	clamped.cancel()
	if decision := clamped.Coordinate(context.Background(), request, execute); decision.reason != "canceled" {
		t.Fatalf("shutdown worker decision = %+v", decision)
	}
	clamped.Close()

	var nilCoordinator *ResponseElectionCoordinator
	if decision := nilCoordinator.Coordinate(context.Background(), request, execute); decision.reason != "direct" || !decision.visible {
		t.Fatalf("nil coordinator decision = %+v", decision)
	}
	nilCoordinator.Close()
}

func TestResponseElectionAdmissionFailOpenAndClosingExecutor(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: time.Millisecond, ReadTimeout: time.Millisecond, WriteTimeout: time.Millisecond, MaxRetries: 0})
	t.Cleanup(func() { _ = client.Close() })
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Millisecond)
	executions := 0
	decision := coordinator.Coordinate(context.Background(), responseElectionRequest{
		Request: responseElectionTestRequest("fail-open"), BotID: "bot",
	}, func(context.Context) sharedCommandResult {
		executions++
		return responseElectionTestResult("fallback", "bot", false)
	})
	if decision.reason != "admission_fail_open" || !decision.visible || executions != 1 {
		t.Fatalf("admission fail-open decision = %+v, executions=%d", decision, executions)
	}
	coordinator.Close()

	server := miniredis.RunT(t)
	workingClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = workingClient.Close() })
	closing := NewResponseElectionCoordinator(context.Background(), workingClient, time.Millisecond)
	closing.workersMu.Lock()
	closing.closing = true
	closing.workersMu.Unlock()
	result := <-closing.startExecutor(context.Background(), responseElectionLease{}, func(context.Context) sharedCommandResult {
		t.Fatal("executor ran while coordinator was closing")
		return sharedCommandResult{}
	})
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("closing executor error = %v", result.err)
	}
	closing.Close()
}

func TestSharedCommandPanicAndTraceBranches(t *testing.T) {
	result := executeSharedCommandSafely(context.Background(), func(context.Context) sharedCommandResult {
		panic("boom")
	})
	if result.Metadata.Outcome != "error" || result.Metadata.ErrorType != "panic" || result.Response.HTTPStatus != 500 || len(result.Response.JSONBody) == 0 {
		t.Fatalf("panic result = %+v", result)
	}
	if direct := sharedCommandPanicResult(context.Background()); direct.Metadata.ErrorType != "panic" || len(direct.Response.MsgPackBody) == 0 {
		t.Fatalf("direct panic result = %+v", direct)
	}

	nowValues := []time.Time{time.Unix(10, 0), time.Unix(10, 25)}
	nowIndex := 0
	withTrace := executeSharedCommandWithTrace(context.Background(), func(ctx context.Context) sharedCommandResult {
		finishPhase := commandtrace.MeasurePhase(ctx, "phase")
		finishPhase()
		finishOperation := commandtrace.MeasureOperation(ctx, "operation")
		finishOperation()
		return responseElectionTestResult("trace", "bot", false)
	}, func() time.Time {
		value := nowValues[nowIndex]
		nowIndex++
		return value
	})
	if !hasResponseElectionSharedOperation(withTrace.Operations, "command.shared.phase.phase") || !hasResponseElectionSharedOperation(withTrace.Operations, "command.shared.operation.operation") || !hasResponseElectionSharedOperation(withTrace.Operations, "command.shared_total") {
		t.Fatalf("shared trace operations = %+v", withTrace.Operations)
	}
	withDefaultClock := executeSharedCommandWithTrace(context.Background(), func(context.Context) sharedCommandResult { return sharedCommandResult{} }, nil)
	if !hasResponseElectionSharedOperation(withDefaultClock.Operations, "command.shared_total") {
		t.Fatalf("default-clock operations = %+v", withDefaultClock.Operations)
	}

	traceCtx, trace := commandtrace.WithNewTrace(context.Background())
	mergeSharedCommandOperations(traceCtx, []sharedCommandOperation{{Name: "merged", Count: 2, TotalNanos: 3, MaxNanos: 2}})
	snapshot := trace.Snapshot()
	if len(snapshot.Operations) != 1 || snapshot.Operations[0].Name != "merged" || snapshot.Operations[0].Count != 2 {
		t.Fatalf("merged trace snapshot = %+v", snapshot)
	}
}

func TestResponseElectionFailureDecisionConversionAndTTLBranches(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Millisecond)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})
	result := responseElectionTestResult("result", "bot", false)
	if decision := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{result: result, err: errResponseElectionAborted}); decision.reason != "publish_fail_open" || !decision.visible {
		t.Fatalf("aborted publish decision = %+v", decision)
	}
	if decision := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{result: result, err: errors.New("unknown")}); decision.reason != "publish_unknown" || decision.visible {
		t.Fatalf("unknown publish decision = %+v", decision)
	}
	if decision := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{result: result, err: context.Canceled}); decision.reason != "shutdown" {
		t.Fatalf("canceled publish decision = %+v", decision)
	}
	coordinator.cancel()
	if decision := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{result: result, err: errors.New("unknown")}); decision.reason != "shutdown" {
		t.Fatalf("worker-shutdown publish decision = %+v", decision)
	}

	if got := responseElectionTTL(time.Second, time.Second, time.Second); got != 2*time.Second {
		t.Fatalf("normal response-election TTL = %v", got)
	}
	maxDuration := time.Duration(1<<63 - 1)
	if got := responseElectionTTL(time.Second, maxDuration, time.Second); got != maxDuration {
		t.Fatalf("overflow response-election TTL = %v", got)
	}
	intCases := []struct {
		value any
		want  int64
		ok    bool
	}{{int64(1), 1, true}, {"2", 2, true}, {[]byte("3"), 3, true}, {"bad", 0, false}, {1, 0, false}}
	for _, tc := range intCases {
		got, err := responseElectionInt64(tc.value)
		if got != tc.want || (err == nil) != tc.ok {
			t.Fatalf("responseElectionInt64(%#v) = %d, %v", tc.value, got, err)
		}
	}
	if got, err := responseElectionBytes("value"); err != nil || string(got) != "value" {
		t.Fatalf("string bytes = %q, %v", got, err)
	}
	original := []byte("bytes")
	if got, err := responseElectionBytes(original); err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("raw bytes = %q, %v", got, err)
	}
	if _, err := responseElectionBytes(1); err == nil {
		t.Fatal("unexpected byte type succeeded")
	}

	coordinator.leave(responseElectionLease{})
	(*ResponseElectionCoordinator)(nil).leave(responseElectionLease{token: "token", botID: "bot"})
	closedServer := miniredis.RunT(t)
	closedClient := redis.NewClient(&redis.Options{Addr: closedServer.Addr()})
	closedCoordinator := NewResponseElectionCoordinator(context.Background(), closedClient, time.Millisecond)
	_ = closedClient.Close()
	closedCoordinator.leave(responseElectionLease{stateKey: "state", candidatesKey: "candidates", token: "token", botID: "bot"})
	closedCoordinator.Close()
}

func TestResponseElectionPublishAndReconcileFailureBranches(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Millisecond)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})
	lease := responseElectionLease{
		stateKey: "missing-state", candidatesKey: "missing-candidates", rateKey: "missing-rate", legacyLockKey: "missing-lock",
		token: "owner", botID: "bot",
	}
	err := coordinator.publish(context.Background(), lease, responseElectionTestResult("result", "bot", true))
	if !errors.Is(err, errResponseElectionUnknown) {
		t.Fatalf("missing-owner publish error = %v", err)
	}
	if err := coordinator.reconcileFailedPublish(lease, errors.New("publish")); !errors.Is(err, errResponseElectionUnknown) {
		t.Fatalf("missing-state reconciliation error = %v", err)
	}

	closedServer := miniredis.RunT(t)
	closedClient := redis.NewClient(&redis.Options{Addr: closedServer.Addr()})
	closedCoordinator := NewResponseElectionCoordinator(context.Background(), closedClient, time.Millisecond)
	_ = closedClient.Close()
	if err := closedCoordinator.reconcileFailedPublish(lease, errors.New("publish")); !errors.Is(err, errResponseElectionUnknown) {
		t.Fatalf("closed-Redis reconciliation error = %v", err)
	}
	if _, _, err := closedCoordinator.decide(context.Background(), lease); err == nil {
		t.Fatal("closed-Redis decide succeeded")
	}
	closedCoordinator.Close()
}
