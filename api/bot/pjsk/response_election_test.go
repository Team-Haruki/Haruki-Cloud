package pjsk

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestResponseElectionCoordinatesAcrossInstancesAndWaitsForWindow(t *testing.T) {
	rig := newResponseElectionTestRig(t, 2, time.Second)
	request := responseElectionTestRequest("fast-command")
	results := make(chan responseElectionTestOutcome, 3)
	var executions atomic.Int32
	execute := func(context.Context) sharedCommandResult {
		executions.Add(1)
		return responseElectionTestResult("fast-result", "", false)
	}

	rig.start(results, 0, request, "bot-a", execute)
	rig.start(results, 1, request, "bot-b", execute)
	rig.start(results, 0, request, "bot-c", execute)
	rig.waitForCandidateCount(t, request, 3)
	rig.waitForStateField(t, request, "ready", "1")
	{

		got := executions.Load()
		testutil.Require(t, !(got != 1), "command executions before election = %d, want 1", got)
	}

	assertNoResponseElectionDecision(t, results, "fast command returned before the election window closed")

	rig.advancePastWindow()
	outcomes := collectResponseElectionOutcomes(t, results, 3)
	winner := assertSingleVisibleResponseElectionOutcome(t, outcomes)
	{
		got := string(winner.decision.result.Response.JSONBody)
		testutil.Require(t, !(got != "fast-result"), "winner result = %q, want %q", got, "fast-result")
	}
	{

		got := executions.Load()
		testutil.Require(t, !(got != 1), "command executions after election = %d, want 1", got)
	}

	for _, operation := range []string{
		"response_election.redis_join",
		"response_election.window_wait",
		"response_election.result_wait",
		"response_election.redis_decide",
		"response_election.result_deserialize",
	} {
		assertResponseElectionTraceOperation(t, winner.trace, operation)
	}
	testutil.Require(t, hasResponseElectionSharedOperation(winner.decision.result.Operations, "response_election.result_serialize"), "winner shared operations missing result serialization: %+v", winner.decision.result.Operations)

}

func TestResponseElectionSlowCommandReturnsWhenReadyAfterWindow(t *testing.T) {
	rig := newResponseElectionTestRig(t, 2, 2*time.Second)
	request := responseElectionTestRequest("slow-command")
	results := make(chan responseElectionTestOutcome, 3)
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	var executions atomic.Int32
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	execute := func(ctx context.Context) sharedCommandResult {
		executions.Add(1)
		startedOnce.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return responseElectionTestResult("slow-result", "", false)
	}

	rig.start(results, 0, request, "bot-a", execute)
	rig.start(results, 1, request, "bot-b", execute)
	rig.start(results, 0, request, "bot-c", execute)
	waitForResponseElectionSignal(t, started, "executor start")
	rig.waitForCandidateCount(t, request, 3)

	rig.advancePastWindow()
	assertNoResponseElectionDecision(t, results, "slow command returned before execution completed")

	releasedAt := time.Now()
	releaseOnce.Do(func() { close(release) })
	outcomes := collectResponseElectionOutcomes(t, results, 3)
	{
		elapsed := time.Since(releasedAt)
		testutil.Require(t, !(elapsed >= 750*time.Millisecond), "command took %s to return after becoming ready; window had already closed", elapsed)
	}

	winner := assertSingleVisibleResponseElectionOutcome(t, outcomes)
	{
		got := string(winner.decision.result.Response.JSONBody)
		testutil.Require(t, !(got != "slow-result"), "winner result = %q, want %q", got, "slow-result")
	}
	{

		got := executions.Load()
		testutil.Require(t, !(got != 1), "command executions = %d, want 1", got)
	}

}

