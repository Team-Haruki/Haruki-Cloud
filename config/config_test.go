package config

import (
	"haruki-cloud/internal/testutil"
	"os"
	"path/filepath"
	"strings"
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
		testutil.Check(t, !(got != tt.want), "ParseProfile(%q) = %q, want %q", tt.input, got, tt.want)

	}
}

func TestApplyProfileDefaultsProduction(t *testing.T) {
	cfg := &Config{Profile: ProfileProduction}
	ApplyProfileDefaults(cfg)
	testutil.Check(t, !(cfg.Backend.LogLevel != "WARN"), "production log_level = %q, want WARN", cfg.Backend.LogLevel)
	testutil.Check(t, !(cfg.Backend.APICacheTTL != 120*time.Second), "production api_cache_ttl = %v, want 120s", cfg.Backend.APICacheTTL)
	testutil.CheckArgs(t, !(cfg.Backend.AllowInsecureInternalAPI), "production must force AllowInsecureInternalAPI = false")

}

func TestApplyProfileDefaultsDev(t *testing.T) {
	cfg := &Config{Profile: ProfileDev}
	ApplyProfileDefaults(cfg)
	testutil.Check(t, !(cfg.Backend.LogLevel != "DEBUG"), "dev log_level = %q, want DEBUG", cfg.Backend.LogLevel)
	testutil.Check(t, !(cfg.Backend.APICacheTTL != 10*time.Second), "dev api_cache_ttl = %v, want 10s", cfg.Backend.APICacheTTL)

}

func TestApplyProfileDefaultsBeta(t *testing.T) {
	cfg := &Config{Profile: ProfileBeta}
	ApplyProfileDefaults(cfg)
	testutil.Check(t, !(cfg.Backend.LogLevel != "INFO"), "beta log_level = %q, want INFO", cfg.Backend.LogLevel)
	testutil.Check(t, !(cfg.Backend.APICacheTTL != 60*time.Second), "beta api_cache_ttl = %v, want 60s", cfg.Backend.APICacheTTL)

}

func TestApplyProfileDefaultsTemp(t *testing.T) {
	cfg := &Config{Profile: ProfileTemp}
	ApplyProfileDefaults(cfg)
	testutil.Check(t, !(cfg.Backend.LogLevel != "INFO"), "temp log_level = %q, want INFO", cfg.Backend.LogLevel)
	testutil.Check(t, !(cfg.Backend.APICacheTTL != 60*time.Second), "temp api_cache_ttl = %v, want 60s", cfg.Backend.APICacheTTL)

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
	testutil.Check(t, !(cfg.Backend.LogLevel != "DEBUG"), "explicit log_level should be preserved, got %q", cfg.Backend.LogLevel)
	testutil.Check(t, !(cfg.Backend.APICacheTTL != 5*time.Second), "explicit api_cache_ttl should be preserved, got %v", cfg.Backend.APICacheTTL)

}

func TestApplyProfileDefaultsProductionForcesInsecureOff(t *testing.T) {
	cfg := &Config{
		Profile: ProfileProduction,
		Backend: BackendConfig{AllowInsecureInternalAPI: true},
	}
	ApplyProfileDefaults(cfg)
	testutil.CheckArgs(t, !(cfg.Backend.AllowInsecureInternalAPI), "production must force AllowInsecureInternalAPI = false even if YAML says true")

}

func TestEnvOverrideProfile(t *testing.T) {
	t.Setenv("HARUKI_PROFILE", "production")
	cfg := &Config{}
	ApplyEnvOverrides(cfg)
	testutil.Check(t, !(cfg.Profile != ProfileProduction), "HARUKI_PROFILE override = %q, want production", cfg.Profile)

}

