// Package secevent is the single funnel for security-relevant events on the
// bot API (failed logins, replays, rejected builds, credential anomalies).
// Every event is logged with a stable shape; occurrences are counted per bot
// or source inside a window and crossing the threshold raises one alert
// (ERROR log plus an optional webhook POST) so an operator can revoke the
// bot, build or source quickly.
package secevent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Kind identifies the event class.
type Kind string

const (
	// KindAuthFailed is a login with a bad credential, bot id or payload.
	KindAuthFailed Kind = "auth_failed"
	// KindReplayDetected is a reused nonce on login or a command request.
	KindReplayDetected Kind = "replay_detected"
	// KindRateLimited is a login refused by the per-bot rate limit.
	KindRateLimited Kind = "rate_limited"
	// KindBuildRejected is a login that failed the build policy (reported
	// under log-only, rejected under enforce; see Event.Enforced).
	KindBuildRejected Kind = "build_rejected"
	// KindSessionRevoked is an active session refused by a policy revocation.
	KindSessionRevoked Kind = "session_revoked"
	// KindLoginSourceChanged is a successful login from a different IP than
	// the previous one for the same bot.
	KindLoginSourceChanged Kind = "login_source_changed"
	// KindClientChanged is a successful login with a different client
	// version or build_id than the previous one for the same bot.
	KindClientChanged Kind = "client_changed"
	// KindPolicyUnavailable means the build policy could not be loaded and
	// logins were admitted fail-open.
	KindPolicyUnavailable Kind = "policy_unavailable"
)

// Event is one occurrence. BotID is the counting subject when set, else
// SourceIP.
type Event struct {
	Kind          Kind
	BotID         string
	BuildID       string
	ClientVersion string
	SourceIP      string
	Reason        string
	// Enforced tells whether the request was actually rejected.
	Enforced bool
}

// Reporter receives events. Implementations must be safe for concurrent use.
type Reporter interface {
	Report(ctx context.Context, ev Event)
}

// Report is a nil-safe helper so call sites do not need to guard an unset
// reporter.
func Report(ctx context.Context, r Reporter, ev Event) {
	if r == nil {
		return
	}
	r.Report(ctx, ev)
}

// Counter is the minimal Redis surface the monitor needs.
type Counter interface {
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

// Config tunes alerting.
type Config struct {
	WebhookURL string
	Threshold  int
	Window     time.Duration
	// Node names the Cloud instance in alerts.
	Node string
}

const (
	DefaultThreshold = 5
	DefaultWindow    = 10 * time.Minute
	webhookTimeout   = 5 * time.Second
	keyPrefix        = "haruki:sec:"
)

// Monitor is the production Reporter.
type Monitor struct {
	cfg     Config
	counter Counter
	logger  *slog.Logger
	// post delivers one alert payload; replaced in tests.
	post func(ctx context.Context, payload []byte) error
	// spawn runs the delivery; synchronous in tests.
	spawn func(func())
}

// New builds a monitor. counter may be nil, in which case events are logged
// but never aggregated into alerts.
func New(cfg Config, counter Counter) *Monitor {
	if cfg.Threshold <= 0 {
		cfg.Threshold = DefaultThreshold
	}
	if cfg.Window <= 0 {
		cfg.Window = DefaultWindow
	}
	m := &Monitor{cfg: cfg, counter: counter, logger: slog.Default(), spawn: func(f func()) { go f() }}
	client := &http.Client{Timeout: webhookTimeout}
	m.post = func(ctx context.Context, payload []byte) error {
		url := strings.TrimSpace(m.cfg.WebhookURL)
		if url == "" {
			return nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Haruki-Cloud-Security/1")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("webhook status %d", resp.StatusCode)
		}
		return nil
	}
	return m
}

// Report logs the event and, when the subject crosses the threshold inside
// the window, raises one alert.
func (m *Monitor) Report(ctx context.Context, ev Event) {
	if m == nil {
		return
	}
	attrs := eventAttrs(ev)
	m.logger.LogAttrs(ctx, slog.LevelWarn, "security event", attrs...)

	if m.counter == nil {
		return
	}
	subject := ev.BotID
	if subject == "" {
		subject = ev.SourceIP
	}
	if subject == "" {
		subject = "global"
	}
	key := keyPrefix + string(ev.Kind) + ":" + subject
	count, err := m.counter.Incr(ctx, key)
	if err != nil {
		m.logger.LogAttrs(ctx, slog.LevelWarn, "security event counter failed", slog.String("error_type", fmt.Sprintf("%T", err)))
		return
	}
	if count == 1 {
		_ = m.counter.Expire(ctx, key, m.cfg.Window)
	}
	if count != int64(m.cfg.Threshold) {
		return
	}
	alert := alertPayload{
		Kind:          string(ev.Kind),
		BotID:         ev.BotID,
		BuildID:       ev.BuildID,
		ClientVersion: ev.ClientVersion,
		SourceIP:      ev.SourceIP,
		Reason:        ev.Reason,
		Enforced:      ev.Enforced,
		Count:         count,
		Threshold:     m.cfg.Threshold,
		WindowSeconds: int64(m.cfg.Window / time.Second),
		Node:          m.cfg.Node,
		Time:          time.Now().UTC().Format(time.RFC3339),
	}
	m.logger.LogAttrs(ctx, slog.LevelError, "security alert", append(attrs,
		slog.Int64("count", count),
		slog.Int("threshold", m.cfg.Threshold),
		slog.Duration("window", m.cfg.Window),
	)...)
	payload, err := json.Marshal(alert)
	if err != nil {
		return
	}
	m.spawn(func() {
		deliverCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), webhookTimeout)
		defer cancel()
		if err := m.post(deliverCtx, payload); err != nil {
			m.logger.LogAttrs(deliverCtx, slog.LevelWarn, "security alert webhook failed",
				slog.String("kind", string(ev.Kind)), slog.String("error_type", fmt.Sprintf("%T", err)))
		}
	})
}

type alertPayload struct {
	Kind          string `json:"kind"`
	BotID         string `json:"bot_id,omitempty"`
	BuildID       string `json:"build_id,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
	SourceIP      string `json:"source_ip,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Enforced      bool   `json:"enforced"`
	Count         int64  `json:"count"`
	Threshold     int    `json:"threshold"`
	WindowSeconds int64  `json:"window_seconds"`
	Node          string `json:"node,omitempty"`
	Time          string `json:"time"`
}

func eventAttrs(ev Event) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("event", "security"),
		slog.String("kind", string(ev.Kind)),
		slog.Bool("enforced", ev.Enforced),
	}
	if ev.BotID != "" {
		attrs = append(attrs, slog.String("bot_id", ev.BotID))
	}
	if ev.BuildID != "" {
		attrs = append(attrs, slog.String("build_id", ev.BuildID))
	}
	if ev.ClientVersion != "" {
		attrs = append(attrs, slog.String("client_version", ev.ClientVersion))
	}
	if ev.SourceIP != "" {
		attrs = append(attrs, slog.String("source_ip", ev.SourceIP))
	}
	if ev.Reason != "" {
		attrs = append(attrs, slog.String("reason", ev.Reason))
	}
	return attrs
}
