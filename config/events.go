package config

import (
	"strings"
	"time"
)

// Replay policies for realtime events (EventsConfig.ReplayPolicy).
const (
	// EventsReplayLatest keeps only the newest pending event of a
	// subscription version: a new event supersedes the older pending ones.
	EventsReplayLatest = "latest"
	// EventsReplayAll keeps up to ReplayMax pending events per subscription
	// version, oldest superseded first.
	EventsReplayAll = "all"
)

// Defaults of the realtime events role.
const (
	DefaultEventsHost                    = "0.0.0.0"
	DefaultEventsPort                    = 7911
	DefaultEventsReplayMax               = 5
	DefaultEventsMaxConns                = 5000
	DefaultEventsMaxConnsPerSubscription = 3
	DefaultEventsHeartbeatInterval       = 15 * time.Second
	DefaultEventsClientRetry             = 3 * time.Second
	DefaultEventsRedeliverAfter          = 90 * time.Second
	DefaultEventsMaxDeliveries           = 3
	DefaultEventsSweepInterval           = 15 * time.Second
	DefaultEventsGCInterval              = time.Hour
	DefaultEventsRetention               = 7 * 24 * time.Hour
	DefaultEventsExpiryGrace             = 10 * time.Minute
	DefaultEventsReplayGrace             = 20 * time.Second
	DefaultEventsForwardTimeout          = 5 * time.Second
	DefaultEventsCloseTimeout            = 5 * time.Second
	DefaultEventsShutdownTimeout         = 10 * time.Second
)

