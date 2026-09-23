package config

import (
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

func TestApplyEnvOverridesDrawingCache(t *testing.T) {
	t.Setenv("CACHE_STORAGE_DIR", "/legacy/cache")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_STORAGE_DIR", "/data/drawing-cache")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_CACHE_TTL", "10m")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)
	testutil.Require(t, !(cfg.PJSKRender.DrawingCache.StorageDir != "/data/drawing-cache"), "unexpected drawing cache storage dir: %q", cfg.PJSKRender.DrawingCache.StorageDir)
	testutil.Require(t, !(cfg.PJSKRender.DrawingCache.TTL != 10*time.Minute), "unexpected drawing cache ttl: %v", cfg.PJSKRender.DrawingCache.TTL)

}

func TestApplyEnvOverridesPJSKRenderAssetProbe(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_ASSET_PROBE_POSITIVE_TTL", "2h")
	t.Setenv("HARUKI_PJSK_RENDER_ASSET_PROBE_NEGATIVE_TTL", "3m")
	t.Setenv("HARUKI_PJSK_RENDER_ASSET_PROBE_TIMEOUT", "1500ms")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)
	testutil.Require(t, cfg.PJSKRender.AssetProbe.PositiveTTL == 2*time.Hour, "unexpected asset probe positive ttl: %v", cfg.PJSKRender.AssetProbe.PositiveTTL)
	testutil.Require(t, cfg.PJSKRender.AssetProbe.NegativeTTL == 3*time.Minute, "unexpected asset probe negative ttl: %v", cfg.PJSKRender.AssetProbe.NegativeTTL)
	testutil.Require(t, cfg.PJSKRender.AssetProbe.Timeout == 1500*time.Millisecond, "unexpected asset probe timeout: %v", cfg.PJSKRender.AssetProbe.Timeout)
}

func TestApplyEnvOverridesPJSKRenderImageCacheAndDrawing(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_URI", "https://image-cache.example")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_DIR", "/data/imagecache")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_TIMEOUT", "45s")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_RETRY_COUNT", "4")

	cfg := &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	render := cfg.PJSKRender
	testutil.Require(t, render.ImageCache.URI == "https://image-cache.example", "image cache uri = %q", render.ImageCache.URI)
	testutil.Require(t, render.ImageCache.Dir == "/data/imagecache", "image cache dir = %q", render.ImageCache.Dir)
	testutil.Require(t, render.DrawingTimeout == 45*time.Second, "drawing timeout = %v", render.DrawingTimeout)
	testutil.Require(t, render.DrawingRetryCount == 4, "drawing retry count = %d", render.DrawingRetryCount)
}

func TestApplyEnvOverridesStorageSlots(t *testing.T) {
	for _, slot := range storage.Slots {
		t.Run(string(slot), func(t *testing.T) {
			prefix := "HARUKI_PJSK_RENDER_STORAGE_" + strings.ToUpper(string(slot))
			t.Setenv(prefix+"_PROVIDER", "garage")
			t.Setenv(prefix+"_SCHEME", "s3")
			t.Setenv(prefix+"_ENDPOINT", "http://single:3900")
			t.Setenv(prefix+"_ENDPOINTS", "http://a:3900, http://b:3900")
			t.Setenv(prefix+"_TLS", "false")
			t.Setenv(prefix+"_BUCKET", "bucket-"+string(slot))
			t.Setenv(prefix+"_ROOT", "root")
			t.Setenv(prefix+"_PREFIX", "prefix")
			t.Setenv(prefix+"_REGION", "garage")
			t.Setenv(prefix+"_ACCESS_KEY_ID", "AK")
			t.Setenv(prefix+"_SECRET_ACCESS_KEY", "SK")
			t.Setenv(prefix+"_PUBLIC_READ", "true")
			t.Setenv(prefix+"_PATH_STYLE", "true")
			t.Setenv(prefix+"_BASE_URL", "https://cdn.example")
			t.Setenv(prefix+"_OPTIONS", "request_timeout=10s,proxy=none")

			cfg := &Config{}
			testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
			got := cfg.PJSKRender.Storage.Provider(slot)
			testutil.Require(t, got.Provider == "garage" && got.Scheme == "s3" && got.Endpoint == "http://single:3900", "identity = %+v", got)
			testutil.Require(t, len(got.Endpoints) == 2 && got.Endpoints[1] == "http://b:3900", "endpoints = %#v", got.Endpoints)
			testutil.Require(t, got.TLS != nil && !*got.TLS, "tls = %v", got.TLS)
			testutil.Require(t, got.Bucket == "bucket-"+string(slot) && got.Root == "root" && got.Prefix == "prefix" && got.Region == "garage", "location = %+v", got)
			testutil.Require(t, got.AccessKeyID == "AK" && got.SecretAccessKey == "SK", "credentials not applied")
			testutil.Require(t, got.PublicRead && got.PathStyle != nil && *got.PathStyle, "flags = %+v", got)
			testutil.Require(t, got.BaseURL == "https://cdn.example", "base url = %q", got.BaseURL)
			testutil.Require(t, got.Options["request_timeout"] == "10s" && got.Options["proxy"] == "none", "options = %#v", got.Options)
			testutil.Require(t, got.Mirror == nil, "mirror unexpectedly created")
			for _, other := range storage.Slots {
				if other != slot {
					testutil.Require(t, cfg.PJSKRender.Storage.Provider(other).IsZero(), "slot %s touched", other)
				}
			}
		})
	}
}