func TestApplyEnvOverridesModerationAdminQQIDs(t *testing.T) {
	t.Setenv("HARUKI_MODERATION_ADMIN_QQ_IDS", "3164679932, 123456789")
	cfg := &Config{}
	{
		err := ApplyEnvOverrides(cfg)
		testutil.Require(t, !(err != nil), "ApplyEnvOverrides() error = %v", err)
	}
	{

		testutil.Require(t, !(len(cfg.Moderation.AdminQQIDs) != 2), "unexpected moderation admins: %#v", cfg.Moderation.AdminQQIDs)
		testutil.Require(t, !(cfg.Moderation.AdminQQIDs[0] != "3164679932"), "unexpected moderation admins: %#v", cfg.Moderation.AdminQQIDs)
		testutil.Require(t, !(cfg.Moderation.AdminQQIDs[1] != "123456789"), "unexpected moderation admins: %#v", cfg.Moderation.AdminQQIDs)
	}

}

func TestReadConfigModerationAdminQQIDs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	data := []byte("profile: dev\nmoderation:\n  admin_qq_ids: [\"3164679932\"]\n")
	{
		err := os.WriteFile(configPath, data, 0o600)
		testutil.Require(t, !(err != nil), "write config: %v", err)
	}

	cfg, err := ReadConfig(configPath)
	testutil.Require(t, !(err != nil), "ReadConfig() error = %v", err)
	{
		testutil.Require(t, !(len(cfg.Moderation.AdminQQIDs) != 1), "unexpected moderation admins: %#v", cfg.Moderation.AdminQQIDs)
		testutil.Require(t, !(cfg.Moderation.AdminQQIDs[0] != "3164679932"), "unexpected moderation admins: %#v", cfg.Moderation.AdminQQIDs)
	}

}

func TestApplyEnvOverridesResponseElectionWindow(t *testing.T) {
	t.Setenv("HARUKI_BOT_RESPONSE_ELECTION_WINDOW", "275ms")
	cfg := &Config{}
	{

		err := ApplyEnvOverrides(cfg)
		testutil.Require(t, !(err != nil), "ApplyEnvOverrides() error = %v", err)
	}
	{

		got := cfg.HarukiBotDB.ResponseElectionWindow
		testutil.Require(t, !(got != 275*time.Millisecond), "response election window = %v, want 275ms", got)
	}

}

func TestApplyEnvOverridesResponseElectionWindowPreservesNegativeDisable(t *testing.T) {
	t.Setenv("HARUKI_BOT_RESPONSE_ELECTION_WINDOW", "-1ms")
	cfg := &Config{}
	{

		err := ApplyEnvOverrides(cfg)
		testutil.Require(t, !(err != nil), "ApplyEnvOverrides() error = %v", err)
	}
	{

		got := cfg.HarukiBotDB.ResponseElectionWindow
		testutil.Require(t, !(got != -time.Millisecond), "response election window = %v, want -1ms", got)
	}

}

func TestReadConfigResponseElectionWindow(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	data := []byte("profile: dev\nharuki_bot:\n  response_election_window: 350ms\n")
	{
		err := os.WriteFile(configPath, data, 0o600)
		testutil.Require(t, !(err != nil), "write config: %v", err)
	}

	cfg, err := ReadConfig(configPath)
	testutil.Require(t, !(err != nil), "ReadConfig() error = %v", err)
	{

		got := cfg.HarukiBotDB.ResponseElectionWindow
		testutil.Require(t, !(got != 350*time.Millisecond), "response election window = %v, want 350ms", got)
	}

}