func TestResponseElectionRejectsLateDuplicatesWithoutExecuting(t *testing.T) {
	rig := newResponseElectionTestRig(t, 2, time.Second)
	request := responseElectionTestRequest("late-duplicate")
	initialResults := make(chan responseElectionTestOutcome, 2)
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	var executions atomic.Int32
	var lateExecutions atomic.Int32
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	execute := func(ctx context.Context) sharedCommandResult {
		executions.Add(1)
		startedOnce.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return responseElectionTestResult("original-result", "", false)
	}
	lateExecute := func(context.Context) sharedCommandResult {
		lateExecutions.Add(1)
		return responseElectionTestResult("late-result", "", false)
	}

	rig.start(initialResults, 0, request, "bot-a", execute)
	rig.start(initialResults, 1, request, "bot-b", execute)
	waitForResponseElectionSignal(t, started, "executor start")
	rig.waitForCandidateCount(t, request, 2)
	rig.advancePastWindow()

	lateResult := make(chan responseElectionTestOutcome, 1)
	rig.start(lateResult, 1, request, "bot-after-window", lateExecute)
	late := collectResponseElectionOutcomes(t, lateResult, 1)[0]
	{
		testutil.Require(t, !(late.decision.visible), "post-window decision = %+v, want rejected and invisible", late.decision)
		testutil.Require(t, !(late.decision.reason != "rejected"), "post-window decision = %+v, want rejected and invisible", late.decision)
	}
	{

		got := lateExecutions.Load()
		testutil.Require(t, !(got != 0), "post-window duplicate executions = %d, want 0", got)
	}

	releaseOnce.Do(func() { close(release) })
	initial := collectResponseElectionOutcomes(t, initialResults, 2)
	assertSingleVisibleResponseElectionOutcome(t, initial)

	postDelivery := rig.coordinators[0].Coordinate(
		context.Background(),
		responseElectionRequest{Request: request, BotID: "bot-after-delivery"},
		lateExecute,
	)
	{
		testutil.Require(t, !(postDelivery.visible), "post-delivery decision = %+v, want rejected and invisible", postDelivery)
		testutil.Require(t, !(postDelivery.reason != "rejected"), "post-delivery decision = %+v, want rejected and invisible", postDelivery)
	}
	{

		got := lateExecutions.Load()
		testutil.Require(t, !(got != 0), "late duplicate executions = %d, want 0", got)
	}
	{

		got := executions.Load()
		testutil.Require(t, !(got != 1), "original command executions = %d, want 1", got)
	}

}

func TestResponseElectionForceExecutorSelectsOwner(t *testing.T) {
	rig := newResponseElectionTestRig(t, 2, time.Second)
	request := responseElectionTestRequest("force-executor")
	results := make(chan responseElectionTestOutcome, 2)
	const executorBotID = "bot-executor"
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	var executions atomic.Int32
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	execute := func(ctx context.Context) sharedCommandResult {
		executions.Add(1)
		startedOnce.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return responseElectionTestResult("executor-result", executorBotID, true)
	}

	rig.start(results, 0, request, executorBotID, execute)
	waitForResponseElectionSignal(t, started, "executor start")
	rig.waitForCandidateCount(t, request, 1)
	seed := rig.stateField(t, request, "seed")
	followerBotID := responseElectionFollowerWithLowerScore(t, seed, executorBotID)
	rig.start(results, 1, request, followerBotID, execute)
	rig.waitForCandidateCount(t, request, 2)

	releaseOnce.Do(func() { close(release) })
	rig.waitForStateField(t, request, "ready", "1")
	rig.advancePastWindow()
	outcomes := collectResponseElectionOutcomes(t, results, 2)
	winner := assertSingleVisibleResponseElectionOutcome(t, outcomes)
	testutil.Require(t, !(winner.botID != executorBotID), "selected bot = %q, want executor %q", winner.botID, executorBotID)
	{

		got := winner.decision.result.Metadata.ExecutorBotID
		testutil.Require(t, !(got != executorBotID), "result executor bot = %q, want %q", got, executorBotID)
	}
	{

		got := executions.Load()
		testutil.Require(t, !(got != 1), "command executions = %d, want 1", got)
	}

}

func TestResponseElectionJoinIsIdempotentAfterAmbiguousRedisReply(t *testing.T) {
	server := miniredis.RunT(t)
	baseTime := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	server.SetTime(baseTime)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	request := responseElectionTestRequest("idempotent-join")
	identity := responseElectionIdentity(request)
	keys := []string{
		responseElectionStateKey(identity),
		responseElectionCandidatesKey(identity),
		rateLimitKey(request),
		dedupKey(request),
	}
	const token = "same-admission-token"
	const botID = "bot-owner"
	const window = time.Second
	runJoin := func() []any {
		t.Helper()
		result, err := joinResponseElectionScript.Run(
			context.Background(),
			client,
			keys,
			token,
			botID,
			window.Milliseconds(),
			dedupInFlightTTL.Milliseconds(),
		).Slice()
		testutil.Require(t, !(err != nil), "join script: %v", err)

		return result
	}

	first := runJoin()
	{
		status, err := responseElectionInt64(first[0])
		{
			testutil.Require(t, !(err != nil), "first join status = %v (%v), want executor", first[0], err)
			testutil.Require(t, !(status != int64(responseElectionExecutor)), "first join status = %v (%v), want executor", first[0], err)
		}
	}

	// A command timeout can make go-redis repeat a script after Redis already
	// committed it. The original token must recover its role even if W elapsed.
	server.SetTime(baseTime.Add(window + time.Millisecond))
	replayed := runJoin()
	{
		status, err := responseElectionInt64(replayed[0])
		{
			testutil.Require(t, !(err != nil), "replayed join status = %v (%v), want executor", replayed[0], err)
			testutil.Require(t, !(status != int64(responseElectionExecutor)), "replayed join status = %v (%v), want executor", replayed[0], err)
		}
	}

}

