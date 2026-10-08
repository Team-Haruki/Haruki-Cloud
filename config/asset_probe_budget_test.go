package config

import (
	"testing"
	"time"
)

func TestAssetProbeBudgetEnvOverrides(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_ASSET_PROBE_WARM_INTERVAL", "10m")
	t.Setenv("HARUKI_PJSK_RENDER_ASSET_PROBE_REQUEST_BUDGET", "3s")
	t.Setenv("HARUKI_PJSK_RENDER_ASSET_PROBE_REQUEST_MAX_STORE_CALLS", "64")
	t.Setenv("HARUKI_PJSK_RENDER_ASSET_PROBE_REQUEST_CONCURRENCY", "4")
	var cfg Config
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	probe := cfg.PJSKRender.AssetProbe
	if probe.WarmInterval != 10*time.Minute || probe.RequestBudget != 3*time.Second || probe.RequestMaxStoreCalls != 64 || probe.RequestConcurrency != 4 {
		t.Fatalf("asset probe = %+v", probe)
	}
}