func TestApplyEnvOverridesStorageUserUploadMirror(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_MIRROR_SCHEME", "fs")
	t.Setenv("HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_MIRROR_ROOT", "/asset")
	t.Setenv("HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_MIRROR_BUCKET", "user-upload")
	t.Setenv("HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_MIRROR_ENDPOINTS", `["http://a:3900"]`)
	t.Setenv("HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_MIRROR_MODE", "write_only")

	cfg := &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	upload := cfg.PJSKRender.Storage.UserUpload
	testutil.Require(t, upload.Mirror != nil, "mirror not created")
	testutil.Require(t, upload.Mirror.Scheme == "fs" && upload.Mirror.Root == "/asset" && upload.Mirror.Bucket == "user-upload", "mirror = %+v", upload.Mirror)
	testutil.Require(t, len(upload.Mirror.Endpoints) == 1, "mirror endpoints = %#v", upload.Mirror.Endpoints)
	testutil.Require(t, upload.MirrorMode == "write_only", "mirror mode = %q", upload.MirrorMode)
}

func TestApplyEnvOverridesStorageMirrorKeepsYAMLMirror(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_MIRROR_ROOT", "/override")
	cfg := &Config{}
	cfg.PJSKRender.Storage.UserUpload.Mirror = &storage.ProviderConfig{Scheme: "fs", Root: "/asset"}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	mirror := cfg.PJSKRender.Storage.UserUpload.Mirror
	testutil.Require(t, mirror != nil && mirror.Scheme == "fs" && mirror.Root == "/override", "mirror = %+v", mirror)

	cfg = &Config{}
	t.Setenv("HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_MIRROR_ROOT", "")
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	testutil.Require(t, cfg.PJSKRender.Storage.UserUpload.Mirror == nil, "empty env created a mirror")
}

func TestApplyEnvOverridesStorageRejectsMalformedLists(t *testing.T) {
	for _, name := range []string{
		"HARUKI_PJSK_RENDER_STORAGE_CACHE_ENDPOINTS",
		"HARUKI_PJSK_RENDER_STORAGE_CACHE_OPTIONS",
		"HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_ENDPOINTS",
		"HARUKI_PJSK_RENDER_STORAGE_USER_UPLOAD_MIRROR_ENDPOINTS",
	} {
		t.Run(name, func(t *testing.T) {
			value := `["unterminated`
			if strings.HasSuffix(name, "_OPTIONS") {
				value = "missing-equals"
			}
			t.Setenv(name, value)
			err := ApplyEnvOverrides(&Config{})
			testutil.Require(t, err != nil && strings.Contains(err.Error(), name), "error = %v", err)
		})
	}
}

func TestEnvBoolPtr(t *testing.T) {
	var dst *bool
	envBoolPtr("HARUKI_TEST_BOOL_PTR_UNSET", &dst)
	testutil.Require(t, dst == nil, "unset variable set the pointer")
	t.Setenv("HARUKI_TEST_BOOL_PTR", "not-a-bool")
	envBoolPtr("HARUKI_TEST_BOOL_PTR", &dst)
	testutil.Require(t, dst == nil, "invalid variable set the pointer")
	t.Setenv("HARUKI_TEST_BOOL_PTR", "false")
	envBoolPtr("HARUKI_TEST_BOOL_PTR", &dst)
	testutil.Require(t, dst != nil && !*dst, "false not applied")
	t.Setenv("HARUKI_TEST_BOOL_PTR", "1")
	envBoolPtr("HARUKI_TEST_BOOL_PTR", &dst)
	testutil.Require(t, dst != nil && *dst, "true not applied")
}

