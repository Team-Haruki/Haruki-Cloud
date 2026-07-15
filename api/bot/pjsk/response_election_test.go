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

	if got := executions.Load(); got != 1 {
		t.Fatalf("command executions before election = %d, want 1", got)
	}
	assertNoResponseElectionDecision(t, results, "fast command returned before the election window closed")

	rig.advancePastWindow()
	outcomes := collectResponseElectionOutcomes(t, results, 3)
	winner := assertSingleVisibleResponseElectionOutcome(t, outcomes)
	if got := string(winner.decision.result.Response.JSONBody); got != "fast-result" {
		t.Fatalf("winner result = %q, want %q", got, "fast-result")
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("command executions after election = %d, want 1", got)
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
	if !hasResponseElectionSharedOperation(winner.decision.result.Operations, "response_election.result_serialize") {
		t.Fatalf("winner shared operations missing result serialization: %+v", winner.decision.result.Operations)
	}
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
	if elapsed := time.Since(releasedAt); elapsed >= 750*time.Millisecond {
		t.Fatalf("command took %s to return after becoming ready; window had already closed", elapsed)
	}
	winner := assertSingleVisibleResponseElectionOutcome(t, outcomes)
	if got := string(winner.decision.result.Response.JSONBody); got != "slow-result" {
		t.Fatalf("winner result = %q, want %q", got, "slow-result")
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("command executions = %d, want 1", got)
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
	if late.decision.visible || late.decision.reason != "rejected" {
		t.Fatalf("post-window decision = %+v, want rejected and invisible", late.decision)
	}
	if got := lateExecutions.Load(); got != 0 {
		t.Fatalf("post-window duplicate executions = %d, want 0", got)
	}

	releaseOnce.Do(func() { close(release) })
	initial := collectResponseElectionOutcomes(t, initialResults, 2)
	assertSingleVisibleResponseElectionOutcome(t, initial)

	postDelivery := rig.coordinators[0].Coordinate(
		context.Background(),
		responseElectionRequest{Request: request, BotID: "bot-after-delivery"},
		lateExecute,
	)
	if postDelivery.visible || postDelivery.reason != "rejected" {
		t.Fatalf("post-delivery decision = %+v, want rejected and invisible", postDelivery)
	}
	if got := lateExecutions.Load(); got != 0 {
		t.Fatalf("late duplicate executions = %d, want 0", got)
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("original command executions = %d, want 1", got)
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
	if winner.botID != executorBotID {
		t.Fatalf("selected bot = %q, want executor %q", winner.botID, executorBotID)
	}
	if got := winner.decision.result.Metadata.ExecutorBotID; got != executorBotID {
		t.Fatalf("result executor bot = %q, want %q", got, executorBotID)
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("command executions = %d, want 1", got)
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
		if err != nil {
			t.Fatalf("join script: %v", err)
		}
		return result
	}

	first := runJoin()
	if status, err := responseElectionInt64(first[0]); err != nil || status != int64(responseElectionExecutor) {
		t.Fatalf("first join status = %v (%v), want executor", first[0], err)
	}

	// A command timeout can make go-redis repeat a script after Redis already
	// committed it. The original token must recover its role even if W elapsed.
	server.SetTime(baseTime.Add(window + time.Millisecond))
	replayed := runJoin()
	if status, err := responseElectionInt64(replayed[0]); err != nil || status != int64(responseElectionExecutor) {
		t.Fatalf("replayed join status = %v (%v), want executor", replayed[0], err)
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
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	second, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-a"})
	if err != nil {
		t.Fatalf("duplicate join: %v", err)
	}
	if first.role != responseElectionExecutor || second.role != responseElectionRejected {
		t.Fatalf("first/second roles = %d/%d, want executor/rejected", first.role, second.role)
	}
	count, err := client.HLen(context.Background(), first.candidatesKey).Result()
	if err != nil || count != 1 {
		t.Fatalf("candidate count = %d (%v), want 1", count, err)
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
		if err := client.Set(context.Background(), dedupKey(request), "legacy-token", dedupInFlightTTL).Err(); err != nil {
			t.Fatalf("seed legacy lock: %v", err)
		}
		lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-new"})
		if err != nil {
			t.Fatalf("join: %v", err)
		}
		if lease.role != responseElectionRejected {
			t.Fatalf("election role = %d, want rejected", lease.role)
		}
		if exists, err := client.Exists(context.Background(), responseElectionStateKey(responseElectionIdentity(request))).Result(); err != nil || exists != 0 {
			t.Fatalf("election state exists = %d (%v), want 0", exists, err)
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
		if err != nil {
			t.Fatalf("join: %v", err)
		}
		if lease.role != responseElectionExecutor {
			t.Fatalf("election role = %d, want executor", lease.role)
		}
		if owner, err := client.Get(context.Background(), dedupKey(request)).Result(); err != nil || owner != lease.token {
			t.Fatalf("legacy compatibility owner = %q (%v), want %q", owner, err, lease.token)
		}
		if legacyLease := legacyGuard.Acquire(context.Background(), request); legacyLease.proceed {
			t.Fatal("legacy guard acquired an event owned by response election")
		}
		if err := coordinator.publish(
			context.Background(),
			lease,
			responseElectionTestResult("compat-result", lease.botID, false),
		); err != nil {
			t.Fatalf("publish: %v", err)
		}
		server.SetTime(baseTime.Add(coordinator.window + time.Millisecond))
		decision, waiting, err := coordinator.decide(context.Background(), lease)
		if err != nil || waiting || !decision.visible {
			t.Fatalf("consume = %+v, waiting=%v, err=%v", decision, waiting, err)
		}
		if exists, err := client.Exists(context.Background(), dedupKey(request)).Result(); err != nil || exists != 0 {
			t.Fatalf("legacy lock exists after consume = %d (%v), want 0", exists, err)
		}
		if exists, err := client.Exists(context.Background(), rateLimitKey(request)).Result(); err != nil || exists != 1 {
			t.Fatalf("shared rate key exists = %d (%v), want 1", exists, err)
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
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if lease.role != responseElectionExecutor {
		t.Fatalf("join role = %d, want executor", lease.role)
	}
	shared := responseElectionTestResult("idempotent-result", "bot-owner", false)
	if err := coordinator.publish(context.Background(), lease, shared); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := coordinator.reconcileFailedPublish(lease, errors.New("ambiguous Redis reply")); err != nil {
		t.Fatalf("reconcile committed publication: %v", err)
	}

	server.SetTime(baseTime.Add(coordinator.window + time.Millisecond))
	first, waiting, err := coordinator.decide(context.Background(), lease)
	if err != nil || waiting || !first.visible {
		t.Fatalf("first consume = %+v, waiting=%v, err=%v", first, waiting, err)
	}
	replayed, waiting, err := coordinator.decide(context.Background(), lease)
	if err != nil || waiting || !replayed.visible {
		t.Fatalf("replayed consume = %+v, waiting=%v, err=%v", replayed, waiting, err)
	}
	if got := string(replayed.result.Response.JSONBody); got != "idempotent-result" {
		t.Fatalf("replayed result = %q, want idempotent-result", got)
	}
	if err := coordinator.publish(context.Background(), lease, shared); err != nil {
		t.Fatalf("replayed publish after consume: %v", err)
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
	if err != nil {
		t.Fatalf("join owner: %v", err)
	}
	follower, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-follower"})
	if err != nil {
		t.Fatalf("join follower: %v", err)
	}
	publishErr := errors.New("publish failed")
	if err := coordinator.reconcileFailedPublish(owner, publishErr); !errors.Is(err, publishErr) || !errors.Is(err, errResponseElectionAborted) {
		t.Fatalf("reconcile error = %v, want confirmed abort with original publish error", err)
	}
	if closed, err := client.HGet(context.Background(), owner.stateKey, "closed").Result(); err != nil || closed != "1" {
		t.Fatalf("closed state = %q (%v), want 1", closed, err)
	}
	if exists, err := client.Exists(context.Background(), owner.legacyLockKey).Result(); err != nil || exists != 0 {
		t.Fatalf("legacy lock exists after abort = %d (%v), want 0", exists, err)
	}
	decision, waiting, err := coordinator.decide(context.Background(), follower)
	if err != nil || waiting || decision.visible || decision.reason != "not_selected" {
		t.Fatalf("follower after abort = %+v, waiting=%v, err=%v", decision, waiting, err)
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
	if !aborted.visible || aborted.reason != "publish_fail_open" {
		t.Fatalf("confirmed abort decision = %+v, want visible fail-open", aborted)
	}
	unknown := coordinator.executorPublishFailureDecision(responseElectionExecutorResult{
		result: result,
		err:    errors.Join(errResponseElectionUnknown, errors.New("Redis unavailable")),
	})
	if unknown.visible || unknown.reason != "publish_unknown" {
		t.Fatalf("unknown publication decision = %+v, want invisible unknown", unknown)
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
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	assertTTLExceedsResponseElectionWindow(t, client, lease.stateKey, window)
	assertTTLExceedsResponseElectionWindow(t, client, lease.legacyLockKey, window)
	if err := coordinator.publish(
		context.Background(),
		lease,
		responseElectionTestResult("long-window-result", lease.botID, false),
	); err != nil {
		t.Fatalf("publish: %v", err)
	}
	assertTTLExceedsResponseElectionWindow(t, client, lease.stateKey, window)
	assertTTLExceedsResponseElectionWindow(t, client, lease.legacyLockKey, window)
}

func assertTTLExceedsResponseElectionWindow(t *testing.T, client *redis.Client, key string, window time.Duration) {
	t.Helper()
	ttl, err := client.PTTL(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("PTTL %q: %v", key, err)
	}
	if ttl <= window {
		t.Fatalf("PTTL %q = %s, must exceed window %s", key, ttl, window)
	}
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
	if err != nil {
		t.Fatalf("join executor: %v", err)
	}
	seed, err := client.HGet(context.Background(), executorLease.stateKey, "seed").Result()
	if err != nil {
		t.Fatalf("read election seed: %v", err)
	}
	ghostBotID := responseElectionFollowerWithLowerScore(t, seed, executorLease.botID)
	ghostLease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: ghostBotID})
	if err != nil {
		t.Fatalf("join disappearing candidate: %v", err)
	}
	if ghostLease.role != responseElectionFollower {
		t.Fatalf("disappearing candidate role = %d, want follower", ghostLease.role)
	}
	if err := coordinator.publish(
		context.Background(),
		executorLease,
		responseElectionTestResult("reelected-result", executorLease.botID, false),
	); err != nil {
		t.Fatalf("publish: %v", err)
	}

	server.SetTime(baseTime.Add(coordinator.window + time.Millisecond))
	decision, waiting, err := coordinator.decide(context.Background(), executorLease)
	if err != nil || !waiting || decision.visible {
		t.Fatalf("decision while ghost owns claim = %+v, waiting=%v, err=%v", decision, waiting, err)
	}
	if got, err := client.HGet(context.Background(), executorLease.stateKey, "winner_bot").Result(); err != nil || got != ghostBotID {
		t.Fatalf("initial winner = %q (%v), want ghost %q", got, err, ghostBotID)
	}

	server.SetTime(baseTime.Add(coordinator.window + responseElectionWinnerClaim + 2*time.Millisecond))
	decision, waiting, err = coordinator.decide(context.Background(), executorLease)
	if err != nil || waiting || !decision.visible {
		t.Fatalf("decision after stale claim = %+v, waiting=%v, err=%v", decision, waiting, err)
	}
	if got := string(decision.result.Response.JSONBody); got != "reelected-result" {
		t.Fatalf("reelected result = %q, want reelected-result", got)
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
		if outcome.decision.reason != "not_selected" {
			t.Errorf("losing bot %s reason = %q, want not_selected", outcome.botID, outcome.decision.reason)
		}
	}
	if len(visible) != 1 {
		t.Fatalf("visible decisions = %d, want 1; outcomes=%+v", len(visible), outcomes)
	}
	if visible[0].decision.reason != "selected" {
		t.Fatalf("winner reason = %q, want selected", visible[0].decision.reason)
	}
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