func TestResponseElectionAdmitsOnlyOneRequestPerBot(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	request := responseElectionTestRequest("same-bot")
	first, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-a"})
	testutil.Require(t, !(err != nil), "first join: %v", err)

	second, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-a"})
	testutil.Require(t, !(err != nil), "duplicate join: %v", err)
	{
		testutil.Require(t, !(first.role != responseElectionExecutor), "first/second roles = %d/%d, want executor/rejected", first.role, second.role)
		testutil.Require(t, !(second.role != responseElectionRejected), "first/second roles = %d/%d, want executor/rejected", first.role, second.role)
	}

	count, err := client.HLen(context.Background(), first.candidatesKey).Result()
	{
		testutil.Require(t, !(err != nil), "candidate count = %d (%v), want 1", count, err)
		testutil.Require(t, !(count != 1), "candidate count = %d (%v), want 1", count, err)
	}

}

func TestResponseElectionInteroperatesWithLegacyRequestGuard(t *testing.T) {
	t.Run("legacy owner excludes election", func(t *testing.T) {
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
		t.Cleanup(func() {
			coordinator.Close()
			_ = client.Close()
		})

		request := responseElectionTestRequest("legacy-owner")
		{
			err := client.Set(context.Background(), dedupKey(request), "legacy-token", dedupInFlightTTL).Err()
			testutil.Require(t, !(err != nil), "seed legacy lock: %v", err)
		}

		lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-new"})
		testutil.Require(t, !(err != nil), "join: %v", err)
		testutil.Require(t, !(lease.role != responseElectionRejected), "election role = %d, want rejected", lease.role)
		{

			exists, err := client.Exists(context.Background(), responseElectionStateKey(responseElectionIdentity(request))).Result()
			{
				testutil.Require(t, !(err != nil), "election state exists = %d (%v), want 0", exists, err)
				testutil.Require(t, !(exists != 0), "election state exists = %d (%v), want 0", exists, err)
			}
		}

	})

	t.Run("election owner excludes legacy and releases compat lock", func(t *testing.T) {
		server := miniredis.RunT(t)
		baseTime := time.Date(2026, time.July, 15, 17, 0, 0, 0, time.UTC)
		server.SetTime(baseTime)
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
		legacyGuard := NewRequestGuard(client)
		t.Cleanup(func() {
			coordinator.Close()
			legacyGuard.Close()
			_ = client.Close()
		})

		request := responseElectionTestRequest("election-owner")
		lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-new"})
		testutil.Require(t, !(err != nil), "join: %v", err)
		testutil.Require(t, !(lease.role != responseElectionExecutor), "election role = %d, want executor", lease.role)
		{

			owner, err := client.Get(context.Background(), dedupKey(request)).Result()
			{
				testutil.Require(t, !(err != nil), "legacy compatibility owner = %q (%v), want %q", owner, err, lease.token)
				testutil.Require(t, !(owner != lease.token), "legacy compatibility owner = %q (%v), want %q", owner, err, lease.token)
			}
		}
		{

			legacyLease := legacyGuard.Acquire(context.Background(), request)
			testutil.RequireArgs(t, !(legacyLease.proceed), "legacy guard acquired an event owned by response election")
		}
		{

			err := coordinator.publish(
				context.Background(),
				lease,
				responseElectionTestResult("compat-result", lease.botID, false),
			)
			testutil.Require(t, !(err != nil), "publish: %v", err)
		}

		server.SetTime(baseTime.Add(coordinator.window + time.Millisecond))
		decision, waiting, err := coordinator.decide(context.Background(), lease)
		{
			testutil.Require(t, !(err != nil), "consume = %+v, waiting=%v, err=%v", decision, waiting, err)
			testutil.Require(t, !(waiting), "consume = %+v, waiting=%v, err=%v", decision, waiting, err)
			testutil.Require(t, decision.visible, "consume = %+v, waiting=%v, err=%v", decision, waiting, err)
		}
		{

			exists, err := client.Exists(context.Background(), dedupKey(request)).Result()
			{
				testutil.Require(t, !(err != nil), "legacy lock exists after consume = %d (%v), want 0", exists, err)
				testutil.Require(t, !(exists != 0), "legacy lock exists after consume = %d (%v), want 0", exists, err)
			}
		}
		{

			exists, err := client.Exists(context.Background(), rateLimitKey(request)).Result()
			{
				testutil.Require(t, !(err != nil), "shared rate key exists = %d (%v), want 1", exists, err)
				testutil.Require(t, !(exists != 1), "shared rate key exists = %d (%v), want 1", exists, err)
			}
		}

	})
}

