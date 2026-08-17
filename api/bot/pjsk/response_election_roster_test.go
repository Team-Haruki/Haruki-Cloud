package pjsk

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRosterForTest(t *testing.T) (*responseElectionRoster, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	roster := newResponseElectionRoster(client)
	return roster, client, server
}

// waitForRosterField polls Redis until the roster hash field appears; the
// roster writes asynchronously off the join critical path.
func waitForRosterField(t *testing.T, client *redis.Client, key, field string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ok, err := client.HExists(context.Background(), key, field).Result()
		if err == nil && ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("roster field %s never appeared in %s", field, key)
}

func TestRosterDMAlwaysExpectsSingleCandidate(t *testing.T) {
	roster, _, _ := newRosterForTest(t)
	if got := roster.expectedCandidates(context.Background(), "qq", "", "bot-a"); got != 1 {
		t.Fatalf("DM expected candidates = %d, want 1", got)
	}
	if got := roster.expectedCandidates(context.Background(), "qq", "   ", "bot-a"); got != 1 {
		t.Fatalf("blank group expected candidates = %d, want 1", got)
	}
}

func TestRosterUnknownGroupReturnsZero(t *testing.T) {
	roster, _, _ := newRosterForTest(t)
	if got := roster.expectedCandidates(context.Background(), "qq", "group-x", "bot-a"); got != 0 {
		t.Fatalf("cold roster expected candidates = %d, want 0 (unknown)", got)
	}
}

func TestRosterNilReceiverIsUnknown(t *testing.T) {
	var roster *responseElectionRoster
	if got := roster.expectedCandidates(context.Background(), "qq", "group-x", "bot-a"); got != 0 {
		t.Fatalf("nil roster expected candidates = %d, want 0", got)
	}
	roster.recordJoin(context.Background(), "qq", "group-x", "bot-a") // must not panic
	roster.reconcile(context.Background(), "qq", "group-x", nil, "bot-a")
}

func TestRosterCountsCreatorAndRecordedMembers(t *testing.T) {
	roster, client, _ := newRosterForTest(t)
	key := rosterKey("qq", "group-1")

	roster.recordJoin(context.Background(), "qq", "group-1", "bot-a")
	waitForRosterField(t, client, key, "bot-a")
	roster.refresh(context.Background(), key)

	// Creator already in the roster: count stays at the member count.
	if got := roster.expectedCandidates(context.Background(), "qq", "group-1", "bot-a"); got != 1 {
		t.Fatalf("single-member roster expected = %d, want 1", got)
	}
	// A creator the roster has not seen yet counts itself on top.
	if got := roster.expectedCandidates(context.Background(), "qq", "group-1", "bot-b"); got != 2 {
		t.Fatalf("unseen creator expected = %d, want 2", got)
	}
}

