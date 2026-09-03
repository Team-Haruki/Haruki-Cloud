package api

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/buildpolicy"
	"haruki-cloud/internal/core/secevent"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type fixedSessionPolicy struct {
	decision buildpolicy.Decision
	calls    []string
}

func (p *fixedSessionPolicy) SessionAllowed(botID, clientVersion, buildID string, _ time.Time) buildpolicy.Decision {
	p.calls = append(p.calls, botID+"/"+clientVersion+"/"+buildID)
	return p.decision
}

type collectingReporter struct {
	mu     sync.Mutex
	events []secevent.Event
}

func (r *collectingReporter) Report(_ context.Context, ev secevent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func TestVerifyBotSessionTokenWithPolicy(t *testing.T) {
	prev := config.Cfg
	config.Cfg.HarukiBotDB.SessionSignToken = "policy-session-sign"
	t.Cleanup(func() { config.Cfg = prev })

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"bot_id": "7", "exp": time.Now().Add(time.Hour).Unix(), "bid": "build-1", "cv": "3.1.0",
	}).SignedString([]byte(config.Cfg.HarukiBotDB.SessionSignToken))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Set(context.Background(), fmt.Sprintf(RedisKeyBotSession, "7"), token, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}

	// No policy: unchanged behaviour.
	if failure := VerifyBotSessionToken(context.Background(), client, "7", token); failure != nil {
		t.Fatalf("no policy: %+v", failure)
	}

	// Enforced revocation: 403 with the revocation message, event enforced.
	policy := &fixedSessionPolicy{decision: buildpolicy.Decision{Allowed: false, Passed: false, Code: buildpolicy.CodeBuildRevoked, Reason: "pulled", Enforce: true}}
	reporter := &collectingReporter{}
	failure := VerifyBotSessionTokenWithPolicy(context.Background(), client, policy, reporter, "7", token)
	if failure == nil || failure.Status != fiber.StatusForbidden || failure.Message != ErrSessionRevokedByPolicy {
		t.Fatalf("revoked session = %+v", failure)
	}
	if len(policy.calls) != 1 || policy.calls[0] != "7/3.1.0/build-1" {
		t.Fatalf("policy saw %v", policy.calls)
	}
	if len(reporter.events) != 1 || reporter.events[0].Kind != secevent.KindSessionRevoked || !reporter.events[0].Enforced || reporter.events[0].BuildID != "build-1" {
		t.Fatalf("events = %+v", reporter.events)
	}

	// Log-only revocation: admitted, reported as not enforced.
	policy.decision = buildpolicy.Decision{Allowed: true, Passed: false, Code: buildpolicy.CodeVersionRevoked, Reason: "old"}
	reporter.events = nil
	if failure := VerifyBotSessionTokenWithPolicy(context.Background(), client, policy, reporter, "7", token); failure != nil {
		t.Fatalf("log-only session = %+v", failure)
	}
	if len(reporter.events) != 1 || reporter.events[0].Enforced {
		t.Fatalf("events = %+v", reporter.events)
	}

	// Unavailable policy: silent pass.
	policy.decision = buildpolicy.Decision{Allowed: true, Passed: false, Code: buildpolicy.CodePolicyUnavailable, Enforce: true}
	reporter.events = nil
	if failure := VerifyBotSessionTokenWithPolicy(context.Background(), client, policy, reporter, "7", token); failure != nil || len(reporter.events) != 0 {
		t.Fatalf("unavailable policy = %+v events %+v", failure, reporter.events)
	}

	// Stored-session checks still run first: a revoked Redis session never
	// reaches the policy.
	server.Del(fmt.Sprintf(RedisKeyBotSession, "7"))
	policy.calls = nil
	if failure := VerifyBotSessionTokenWithPolicy(context.Background(), client, policy, reporter, "7", token); failure == nil || failure.Status != fiber.StatusUnauthorized || len(policy.calls) != 0 {
		t.Fatalf("deleted session = %+v calls %v", failure, policy.calls)
	}
}