func TestEnvStringMapGenericErrorText(t *testing.T) {
	var dst map[string]string
	t.Setenv("HARUKI_TEST_MAP", "novalue")
	err := envStringMap("HARUKI_TEST_MAP", &dst)
	testutil.Require(t, err != nil && strings.Contains(err.Error(), "expected key=value"), "error = %v", err)
	testutil.Require(t, !strings.Contains(err.Error(), "region"), "error still mentions region: %v", err)
	t.Setenv("HARUKI_TEST_MAP", `{"k":" "}`)
	err = envStringMap("HARUKI_TEST_MAP", &dst)
	testutil.Require(t, err != nil && strings.Contains(err.Error(), "key and value must not be empty"), "error = %v", err)
}

func TestReadConfigStorageBlock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	doc := `pjsk_render:
  storage:
    assets:      { scheme: fs, root: "/asset" }
    user_upload:
      kind: s3
      name: garage
      bucket: user-upload
      endpoints: ["http://100.64.0.11:3900"]
      endpoint: "http://ignored:3900"
      access_key_id: "GK"
      secret_access_key: "secret"
      public_base_url: "https://cdn.example"
      options: { max_attempts: 2 }
      mirror: { scheme: local, root: "/asset" }
      mirror_mode: write
    cache: { scheme: fs, root: "/data/haruki/cache/cloud" }
    image_cache:
      scheme: s3
      bucket: image-cache
      endpoint: "garage:3900"
      tls: false
      path_style: false
`
	testutil.Require(t, os.WriteFile(configPath, []byte(doc), 0o600) == nil, "write config")
	cfg, err := ReadConfig(configPath)
	testutil.Require(t, err == nil, "ReadConfig error = %v", err)
	st := cfg.PJSKRender.Storage
	testutil.Require(t, st.Assets.Root == "/asset" && st.Static.IsZero(), "assets/static = %+v / %+v", st.Assets, st.Static)
	testutil.Require(t, st.UserUpload.Scheme == "s3" && st.UserUpload.Provider == "garage" && st.UserUpload.BaseURL == "https://cdn.example", "aliases = %+v", st.UserUpload)
	testutil.Require(t, st.UserUpload.Options["max_attempts"] == "2", "options = %#v", st.UserUpload.Options)
	testutil.Require(t, st.UserUpload.Mirror != nil && st.UserUpload.Mirror.Root == "/asset", "mirror = %+v", st.UserUpload.Mirror)
	testutil.Require(t, st.ImageCache.TLS != nil && !*st.ImageCache.TLS && st.ImageCache.PathStyle != nil && !*st.ImageCache.PathStyle, "image_cache = %+v", st.ImageCache)
	for _, slot := range storage.Slots {
		testutil.Require(t, storage.Validate(string(slot), *st.Provider(slot)) == nil || st.Provider(slot).IsZero(), "slot %s invalid", slot)
	}
}

func TestApplyEnvOverridesPublicHosts(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_HOSTS", "cn09=https://ic-cn09.example, cn01=https://ic-cn01.example")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_HOST_ORDER", "cn09,cn01")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_HOSTS_PROBE_PATH", "/health")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_HOSTS_PROBE_INTERVAL", "15s")
	t.Setenv("HARUKI_PJSK_RENDER_ASSETS_BASE_URL", "https://assets.example")
	t.Setenv("HARUKI_PJSK_RENDER_ASSETS_BASE_URLS", "https://assets-cn09.example, https://assets-cn01.example")

	cfg := &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	ic := cfg.PJSKRender.ImageCache
	testutil.Require(t, len(ic.Hosts) == 2 && ic.Hosts["cn01"] == "https://ic-cn01.example", "hosts = %#v", ic.Hosts)
	testutil.Require(t, len(ic.HostOrder) == 2 && ic.HostOrder[0] == "cn09", "host order = %#v", ic.HostOrder)
	testutil.Require(t, ic.HostsProbePath == "/health" && ic.HostsProbeInterval == 15*time.Second, "probe = %q %v", ic.HostsProbePath, ic.HostsProbeInterval)
	ad := cfg.PJSKRender.AssetDirs
	testutil.Require(t, ad.AssetsBaseURL == "https://assets.example", "assets base url = %q", ad.AssetsBaseURL)
	testutil.Require(t, len(ad.AssetsBaseURLs) == 2 && ad.AssetsBaseURLs[1] == "https://assets-cn01.example", "assets base urls = %#v", ad.AssetsBaseURLs)
}

