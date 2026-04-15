package config

import (
	"testing"
	"time"
)

// ================= Profile Tests =================

func TestParseProfile(t *testing.T) {
	tests := []struct {
		input   string
		want    Profile
		wantErr bool
	}{
		{"production", ProfileProduction, false},
		{"prod", ProfileProduction, false},
		{"PRODUCTION", ProfileProduction, false},
		{"beta", ProfileBeta, false},
		{"test", ProfileBeta, false},
		{"staging", ProfileBeta, false},
		{"dev", ProfileDev, false},
		{"development", ProfileDev, false},
		{"", ProfileDev, false},
		{"  Dev  ", ProfileDev, false},
		{"unknown", "", true},
	}
	for _, tt := range tests {
		got, err := ParseProfile(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseProfile(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseProfile(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestApplyProfileDefaultsProduction(t *testing.T) {
	cfg := &Config{Profile: ProfileProduction}
	ApplyProfileDefaults(cfg)

	if cfg.Backend.LogLevel != "WARN" {
		t.Errorf("production log_level = %q, want WARN", cfg.Backend.LogLevel)
	}
	if cfg.Backend.APICacheTTL != 120*time.Second {
		t.Errorf("production api_cache_ttl = %v, want 120s", cfg.Backend.APICacheTTL)
	}
	if cfg.Backend.AllowInsecureInternalAPI {
		t.Error("production must force AllowInsecureInternalAPI = false")
	}
}

func TestApplyProfileDefaultsDev(t *testing.T) {
	cfg := &Config{Profile: ProfileDev}
	ApplyProfileDefaults(cfg)

	if cfg.Backend.LogLevel != "DEBUG" {
		t.Errorf("dev log_level = %q, want DEBUG", cfg.Backend.LogLevel)
	}
	if cfg.Backend.APICacheTTL != 10*time.Second {
		t.Errorf("dev api_cache_ttl = %v, want 10s", cfg.Backend.APICacheTTL)
	}
}

func TestApplyProfileDefaultsBeta(t *testing.T) {
	cfg := &Config{Profile: ProfileBeta}
	ApplyProfileDefaults(cfg)

	if cfg.Backend.LogLevel != "INFO" {
		t.Errorf("beta log_level = %q, want INFO", cfg.Backend.LogLevel)
	}
	if cfg.Backend.APICacheTTL != 60*time.Second {
		t.Errorf("beta api_cache_ttl = %v, want 60s", cfg.Backend.APICacheTTL)
	}
}

func TestApplyProfileDefaultsDoesNotOverrideExplicit(t *testing.T) {
	cfg := &Config{
		Profile: ProfileProduction,
		Backend: BackendConfig{
			LogLevel:    "DEBUG",
			APICacheTTL: 5 * time.Second,
		},
	}
	ApplyProfileDefaults(cfg)

	if cfg.Backend.LogLevel != "DEBUG" {
		t.Errorf("explicit log_level should be preserved, got %q", cfg.Backend.LogLevel)
	}
	if cfg.Backend.APICacheTTL != 5*time.Second {
		t.Errorf("explicit api_cache_ttl should be preserved, got %v", cfg.Backend.APICacheTTL)
	}
}

func TestApplyProfileDefaultsProductionForcesInsecureOff(t *testing.T) {
	cfg := &Config{
		Profile: ProfileProduction,
		Backend: BackendConfig{AllowInsecureInternalAPI: true},
	}
	ApplyProfileDefaults(cfg)

	if cfg.Backend.AllowInsecureInternalAPI {
		t.Error("production must force AllowInsecureInternalAPI = false even if YAML says true")
	}
}

func TestEnvOverrideProfile(t *testing.T) {
	t.Setenv("HARUKI_PROFILE", "production")
	cfg := &Config{}
	ApplyEnvOverrides(cfg)
	if cfg.Profile != ProfileProduction {
		t.Errorf("HARUKI_PROFILE override = %q, want production", cfg.Profile)
	}
}

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
