package pjsk

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"haruki-cloud/internal/onebot11"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRequestGuardRedisLeaseLifecycle(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	guard := NewRequestGuard(client)
	t.Cleanup(guard.Close)
	req := requestGuardCoverageRequest()

	lease := guard.Acquire(context.Background(), req)
	assertOwnedRequestGuardLease(t, lease)
	if second := guard.Acquire(context.Background(), req); second.proceed {
		t.Fatal("duplicate request acquired an existing owner lock")
	}

	matched, err := guard.complete(context.Background(), requestGuardCleanupJob{
		lockKey: lease.lockKey,
		rateKey: lease.rateKey,
		owner:   lease.token,
	})
	if err != nil || !matched {
		t.Fatalf("complete() = (%v, %v), want (true, nil)", matched, err)
	}
	if limited := guard.Acquire(context.Background(), req); limited.proceed {
		t.Fatal("completed request bypassed the rate limit")
	}

	server.FastForward(rateLimitTTL)
	assertOwnedRequestGuardLease(t, guard.Acquire(context.Background(), req))
}

func TestRequestGuardCompletionPreservesDifferentOwner(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	guard := NewRequestGuard(client)
	t.Cleanup(guard.Close)
	lease := guard.Acquire(context.Background(), requestGuardCoverageRequest())
	assertOwnedRequestGuardLease(t, lease)

	matched, err := guard.complete(context.Background(), requestGuardCleanupJob{
		lockKey: lease.lockKey,
		rateKey: lease.rateKey,
		owner:   "different-owner",
	})
	if err != nil || matched {
		t.Fatalf("complete() = (%v, %v), want (false, nil)", matched, err)
	}
	got, err := server.Get(lease.lockKey)
	if err != nil || got != lease.token {
		t.Fatalf("owner lock = %q with error %v, want %q", got, err, lease.token)
	}
	if server.Exists(lease.rateKey) {
		t.Fatal("owner mismatch unexpectedly armed the rate limit")
	}
}

func TestRequestGuardFailsOpenWhenRedisIsUnavailable(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	guard := NewRequestGuard(client)
	t.Cleanup(guard.Close)
	server.Close()

	lease := guard.Acquire(context.Background(), requestGuardCoverageRequest())
	if !lease.proceed || lease.token != "" || lease.lockKey != "" || lease.rateKey != "" {
		t.Fatalf("unavailable Redis lease = %+v, want ownerless fail-open lease", lease)
	}
}

func TestRequestGuardNilAndHelperPaths(t *testing.T) {
	if NewRequestGuard(nil) != nil {
		t.Fatal("NewRequestGuard(nil) returned a guard")
	}
	var nilGuard *RequestGuard
	if lease := nilGuard.Acquire(context.Background(), BotCommandRequest{}); !lease.proceed {
		t.Fatal("nil request guard did not fail open")
	}
	if lease := acquireRequestGuard(context.Background(), nil, BotCommandRequest{}); !lease.proceed {
		t.Fatal("nil command guard did not fail open")
	}

	recorder := &recordingCommandRequestGuard{lease: requestGuardLease{proceed: true, token: "owner"}}
	lease := acquireRequestGuard(context.Background(), recorder, BotCommandRequest{})
	if !recorder.acquired || lease.token != "owner" {
		t.Fatalf("acquire helper did not delegate: acquired=%v lease=%+v", recorder.acquired, lease)
	}
	markRequestGuardComplete(context.Background(), recorder, BotCommandRequest{}, requestGuardLease{})
	markRequestGuardComplete(context.Background(), nil, BotCommandRequest{}, lease)
	if recorder.completed {
		t.Fatal("completion helper delegated a rejected lease")
	}
	markRequestGuardComplete(context.Background(), recorder, BotCommandRequest{}, lease)
	if !recorder.completed {
		t.Fatal("completion helper did not delegate an accepted lease")
	}
}

func TestRequestGuardMarkCompleteFallbackPaths(t *testing.T) {
	lease := requestGuardLease{proceed: true, token: "owner", lockKey: "lock", rateKey: "rate"}
	(&RequestGuard{}).MarkComplete(context.Background(), BotCommandRequest{}, lease)
	(&RequestGuard{}).MarkComplete(context.Background(), BotCommandRequest{}, requestGuardLease{proceed: true})

	t.Run("success", func(t *testing.T) {
		called := false
		guard := requestGuardWithClosedDispatcher(t, func(_ context.Context, job requestGuardCleanupJob) (bool, error) {
			called = job.lockKey == lease.lockKey && job.rateKey == lease.rateKey && job.owner == lease.token
			return true, nil
		})
		guard.MarkComplete(context.Background(), BotCommandRequest{}, lease)
		if !called {
			t.Fatal("fallback cleanup did not receive the owned lease")
		}
	})

	t.Run("error", func(t *testing.T) {
		wantErr := errors.New("cleanup failed")
		guard := requestGuardWithClosedDispatcher(t, func(context.Context, requestGuardCleanupJob) (bool, error) {
			return false, wantErr
		})
		guard.MarkComplete(context.Background(), BotCommandRequest{}, lease)
	})
}

func TestRequestGuardTokenAndMessageExtraction(t *testing.T) {
	token, err := newRequestGuardToken()
	if err != nil {
		t.Fatalf("newRequestGuardToken() error = %v", err)
	}
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 16 {
		t.Fatalf("token %q decoded to %d bytes with error %v", token, len(decoded), err)
	}

	message := onebot11.Message{
		onebot11.Text("typed"),
		{Type: onebot11.TypeText, Data: map[string]any{onebot11.KeyText: 42}},
		{Type: onebot11.TypeText, Data: map[string]string{onebot11.KeyText: "strings"}},
		{Type: onebot11.TypeText, Data: map[any]any{onebot11.KeyText: "interfaces"}},
		onebot11.Image("ignored", ""),
		{Type: onebot11.TypeText, Data: struct{}{}},
	}
	if got := extractMessageText(message); got != "typed 42 strings interfaces" {
		t.Fatalf("extractMessageText() = %q", got)
	}
}

func requestGuardWithClosedDispatcher(t *testing.T, complete requestGuardCleanupFunc) *RequestGuard {
	t.Helper()
	dispatcher := newRequestGuardCleanupDispatcher(1, 1, complete)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.CloseContext(ctx); err != nil {
		t.Fatalf("CloseContext() error = %v", err)
	}
	return &RequestGuard{cleanup: dispatcher}
}

func requestGuardCoverageRequest() BotCommandRequest {
	return BotCommandRequest{
		Platform:        "qq",
		PlatformUserID:  "10001",
		PlatformGroupID: "20001",
		MatchedCommand:  "/card",
		Message:         onebot11.Message{onebot11.Text("miku")},
		EventTime:       123456,
	}
}

func assertOwnedRequestGuardLease(t *testing.T, lease requestGuardLease) {
	t.Helper()
	if !lease.proceed || lease.token == "" || lease.lockKey == "" || lease.rateKey == "" {
		t.Fatalf("lease = %+v, want an owned lease", lease)
	}
}

type recordingCommandRequestGuard struct {
	lease     requestGuardLease
	acquired  bool
	completed bool
}

func (g *recordingCommandRequestGuard) Acquire(context.Context, BotCommandRequest) requestGuardLease {
	g.acquired = true
	return g.lease
}

func (g *recordingCommandRequestGuard) MarkComplete(context.Context, BotCommandRequest, requestGuardLease) {
	g.completed = true
}
