package pjsk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/onebot11"

	"github.com/redis/go-redis/v9"
)

const (
	// dedupTTL is how long the dedup lock is held. Any duplicate event arriving
	// within this window is silently dropped (empty segment list returned).
	dedupTTL = 3 * time.Second

	// rateLimitTTL is the per-user cooldown after any response is sent.
	rateLimitTTL = 3 * time.Second

	jitterMin = 100 * time.Millisecond
	jitterMax = 1 * time.Second
)

// RequestGuard provides per-event deduplication and per-user rate limiting
// backed by Redis. A nil Guard is safe to use — all checks are skipped.
//
// Flow for each incoming request:
//  1. Sleep a random 100–1000 ms jitter to spread concurrent instances.
//  2. Check per-user rate limit key — drop silently if set.
//  3. Try SET NX on the dedup lock keyed by (platform, group, user, command) —
//     drop silently if another instance already holds the lock.
//  4. Process the request normally.
//  5. Call MarkComplete to arm the per-user rate limit for the next 3 s.
type RequestGuard struct {
	redis *redis.Client
}

// NewRequestGuard returns a new RequestGuard. Returns nil if rc is nil.
func NewRequestGuard(rc *redis.Client) *RequestGuard {
	if rc == nil {
		return nil
	}
	return &RequestGuard{redis: rc}
}

// Acquire performs jitter, rate-limit check, and dedup lock acquisition.
// Returns true when the caller should proceed with processing.
// Returns false when the request should be silently dropped — respond with
// an empty segment list and do NOT call MarkComplete.
// On Redis errors the guard fails open (returns true) to avoid blocking traffic.
func (g *RequestGuard) Acquire(ctx context.Context, req BotCommandRequest) bool {
	if g == nil {
		return true
	}

	// 1. Random jitter delay.
	jitter := jitterMin + time.Duration(rand.Int63n(int64(jitterMax-jitterMin)))
	select {
	case <-time.After(jitter):
	case <-ctx.Done():
		return false
	}

	// 2. Per-user rate limit: drop if the user already received a response recently.
	rlKey := rateLimitKey(req)
	if limited, err := g.redis.Exists(ctx, rlKey).Result(); err == nil && limited > 0 {
		return false
	}

	// 3. Dedup lock: only the first instance to SET NX for this exact event proceeds.
	lockKey := dedupKey(req)
	res, err := g.redis.SetArgs(ctx, lockKey, "1", redis.SetArgs{TTL: dedupTTL, Mode: "NX"}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return true // fail open on real Redis error
	}
	return res == "OK"
}

// MarkComplete arms the per-user rate limit key. Call this after any request
// where a response (success or user-visible error) was sent to the user.
// Do NOT call it when Acquire returned false.
func (g *RequestGuard) MarkComplete(ctx context.Context, req BotCommandRequest) {
	if g == nil {
		return
	}
	_ = g.redis.Set(ctx, rateLimitKey(req), "1", rateLimitTTL).Err()
}

// dedupKey returns the Redis key that uniquely identifies a command event.
// It hashes platform × group × user × matched_command × message_text so the
// same event maps to the same key regardless of which bot instance receives it.
func dedupKey(req BotCommandRequest) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%s",
		req.Platform,
		req.PlatformGroupID,
		req.PlatformUserID,
		req.MatchedCommand,
		extractMessageText(req.Message),
	)
	return "haruki:bot:dedup:" + hex.EncodeToString(h.Sum(nil))
}

// rateLimitKey returns the Redis key for per-user rate limiting.
func rateLimitKey(req BotCommandRequest) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s", req.Platform, req.PlatformUserID)
	return "haruki:bot:ratelimit:" + hex.EncodeToString(h.Sum(nil))
}

// extractMessageText concatenates all text segments to form the command payload
// component of the dedup key.
func extractMessageText(msg onebot11.Message) string {
	var sb strings.Builder
	for _, seg := range msg {
		if seg.Type != onebot11.TypeText {
			continue
		}
		switch d := seg.Data.(type) {
		case onebot11.TextData:
			sb.WriteString(d.Text)
		case map[string]string:
			sb.WriteString(d[onebot11.KeyText])
		}
	}
	return sb.String()
}