func TestResponseElectionPublishAndConsumeAreIdempotent(t *testing.T) {
	server := miniredis.RunT(t)
	baseTime := time.Date(2026, time.July, 15, 15, 0, 0, 0, time.UTC)
	server.SetTime(baseTime)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	request := responseElectionTestRequest("idempotent-result")
	lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-owner"})
	testutil.Require(t, !(err != nil), "join: %v", err)
	testutil.Require(t, !(lease.role != responseElectionExecutor), "join role = %d, want executor", lease.role)

	shared := responseElectionTestResult("idempotent-result", "bot-owner", false)
	{
		err := coordinator.publish(context.Background(), lease, shared)
		testutil.Require(t, !(err != nil), "publish: %v", err)
	}
	{

		err := coordinator.reconcileFailedPublish(lease, errors.New("ambiguous Redis reply"))
		testutil.Require(t, !(err != nil), "reconcile committed publication: %v", err)
	}

	server.SetTime(baseTime.Add(coordinator.window + time.Millisecond))
	first, waiting, err := coordinator.decide(context.Background(), lease)
	{
		testutil.Require(t, !(err != nil), "first consume = %+v, waiting=%v, err=%v", first, waiting, err)
		testutil.Require(t, !(waiting), "first consume = %+v, waiting=%v, err=%v", first, waiting, err)
		testutil.Require(t, first.visible, "first consume = %+v, waiting=%v, err=%v", first, waiting, err)
	}

	replayed, waiting, err := coordinator.decide(context.Background(), lease)
	{
		testutil.Require(t, !(err != nil), "replayed consume = %+v, waiting=%v, err=%v", replayed, waiting, err)
		testutil.Require(t, !(waiting), "replayed consume = %+v, waiting=%v, err=%v", replayed, waiting, err)
		testutil.Require(t, replayed.visible, "replayed consume = %+v, waiting=%v, err=%v", replayed, waiting, err)
	}
	{

		got := string(replayed.result.Response.JSONBody)
		testutil.Require(t, !(got != "idempotent-result"), "replayed result = %q, want idempotent-result", got)
	}
	{

		err := coordinator.publish(context.Background(), lease, shared)
		testutil.Require(t, !(err != nil), "replayed publish after consume: %v", err)
	}

}

func TestResponseElectionFailedPublishAbortsFollowers(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	request := responseElectionTestRequest("publish-abort")
	owner, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-owner"})
	testutil.Require(t, !(err != nil), "join owner: %v", err)

	follower, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-follower"})
	testutil.Require(t, !(err != nil), "join follower: %v", err)

	publishErr := errors.New("publish failed")
	{
		err := coordinator.reconcileFailedPublish(owner, publishErr)
		{
			testutil.Require(t, errors.Is(err, publishErr), "reconcile error = %v, want confirmed abort with original publish error", err)
			testutil.Require(t, errors.Is(err, errResponseElectionAborted), "reconcile error = %v, want confirmed abort with original publish error", err)
		}
	}
	{

		closed, err := client.HGet(context.Background(), owner.stateKey, "closed").Result()
		{
			testutil.Require(t, !(err != nil), "closed state = %q (%v), want 1", closed, err)
			testutil.Require(t, !(closed != "1"), "closed state = %q (%v), want 1", closed, err)
		}
	}
	{

		exists, err := client.Exists(context.Background(), owner.legacyLockKey).Result()
		{
			testutil.Require(t, !(err != nil), "legacy lock exists after abort = %d (%v), want 0", exists, err)
			testutil.Require(t, !(exists != 0), "legacy lock exists after abort = %d (%v), want 0", exists, err)
		}
	}

	decision, waiting, err := coordinator.decide(context.Background(), follower)
	{
		testutil.Require(t, !(err != nil), "follower after abort = %+v, waiting=%v, err=%v", decision, waiting, err)
		testutil.Require(t, !(waiting), "follower after abort = %+v, waiting=%v, err=%v", decision, waiting, err)
		testutil.Require(t, !(decision.visible), "follower after abort = %+v, waiting=%v, err=%v", decision, waiting, err)
		testutil.Require(t, !(decision.reason != "not_selected"), "follower after abort = %+v, waiting=%v, err=%v", decision, waiting, err)
	}

}