func TestApplyEnvOverridesImageCacheIndex(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_PG_MAX_OPEN", "16")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_RENDER_INDEX_REQUIRE_PG", "true")
	cfg := &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	ic := cfg.PJSKRender.ImageCache
	testutil.Require(t, !ic.RenderIndex.DDLEnabled && !ic.RenderIndex.LookupEnabled && ic.RenderIndex.TouchInterval == 0, "render index flags default on: %+v", ic.RenderIndex)
	testutil.Require(t, ic.PGMaxOpen == 16 && ic.RenderIndex.RequirePG, "image cache index = %+v", ic)

	var decoded PJSKRenderConfig
	err := yaml.Unmarshal([]byte("image_cache:\n  pg_max_open: 4\n  render_index:\n    require_pg: true\n"), &decoded)
	testutil.Require(t, err == nil && decoded.ImageCache.PGMaxOpen == 4 && decoded.ImageCache.RenderIndex.RequirePG, "yaml = %+v, %v", decoded.ImageCache, err)
}

func TestApplyEnvOverridesRenderIndexFlags(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_RENDER_INDEX_DDL_ENABLED", "true")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_RENDER_INDEX_LOOKUP_ENABLED", "true")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_RENDER_INDEX_TOUCH_INTERVAL", "90s")
	cfg := &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	ri := cfg.PJSKRender.ImageCache.RenderIndex
	testutil.Require(t, ri.DDLEnabled && ri.LookupEnabled && ri.TouchInterval == 90*time.Second, "render index env = %+v", ri)

	var decoded PJSKRenderConfig
	err := yaml.Unmarshal([]byte("image_cache:\n  render_index:\n    ddl_enabled: true\n    lookup_enabled: true\n    touch_interval: 2m\n"), &decoded)
	ri = decoded.ImageCache.RenderIndex
	testutil.Require(t, err == nil && ri.DDLEnabled && ri.LookupEnabled && ri.TouchInterval == 2*time.Minute, "yaml = %+v, %v", ri, err)
}

func TestDrawingArtifactConfigEnvAndYAML(t *testing.T) {
	cfg := &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	da := cfg.PJSKRender.DrawingArtifact
	testutil.Require(t, len(da.Endpoints) == 0 && da.FetchTimeout == 0 && da.ArtifactTimeout == 0, "drawing artifact defaults on: %+v", da)

	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_ENDPOINTS", "api/pjsk/card/box, api/pjsk/event/list")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_FETCH_TIMEOUT", "3s")
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_ARTIFACT_TIMEOUT", "20s")
	cfg = &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	da = cfg.PJSKRender.DrawingArtifact
	testutil.Require(t, len(da.Endpoints) == 2 && da.Endpoints[0] == "api/pjsk/card/box" && da.Endpoints[1] == "api/pjsk/event/list" &&
		da.FetchTimeout == 3*time.Second && da.ArtifactTimeout == 20*time.Second, "drawing artifact env = %+v", da)

	var decoded PJSKRenderConfig
	err := yaml.Unmarshal([]byte("drawing_artifact:\n  endpoints: [\"*\"]\n  fetch_timeout: 10s\n  artifact_timeout: 15s\n"), &decoded)
	da = decoded.DrawingArtifact
	testutil.Require(t, err == nil && len(da.Endpoints) == 1 && da.Endpoints[0] == "*" && da.FetchTimeout == 10*time.Second && da.ArtifactTimeout == 15*time.Second, "yaml = %+v, %v", da, err)
}

func TestApplyEnvOverridesDrawingArtifactEndpointsMalformed(t *testing.T) {
	t.Setenv("HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_ENDPOINTS", `["api/pjsk/card/box"`)
	err := ApplyEnvOverrides(&Config{})
	testutil.Require(t, err != nil && strings.Contains(err.Error(), "HARUKI_PJSK_RENDER_DRAWING_ARTIFACT_ENDPOINTS"), "error = %v", err)
}

