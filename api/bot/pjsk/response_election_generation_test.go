package pjsk

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// consumeGenerationElection drives one election through publish and decide so
// its state reaches the consumed/closed form the generation logic inspects.
func consumeGenerationElection(
	t *testing.T,
	server *miniredis.Miniredis,
	base time.Time,
	c *ResponseElectionCoordinator,
	lease responseElectionLease,
) {
	t.Helper()
	if err := c.publish(context.Background(), lease, responseElectionTestResult("gen-result", lease.botID, false)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	server.SetTime(base.Add(50 * time.Millisecond))
	decision, waiting, err := c.decide(context.Background(), lease)
	if err != nil || waiting {
		t.Fatalf("decide: waiting=%v err=%v", waiting, err)
	}
	if decision.reason != "selected" {
		t.Fatalf("decide reason = %q, want selected", decision.reason)
	}
}

func newGenerationTestRig(t *testing.T) (*miniredis.Miniredis, *redis.Client, *ResponseElectionCoordinator, time.Time) {
	t.Helper()
	server := miniredis.RunT(t)
	base := time.Now().Truncate(time.Second)
	server.SetTime(base)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Millisecond)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})
	return server, client, coordinator, base
}

func TestResponseElectionEventTimeExtendsDedupBeyondLegacyWindow(t *testing.T) {
	server, client, coordinator, base := newGenerationTestRig(t)
	request := responseElectionTestRequest("event-time-dedup")
	request.EventTime = time.Now().Unix() - 30

	lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-a"})
	if err != nil || lease.role != responseElectionExecutor {
		t.Fatalf("creator join role = %d (%v), want executor", lease.role, err)
	}
	consumeGenerationElection(t, server, base, coordinator, lease)

	ttl, err := client.PTTL(context.Background(), lease.stateKey).Result()
	if err != nil || ttl < 100*time.Second {
		t.Fatalf("consumed state TTL = %v (%v), want ~%v", ttl, err, responseElectionCompletedTTL)
	}

	// A duplicate delivery of the SAME event arriving well past the legacy 3s
	// grace — the exact failure mode behind cross-bot double responses — must
	// still be rejected.
	server.FastForward(10 * time.Second)
	late, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-b"})
	if err != nil || late.role != responseElectionRejected {
		t.Fatalf("late duplicate role = %d (%v), want rejected", late.role, err)
	}
	server.FastForward(60 * time.Second)
	later, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-b"})
	if err != nil || later.role != responseElectionRejected {
		t.Fatalf("70s duplicate role = %d (%v), want rejected", later.role, err)
	}
}

func TestResponseElectionNewerEventTimeStartsNewElection(t *testing.T) {
	server, client, coordinator, base := newGenerationTestRig(t)
	request := responseElectionTestRequest("event-time-resend")
	request.EventTime = time.Now().Unix() - 30

	lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-a"})
	if err != nil || lease.role != responseElectionExecutor {
		t.Fatalf("creator join role = %d (%v), want executor", lease.role, err)
	}
	consumeGenerationElection(t, server, base, coordinator, lease)

	// A newer event time inside the per-user cooldown is still rate limited,
	// and the rejected attempt must NOT tear down the old dedup state.
	resend := request
	resend.EventTime = request.EventTime + 5
	blocked, err := coordinator.join(context.Background(), responseElectionRequest{Request: resend, BotID: "bot-a"})
	if err != nil || blocked.role != responseElectionRejected {
		t.Fatalf("cooldown re-send role = %d (%v), want rejected", blocked.role, err)
	}
	if exists, err := client.Exists(context.Background(), lease.stateKey).Result(); err != nil || exists != 1 {
		t.Fatalf("state after cooldown rejection exists = %d (%v), want 1", exists, err)
	}

	// Past the cooldown the newer generation replaces the consumed election.
	server.FastForward(4 * time.Second)
	renewed, err := coordinator.join(context.Background(), responseElectionRequest{Request: resend, BotID: "bot-a"})
	if err != nil || renewed.role != responseElectionExecutor {
		t.Fatalf("re-send join role = %d (%v), want executor", renewed.role, err)
	}
	stored, err := client.HGet(context.Background(), renewed.stateKey, "event_time").Result()
	if err != nil || stored != strconv.FormatInt(resend.EventTime, 10) {
		t.Fatalf("new election event_time = %q (%v), want %d", stored, err, resend.EventTime)
	}
}

func TestResponseElectionLegacyRequestsKeepShortRetention(t *testing.T) {
	server, client, coordinator, base := newGenerationTestRig(t)
	request := responseElectionTestRequest("legacy-retention")

	lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-a"})
	if err != nil || lease.role != responseElectionExecutor {
		t.Fatalf("creator join role = %d (%v), want executor", lease.role, err)
	}
	consumeGenerationElection(t, server, base, coordinator, lease)

	ttl, err := client.PTTL(context.Background(), lease.stateKey).Result()
	if err != nil || ttl > 5*time.Second {
		t.Fatalf("legacy consumed state TTL = %v (%v), want <=%v", ttl, err, rateLimitTTL)
	}

	// Within the grace a duplicate is rejected; once the state expires the
	// next delivery starts fresh — exactly the pre-generation behavior.
	quick, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-b"})
	if err != nil || quick.role != responseElectionRejected {
		t.Fatalf("in-grace duplicate role = %d (%v), want rejected", quick.role, err)
	}
	server.FastForward(4 * time.Second)
	fresh, err := coordinator.join(context.Background(), responseElectionRequest{Request: request, BotID: "bot-b"})
	if err != nil || fresh.role != responseElectionExecutor {
		t.Fatalf("post-grace join role = %d (%v), want executor", fresh.role, err)
	}
}

func TestResponseElectionAdoptsFollowerEventTime(t *testing.T) {
	server := miniredis.RunT(t)
	base := time.Now().Truncate(time.Second)
	server.SetTime(base)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	coordinator := NewResponseElectionCoordinator(context.Background(), client, time.Second)
	t.Cleanup(func() {
		coordinator.Close()
		_ = client.Close()
	})

	legacy := responseElectionTestRequest("mixed-adoption")
	lease, err := coordinator.join(context.Background(), responseElectionRequest{Request: legacy, BotID: "bot-old"})
	if err != nil || lease.role != responseElectionExecutor {
		t.Fatalf("creator join role = %d (%v), want executor", lease.role, err)
	}

	updated := legacy
	updated.EventTime = time.Now().Unix() - 5
	follower, err := coordinator.join(context.Background(), responseElectionRequest{Request: updated, BotID: "bot-new"})
	if err != nil || follower.role != responseElectionFollower {
		t.Fatalf("follower join role = %d (%v), want follower", follower.role, err)
	}
	stored, err := client.HGet(context.Background(), lease.stateKey, "event_time").Result()
	if err != nil || stored == "0" || stored == "" {
		t.Fatalf("adopted event_time = %q (%v), want the follower's value", stored, err)
	}
}

func TestSanitizeEventTime(t *testing.T) {
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		value int64
		want  int64
	}{
		{name: "absent", value: 0, want: 0},
		{name: "negative", value: -5, want: 0},
		{name: "recent", value: now.Unix() - 90, want: now.Unix() - 90},
		{name: "slightly future", value: now.Unix() + 30, want: now.Unix() + 30},
		{name: "too old", value: now.Add(-11 * time.Minute).Unix(), want: 0},
		{name: "too future", value: now.Add(11 * time.Minute).Unix(), want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeEventTime(tc.value, now); got != tc.want {
				t.Fatalf("sanitizeEventTime(%d) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}
