//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package pjsk

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/testutil"

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
	{
		decision := legacy.Coordinate(context.Background(), request, nil)
		testutil.Require(t, !(decision.reason != "missing_executor"), "missing legacy executor decision = %+v", decision)
	}
	{

		decision := legacy.Coordinate(context.Background(), request, execute)
		{
			testutil.Require(t, !(decision.reason != "guard_rejected"), "rejected legacy decision = %+v", decision)
			testutil.Require(t, !(decision.visible), "rejected legacy decision = %+v", decision)
		}
	}

	guard.allow = true
	{
		decision := legacy.Coordinate(context.Background(), request, execute)
		{
			testutil.Require(t, !(decision.reason != "guard_selected"), "selected legacy decision = %+v", decision)
			testutil.Require(t, decision.visible, "selected legacy decision = %+v", decision)
			testutil.Require(t, !(string(decision.result.Response.JSONBody) != "result"), "selected legacy decision = %+v", decision)
		}
	}

	legacy.Close()
	{

		decision := coordinateCommandResponse(context.Background(), nil, request, nil)
		testutil.Require(t, !(decision.reason != "missing_executor"), "missing direct executor decision = %+v", decision)
	}
	{

		decision := coordinateCommandResponse(context.Background(), nil, request, execute)
		{
			testutil.Require(t, !(decision.reason != "direct"), "direct decision = %+v", decision)
			testutil.Require(t, decision.visible, "direct decision = %+v", decision)
		}
	}

	stub := &responseElectionCoverageStub{decision: responseElectionDecision{reason: "stub"}}
	{
		decision := coordinateCommandResponse(context.Background(), stub, request, execute)
		{
			testutil.Require(t, !(decision.reason != "stub"), "stub decision = %+v, calls=%d", decision, stub.called)
			testutil.Require(t, !(stub.called != 1), "stub decision = %+v, calls=%d", decision, stub.called)
		}
	}

	stub.Close()
	testutil.Require(t, !(stub.closed != 1), "stub close count = %d", stub.closed)
	testutil.RequireArgs(t, !(NewResponseElectionCoordinator(context.Background(), nil, time.Second) != nil), "nil Redis client produced a coordinator")

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	testutil.RequireArgs(t, !(NewResponseElectionCoordinator(context.Background(), client, -time.Second) != nil), "negative election window produced a coordinator")

	defaulted := NewResponseElectionCoordinator(nil, client, 0)
	{
		testutil.Require(t, !(defaulted == nil), "default coordinator window = %+v", defaulted)
		testutil.Require(t, !(defaulted.window != defaultResponseElectionWindow), "default coordinator window = %+v", defaulted)
	}

	defaulted.Close()
	clamped := NewResponseElectionCoordinator(context.Background(), client, time.Nanosecond)
	{
		testutil.Require(t, !(clamped.window != time.Millisecond), "clamped/roster coordinator = %+v", clamped)
		testutil.Require(t, !((*ResponseElectionCoordinator)(nil).WithRoster(nil) != nil), "clamped/roster coordinator = %+v", clamped)
		testutil.Require(t, !(clamped.WithRoster(nil) != clamped), "clamped/roster coordinator = %+v", clamped)
	}
	{

		decision := clamped.Coordinate(context.Background(), request, nil)
		testutil.Require(t, !(decision.reason != "missing_executor"), "missing coordinator executor decision = %+v", decision)
	}
	{

		decision := clamped.Coordinate(nil, responseElectionRequest{Request: request.Request}, execute)
		{
			testutil.Require(t, !(decision.reason != "direct"), "empty bot direct decision = %+v", decision)
			testutil.Require(t, decision.visible, "empty bot direct decision = %+v", decision)
		}
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	{
		decision := clamped.Coordinate(canceledCtx, request, execute)
		testutil.Require(t, !(decision.reason != "canceled"), "canceled request decision = %+v", decision)
	}

	clamped.cancel()
	{
		decision := clamped.Coordinate(context.Background(), request, execute)
		testutil.Require(t, !(decision.reason != "canceled"), "shutdown worker decision = %+v", decision)
	}

	clamped.Close()

	var nilCoordinator *ResponseElectionCoordinator
	{
		decision := nilCoordinator.Coordinate(context.Background(), request, execute)
		{
			testutil.Require(t, !(decision.reason != "direct"), "nil coordinator decision = %+v", decision)
			testutil.Require(t, decision.visible, "nil coordinator decision = %+v", decision)
		}
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
	{
		testutil.Require(t, !(decision.reason != "admission_fail_open"), "admission fail-open decision = %+v, executions=%d", decision, executions)
		testutil.Require(t, decision.visible, "admission fail-open decision = %+v, executions=%d", decision, executions)
		testutil.Require(t, !(executions != 1), "admission fail-open decision = %+v, executions=%d", decision, executions)
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
	testutil.Require(t, errors.Is(result.err, context.Canceled), "closing executor error = %v", result.err)

	closing.Close()
}

func TestSharedCommandPanicAndTraceBranches(t *testing.T) {
	result := executeSharedCommandSafely(context.Background(), func(context.Context) sharedCommandResult {
		panic("boom")
	})
	{
		testutil.Require(t, !(result.Metadata.Outcome != "error"), "panic result = %+v", result)
		testutil.Require(t, !(result.Metadata.ErrorType != "panic"), "panic result = %+v", result)
		testutil.Require(t, !(result.Response.HTTPStatus != 500), "panic result = %+v", result)
		testutil.Require(t, !(len(result.Response.JSONBody) == 0), "panic result = %+v", result)
	}
	{

		direct := sharedCommandPanicResult(context.Background())
		{
			testutil.Require(t, !(direct.Metadata.ErrorType != "panic"), "direct panic result = %+v", direct)
			testutil.Require(t, !(len(direct.Response.MsgPackBody) == 0), "direct panic result = %+v", direct)
		}
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
	{
		testutil.Require(t, hasResponseElectionSharedOperation(withTrace.Operations, "command.shared.phase.phase"), "shared trace operations = %+v", withTrace.Operations)
		testutil.Require(t, hasResponseElectionSharedOperation(withTrace.Operations, "command.shared.operation.operation"), "shared trace operations = %+v", withTrace.Operations)
		testutil.Require(t, hasResponseElectionSharedOperation(withTrace.Operations, "command.shared_total"), "shared trace operations = %+v", withTrace.Operations)
	}

	withDefaultClock := executeSharedCommandWithTrace(context.Background(), func(context.Context) sharedCommandResult { return sharedCommandResult{} }, nil)
	testutil.Require(t, hasResponseElectionSharedOperation(withDefaultClock.Operations, "command.shared_total"), "default-clock operations = %+v", withDefaultClock.Operations)

	traceCtx, trace := commandtrace.WithNewTrace(context.Background())
	mergeSharedCommandOperations(traceCtx, []sharedCommandOperation{{Name: "merged", Count: 2, TotalNanos: 3, MaxNanos: 2}})
	snapshot := trace.Snapshot()
	{
		testutil.Require(t, !(len(snapshot.Operations) != 1), "merged trace snapshot = %+v", snapshot)
		testutil.Require(t, !(snapshot.Operations[0].Name != "merged"), "merged trace snapshot = %+v", snapshot)
		testutil.Require(t, !(snapshot.Operations[0].Count != 2), "merged trace snapshot = %+v", snapshot)
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
	{
		decision := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{result: result, err: errResponseElectionAborted})
		{
			testutil.Require(t, !(decision.reason != "publish_fail_open"), "aborted publish decision = %+v", decision)
			testutil.Require(t, decision.visible, "aborted publish decision = %+v", decision)
		}
	}
	{

		decision := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{result: result, err: errors.New("unknown")})
		{
			testutil.Require(t, !(decision.reason != "publish_unknown"), "unknown publish decision = %+v", decision)
			testutil.Require(t, !(decision.visible), "unknown publish decision = %+v", decision)
		}
	}
	{

		decision := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{result: result, err: context.Canceled})
		testutil.Require(t, !(decision.reason != "shutdown"), "canceled publish decision = %+v", decision)
	}

	coordinator.cancel()
	{
		decision := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{result: result, err: errors.New("unknown")})
		testutil.Require(t, !(decision.reason != "shutdown"), "worker-shutdown publish decision = %+v", decision)
	}
	{

		got := responseElectionTTL(time.Second, time.Second, time.Second)
		testutil.Require(t, !(got != 2*time.Second), "normal response-election TTL = %v", got)
	}

	maxDuration := time.Duration(1<<63 - 1)
	{
		got := responseElectionTTL(time.Second, maxDuration, time.Second)
		testutil.Require(t, !(got != maxDuration), "overflow response-election TTL = %v", got)
	}

	intCases := []struct {
		value any
		want  int64
		ok    bool
	}{{int64(1), 1, true}, {"2", 2, true}, {[]byte("3"), 3, true}, {"bad", 0, false}, {1, 0, false}}
	for _, tc := range intCases {
		got, err := responseElectionInt64(tc.value)
		{
			testutil.Require(t, !(got != tc.want), "responseElectionInt64(%#v) = %d, %v", tc.value, got, err)
			testutil.Require(t, !((err == nil) != tc.ok), "responseElectionInt64(%#v) = %d, %v", tc.value, got, err)
		}

	}
	{
		got, err := responseElectionBytes("value")
		{
			testutil.Require(t, !(err != nil), "string bytes = %q, %v", got, err)
			testutil.Require(t, !(string(got) != "value"), "string bytes = %q, %v", got, err)
		}
	}

	original := []byte("bytes")
	{
		got, err := responseElectionBytes(original)
		{
			testutil.Require(t, !(err != nil), "raw bytes = %q, %v", got, err)
			testutil.Require(t, reflect.DeepEqual(got, original), "raw bytes = %q, %v", got, err)
		}
	}
	{

		_, err := responseElectionBytes(1)
		testutil.RequireArgs(t, !(err == nil), "unexpected byte type succeeded")
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
	testutil.Require(t, errors.Is(err, errResponseElectionUnknown), "missing-owner publish error = %v", err)
	{

		err := coordinator.reconcileFailedPublish(lease, errors.New("publish"))
		testutil.Require(t, errors.Is(err, errResponseElectionUnknown), "missing-state reconciliation error = %v", err)
	}

	closedServer := miniredis.RunT(t)
	closedClient := redis.NewClient(&redis.Options{Addr: closedServer.Addr()})
	closedCoordinator := NewResponseElectionCoordinator(context.Background(), closedClient, time.Millisecond)
	_ = closedClient.Close()
	{
		err := closedCoordinator.reconcileFailedPublish(lease, errors.New("publish"))
		testutil.Require(t, errors.Is(err, errResponseElectionUnknown), "closed-Redis reconciliation error = %v", err)
	}
	{

		_, _, err := closedCoordinator.decide(context.Background(), lease)
		testutil.RequireArgs(t, !(err == nil), "closed-Redis decide succeeded")
	}

	closedCoordinator.Close()
}