// EventsConfig configures realtime event delivery (the SSE gateway that
// replaced the standalone HMES). The events role (`haruki-server events` or
// HARUKI_ROLE=events) serves it on its own port; Embedded mounts the same
// routes on the main app instead.
type EventsConfig struct {
	// Embedded mounts /sse, /healthz, /internal/events and the close route on
	// the main app and runs the sweeps there (development, integration tests,
	// emergency fallback). Leave false when a separate events role runs.
	Embedded bool   `yaml:"embedded"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	// InternalBaseURL is where the API role reaches the events role to close
	// streams of a replaced or cancelled subscription. Empty disables it.
	InternalBaseURL string `yaml:"internal_base_url"`
	// IngestTokenSHA256 is the lowercase hex SHA-256 of the dedicated token
	// Toolbox sends to /internal/events. Empty rejects every ingest.
	IngestTokenSHA256 string `yaml:"ingest_token_sha256"`
	// ReplayPolicy is "latest" (default) or "all".
	ReplayPolicy string `yaml:"replay_policy"`
	// ReplayMax bounds pending events per subscription version under "all".
	ReplayMax int `yaml:"replay_max"`
	// MaxConns caps concurrent SSE streams; further connects get 503.
	MaxConns int `yaml:"max_conns"`
	// MaxConnsPerSubscription caps streams per subscription version; a new
	// stream over the cap closes the oldest one.
	MaxConnsPerSubscription int           `yaml:"max_conns_per_subscription"`
	HeartbeatInterval       time.Duration `yaml:"heartbeat_interval"`
	// ClientRetry is sent as the SSE retry field.
	ClientRetry time.Duration `yaml:"client_retry"`
	// RedeliverAfter is how long a delivered event may stay unacknowledged
	// before it is sent again on a live stream.
	RedeliverAfter time.Duration `yaml:"redeliver_after"`
	// MaxDeliveries supersedes an event that was delivered this many times
	// without an acknowledgement.
	MaxDeliveries int `yaml:"max_deliveries"`
	// SweepInterval runs redelivery and the stale-stream check.
	SweepInterval time.Duration `yaml:"sweep_interval"`
	GCInterval    time.Duration `yaml:"gc_interval"`
	// Retention keeps rows this long after their expires_at.
	Retention time.Duration `yaml:"retention"`
	// ExpiryGrace is added to the subscription expiry for a row's expires_at.
	ExpiryGrace time.Duration `yaml:"expiry_grace"`
	// ReplayGrace skips, on (re)connect, events delivered less than this long
	// ago: the Client may still be rendering them, and the RedeliverAfter
	// redelivery covers them if they are never acknowledged. Zero means the
	// default; a negative value replays them immediately.
	ReplayGrace time.Duration `yaml:"replay_grace"`
	// LegacyForwardURL, when set, receives every newly accepted ingest body
	// verbatim (best effort), for running the old gateway side by side.
	LegacyForwardURL   string `yaml:"legacy_forward_url"`
	LegacyForwardToken string `yaml:"legacy_forward_token"`
}

// WithDefaults returns c with zero or invalid values replaced by defaults.
func (c EventsConfig) WithDefaults() EventsConfig {
	c.Host = strings.TrimSpace(c.Host)
	if c.Host == "" {
		c.Host = DefaultEventsHost
	}
	if c.Port <= 0 {
		c.Port = DefaultEventsPort
	}
	c.InternalBaseURL = strings.TrimRight(strings.TrimSpace(c.InternalBaseURL), "/")
	c.IngestTokenSHA256 = strings.ToLower(strings.TrimSpace(c.IngestTokenSHA256))
	switch strings.ToLower(strings.TrimSpace(c.ReplayPolicy)) {
	case EventsReplayAll:
		c.ReplayPolicy = EventsReplayAll
	default:
		c.ReplayPolicy = EventsReplayLatest
	}
	if c.ReplayMax <= 0 {
		c.ReplayMax = DefaultEventsReplayMax
	}
	if c.MaxConns <= 0 {
		c.MaxConns = DefaultEventsMaxConns
	}
	if c.MaxConnsPerSubscription <= 0 {
		c.MaxConnsPerSubscription = DefaultEventsMaxConnsPerSubscription
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = DefaultEventsHeartbeatInterval
	}
	if c.ClientRetry <= 0 {
		c.ClientRetry = DefaultEventsClientRetry
	}
	if c.RedeliverAfter <= 0 {
		c.RedeliverAfter = DefaultEventsRedeliverAfter
	}
	if c.MaxDeliveries <= 0 {
		c.MaxDeliveries = DefaultEventsMaxDeliveries
	}
	if c.SweepInterval <= 0 {
		c.SweepInterval = DefaultEventsSweepInterval
	}
	if c.GCInterval <= 0 {
		c.GCInterval = DefaultEventsGCInterval
	}
	if c.Retention <= 0 {
		c.Retention = DefaultEventsRetention
	}
	if c.ExpiryGrace <= 0 {
		c.ExpiryGrace = DefaultEventsExpiryGrace
	}
	switch {
	case c.ReplayGrace == 0:
		c.ReplayGrace = DefaultEventsReplayGrace
	case c.ReplayGrace < 0:
		c.ReplayGrace = -1
	}
	c.LegacyForwardURL = strings.TrimSpace(c.LegacyForwardURL)
	c.LegacyForwardToken = strings.TrimSpace(c.LegacyForwardToken)
	return c
}

func applyEventsEnvOverrides(cfg *EventsConfig) {
	envBool("HARUKI_EVENTS_EMBEDDED", &cfg.Embedded)
	envStr("HARUKI_EVENTS_HOST", &cfg.Host)
	envInt("HARUKI_EVENTS_PORT", &cfg.Port)
	envStr("HARUKI_EVENTS_INTERNAL_BASE_URL", &cfg.InternalBaseURL)
	envStr("HARUKI_EVENTS_INGEST_TOKEN_SHA256", &cfg.IngestTokenSHA256)
	envStr("HARUKI_EVENTS_REPLAY_POLICY", &cfg.ReplayPolicy)
	envInt("HARUKI_EVENTS_REPLAY_MAX", &cfg.ReplayMax)
	envInt("HARUKI_EVENTS_MAX_CONNS", &cfg.MaxConns)
	envInt("HARUKI_EVENTS_MAX_CONNS_PER_SUBSCRIPTION", &cfg.MaxConnsPerSubscription)
	envDuration("HARUKI_EVENTS_HEARTBEAT_INTERVAL", &cfg.HeartbeatInterval)
	envDuration("HARUKI_EVENTS_CLIENT_RETRY", &cfg.ClientRetry)
	envDuration("HARUKI_EVENTS_REDELIVER_AFTER", &cfg.RedeliverAfter)
	envInt("HARUKI_EVENTS_MAX_DELIVERIES", &cfg.MaxDeliveries)
	envDuration("HARUKI_EVENTS_SWEEP_INTERVAL", &cfg.SweepInterval)
	envDuration("HARUKI_EVENTS_GC_INTERVAL", &cfg.GCInterval)
	envDuration("HARUKI_EVENTS_RETENTION", &cfg.Retention)
	envDuration("HARUKI_EVENTS_EXPIRY_GRACE", &cfg.ExpiryGrace)
	envDuration("HARUKI_EVENTS_REPLAY_GRACE", &cfg.ReplayGrace)
	envStr("HARUKI_EVENTS_LEGACY_FORWARD_URL", &cfg.LegacyForwardURL)
	envStr("HARUKI_EVENTS_LEGACY_FORWARD_TOKEN", &cfg.LegacyForwardToken)
}
