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
		{"temp", ProfileTemp, false},
		{"temporary", ProfileTemp, false},
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

func TestApplyProfileDefaultsTemp(t *testing.T) {
	cfg := &Config{Profile: ProfileTemp}
	ApplyProfileDefaults(cfg)

	if cfg.Backend.LogLevel != "INFO" {
		t.Errorf("temp log_level = %q, want INFO", cfg.Backend.LogLevel)
	}
	if cfg.Backend.APICacheTTL != 60*time.Second {
		t.Errorf("temp api_cache_ttl = %v, want 60s", cfg.Backend.APICacheTTL)
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

func TestApplyEnvOverridesTrackerHotProtection(t *testing.T) {
	t.Setenv("HARUKI_TRACKER_TRACE_BATCH_WINDOW", "150ms")
	t.Setenv("HARUKI_TRACKER_TRACE_BATCH_MAX_WAIT", "250ms")
	t.Setenv("HARUKI_TRACKER_TRACE_BATCH_FLUSH_RANKS", "8")
	t.Setenv("HARUKI_TRACKER_TRACE_LEADERBOARD_MAX_CONCURRENCY", "4")
	t.Setenv("HARUKI_TRACKER_LATEST_LEADERBOARD_MAX_CONCURRENCY", "8")
	t.Setenv("HARUKI_TRACKER_ACQUIRE_TIMEOUT", "300ms")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_SK_MAX_CONCURRENCY", "6")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_SK_ACQUIRE_TIMEOUT", "400ms")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_MAX_CONCURRENCY", "12")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	if cfg.Tracker.TraceBatchWindow != 150*time.Millisecond {
		t.Fatalf("unexpected trace batch window: %v", cfg.Tracker.TraceBatchWindow)
	}
	if cfg.Tracker.TraceBatchMaxWait != 250*time.Millisecond {
		t.Fatalf("unexpected trace batch max wait: %v", cfg.Tracker.TraceBatchMaxWait)
	}
	if cfg.Tracker.TraceBatchFlushRanks != 8 {
		t.Fatalf("unexpected trace batch flush ranks: %d", cfg.Tracker.TraceBatchFlushRanks)
	}
	if cfg.Tracker.TraceLeaderboardMaxConcurrency != 4 {
		t.Fatalf("unexpected trace leaderboard concurrency: %d", cfg.Tracker.TraceLeaderboardMaxConcurrency)
	}
	if cfg.Tracker.LatestLeaderboardMaxConcurrency != 8 {
		t.Fatalf("unexpected latest leaderboard concurrency: %d", cfg.Tracker.LatestLeaderboardMaxConcurrency)
	}
	if cfg.Tracker.AcquireTimeout != 300*time.Millisecond {
		t.Fatalf("unexpected tracker acquire timeout: %v", cfg.Tracker.AcquireTimeout)
	}
	if cfg.PJSKRender.DrawingSKMaxConcurrency != 6 {
		t.Fatalf("unexpected drawing SK concurrency: %d", cfg.PJSKRender.DrawingSKMaxConcurrency)
	}
	if cfg.PJSKRender.DrawingSKAcquireTimeout != 400*time.Millisecond {
		t.Fatalf("unexpected drawing SK acquire timeout: %v", cfg.PJSKRender.DrawingSKAcquireTimeout)
	}
	if cfg.PJSKRender.DrawingMaxConcurrency != 12 {
		t.Fatalf("unexpected drawing max concurrency: %d", cfg.PJSKRender.DrawingMaxConcurrency)
	}
}

