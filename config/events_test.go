package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventsConfigDefaults(t *testing.T) {
	got := EventsConfig{
		ReplayPolicy:      " ALL ",
		IngestTokenSHA256: "  ABCDEF  ",
		InternalBaseURL:   " http://events:7911/ ",
		LegacyForwardURL:  " http://legacy/internal/events ",
	}.WithDefaults()
	if got.Host != DefaultEventsHost || got.Port != DefaultEventsPort ||
		got.ReplayPolicy != EventsReplayAll || got.ReplayMax != DefaultEventsReplayMax ||
		got.MaxConns != DefaultEventsMaxConns || got.MaxConnsPerSubscription != DefaultEventsMaxConnsPerSubscription ||
		got.HeartbeatInterval != DefaultEventsHeartbeatInterval || got.ClientRetry != DefaultEventsClientRetry ||
		got.RedeliverAfter != DefaultEventsRedeliverAfter || got.MaxDeliveries != DefaultEventsMaxDeliveries ||
		got.SweepInterval != DefaultEventsSweepInterval || got.GCInterval != DefaultEventsGCInterval ||
		got.Retention != DefaultEventsRetention || got.ExpiryGrace != DefaultEventsExpiryGrace ||
		got.IngestTokenSHA256 != "abcdef" || got.InternalBaseURL != "http://events:7911" ||
		got.LegacyForwardURL != "http://legacy/internal/events" {
		t.Fatalf("defaults = %+v", got)
	}
	if policy := (EventsConfig{ReplayPolicy: "bogus"}).WithDefaults().ReplayPolicy; policy != EventsReplayLatest {
		t.Fatalf("unknown policy = %q", policy)
	}
	kept := EventsConfig{Host: "127.0.0.1", Port: 1, ReplayMax: 2, MaxConns: 3, MaxConnsPerSubscription: 4,
		HeartbeatInterval: time.Second, ClientRetry: time.Second, RedeliverAfter: time.Second, MaxDeliveries: 5,
		SweepInterval: time.Second, GCInterval: time.Second, Retention: time.Second, ExpiryGrace: time.Second}.WithDefaults()
	if kept.Host != "127.0.0.1" || kept.Port != 1 || kept.ReplayMax != 2 || kept.MaxConns != 3 || kept.MaxConnsPerSubscription != 4 || kept.MaxDeliveries != 5 || kept.Retention != time.Second {
		t.Fatalf("explicit values replaced: %+v", kept)
	}
}

func TestEventsConfigFromYAMLAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cloud.yaml")
	body := "events:\n  embedded: true\n  port: 7000\n  replay_policy: all\n  redeliver_after: 2m\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"HARUKI_EVENTS_PORT":                       "7911",
		"HARUKI_EVENTS_HOST":                       "127.0.0.1",
		"HARUKI_EVENTS_INTERNAL_BASE_URL":          "http://events:7911",
		"HARUKI_EVENTS_INGEST_TOKEN_SHA256":        "abc",
		"HARUKI_EVENTS_REPLAY_MAX":                 "4",
		"HARUKI_EVENTS_MAX_CONNS":                  "10",
		"HARUKI_EVENTS_MAX_CONNS_PER_SUBSCRIPTION": "2",
		"HARUKI_EVENTS_HEARTBEAT_INTERVAL":         "20s",
		"HARUKI_EVENTS_CLIENT_RETRY":               "4s",
		"HARUKI_EVENTS_MAX_DELIVERIES":             "5",
		"HARUKI_EVENTS_SWEEP_INTERVAL":             "30s",
		"HARUKI_EVENTS_GC_INTERVAL":                "2h",
		"HARUKI_EVENTS_RETENTION":                  "48h",
		"HARUKI_EVENTS_EXPIRY_GRACE":               "5m",
		"HARUKI_EVENTS_LEGACY_FORWARD_URL":         "http://legacy/internal/events",
		"HARUKI_EVENTS_LEGACY_FORWARD_TOKEN":       "old",
		"HARUKI_EVENTS_EMBEDDED":                   "false",
		"HARUKI_BACKEND_MAIN_LOG_FILE":             "/tmp/events.log",
	} {
		t.Setenv(key, value)
	}
	cfg, err := ReadConfig(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	e := cfg.Events
	if e.Embedded || e.Port != 7911 || e.Host != "127.0.0.1" || e.ReplayPolicy != "all" || e.RedeliverAfter != 2*time.Minute ||
		e.InternalBaseURL != "http://events:7911" || e.IngestTokenSHA256 != "abc" || e.ReplayMax != 4 || e.MaxConns != 10 ||
		e.MaxConnsPerSubscription != 2 || e.HeartbeatInterval != 20*time.Second || e.ClientRetry != 4*time.Second ||
		e.MaxDeliveries != 5 || e.SweepInterval != 30*time.Second || e.GCInterval != 2*time.Hour || e.Retention != 48*time.Hour ||
		e.ExpiryGrace != 5*time.Minute || e.LegacyForwardURL != "http://legacy/internal/events" || e.LegacyForwardToken != "old" {
		t.Fatalf("events config = %+v", e)
	}
	if cfg.Backend.MainLogFile != "/tmp/events.log" {
		t.Fatalf("main log file override = %q", cfg.Backend.MainLogFile)
	}
}