func TestRosterRecordJoinClearsMissCounter(t *testing.T) {
	roster, client, _ := newRosterForTest(t)
	key := rosterKey("qq", "group-1")
	ctx := context.Background()
	if err := client.HSet(ctx, key, rosterMissFieldPrefix+"bot-a", "2").Err(); err != nil {
		t.Fatalf("seed miss counter: %v", err)
	}

	roster.recordJoin(ctx, "qq", "group-1", "bot-a")
	waitForRosterField(t, client, key, "bot-a")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if exists, _ := client.HExists(ctx, key, rosterMissFieldPrefix+"bot-a").Result(); !exists {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("miss counter should be cleared by a recorded join")
}

func TestRosterReconcileDemotesAfterConsecutiveMisses(t *testing.T) {
	roster, client, _ := newRosterForTest(t)
	ctx := context.Background()
	key := rosterKey("qq", "group-1")
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if err := client.HSet(ctx, key, "bot-a", now, "bot-b", now).Err(); err != nil {
		t.Fatalf("seed roster: %v", err)
	}

	waitForMiss := func(want string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			got, err := client.HGet(ctx, key, rosterMissFieldPrefix+"bot-b").Result()
			if err == nil && got == want {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("miss counter for bot-b never reached %s", want)
	}

	// bot-b absent from two elections: miss counter accrues.
	roster.reconcile(ctx, "qq", "group-1", []string{"bot-a"}, "bot-a")
	waitForMiss("1")
	roster.reconcile(ctx, "qq", "group-1", nil, "bot-a") // self still counts as joined
	waitForMiss("2")

	// Third absence crosses the threshold: bot-b is demoted entirely.
	roster.reconcile(ctx, "qq", "group-1", []string{"bot-a"}, "bot-a")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		memberExists, _ := client.HExists(ctx, key, "bot-b").Result()
		missExists, _ := client.HExists(ctx, key, rosterMissFieldPrefix+"bot-b").Result()
		if !memberExists && !missExists {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if exists, _ := client.HExists(ctx, key, "bot-b").Result(); exists {
		t.Fatal("bot-b should be demoted after three consecutive misses")
	}

	// The reconcile refreshed the local cache: only bot-a remains expected.
	if got := roster.expectedCandidates(ctx, "qq", "group-1", "bot-a"); got != 1 {
		t.Fatalf("post-demotion expected = %d, want 1", got)
	}
}

func TestRosterReconcilePresentMemberClearsMissCounter(t *testing.T) {
	roster, client, _ := newRosterForTest(t)
	ctx := context.Background()
	key := rosterKey("qq", "group-1")
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	// bot-b carries prior misses but joined this election: the winner's
	// authoritative snapshot must clear the counter, otherwise non-adjacent
	// misses would accumulate and demote a live, participating bot.
	if err := client.HSet(ctx, key, "bot-a", now, "bot-b", now, rosterMissFieldPrefix+"bot-b", "2").Err(); err != nil {
		t.Fatalf("seed roster: %v", err)
	}

	roster.reconcile(ctx, "qq", "group-1", []string{"bot-a", "bot-b"}, "bot-a")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if exists, _ := client.HExists(ctx, key, rosterMissFieldPrefix+"bot-b").Result(); !exists {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if exists, _ := client.HExists(ctx, key, rosterMissFieldPrefix+"bot-b").Result(); exists {
		t.Fatal("present member's miss counter must be cleared by reconcile")
	}
	if exists, _ := client.HExists(ctx, key, "bot-b").Result(); !exists {
		t.Fatal("present member must stay in the roster")
	}
}

func TestRosterRecordJoinBypassesDedupForUnknownMember(t *testing.T) {
	roster, client, _ := newRosterForTest(t)
	ctx := context.Background()
	key := rosterKey("qq", "group-1")

	// First join registers bot-a and warms the write dedup.
	roster.recordJoin(ctx, "qq", "group-1", "bot-a")
	waitForRosterField(t, client, key, "bot-a")
	roster.refresh(ctx, key)

	// Simulate a demotion on another node: bot-a vanishes from Redis and from
	// this node's refreshed view.
	if err := client.HDel(ctx, key, "bot-a").Err(); err != nil {
		t.Fatalf("simulate demotion: %v", err)
	}
	roster.refresh(ctx, key)

	// Re-registration must NOT wait out the dedup window: the local view no
	// longer lists bot-a, so the write goes through immediately.
	roster.recordJoin(ctx, "qq", "group-1", "bot-a")
	waitForRosterField(t, client, key, "bot-a")
}

func TestJoinWindowSkippedOnlyForZeroWindowCreator(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second).
		WithRoster(newResponseElectionRoster(client))
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	request := responseElectionTestRequest("follower-window-skip")
	identity := responseElectionIdentity(request)

	// bot-a creates a FULL-window election (expected=2 via raw script call).
	if _, err := joinResponseElectionScript.Run(
		context.Background(),
		client,
		[]string{
			responseElectionStateKey(identity),
			responseElectionCandidatesKey(identity),
			rateLimitKey(request),
			dedupKey(request),
			responseElectionHistoryKey(identity),
		},
		"token-a", "bot-a", time.Second.Milliseconds(), dedupInFlightTTL.Milliseconds(), 2,
		responseElectionReplayWindow.Milliseconds(),
		(responseElectionReplayWindow + responseElectionRedisTimeout).Milliseconds(),
	).Slice(); err != nil {
		t.Fatalf("create election: %v", err)
	}

	// bot-b's stale local roster claims it is alone (expected=1), but it joins
	// an existing full-window election as follower: windowSkipped must stay
	// false or it would suppress reconciliation and corrupt the metric.
	rosterHash := rosterKey(request.Platform, request.PlatformGroupID)
	if err := client.HSet(context.Background(), rosterHash, "bot-b", strconv.FormatInt(time.Now().UnixMilli(), 10)).Err(); err != nil {
		t.Fatalf("seed stale roster: %v", err)
	}
	coordinator.roster.refresh(context.Background(), rosterHash)

	lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-b"})
	if err != nil {
		t.Fatalf("follower join: %v", err)
	}
	if lease.role != responseElectionFollower {
		t.Fatalf("lease role = %d, want follower", lease.role)
	}
	if lease.expectedCandidates != 1 {
		t.Fatalf("expectedCandidates = %d, want stale 1", lease.expectedCandidates)
	}
	if lease.windowSkipped {
		t.Fatal("follower joining a full-window election must not report windowSkipped")
	}
}

func TestJoinScriptZeroesWindowForSingleExpectedCandidate(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	runJoin := func(eventID string, expected int) (int64, int64) {
		t.Helper()
		request := responseElectionTestRequest(eventID)
		identity := responseElectionIdentity(request)
		result, err := joinResponseElectionScript.Run(
			context.Background(),
			client,
			[]string{
				responseElectionStateKey(identity),
				responseElectionCandidatesKey(identity),
				rateLimitKey(request),
				dedupKey(request),
				responseElectionHistoryKey(identity),
			},
			"token-"+eventID,
			"bot-a",
			time.Second.Milliseconds(),
			dedupInFlightTTL.Milliseconds(),
			expected,
			responseElectionReplayWindow.Milliseconds(),
			(responseElectionReplayWindow + responseElectionRedisTimeout).Milliseconds(),
		).Slice()
		if err != nil {
			t.Fatalf("join script: %v", err)
		}
		status, err := responseElectionInt64(result[0])
		if err != nil {
			t.Fatalf("join status: %v", err)
		}
		remaining, err := responseElectionInt64(result[1])
		if err != nil {
			t.Fatalf("join remaining: %v", err)
		}
		return status, remaining
	}

	if status, remaining := runJoin("single", 1); status != int64(responseElectionExecutor) || remaining != 0 {
		t.Fatalf("expected=1 join = (%d, %d), want executor with zero window", status, remaining)
	}
	if status, remaining := runJoin("unknown", 0); status != int64(responseElectionExecutor) || remaining != 1000 {
		t.Fatalf("expected=0 join = (%d, %d), want executor with full window", status, remaining)
	}
	if status, remaining := runJoin("pair", 2); status != int64(responseElectionExecutor) || remaining != 1000 {
		t.Fatalf("expected=2 join = (%d, %d), want executor with full window", status, remaining)
	}
}

func TestCoordinateSkipsWindowForSingleBotGroup(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, 30*time.Second).
		WithRoster(newResponseElectionRoster(client))
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	request := responseElectionTestRequest("roster-fast-path")
	rosterHash := rosterKey(request.Platform, request.PlatformGroupID)
	if err := client.HSet(context.Background(), rosterHash, "bot-a", strconv.FormatInt(time.Now().UnixMilli(), 10)).Err(); err != nil {
		t.Fatalf("seed roster: %v", err)
	}
	coordinator.roster.refresh(context.Background(), rosterHash)

	// The 30s election window would dwarf the test timeout; with the roster
	// fast path the single expected candidate must respond immediately.
	done := make(chan responseElectionDecision, 1)
	go func() {
		done <- coordinator.Coordinate(
			context.Background(),
			responseElectionRequest{Request: request, BotID: "bot-a"},
			func(context.Context) sharedCommandResult {
				return responseElectionTestResult("fast", "bot-a", false)
			},
		)
	}()
	select {
	case decision := <-done:
		if !decision.visible || decision.reason != "selected" {
			t.Fatalf("decision = %+v, want selected/visible", decision)
		}
		if !decision.windowSkipped || decision.expectedCandidates != 1 {
			t.Fatalf("windowSkipped/expected = %v/%d, want true/1", decision.windowSkipped, decision.expectedCandidates)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("single-bot fast path should complete well before the 30s window")
	}
}

func TestCoordinateKeepsFullWindowForMultiBotGroup(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, 150*time.Millisecond).
		WithRoster(newResponseElectionRoster(client))
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	request := responseElectionTestRequest("roster-two-bots")
	rosterHash := rosterKey(request.Platform, request.PlatformGroupID)
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if err := client.HSet(context.Background(), rosterHash, "bot-a", now, "bot-b", now).Err(); err != nil {
		t.Fatalf("seed roster: %v", err)
	}
	coordinator.roster.refresh(context.Background(), rosterHash)

	decision := coordinator.Coordinate(
		context.Background(),
		responseElectionRequest{Request: request, BotID: "bot-a"},
		func(context.Context) sharedCommandResult {
			return responseElectionTestResult("full-window", "bot-a", false)
		},
	)
	if !decision.visible || decision.reason != "selected" {
		t.Fatalf("decision = %+v, want selected/visible", decision)
	}
	if decision.windowSkipped {
		t.Fatal("multi-bot group must keep the full election window")
	}
	if decision.expectedCandidates != 2 {
		t.Fatalf("expectedCandidates = %d, want 2", decision.expectedCandidates)
	}

	// The full-window election reconciles the roster: bot-b was absent once.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := client.HGet(context.Background(), rosterHash, rosterMissFieldPrefix+"bot-b").Result(); err == nil && got == "1" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("absent roster member should accrue a miss after a full-window election")
}