func TestResponseElectionOnlyFailsOpenAfterConfirmedAbort(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})
	result := responseElectionTestResult("local-result", "bot-owner", false)

	aborted := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{
		result: result,
		err:    errors.Join(errResponseElectionAborted, errors.New("publish failed")),
	})
	{
		testutil.Require(t, aborted.visible, "confirmed abort decision = %+v, want visible fail-open", aborted)
		testutil.Require(t, !(aborted.reason != "publish_fail_open"), "confirmed abort decision = %+v, want visible fail-open", aborted)
	}

	unknown := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{
		result: result,
		err:    errors.Join(errResponseElectionUnknown, errors.New("Redis unavailable")),
	})
	{
		testutil.Require(t, !(unknown.visible), "unknown publication decision = %+v, want invisible unknown", unknown)
		testutil.Require(t, !(unknown.reason != "publish_unknown"), "unknown publication decision = %+v, want invisible unknown", unknown)
	}

}

func TestResponseElectionTTLAlwaysOutlivesConfiguredWindow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	window := 2 * responseElectionResultTTL
	coordinator := NewResponseElectionCoordinator(context.Background(), client, window)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	request := responseElectionTestRequest("long-window")
	lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-owner"})
	testutil.Require(t, !(err != nil), "join: %v", err)

	assertTTLExceedsResponseElectionWindow(t, client, lease.stateKey, window)
	assertTTLExceedsResponseElectionWindow(t, client, lease.legacyLockKey, window)
	{
		err := coordinator.publish(
			context.Background(),
			lease,
			responseElectionTestResult("long-window-result", lease.botID, false),
		)
		testutil.Require(t, !(err != nil), "publish: %v", err)
	}

	assertTTLExceedsResponseElectionWindow(t, client, lease.stateKey, window)
	assertTTLExceedsResponseElectionWindow(t, client, lease.legacyLockKey, window)
}

func assertTTLExceedsResponseElectionWindow(t *testing.T, client *redis.Client, key string, window time.Duration) {
	t.Helper()
	ttl, err := client.PTTL(context.Background(), key).Result()
	testutil.Require(t, !(err != nil), "PTTL %q: %v", key, err)
	testutil.Require(t, !(ttl <= window), "PTTL %q = %s, must exceed window %s", key, ttl, window)

}