func TestResponseElectionWindowZeroRemainsRuntimeDefaultSentinel(t *testing.T) {
	cfg := &Config{Profile: ProfileDev}
	ApplyProfileDefaults(cfg)
	{
		got := cfg.HarukiBotDB.ResponseElectionWindow
		testutil.Require(t, !(got != 0), "response election window = %v, want zero runtime-default sentinel", got)
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
	testutil.Require(t, !(cfg.Tracker.TraceBatchWindow != 150*time.Millisecond), "unexpected trace batch window: %v", cfg.Tracker.TraceBatchWindow)
	testutil.Require(t, !(cfg.Tracker.TraceBatchMaxWait != 250*time.Millisecond), "unexpected trace batch max wait: %v", cfg.Tracker.TraceBatchMaxWait)
	testutil.Require(t, !(cfg.Tracker.TraceBatchFlushRanks != 8), "unexpected trace batch flush ranks: %d", cfg.Tracker.TraceBatchFlushRanks)
	testutil.Require(t, !(cfg.Tracker.TraceLeaderboardMaxConcurrency != 4), "unexpected trace leaderboard concurrency: %d", cfg.Tracker.TraceLeaderboardMaxConcurrency)
	testutil.Require(t, !(cfg.Tracker.LatestLeaderboardMaxConcurrency != 8), "unexpected latest leaderboard concurrency: %d", cfg.Tracker.LatestLeaderboardMaxConcurrency)
	testutil.Require(t, !(cfg.Tracker.AcquireTimeout != 300*time.Millisecond), "unexpected tracker acquire timeout: %v", cfg.Tracker.AcquireTimeout)
	testutil.Require(t, !(cfg.PJSKRender.DrawingSKMaxConcurrency != 6), "unexpected drawing SK concurrency: %d", cfg.PJSKRender.DrawingSKMaxConcurrency)
	testutil.Require(t, !(cfg.PJSKRender.DrawingSKAcquireTimeout != 400*time.Millisecond), "unexpected drawing SK acquire timeout: %v", cfg.PJSKRender.DrawingSKAcquireTimeout)
	testutil.Require(t, !(cfg.PJSKRender.DrawingMaxConcurrency != 12), "unexpected drawing max concurrency: %d", cfg.PJSKRender.DrawingMaxConcurrency)

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
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_ENGINE_BASE_URLS", `{"jp":"http://jp-engine:8080","cn":"http://cn-engine:8080"}`)
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_STATIC_RELATIVE_DIR", "static_images/pjsk_3d_preview")
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_STATIC_OUTPUT_DIR", "/data/haruki/drawing/static_images/pjsk_3d_preview")
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
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_CAMERA_PROFILE", "official-default")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)
	testutil.Require(t, !(cfg.PJSKRender.DeckRecommend.MasterdataDir != "/srv/masterdata"), "unexpected masterdata dir override: %q", cfg.PJSKRender.DeckRecommend.MasterdataDir)
	testutil.Require(t, !(cfg.PJSKRender.DeckRecommend.ServiceBaseURL != "http://127.0.0.1:48080"), "unexpected deck service url override: %q", cfg.PJSKRender.DeckRecommend.ServiceBaseURL)
	testutil.Require(t, !(cfg.PJSKRender.DeckRecommend.MasterdataRefreshInterval != 5*time.Minute), "unexpected deck masterdata refresh interval: %v", cfg.PJSKRender.DeckRecommend.MasterdataRefreshInterval)
	testutil.Require(t, cfg.PJSKRender.DeckRecommend.Disable, "expected deck recommend disable override to be true")
	testutil.Require(t, !(cfg.PJSKRender.DeckRecommend.DisableReason != "maintenance"), "unexpected deck recommend disable reason: %q", cfg.PJSKRender.DeckRecommend.DisableReason)
	testutil.Require(t, !(cfg.PJSKRender.LocalMasterdata.RefreshInterval != 6*time.Minute), "unexpected local masterdata refresh interval: %v", cfg.PJSKRender.LocalMasterdata.RefreshInterval)
	testutil.Require(t, cfg.PJSKRender.LocalMasterdata.Enabled, "expected local masterdata enabled override to be true")
	testutil.Require(t, cfg.PJSKRender.LocalMasterdata.AllowFallback, "expected local masterdata allow_fallback override to be true")
	testutil.Require(t, !(cfg.PJSKRender.MusicMeta.RefreshInterval != 45*time.Minute), "unexpected music meta refresh interval: %v", cfg.PJSKRender.MusicMeta.RefreshInterval)
	testutil.Require(t, !(cfg.PJSKRender.MusicMeta.OutputDir != "/srv/masterdata"), "unexpected music meta output dir override: %q", cfg.PJSKRender.MusicMeta.OutputDir)
	testutil.Require(t, !(cfg.PJSKRender.SKForecast.LocalBaseURL != "http://100.109.13.111:18746"), "unexpected sk forecast local base url override: %q", cfg.PJSKRender.SKForecast.LocalBaseURL)
	testutil.Require(t, !(cfg.PJSKRender.SKForecast.CachePath != "/data/haruki/cache/sk_forecast_cache.json"), "unexpected sk forecast cache path override: %q", cfg.PJSKRender.SKForecast.CachePath)
	testutil.Require(t, !(cfg.PJSKRender.MySekaiHousingCompetition.CachePath != "/data/haruki/cache/housing_stats.json"), "unexpected mysekai housing competition cache path override: %q", cfg.PJSKRender.MySekaiHousingCompetition.CachePath)
	testutil.Require(t, !(cfg.PJSKRender.MySekaiHousingCompetition.RefreshInterval != 10*time.Second), "unexpected mysekai housing competition refresh interval: %v", cfg.PJSKRender.MySekaiHousingCompetition.RefreshInterval)
	testutil.Require(t, cfg.PJSKRender.LocalMasterdata.AllowLeaks, "expected local masterdata allow_leaks override to be true")
	testutil.Require(t, cfg.PJSKRender.Preview3D.Enabled, "expected 3d preview enabled override to be true")
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.EngineBaseURL != "http://127.0.0.1:38080"), "unexpected 3d preview engine url: %q", cfg.PJSKRender.Preview3D.EngineBaseURL)
	{

		got := cfg.PJSKRender.Preview3D.EngineBaseURLs["cn"]
		testutil.Require(t, !(got != "http://cn-engine:8080"), "unexpected cn 3d preview engine url: %q", got)
	}

	testutil.Require(t, !(cfg.PJSKRender.Preview3D.StaticRelativeDir != "static_images/pjsk_3d_preview"), "unexpected 3d preview static dir: %q", cfg.PJSKRender.Preview3D.StaticRelativeDir)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.StaticOutputDir != "/data/haruki/drawing/static_images/pjsk_3d_preview"), "unexpected 3d preview static output dir: %q", cfg.PJSKRender.Preview3D.StaticOutputDir)
	{
		testutil.Require(t, !(cfg.PJSKRender.Preview3D.Width != 1400), "unexpected 3d preview size: %dx%d", cfg.PJSKRender.Preview3D.Width, cfg.PJSKRender.Preview3D.Height)
		testutil.Require(t, !(cfg.PJSKRender.Preview3D.Height != 1000), "unexpected 3d preview size: %dx%d", cfg.PJSKRender.Preview3D.Width, cfg.PJSKRender.Preview3D.Height)
	}
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.Scale != 2), "unexpected 3d preview scale: %v", cfg.PJSKRender.Preview3D.Scale)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.Timeout != 45*time.Second), "unexpected 3d preview timeout: %v", cfg.PJSKRender.Preview3D.Timeout)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.RegistryCacheTTL != 2*time.Minute), "unexpected 3d preview registry cache ttl: %v", cfg.PJSKRender.Preview3D.RegistryCacheTTL)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.CaptureExistsTTL != 45*time.Second), "unexpected 3d preview capture exists ttl: %v", cfg.PJSKRender.Preview3D.CaptureExistsTTL)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.CaptureMaxConcurrency != 2), "unexpected 3d preview capture max concurrency: %d", cfg.PJSKRender.Preview3D.CaptureMaxConcurrency)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.CaptureAcquireTimeout != 3*time.Second), "unexpected 3d preview capture acquire timeout: %v", cfg.PJSKRender.Preview3D.CaptureAcquireTimeout)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.TemporaryCaptureTTL != 24*time.Hour), "unexpected 3d preview temporary capture ttl: %v", cfg.PJSKRender.Preview3D.TemporaryCaptureTTL)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.CaptureCacheVersion != "preview-v2"), "unexpected 3d preview capture cache version: %q", cfg.PJSKRender.Preview3D.CaptureCacheVersion)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.CameraPreset != "capture"), "unexpected 3d preview camera preset: %q", cfg.PJSKRender.Preview3D.CameraPreset)
	testutil.Require(t, !(cfg.PJSKRender.Preview3D.CameraProfile != "official-default"), "unexpected 3d preview camera profile: %q", cfg.PJSKRender.Preview3D.CameraProfile)

}

