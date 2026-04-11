package config

import (
	"testing"
	"time"
)

func TestApplyEnvOverridesPJSKRenderDeckRecommendMasterdataDir(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_DECK_RECOMMEND_MASTERDATA_DIR", "/srv/masterdata")
	t.Setenv("HARUKI_PJSK_RENDER_MUSIC_META_REFRESH_INTERVAL", "45m")
	t.Setenv("HARUKI_PJSK_RENDER_DECK_RECOMMEND_SERVICE_BASE_URL", "http://127.0.0.1:48080")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	if cfg.PJSKRender.DeckRecommend.MasterdataDir != "/srv/masterdata" {
		t.Fatalf("unexpected masterdata dir override: %q", cfg.PJSKRender.DeckRecommend.MasterdataDir)
	}
	if cfg.PJSKRender.DeckRecommend.ServiceBaseURL != "http://127.0.0.1:48080" {
		t.Fatalf("unexpected deck service url override: %q", cfg.PJSKRender.DeckRecommend.ServiceBaseURL)
	}
	if cfg.PJSKRender.MusicMeta.RefreshInterval != 45*time.Minute {
		t.Fatalf("unexpected music meta refresh interval: %v", cfg.PJSKRender.MusicMeta.RefreshInterval)
	}
}
