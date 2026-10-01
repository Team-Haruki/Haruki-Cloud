package handler

import (
	"context"
	"strings"
	"sync"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
)

const (
	defaultForceRenderCooldown = time.Minute
	// forceRenderLimiterSweepAt bounds the map: once it holds this many users,
	// expired entries are dropped before the next one is added.
	forceRenderLimiterSweepAt = 4096
)

// forceRenderLimiter grants one forced render per (platform user, command)
// per cooldown. It is process-local: Cloud runs a single instance, and a
// second replica would at worst double the allowance.
type forceRenderLimiter struct {
	mu   sync.Mutex
	next map[string]time.Time
	now  func() time.Time
}

func newForceRenderLimiter() *forceRenderLimiter {
	return &forceRenderLimiter{next: map[string]time.Time{}, now: time.Now}
}

var commandForceLimiter = newForceRenderLimiter()

func (l *forceRenderLimiter) allow(key string, cooldown time.Duration) bool {
	if cooldown < 0 {
		return false
	}
	if cooldown == 0 {
		cooldown = defaultForceRenderCooldown
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if until, ok := l.next[key]; ok && now.Before(until) {
		return false
	}
	if len(l.next) >= forceRenderLimiterSweepAt {
		for k, until := range l.next {
			if !now.Before(until) {
				delete(l.next, k)
			}
		}
	}
	l.next[key] = now.Add(cooldown)
	return true
}

func forceRenderLimitKey(resolved *CommandRequest) string {
	command := strings.TrimSpace(resolved.CommandPath)
	if command == "" {
		command = strings.TrimSpace(resolved.TriggerCommand)
	}
	return strings.TrimSpace(resolved.RequesterPlatform) + "\x00" + strings.TrimSpace(resolved.RequesterUserID) + "\x00" + command
}

// applyForceRender marks ctx for fresh renders when the request asked for it
// and the requester is outside the cooldown. Over the limit, or without a
// requester to rate-limit, the request silently keeps the cached path.
func applyForceRender(ctx context.Context, resolved *CommandRequest, limiter *forceRenderLimiter, cooldown time.Duration) context.Context {
	if resolved == nil || !resolved.IsForce {
		return ctx
	}
	if strings.TrimSpace(resolved.RequesterUserID) == "" || !limiter.allow(forceRenderLimitKey(resolved), cooldown) {
		commandtrace.RecordOperation(ctx, "drawing.force_denied", 0)
		return ctx
	}
	commandtrace.RecordOperation(ctx, "drawing.force", 0)
	return drawing.WithForceRender(ctx)
}

func forceRenderCooldown() time.Duration {
	return config.Cfg.PJSKRender.DrawingCache.ForceCooldown
}