func TestReadConfigRejectsMalformedPreview3DEngineBaseURLs(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_3D_PREVIEW_ENGINE_BASE_URLS", `{"jp":`)
	configPath := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	{
		err := os.WriteFile(configPath, []byte("{}\n"), 0o600)
		testutil.Require(t, !(err != nil), "write config: %v", err)
	}

	_, err := ReadConfig(configPath)
	testutil.RequireArgs(t, !(err == nil), "expected malformed 3d preview engine map to be rejected")
	{

		got := err.Error()
		testutil.Require(t, strings.Contains(got, "HARUKI_PJSK_RENDER_3D_PREVIEW_ENGINE_BASE_URLS"), "expected environment variable name in error, got %q", got)
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
	testutil.Require(t, sync.Enabled, "expected sekai remote sync to be enabled")
	testutil.Require(t, !(sync.SourceDBType != "postgres"), "unexpected source db type: %q", sync.SourceDBType)
	testutil.Require(t, !(sync.SourceDBURL == ""), "expected source db url override")
	testutil.Require(t, !(sync.Interval != 15*time.Minute), "unexpected sync interval: %v", sync.Interval)
	testutil.Require(t, !(sync.Timeout != 2*time.Minute), "unexpected sync timeout: %v", sync.Timeout)
	testutil.Require(t, sync.Initial, "expected initial sync override to be true")
	testutil.Require(t, sync.FailStartup, "expected fail_startup override to be true")
	testutil.Require(t, !(sync.PgDumpPath != "/usr/bin/pg_dump"), "unexpected pg_dump path: %q", sync.PgDumpPath)
	testutil.Require(t, !(sync.PgRestorePath != "/usr/bin/pg_restore"), "unexpected pg_restore path: %q", sync.PgRestorePath)

}

func TestApplyEnvOverridesPJSKRenderChartsURI(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_CHARTS_URI", "https://public-beta-image-cache-sha01-direct.example.haruki.local:40011/charts")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)
	testutil.Require(t, !(cfg.PJSKRender.ImageCache.ChartsURI != "https://public-beta-image-cache-sha01-direct.example.haruki.local:40011/charts"), "unexpected charts uri override: %q", cfg.PJSKRender.ImageCache.ChartsURI)

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
	testutil.Require(t, !(cfg.PJSKRender.DrawingCache.BaseURL != "http://haruki-cloud:6666"), "unexpected drawing cache base url: %q", cfg.PJSKRender.DrawingCache.BaseURL)
	testutil.Require(t, !(cfg.PJSKRender.DrawingCache.StorageDir != "/data/drawing-cache"), "unexpected drawing cache storage dir: %q", cfg.PJSKRender.DrawingCache.StorageDir)
	testutil.Require(t, !(cfg.PJSKRender.DrawingCache.DBPath != "/data/drawing-cache/cache.db"), "unexpected drawing cache db path: %q", cfg.PJSKRender.DrawingCache.DBPath)
	testutil.Require(t, !(cfg.PJSKRender.DrawingCache.TTL != 10*time.Minute), "unexpected drawing cache ttl: %v", cfg.PJSKRender.DrawingCache.TTL)
	testutil.Require(t, !(cfg.PJSKRender.DrawingCache.GCInterval != 24*time.Hour), "unexpected drawing cache gc interval: %v", cfg.PJSKRender.DrawingCache.GCInterval)

}
