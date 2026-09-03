package pjsk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"haruki-cloud/internal/core/secevent"
	"strings"
	"time"

	"haruki-cloud/utils/logger"

	"github.com/redis/go-redis/v9"
)

// defaultReplayWindow is the accepted timestamp skew and single-use nonce TTL
// when no window is configured.
const defaultReplayWindow = 5 * time.Minute

var replayGuardLogger = logger.NewLoggerFromGlobal("ReplayGuard")

// nonceStore stores single-use request nonces. storeNonce reports whether the
// nonce was newly stored (true) or was already present (false = replay).
type nonceStore interface {
	storeNonce(ctx context.Context, key string, ttl time.Duration) (stored bool, err error)
}

type redisNonceStore struct{ rc *redis.Client }

func (s redisNonceStore) storeNonce(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	res, err := s.rc.SetArgs(ctx, key, "1", redis.SetArgs{TTL: ttl, Mode: "NX"}).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil // key already present -> replay
	}
	if err != nil {
		return false, err
	}
	return res == "OK", nil
}

// replayGuard enforces request freshness for the Noise-encrypted bot channel:
// Noise NK message-1 is replayable (the responder static key is reused), so a
// captured request stays valid for as long as the session JWT. The guard
// requires the request timestamp to fall inside the window and consumes the
// request nonce exactly once, mirroring the /auth nonce defense.
//
// Rollout is non-breaking: with requireNonce false (default) requests without
// the fields pass untouched, so legacy clients keep working; a present nonce is
// always validated. Redis errors fail open, consistent with the request guard.
// A nil guard allows everything.
type replayGuard struct {
	nonces       nonceStore
	window       time.Duration
	requireNonce bool
	now          func() time.Time
	security     secevent.Reporter
}

func newReplayGuard(rc *redis.Client, window time.Duration, requireNonce bool, security secevent.Reporter) *replayGuard {
	if rc == nil {
		return nil
	}
	if window <= 0 {
		window = defaultReplayWindow
	}
	return &replayGuard{
		nonces:       redisNonceStore{rc: rc},
		window:       window,
		requireNonce: requireNonce,
		now:          time.Now,
		security:     security,
	}
}

// allow reports whether the request passes replay validation. Rejected
// requests must be dropped silently (empty OK response), indistinguishable
// from a dedup drop.
func (g *replayGuard) allow(ctx context.Context, botID string, req BotCommandRequest) bool {
	if g == nil || g.nonces == nil {
		return true
	}

	nonce := strings.TrimSpace(req.Nonce)
	if nonce == "" || req.Timestamp == 0 {
		// Lenient rollout: only reject missing fields once enforcement is on.
		if g.requireNonce {
			replayGuardLogger.InfoContext(ctx, "bot request rejected without nonce",
				"event", "request_replay",
				"outcome", "missing_nonce",
			)
			return false
		}
		return true
	}

	now := g.now().Unix()
	windowSecs := int64(g.window.Seconds())
	if req.Timestamp < now-windowSecs || req.Timestamp > now+windowSecs {
		replayGuardLogger.InfoContext(ctx, "bot request rejected as stale",
			"event", "request_replay",
			"outcome", "stale_timestamp",
			"skew_seconds", req.Timestamp-now,
		)
		return false
	}

	sum := sha256.Sum256([]byte(nonce))
	key := "haruki:bot:nonce:" + hex.EncodeToString(sum[:])
	stored, err := g.nonces.storeNonce(ctx, key, g.window)
	if err != nil {
		return true // fail open on store error
	}
	if !stored {
		replayGuardLogger.InfoContext(ctx, "bot request rejected as replay",
			"event", "request_replay",
			"outcome", "nonce_reused",
		)
		secevent.Report(ctx, g.security, secevent.Event{
			Kind: secevent.KindReplayDetected, BotID: botID, Reason: "command nonce reused", Enforced: true,
		})
	}
	return stored
}