func TestResponseElectionReelectsWhenSelectedCandidateDisappears(t *testing.T) {
	server := miniredis.RunT(t)
	baseTime := time.Date(2026, time.July, 15, 16, 0, 0, 0, time.UTC)
	server.SetTime(baseTime)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	request := responseElectionTestRequest("stale-winner")
	executorLease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-executor"})
	testutil.Require(t, !(err != nil), "join executor: %v", err)

	seed, err := client.HGet(context.Background(), executorLease.stateKey, "seed").Result()
	testutil.Require(t, !(err != nil), "read election seed: %v", err)

	ghostBotID := responseElectionFollowerWithLowerScore(t, seed, executorLease.botID)
	ghostLease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: ghostBotID})
	testutil.Require(t, !(err != nil), "join disappearing candidate: %v", err)
	testutil.Require(t, !(ghostLease.role != responseElectionFollower), "disappearing candidate role = %d, want follower", ghostLease.role)
	{

		err := coordinator.publish(
			context.Background(),
			executorLease,
			responseElectionTestResult("reelected-result", executorLease.botID, false),
		)
		testutil.Require(t, !(err != nil), "publish: %v", err)
	}

	server.SetTime(baseTime.Add(coordinator.window + time.Millisecond))
	decision, waiting, err := coordinator.decide(context.Background(), executorLease)
	{
		testutil.Require(t, !(err != nil), "decision while ghost owns claim = %+v, waiting=%v, err=%v", decision, waiting, err)
		testutil.Require(t, waiting, "decision while ghost owns claim = %+v, waiting=%v, err=%v", decision, waiting, err)
		testutil.Require(t, !(decision.visible), "decision while ghost owns claim = %+v, waiting=%v, err=%v", decision, waiting, err)
	}
	{

		got, err := client.HGet(context.Background(), executorLease.stateKey, "winner_bot").Result()
		{
			testutil.Require(t, !(err != nil), "initial winner = %q (%v), want ghost %q", got, err, ghostBotID)
			testutil.Require(t, !(got != ghostBotID), "initial winner = %q (%v), want ghost %q", got, err, ghostBotID)
		}
	}

	server.SetTime(baseTime.Add(coordinator.window + responseElectionWinnerClaim + 2*time.Millisecond))
	decision, waiting, err = coordinator.decide(context.Background(), executorLease)
	{
		testutil.Require(t, !(err != nil), "decision after stale claim = %+v, waiting=%v, err=%v", decision, waiting, err)
		testutil.Require(t, !(waiting), "decision after stale claim = %+v, waiting=%v, err=%v", decision, waiting, err)
		testutil.Require(t, decision.visible, "decision after stale claim = %+v, waiting=%v, err=%v", decision, waiting, err)
	}
	{

		got := string(decision.result.Response.JSONBody)
		testutil.Require(t, !(got != "reelected-result"), "reelected result = %q, want reelected-result", got)
	}

}

type responseElectionTestRig struct {
	server       *miniredis.Miniredis
	clients      []*redis.Client
	coordinators []*ResponseElectionCoordinator
	baseTime     time.Time
	window       time.Duration
	windowReady  chan time.Time
	windowOnce   sync.Once
}

type responseElectionTestOutcome struct {
	botID    string
	decision responseElectionDecision
	trace    commandtrace.Snapshot
}

func newResponseElectionTestRig(t *testing.T, coordinatorCount int, window time.Duration) *responseElectionTestRig {
	t.Helper()
	server := miniredis.RunT(t)
	baseTime := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	server.SetTime(baseTime)
	rig := &responseElectionTestRig{
		server:      server,
		baseTime:    baseTime,
		window:      window,
		windowReady: make(chan time.Time),
	}
	for range coordinatorCount {
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		coordinator := NewResponseElectionCoordinator(context.Background(), client, window)
		coordinator.now = func() time.Time { return baseTime }
		coordinator.after = func(time.Duration) <-chan time.Time { return rig.windowReady }
		coordinator.pollInterval = time.Millisecond
		rig.clients = append(rig.clients, client)
		rig.coordinators = append(rig.coordinators, coordinator)
	}
	t.Cleanup(func() {
		rig.windowOnce.Do(func() { close(rig.windowReady) })
		for _, coordinator := range rig.coordinators {
			coordinator.Close()
		}
		for _, client := range rig.clients {
			_ = client.Close()
		}
	})
	return rig
}

func (r *responseElectionTestRig) start(
	results chan<- responseElectionTestOutcome,
	coordinatorIndex int,
	request BotCommandRequest,
	botID string,
	execute func(context.Context) sharedCommandResult,
) {
	go func() {
		ctx, trace := commandtrace.WithTrace(context.Background())
		decision := r.coordinators[coordinatorIndex].Coordinate(
			ctx,
			responseElectionRequest{Request: request, BotID: botID},
			execute,
		)
		results <- responseElectionTestOutcome{botID: botID, decision: decision, trace: trace.Snapshot()}
	}()
}

func assertResponseElectionTraceOperation(t *testing.T, snapshot commandtrace.Snapshot, name string) {
	t.Helper()
	for _, operation := range snapshot.Operations {
		if operation.Name == name && operation.Count > 0 {
			return
		}
	}
	t.Fatalf("trace operation %q missing from %+v", name, snapshot.Operations)
}

func hasResponseElectionSharedOperation(operations []sharedCommandOperation, name string) bool {
	for _, operation := range operations {
		if operation.Name == name && operation.Count > 0 {
			return true
		}
	}
	return false
}

func (r *responseElectionTestRig) advancePastWindow() {
	r.server.SetTime(r.baseTime.Add(r.window + time.Millisecond))
	r.windowOnce.Do(func() { close(r.windowReady) })
}

