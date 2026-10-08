package config

import (
	"testing"
	"time"
)

func TestAliasAPICacheTTLDefaults(t *testing.T) {
	cfg := Config{Profile: ProfileProduction}
	ApplyProfileDefaults(&cfg)
	if cfg.Backend.AliasAPICacheTTL != 12*time.Hour || cfg.Backend.AliasAPINotFoundCacheTTL != time.Hour {
		t.Fatalf("alias TTL defaults = %v / %v", cfg.Backend.AliasAPICacheTTL, cfg.Backend.AliasAPINotFoundCacheTTL)
	}
	if cfg.Backend.AliasAPICacheTTL <= 6*time.Hour {
		t.Fatal("the alias TTL must outlast the 6h crawl interval")
	}

	explicit := Config{Backend: BackendConfig{AliasAPICacheTTL: time.Hour, AliasAPINotFoundCacheTTL: -1}}
	ApplyProfileDefaults(&explicit)
	if explicit.Backend.AliasAPICacheTTL != time.Hour || explicit.Backend.AliasAPINotFoundCacheTTL != -1 {
		t.Fatalf("explicit values must be kept: %+v", explicit.Backend)
	}

	t.Setenv("HARUKI_BACKEND_ALIAS_API_CACHE_TTL", "8h")
	t.Setenv("HARUKI_BACKEND_ALIAS_API_NOT_FOUND_CACHE_TTL", "10m")
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Backend.AliasAPICacheTTL != 8*time.Hour || cfg.Backend.AliasAPINotFoundCacheTTL != 10*time.Minute {
		t.Fatalf("env overrides = %v / %v", cfg.Backend.AliasAPICacheTTL, cfg.Backend.AliasAPINotFoundCacheTTL)
	}
}