func TestApplyEnvOverridesPJSKRenderDeckRecommendMasterdataDir(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_DECK_RECOMMEND_MASTERDATA_DIR", "/srv/masterdata")
	t.Setenv("HARUKI_PJSK_RENDER_MUSIC_META_REFRESH_INTERVAL", "45m")
	t.Setenv("HARUKI_PJSK_RENDER_MUSIC_META_OUTPUT_DIR", "/srv/masterdata")
	t.Setenv("HARUKI_PJSK_RENDER_SK_FORECAST_LOCAL_BASE_URL", "http://100.109.13.111:18746")
	t.Setenv("HARUKI_PJSK_RENDER_SK_FORECAST_CACHE_PATH", "/data/haruki/cache/sk_forecast_cache.json")
	t.Setenv("HARUKI_PJSK_RENDER_MYSEKAI_HOUSING_COMPETITION_CACHE_PATH", "/data/haruki/cache/housing_stats.json")
	t.Setenv("HARUKI_PJSK_RENDER_MYSEKAI_HOUSING_COMPETITION_REFRESH_INTERVAL", "10s")
	t.Setenv("HARUKI_PJSK_RENDER_DECK_RECOMMEND_SERVICE_BASE_URL", "http://127.0.0.1:48080")
	t.Setenv("HARUKI_PJSK_RENDER_DECK_RECOMMEND_MASTERDATA_REFRESH_INTERVAL", "5m")
	t.Setenv("HARUKI_PJSK_RENDER_DECK_RECOMMEND_DISABLE", "true")
	t.Setenv("HARUKI_PJSK_RENDER_DECK_RECOMMEND_DISABLE_REASON", "maintenance")
	t.Setenv("HARUKI_PJSK_RENDER_LOCAL_MASTERDATA_ENABLED", "true")
	t.Setenv("HARUKI_PJSK_RENDER_LOCAL_MASTERDATA_ALLOW_FALLBACK", "true")
	t.Setenv("HARUKI_PJSK_RENDER_LOCAL_MASTERDATA_REFRESH_INTERVAL", "6m")
	t.Setenv("HARUKI_PJSK_RENDER_LOCAL_MASTERDATA_ALLOW_LEAKS", "true")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_ENABLED", "true")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_ENGINE_BASE_URL", "http://127.0.0.1:38080")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_STATIC_RELATIVE_DIR", "static_images/pjsk_3d_preview")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_WIDTH", "1400")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_HEIGHT", "1000")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_SCALE", "2")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_TIMEOUT", "45s")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_REGISTRY_CACHE_TTL", "2m")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_CAPTURE_EXISTS_TTL", "45s")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_CAPTURE_MAX_CONCURRENCY", "2")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_CAPTURE_ACQUIRE_TIMEOUT", "3s")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_TEMPORARY_CAPTURE_TTL", "24h")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_CAPTURE_CACHE_VERSION", "preview-v2")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_CAMERA_PRESET", "capture")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	if cfg.PJSKRender.DeckRecommend.MasterdataDir != "/srv/masterdata" {
		t.Fatalf("unexpected masterdata dir override: %q", cfg.PJSKRender.DeckRecommend.MasterdataDir)
	}
	if cfg.PJSKRender.DeckRecommend.ServiceBaseURL != "http://127.0.0.1:48080" {
		t.Fatalf("unexpected deck service url override: %q", cfg.PJSKRender.DeckRecommend.ServiceBaseURL)
	}
	if cfg.PJSKRender.DeckRecommend.MasterdataRefreshInterval != 5*time.Minute {
		t.Fatalf("unexpected deck masterdata refresh interval: %v", cfg.PJSKRender.DeckRecommend.MasterdataRefreshInterval)
	}
	if !cfg.PJSKRender.DeckRecommend.Disable {
		t.Fatalf("expected deck recommend disable override to be true")
	}
	if cfg.PJSKRender.DeckRecommend.DisableReason != "maintenance" {
		t.Fatalf("unexpected deck recommend disable reason: %q", cfg.PJSKRender.DeckRecommend.DisableReason)
	}
	if cfg.PJSKRender.LocalMasterdata.RefreshInterval != 6*time.Minute {
		t.Fatalf("unexpected local masterdata refresh interval: %v", cfg.PJSKRender.LocalMasterdata.RefreshInterval)
	}
	if !cfg.PJSKRender.LocalMasterdata.Enabled {
		t.Fatalf("expected local masterdata enabled override to be true")
	}
	if !cfg.PJSKRender.LocalMasterdata.AllowFallback {
		t.Fatalf("expected local masterdata allow_fallback override to be true")
	}
	if cfg.PJSKRender.MusicMeta.RefreshInterval != 45*time.Minute {
		t.Fatalf("unexpected music meta refresh interval: %v", cfg.PJSKRender.MusicMeta.RefreshInterval)
	}
	if cfg.PJSKRender.MusicMeta.OutputDir != "/srv/masterdata" {
		t.Fatalf("unexpected music meta output dir override: %q", cfg.PJSKRender.MusicMeta.OutputDir)
	}
	if cfg.PJSKRender.SKForecast.LocalBaseURL != "http://100.109.13.111:18746" {
		t.Fatalf("unexpected sk forecast local base url override: %q", cfg.PJSKRender.SKForecast.LocalBaseURL)
	}
	if cfg.PJSKRender.SKForecast.CachePath != "/data/haruki/cache/sk_forecast_cache.json" {
		t.Fatalf("unexpected sk forecast cache path override: %q", cfg.PJSKRender.SKForecast.CachePath)
	}
	if cfg.PJSKRender.MySekaiHousingCompetition.CachePath != "/data/haruki/cache/housing_stats.json" {
		t.Fatalf("unexpected mysekai housing competition cache path override: %q", cfg.PJSKRender.MySekaiHousingCompetition.CachePath)
	}
	if cfg.PJSKRender.MySekaiHousingCompetition.RefreshInterval != 10*time.Second {
		t.Fatalf("unexpected mysekai housing competition refresh interval: %v", cfg.PJSKRender.MySekaiHousingCompetition.RefreshInterval)
	}
	if !cfg.PJSKRender.LocalMasterdata.AllowLeaks {
		t.Fatalf("expected local masterdata allow_leaks override to be true")
	}
	if !cfg.PJSKRender.Preview3D.Enabled {
		t.Fatalf("expected 3d preview enabled override to be true")
	}
	if cfg.PJSKRender.Preview3D.EngineBaseURL != "http://127.0.0.1:38080" {
		t.Fatalf("unexpected 3d preview engine url: %q", cfg.PJSKRender.Preview3D.EngineBaseURL)
	}
	if cfg.PJSKRender.Preview3D.StaticRelativeDir != "static_images/pjsk_3d_preview" {
		t.Fatalf("unexpected 3d preview static dir: %q", cfg.PJSKRender.Preview3D.StaticRelativeDir)
	}
	if cfg.PJSKRender.Preview3D.Width != 1400 || cfg.PJSKRender.Preview3D.Height != 1000 {
		t.Fatalf("unexpected 3d preview size: %dx%d", cfg.PJSKRender.Preview3D.Width, cfg.PJSKRender.Preview3D.Height)
	}
	if cfg.PJSKRender.Preview3D.Scale != 2 {
		t.Fatalf("unexpected 3d preview scale: %v", cfg.PJSKRender.Preview3D.Scale)
	}
	if cfg.PJSKRender.Preview3D.Timeout != 45*time.Second {
		t.Fatalf("unexpected 3d preview timeout: %v", cfg.PJSKRender.Preview3D.Timeout)
	}
	if cfg.PJSKRender.Preview3D.RegistryCacheTTL != 2*time.Minute {
		t.Fatalf("unexpected 3d preview registry cache ttl: %v", cfg.PJSKRender.Preview3D.RegistryCacheTTL)
	}
	if cfg.PJSKRender.Preview3D.CaptureExistsTTL != 45*time.Second {
		t.Fatalf("unexpected 3d preview capture exists ttl: %v", cfg.PJSKRender.Preview3D.CaptureExistsTTL)
	}
	if cfg.PJSKRender.Preview3D.CaptureMaxConcurrency != 2 {
		t.Fatalf("unexpected 3d preview capture max concurrency: %d", cfg.PJSKRender.Preview3D.CaptureMaxConcurrency)
	}
	if cfg.PJSKRender.Preview3D.CaptureAcquireTimeout != 3*time.Second {
		t.Fatalf("unexpected 3d preview capture acquire timeout: %v", cfg.PJSKRender.Preview3D.CaptureAcquireTimeout)
	}
	if cfg.PJSKRender.Preview3D.TemporaryCaptureTTL != 24*time.Hour {
		t.Fatalf("unexpected 3d preview temporary capture ttl: %v", cfg.PJSKRender.Preview3D.TemporaryCaptureTTL)
	}
	if cfg.PJSKRender.Preview3D.CaptureCacheVersion != "preview-v2" {
		t.Fatalf("unexpected 3d preview capture cache version: %q", cfg.PJSKRender.Preview3D.CaptureCacheVersion)
	}
	if cfg.PJSKRender.Preview3D.CameraPreset != "capture" {
		t.Fatalf("unexpected 3d preview camera preset: %q", cfg.PJSKRender.Preview3D.CameraPreset)
	}
}