func (r *responseElectionTestRig) waitForCandidateCount(t *testing.T, request BotCommandRequest, want int64) {
	t.Helper()
	key := responseElectionCandidatesKey(responseElectionIdentity(request))
	waitForResponseElectionCondition(t, fmt.Sprintf("%d response candidates", want), func() bool {
		got, err := r.clients[0].HLen(context.Background(), key).Result()
		return err == nil && got == want
	})
}

func (r *responseElectionTestRig) waitForStateField(t *testing.T, request BotCommandRequest, field, want string) {
	t.Helper()
	waitForResponseElectionCondition(t, fmt.Sprintf("state field %s=%s", field, want), func() bool {
		return r.stateFieldValue(request, field) == want
	})
}

func (r *responseElectionTestRig) stateField(t *testing.T, request BotCommandRequest, field string) string {
	t.Helper()
	var value string
	waitForResponseElectionCondition(t, "state field "+field, func() bool {
		value = r.stateFieldValue(request, field)
		return value != ""
	})
	return value
}

func (r *responseElectionTestRig) stateFieldValue(request BotCommandRequest, field string) string {
	key := responseElectionStateKey(responseElectionIdentity(request))
	value, _ := r.clients[0].HGet(context.Background(), key, field).Result()
	return value
}

func responseElectionTestRequest(eventID string) BotCommandRequest {
	request := responseElectionIdentityTestRequest()
	request.Message = onebot11.Message{onebot11.Text("/test " + eventID)}
	return request
}

func responseElectionTestResult(marker, executorBotID string, forceExecutor bool) sharedCommandResult {
	return sharedCommandResult{
		Response: encodedBotResponse{
			HTTPStatus:  200,
			JSONBody:    []byte(marker),
			MsgPackBody: []byte(marker),
		},
		Metadata: sharedCommandMetadata{
			Outcome:       "success",
			ExecutorBotID: executorBotID,
		},
		ForceExecutor: forceExecutor,
	}
}

func collectResponseElectionOutcomes(
	t *testing.T,
	results <-chan responseElectionTestOutcome,
	want int,
) []responseElectionTestOutcome {
	t.Helper()
	outcomes := make([]responseElectionTestOutcome, 0, want)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for len(outcomes) < want {
		select {
		case outcome := <-results:
			outcomes = append(outcomes, outcome)
		case <-timer.C:
			t.Fatalf("received %d/%d response-election decisions before timeout", len(outcomes), want)
		}
	}
	return outcomes
}

func assertNoResponseElectionDecision(
	t *testing.T,
	results <-chan responseElectionTestOutcome,
	message string,
) {
	t.Helper()
	select {
	case outcome := <-results:
		t.Fatalf("%s: bot=%s decision=%+v", message, outcome.botID, outcome.decision)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertSingleVisibleResponseElectionOutcome(
	t *testing.T,
	outcomes []responseElectionTestOutcome,
) responseElectionTestOutcome {
	t.Helper()
	var visible []responseElectionTestOutcome
	for _, outcome := range outcomes {
		if outcome.decision.visible {
			visible = append(visible, outcome)
			continue
		}
		testutil.Check(t, !(outcome.decision.reason != "not_selected"), "losing bot %s reason = %q, want not_selected", outcome.botID, outcome.decision.reason)

	}
	testutil.Require(t, !(len(visible) != 1), "visible decisions = %d, want 1; outcomes=%+v", len(visible), outcomes)
	testutil.Require(t, !(visible[0].decision.reason != "selected"), "winner reason = %q, want selected", visible[0].decision.reason)

	return visible[0]
}

func waitForResponseElectionSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForResponseElectionCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func responseElectionFollowerWithLowerScore(t *testing.T, seed, executorBotID string) string {
	t.Helper()
	executorScore := responseElectionTestScore(seed, executorBotID)
	for index := 0; index < 10_000; index++ {
		candidate := fmt.Sprintf("bot-follower-%d", index)
		if strings.Compare(responseElectionTestScore(seed, candidate), executorScore) < 0 {
			return candidate
		}
	}
	t.Fatal("could not find a follower with a lower rendezvous score than the executor")
	return ""
}

func responseElectionTestScore(seed, botID string) string {
	sum := sha1.Sum([]byte(seed + "|" + botID))
	return hex.EncodeToString(sum[:])
}
