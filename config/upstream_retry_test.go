package config

import (
	"testing"
	"time"
)

func TestUpstreamRetryEnvOverrides(t *testing.T) {
	t.Setenv("HARUKI_SEKAI_API_RETRY_MAX_RETRIES", "2")
	t.Setenv("HARUKI_TOOLBOX_RETRY_BUDGET", "1s")
	t.Setenv("HARUKI_TRACKER_RETRY_MAX_RETRIES", "-1")
	t.Setenv("HARUKI_TRACKER_RETRY_WAIT", "50ms")
	t.Setenv("HARUKI_TRACKER_RETRY_MAX_WAIT", "500ms")
	var cfg Config
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.SekaiAPI.Retry.MaxRetries != 2 || cfg.Toolbox.Retry.Budget != time.Second {
		t.Fatalf("sekai %+v toolbox %+v", cfg.SekaiAPI.Retry, cfg.Toolbox.Retry)
	}
	tracker := cfg.Tracker.Retry.WithDefaults()
	if tracker.MaxRetries != -1 || tracker.Wait != 50*time.Millisecond || tracker.MaxWait != 500*time.Millisecond || tracker.Budget != DefaultUpstreamRetryBudget {
		t.Fatalf("tracker %+v", tracker)
	}
}