func TestApplyEnvOverridesSekaiRemoteSync(t *testing.T) {
	t.Setenv("HARUKI_SEKAI_DB_SYNC_ENABLED", "true")
	t.Setenv("HARUKI_SEKAI_DB_SYNC_SOURCE_DB_TYPE", "postgres")
	t.Setenv("HARUKI_SEKAI_DB_SYNC_SOURCE_DB_URL", "host=remote port=5432 user=sekai dbname=haruki_sekai sslmode=disable")
	t.Setenv("HARUKI_SEKAI_DB_SYNC_INTERVAL", "15m")
	t.Setenv("HARUKI_SEKAI_DB_SYNC_TIMEOUT", "2m")
	t.Setenv("HARUKI_SEKAI_DB_SYNC_INITIAL", "true")
	t.Setenv("HARUKI_SEKAI_DB_SYNC_FAIL_STARTUP", "true")
	t.Setenv("HARUKI_SEKAI_DB_SYNC_PG_DUMP_PATH", "/usr/bin/pg_dump")
	t.Setenv("HARUKI_SEKAI_DB_SYNC_PG_RESTORE_PATH", "/usr/bin/pg_restore")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	sync := cfg.Sekai.RemoteSync
	if !sync.Enabled {
		t.Fatalf("expected sekai remote sync to be enabled")
	}
	if sync.SourceDBType != "postgres" {
		t.Fatalf("unexpected source db type: %q", sync.SourceDBType)
	}
	if sync.SourceDBURL == "" {
		t.Fatalf("expected source db url override")
	}
	if sync.Interval != 15*time.Minute {
		t.Fatalf("unexpected sync interval: %v", sync.Interval)
	}
	if sync.Timeout != 2*time.Minute {
		t.Fatalf("unexpected sync timeout: %v", sync.Timeout)
	}
	if !sync.Initial {
		t.Fatalf("expected initial sync override to be true")
	}
	if !sync.FailStartup {
		t.Fatalf("expected fail_startup override to be true")
	}
	if sync.PgDumpPath != "/usr/bin/pg_dump" {
		t.Fatalf("unexpected pg_dump path: %q", sync.PgDumpPath)
	}
	if sync.PgRestorePath != "/usr/bin/pg_restore" {
		t.Fatalf("unexpected pg_restore path: %q", sync.PgRestorePath)
	}
}

