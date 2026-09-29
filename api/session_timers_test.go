package api

import (
	"context"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"testing"
	"time"
)

func TestSessionTimersIncludeInvalidAndCanceledRequests(t *testing.T) {
	prev := config.Cfg
	config.Cfg.HarukiBotDB.SessionSignToken = "session-secret"
	t.Cleanup(func() { config.Cfg = prev })
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	token := signSessionTokenForTest(t, jwt.SigningMethodHS256, "7")
	if err := client.Set(t.Context(), fmt.Sprintf(RedisKeyBotSession, "7"), token, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"valid", "invalid", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			ctx, trace := commandtrace.WithTrace(ctx)
			candidate := token
			if mode == "invalid" {
				candidate = "invalid"
			}
			failure := VerifyBotSessionToken(ctx, client, "7", candidate)
			if (failure == nil) != (mode == "valid") {
				t.Fatalf("failure=%+v", failure)
			}
			counts := map[string]int{}
			for _, stat := range trace.Snapshot().Operations {
				counts[stat.Name] = stat.Count
			}
			if counts["session.jwt_verify"] != 1 {
				t.Fatalf("JWT timer missing: %v", counts)
			}
			wantStore := 1
			if mode == "invalid" {
				wantStore = 0
			}
			if counts["session.store_lookup"] != wantStore {
				t.Fatalf("store count: %v", counts)
			}
			wantPolicy := 0
			if mode == "valid" {
				wantPolicy = 1
			}
			if counts["session.policy_check"] != wantPolicy {
				t.Fatalf("policy count: %v", counts)
			}
		})
	}
}
