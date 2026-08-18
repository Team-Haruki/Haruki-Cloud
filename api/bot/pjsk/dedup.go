package pjsk

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"haruki-cloud/internal/onebot11"

	"github.com/redis/go-redis/v9"
)

const (
	// dedupInFlightTTL is a crash-safety lease, not a post-command cooldown.
	// MarkComplete removes the event lock as soon as processing ends. The lease
	// must exceed the slowest supported command so retries cannot start a second
	// render while the first request is still running.
	dedupInFlightTTL = 5 * time.Minute
	// rateLimitTTL is the per-user cooldown after any response is sent.
	rateLimitTTL = 3 * time.Second
)

var acquireRequestGuardScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) ~= 0 then
    return 0
end
if redis.call("SET", KEYS[2], ARGV[2], "PX", ARGV[1], "NX") then
    return 1
end
return 0
`)

var completeRequestGuardScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[2] then
    redis.call("DEL", KEYS[1])
    redis.call("SET", KEYS[2], "1", "PX", ARGV[1])
    return 1
end
return 0
`)

// RequestGuard provides per-event deduplication and per-user rate limiting
// backed by Redis. A nil Guard is safe to use — all checks are skipped.
//
// Flow for each incoming request:
//  1. Atomically check the per-user rate limit. A limited request must not
//     refresh the event lock.
//  2. Try SET NX on the dedup lock keyed by (platform, group, user, command) —
//     drop silently if another instance already holds the lock.
//  3. Process the request normally.
//  4. Call MarkComplete to release the event lock and arm the per-user rate
//     limit for the next 3 s.
type RequestGuard struct {
	redis   *redis.Client
	cleanup *requestGuardCleanupDispatcher
}

type requestGuardLease struct {
	proceed bool
	token   string
	lockKey string
	rateKey string
}

type commandRequestGuard interface {
	Acquire(ctx context.Context, req BotCommandRequest) requestGuardLease
	MarkComplete(ctx context.Context, req BotCommandRequest, lease requestGuardLease)
}

// NewRequestGuard returns a new RequestGuard. Returns nil if rc is nil.
func NewRequestGuard(rc *redis.Client) *RequestGuard {
	if rc == nil {
		return nil
	}
	guard := &RequestGuard{redis: rc}
	guard.cleanup = newRequestGuardCleanupDispatcher(
		requestGuardCleanupQueueCapacity,
		requestGuardCleanupWorkerCount,
		guard.complete,
	)
	return guard
}

// Acquire performs the rate-limit check and dedup lock acquisition.
// Returns true when the caller should proceed with processing.
// Returns false when the request should be silently dropped — respond with
// an empty segment list and do NOT call MarkComplete.
// On Redis errors the guard fails open (returns true) to avoid blocking traffic.
func (g *RequestGuard) Acquire(ctx context.Context, req BotCommandRequest) requestGuardLease {
	if g == nil {
		return requestGuardLease{proceed: true}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return requestGuardLease{}
	}
	token, err := newRequestGuardToken()
	if err != nil {
		return requestGuardLease{proceed: true}
	}

	rlKey := rateLimitKey(req)
	lockKey := dedupKey(req)
	result, err := acquireRequestGuardScript.Run(
		ctx,
		g.redis,
		[]string{rlKey, lockKey},
		dedupInFlightTTL.Milliseconds(),
		token,
	).Int64()
	if err != nil {
		if ctx.Err() != nil {
			return requestGuardLease{}
		}
		// Fail open without an owner token. Completion must not release a lock
		// that may have been acquired by another request while Redis recovered.
		return requestGuardLease{proceed: true}
	}
	if result != 1 {
		return requestGuardLease{}
	}
	return requestGuardLease{
		proceed: true,
		token:   token,
		lockKey: lockKey,
		rateKey: rlKey,
	}
}

// MarkComplete arms the per-user rate limit key. Call this after any request
// where a response (success or user-visible error) was sent to the user.
// Do NOT call it when Acquire returned false.
func (g *RequestGuard) MarkComplete(_ context.Context, _ BotCommandRequest, lease requestGuardLease) {
	if g == nil || lease.token == "" || lease.lockKey == "" || lease.rateKey == "" {
		return
	}
	job := requestGuardCleanupJob{
		lockKey: strings.Clone(lease.lockKey),
		rateKey: strings.Clone(lease.rateKey),
		owner:   strings.Clone(lease.token),
	}
	if g.cleanup != nil && g.cleanup.Enqueue(job) {
		return
	}

	// Saturation must not leave an otherwise healthy Redis owner lock behind for
	// the full crash-safety TTL. Use one short owner-safe fallback attempt. This
	// only adds bounded latency when the background queue is unavailable.
	var complete requestGuardCleanupFunc
	if g.cleanup != nil && g.cleanup.complete != nil {
		complete = g.cleanup.complete
	} else if g.redis != nil {
		complete = g.complete
	}
	if complete == nil {
		requestGuardFailureLogger.Warn("request guard cleanup unavailable",
			"event", "request_guard_cleanup_enqueue",
			"outcome", "unavailable",
		)
		return
	}
	fallbackCtx, cancel := context.WithTimeout(context.Background(), requestGuardCleanupFallbackTimeout)
	ownerMatched, err := complete(fallbackCtx, job)
	cancel()
	attrs := []any{
		"event", "request_guard_cleanup_enqueue",
		"outcome", "fallback",
		"owner_matched", ownerMatched,
	}
	if err != nil {
		attrs = append(attrs, "error_type", fmt.Sprintf("%T", err))
		requestGuardFailureLogger.Warn("request guard cleanup fallback failed", attrs...)
		return
	}
	requestGuardLogger.Info("request guard cleanup queue saturated", attrs...)
}

func (g *RequestGuard) complete(ctx context.Context, job requestGuardCleanupJob) (bool, error) {
	result, err := completeRequestGuardScript.Run(
		ctx,
		g.redis,
		[]string{job.lockKey, job.rateKey},
		rateLimitTTL.Milliseconds(),
		job.owner,
	).Int64()
	return result == 1, err
}

func acquireRequestGuard(ctx context.Context, guard commandRequestGuard, req BotCommandRequest) requestGuardLease {
	if guard == nil {
		return requestGuardLease{proceed: true}
	}
	return guard.Acquire(ctx, req)
}

func markRequestGuardComplete(ctx context.Context, guard commandRequestGuard, req BotCommandRequest, lease requestGuardLease) {
	if guard == nil || !lease.proceed {
		return
	}
	guard.MarkComplete(ctx, req, lease)
}

func newRequestGuardToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// dedupKey uses the same transport-independent command identity as response
// election so both layers agree on which requests are duplicates.
func dedupKey(req BotCommandRequest) string {
	return "haruki:bot:dedup:" + responseElectionIdentity(req)
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
	parts := make([]string, 0, len(msg))
	for _, seg := range msg {
		if seg.Type != onebot11.TypeText {
			continue
		}
		switch d := seg.Data.(type) {
		case onebot11.TextData:
			parts = append(parts, d.Text)
		case map[string]any:
			parts = append(parts, fmt.Sprint(d[onebot11.KeyText]))
		case map[string]string:
			parts = append(parts, d[onebot11.KeyText])
		case map[any]any:
			parts = append(parts, fmt.Sprint(d[onebot11.KeyText]))
		}
	}
	return strings.Join(parts, " ")
}