func TestApplyEnvOverridesPJSKRenderChartsURI(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_CHARTS_URI", "https://public-beta-image-cache-sha01-direct.example.haruki.local:40011/charts")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	if cfg.PJSKRender.ImageCache.ChartsURI != "https://public-beta-image-cache-sha01-direct.example.haruki.local:40011/charts" {
		t.Fatalf("unexpected charts uri override: %q", cfg.PJSKRender.ImageCache.ChartsURI)
	}
}

func TestApplyEnvOverridesDrawingCache(t *testing.T) {
	t.Setenv("CACHE_STORAGE_DIR", "/legacy/cache")
	t.Setenv("CACHE_DB_PATH", "/legacy/cache/cache.db")
	t.Setenv("CACHE_GC_INTERVAL", "12h")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_BASE_URL", "http://haruki-cloud:6666")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_STORAGE_DIR", "/data/drawing-cache")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_DB_PATH", "/data/drawing-cache/cache.db")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_TTL", "10m")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_GC_INTERVAL", "24h")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	if cfg.PJSKRender.DrawingCache.BaseURL != "http://haruki-cloud:6666" {
		t.Fatalf("unexpected drawing cache base url: %q", cfg.PJSKRender.DrawingCache.BaseURL)
	}
	if cfg.PJSKRender.DrawingCache.StorageDir != "/data/drawing-cache" {
		t.Fatalf("unexpected drawing cache storage dir: %q", cfg.PJSKRender.DrawingCache.StorageDir)
	}
	if cfg.PJSKRender.DrawingCache.DBPath != "/data/drawing-cache/cache.db" {
		t.Fatalf("unexpected drawing cache db path: %q", cfg.PJSKRender.DrawingCache.DBPath)
	}
	if cfg.PJSKRender.DrawingCache.TTL != 10*time.Minute {
		t.Fatalf("unexpected drawing cache ttl: %v", cfg.PJSKRender.DrawingCache.TTL)
	}
	if cfg.PJSKRender.DrawingCache.GCInterval != 24*time.Hour {
		t.Fatalf("unexpected drawing cache gc interval: %v", cfg.PJSKRender.DrawingCache.GCInterval)
	}
}