func TestApplyEnvOverridesPublicHostsMalformed(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"HARUKI_PJSK_RENDER_IMAGE_CACHE_HOSTS", "cn09"},
		{"HARUKI_PJSK_RENDER_IMAGE_CACHE_HOST_ORDER", `["cn09"`},
		{"HARUKI_PJSK_RENDER_ASSETS_BASE_URLS", `["https://a"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			err := ApplyEnvOverrides(&Config{})
			testutil.Require(t, err != nil && strings.Contains(err.Error(), tc.name), "error = %v", err)
		})
	}
}

func TestReadConfigPublicHostsBlock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	doc := `pjsk_render:
  image_cache:
    uri: https://image-cache.example
    hosts: {cn09: "https://ic-cn09.example", cn01: "https://ic-cn01.example"}
    host_order: [cn09, cn01]
    hosts_probe_path: ""
    hosts_probe_interval: 30s
  asset_dirs:
    assets_base_url: https://assets.example
    assets_base_urls:
      - https://assets-cn09.example
      - https://assets-cn01.example
`
	testutil.Require(t, os.WriteFile(configPath, []byte(doc), 0o600) == nil, "write config")
	cfg, err := ReadConfig(configPath)
	testutil.Require(t, err == nil, "ReadConfig error = %v", err)
	ic := cfg.PJSKRender.ImageCache
	testutil.Require(t, ic.Hosts["cn09"] == "https://ic-cn09.example" && len(ic.HostOrder) == 2, "image cache hosts = %+v", ic)
	testutil.Require(t, ic.HostsProbeInterval == 30*time.Second && ic.HostsProbePath == "", "probe = %+v", ic)
	ad := cfg.PJSKRender.AssetDirs
	testutil.Require(t, len(ad.AssetsBaseURLs) == 2 && ad.AssetsBaseURLs[0] == "https://assets-cn09.example", "asset hosts = %#v", ad.AssetsBaseURLs)
}

func TestImageCacheGCAndLegacyRedirectConfig(t *testing.T) {
	cfg := &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	gc := cfg.PJSKRender.ImageCache.GC
	testutil.Require(t, !gc.Enabled && gc.DryRun == nil && gc.DryRunEnabled() && gc.Interval == 0 && gc.Batch == 0 && gc.ObjectRetentionDays == 0,
		"gc defaults = %+v", gc)
	testutil.Require(t, !cfg.PJSKRender.ImageCache.LegacyRedirect.Enabled, "legacy redirect defaults on")

	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_GC_ENABLED", "true")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_GC_DRY_RUN", "false")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_GC_INTERVAL", "15m")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_GC_BATCH", "250")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_GC_OBJECT_RETENTION_DAYS", "45")
	t.Setenv("HARUKI_PJSK_RENDER_IMAGE_CACHE_LEGACY_REDIRECT_ENABLED", "true")
	cfg = &Config{}
	testutil.Require(t, ApplyEnvOverrides(cfg) == nil, "ApplyEnvOverrides failed")
	gc = cfg.PJSKRender.ImageCache.GC
	testutil.Require(t, gc.Enabled && !gc.DryRunEnabled() && gc.Interval == 15*time.Minute && gc.Batch == 250 && gc.ObjectRetentionDays == 45,
		"gc env = %+v", gc)
	testutil.Require(t, cfg.PJSKRender.ImageCache.LegacyRedirect.Enabled, "legacy redirect env ignored")

	var decoded PJSKRenderConfig
	err := yaml.Unmarshal([]byte("image_cache:\n  gc_enabled: true\n  gc_dry_run: true\n  gc_interval: 2h\n  gc_batch: 10\n  gc_object_retention_days: 7\n  legacy_redirect:\n    enabled: true\n"), &decoded)
	gc = decoded.ImageCache.GC
	testutil.Require(t, err == nil && gc.Enabled && gc.DryRunEnabled() && gc.DryRun != nil && gc.Interval == 2*time.Hour && gc.Batch == 10 && gc.ObjectRetentionDays == 7,
		"yaml gc = %+v, %v", gc, err)
	testutil.Require(t, decoded.ImageCache.LegacyRedirect.Enabled, "yaml legacy redirect ignored")

	// The nested render_index.gc spelling (addendum C-3) must not configure GC.
	var nested PJSKRenderConfig
	err = yaml.Unmarshal([]byte("image_cache:\n  render_index:\n    gc:\n      enabled: true\n"), &nested)
	testutil.Require(t, err == nil && !nested.ImageCache.GC.Enabled, "nested render_index.gc spelling enabled GC: %+v, %v", nested.ImageCache.GC, err)
}
