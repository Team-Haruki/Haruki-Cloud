//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package server

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	noiseCrypto "haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/storage"
	harukiLogger "haruki-cloud/utils/logger"

	"entgo.io/ent"
	"github.com/gofiber/fiber/v3"
)

type startupTestClient struct {
	interceptors int
	hooks        int
	closed       bool
}

type startupContextKey struct{}

func (c *startupTestClient) Close() error {
	c.closed = true
	return nil
}

func (c *startupTestClient) Intercept(interceptors ...ent.Interceptor) {
	c.interceptors += len(interceptors)
}

func (c *startupTestClient) Use(hooks ...ent.Hook) {
	c.hooks += len(hooks)
}

func startupTestLogger(output io.Writer) *harukiLogger.Logger {
	return harukiLogger.NewLogger("server-test", "DEBUG", output)
}

func preserveServerConfig(t *testing.T) {
	t.Helper()
	original := harukiConfig.Cfg
	t.Cleanup(func() { harukiConfig.Cfg = original })
}

func TestEnsureContextAndInitDBClient(t *testing.T) {
	if ensureContext(nil) == nil {
		t.Fatal("nil context was not replaced")
	}
	wantCtx := context.WithValue(context.Background(), startupContextKey{}, "value")
	if got := ensureContext(wantCtx); got != wantCtx {
		t.Fatal("non-nil context was replaced")
	}

	client := &startupTestClient{}
	schemaCalled := false
	got := initDBClient(wantCtx, startupTestLogger(&bytes.Buffer{}), "test",
		func() (*startupTestClient, error) { return client, nil },
		func(gotClient *startupTestClient, gotCtx context.Context) error {
			schemaCalled = true
			if gotClient != client || gotCtx != wantCtx {
				t.Fatal("schema callback received unexpected arguments")
			}
			return nil
		},
	)
	if got != client || !schemaCalled {
		t.Fatal("database client was not initialized")
	}
	if client.interceptors != 1 || client.hooks != 1 {
		t.Fatalf("tracing registration = (%d interceptors, %d hooks)", client.interceptors, client.hooks)
	}

	installEntTracing(struct{}{})
}

func TestDisabledDatabaseInitializers(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg.Chunithm.Enabled = false
	harukiConfig.Cfg.PJSK.Enabled = false
	harukiConfig.Cfg.Sekai.Enabled = false
	harukiConfig.Cfg.UsersDB.DBType = " "
	harukiConfig.Cfg.UsersDB.DBURL = " "

	var output bytes.Buffer
	logger := startupTestLogger(&output)
	app := fiber.New()
	mainDB, musicDB := initChunithmIfEnabled(nil, logger, app, nil)
	if mainDB != nil || musicDB != nil {
		t.Fatal("disabled Chunithm unexpectedly initialized databases")
	}
	if initPJSKIfEnabled(nil, logger, app, nil) != nil {
		t.Fatal("disabled PJSK unexpectedly initialized a database")
	}
	if initSekaiIfEnabled(nil, logger) != nil {
		t.Fatal("disabled Sekai unexpectedly initialized a database")
	}
	if initUsers(nil, logger) != nil {
		t.Fatal("unconfigured users database unexpectedly initialized")
	}
	if !strings.Contains(output.String(), "users database is not configured") {
		t.Fatalf("missing users database warning: %s", output.String())
	}
}

func TestResolveRuntimeCachePaths(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir = "/var/cache/haruki"

	if got := resolveSKForecastCachePath(); got != "/var/cache/haruki/sk_forecast_cache.json" {
		t.Fatalf("forecast fallback path = %q", got)
	}
	if got := resolveMySekaiHousingCompetitionCachePath(); got != "/var/cache/haruki/mysekai_housing_competition_stats.json" {
		t.Fatalf("housing fallback path = %q", got)
	}

	harukiConfig.Cfg.PJSKRender.SKForecast.CachePath = " /tmp/forecast.json "
	harukiConfig.Cfg.PJSKRender.MySekaiHousingCompetition.CachePath = " /tmp/housing.json "
	if got := resolveSKForecastCachePath(); got != "/tmp/forecast.json" {
		t.Fatalf("forecast configured path = %q", got)
	}
	if got := resolveMySekaiHousingCompetitionCachePath(); got != "/tmp/housing.json" {
		t.Fatalf("housing configured path = %q", got)
	}

	harukiConfig.Cfg.PJSKRender.SKForecast.CachePath = ""
	harukiConfig.Cfg.PJSKRender.MySekaiHousingCompetition.CachePath = ""
	harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir = ""
	if resolveSKForecastCachePath() != "" || resolveMySekaiHousingCompetitionCachePath() != "" {
		t.Fatal("empty cache configuration should yield empty paths")
	}
}

func TestBuildRenderStoresLegacyDerivation(t *testing.T) {
	assetDir := t.TempDir()
	cacheDir := t.TempDir()
	var output bytes.Buffer
	cfg := harukiConfig.PJSKRenderConfig{}
	cfg.AssetDirs.Primary = assetDir
	cfg.DrawingCache.StorageDir = cacheDir

	stores, err := buildRenderStores(cfg, startupTestLogger(&output))
	if err != nil {
		t.Fatalf("buildRenderStores error = %v", err)
	}
	ctx := context.Background()
	if err := stores.Cache.Put(ctx, "probe.json", []byte("{}"), storage.PutOptions{}); err != nil {
		t.Fatalf("cache slot Put: %v", err)
	}
	if _, err := stores.Assets.Stat(ctx, "probe.json"); err == nil {
		t.Fatal("assets slot must not share the cache root")
	}
	if _, err := stores.ImageCache.Get(ctx, "x"); !errors.Is(err, storage.ErrNotConfigured) {
		t.Fatalf("image_cache slot without legacy dir should be disabled, got %v", err)
	}
	if got := strings.Count(output.String(), "storage slot configured"); got != 5 {
		t.Fatalf("summary lines = %d:\n%s", got, output.String())
	}
}

func TestBuildRenderStoresValidationFailures(t *testing.T) {
	cases := []struct {
		name string
		edit func(*harukiConfig.PJSKRenderConfig)
		want string
	}{
		{"relative fs root", func(c *harukiConfig.PJSKRenderConfig) { c.Storage.Cache = storage.ProviderConfig{Root: "cache"} }, "storage.cache.root"},
		{"unsupported scheme", func(c *harukiConfig.PJSKRenderConfig) { c.Storage.Assets = storage.ProviderConfig{Scheme: "gcs"} }, "storage.assets.scheme"},
		{"s3 without bucket", func(c *harukiConfig.PJSKRenderConfig) {
			c.Storage.ImageCache = storage.ProviderConfig{Scheme: "s3", Endpoint: "http://garage:3900"}
		}, "storage.image_cache.bucket"},
		{"bad s3 option", func(c *harukiConfig.PJSKRenderConfig) {
			c.Storage.ImageCache = storage.ProviderConfig{Scheme: "s3", Bucket: "b", Endpoint: "http://garage:3900", Options: map[string]string{"max_attempts": "many"}}
		}, "storage.image_cache.options"},
		{"mirror on static", func(c *harukiConfig.PJSKRenderConfig) {
			c.Storage.Static = storage.ProviderConfig{Root: "/asset", Mirror: &storage.ProviderConfig{Root: "/b"}}
		}, "storage.static.mirror"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := harukiConfig.PJSKRenderConfig{}
			tc.edit(&cfg)
			_, err := buildRenderStores(cfg, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestBuildRenderStoresOpensS3Slot(t *testing.T) {
	var output bytes.Buffer
	cfg := harukiConfig.PJSKRenderConfig{}
	cfg.Storage.ImageCache = storage.ProviderConfig{
		Scheme: "s3", Bucket: "image-cache", Endpoints: []string{"http://127.0.0.1:1"},
		AccessKeyID: "GKstartupkey", SecretAccessKey: "startup-secret",
	}
	stores, err := buildRenderStores(cfg, startupTestLogger(&output))
	if err != nil || stores.ImageCache == nil {
		t.Fatalf("buildRenderStores error = %v", err)
	}
	out := output.String()
	if !strings.Contains(out, "creds=set") || strings.Contains(out, "GKstartupkey") || strings.Contains(out, "startup-secret") {
		t.Fatalf("startup summary must say creds=set without the credentials:\n%s", out)
	}
}

func TestValidBotCryptographicConfiguration(t *testing.T) {
	preserveServerConfig(t)
	key := bytes.Repeat([]byte{0x2a}, 32)
	harukiConfig.Cfg.HarukiBotDB.NoisePrivateKey = hex.EncodeToString(key)
	harukiConfig.Cfg.HarukiBotDB.SessionSignToken = "session-secret"
	harukiConfig.Cfg.HarukiBotDB.CredentialSignToken = "credential-secret"

	var output bytes.Buffer
	logger := startupTestLogger(&output)
	validateBotAuthSecrets(logger)
	ring := initNoiseKeyRing(logger)
	if ring == nil || ring.Len() != 1 {
		t.Fatalf("invalid Noise key ring: %#v", ring)
	}
	primary := ring.Primary()
	if primary.ID != noiseCrypto.DefaultKeyID || len(primary.Pair.Private) != 32 || len(primary.Pair.Public) != 32 {
		t.Fatalf("invalid primary Noise key: %#v", primary)
	}
	if !strings.Contains(output.String(), "Noise NK") {
		t.Fatalf("missing crypto startup records: %s", output.String())
	}

	// A rotation key configured alongside the legacy key joins the ring after it.
	harukiConfig.Cfg.HarukiBotDB.NoiseKeys = []harukiConfig.NoiseStaticKeyConfig{
		{KeyID: "next", PrivateKey: hex.EncodeToString(bytes.Repeat([]byte{0x3b}, 32))},
	}
	ring = initNoiseKeyRing(logger)
	if ring.Len() != 2 || ring.Primary().ID != noiseCrypto.DefaultKeyID {
		t.Fatalf("rotation ring = len %d primary %q", ring.Len(), ring.Primary().ID)
	}
	if _, ok := ring.Lookup("next"); !ok {
		t.Fatal("rotation key missing from ring")
	}

	// noise_keys alone (legacy key cleared) makes its first entry primary.
	harukiConfig.Cfg.HarukiBotDB.NoisePrivateKey = ""
	ring = initNoiseKeyRing(logger)
	if ring.Len() != 1 || ring.Primary().ID != "next" {
		t.Fatalf("noise_keys-only ring = len %d primary %q", ring.Len(), ring.Primary().ID)
	}
}

func TestDrawingCacheInitializationBranches(t *testing.T) {
	preserveServerConfig(t)
	var output bytes.Buffer
	logger := startupTestLogger(&output)

	if service := initDrawingCacheIfConfigured(context.Background(), logger, fiber.New()); service != nil {
		t.Fatal("empty drawing cache configuration returned a service")
	}

	storageDir := t.TempDir()
	harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir = storageDir
	harukiConfig.Cfg.PJSKRender.DrawingCache.GCInterval = -1
	app := fiber.New()
	service := initDrawingCacheIfConfigured(context.Background(), logger, app)
	if service == nil {
		t.Fatal("configured drawing cache did not initialize")
	}
	t.Cleanup(func() { _ = service.Close() })
	if got := service.Config().DBPath; got != filepath.Join(storageDir, "cache.db") {
		t.Fatalf("drawing cache database path = %q", got)
	}
	response, err := app.Test(httptest.NewRequest("GET", "/cache/stats", nil))
	if err != nil {
		t.Fatalf("query cache stats: %v", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("cache stats status = %d", response.StatusCode)
	}
}

func TestSekaiRemoteSyncSettingsAndCommands(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg.Sekai.DBType = "postgresql"
	harukiConfig.Cfg.Sekai.DBURL = "target"
	harukiConfig.Cfg.Sekai.RemoteSync.SourceDBType = "pg"
	harukiConfig.Cfg.Sekai.RemoteSync.SourceDBURL = "source"
	harukiConfig.Cfg.Sekai.RemoteSync.Timeout = time.Minute
	harukiConfig.Cfg.Sekai.RemoteSync.PgDumpPath = "/custom/dump"
	harukiConfig.Cfg.Sekai.RemoteSync.PgRestorePath = "/custom/restore"

	settings, err := buildSekaiDBSyncSettings()
	if err != nil {
		t.Fatalf("build custom settings: %v", err)
	}
	if settings.PgDumpPath != "/custom/dump" || settings.PgRestorePath != "/custom/restore" || settings.Timeout != time.Minute {
		t.Fatalf("custom settings = %#v", settings)
	}

	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true command is unavailable")
	}
	if err := runCommand(context.Background(), "true", truePath); err != nil {
		t.Fatalf("run successful command: %v", err)
	}
	falsePath, err := exec.LookPath("false")
	if err == nil {
		err = runCommand(context.Background(), "false", falsePath)
		if err == nil || !strings.Contains(err.Error(), "false failed") || !strings.Contains(err.Error(), "<no stderr>") {
			t.Fatalf("failed command error = %v", err)
		}
	}

	syncSettings := sekaiDBSyncSettings{
		SourceDBURL:   "source",
		TargetDBURL:   "target",
		PgDumpPath:    truePath,
		PgRestorePath: truePath,
		Timeout:       time.Second,
	}
	if err := runSekaiDBRemoteSync(nil, syncSettings); err != nil {
		t.Fatalf("run sync with successful stand-ins: %v", err)
	}
	if falsePath != "" {
		syncSettings.PgDumpPath = falsePath
		if err := runSekaiDBRemoteSync(context.Background(), syncSettings); err == nil {
			t.Fatal("failing dump command unexpectedly succeeded")
		}
		syncSettings.PgDumpPath = truePath
		syncSettings.PgRestorePath = falsePath
		if err := runSekaiDBRemoteSync(context.Background(), syncSettings); err == nil {
			t.Fatal("failing restore command unexpectedly succeeded")
		}
	}
}

func TestSekaiRemoteSyncValidationAndHelpers(t *testing.T) {
	preserveServerConfig(t)
	var output bytes.Buffer
	logger := startupTestLogger(&output)
	startSekaiDBRemoteSync(nil, logger)
	harukiConfig.Cfg.Sekai.Enabled = true
	startSekaiDBRemoteSync(nil, logger)

	tests := []struct {
		name       string
		configure  func()
		wantDetail string
	}{
		{
			name: "unsupported source",
			configure: func() {
				harukiConfig.Cfg.Sekai.DBType = "postgres"
				harukiConfig.Cfg.Sekai.RemoteSync.SourceDBType = "mysql"
				harukiConfig.Cfg.Sekai.RemoteSync.SourceDBURL = "source"
			},
			wantDetail: "source_db_type",
		},
		{
			name: "missing source URL",
			configure: func() {
				harukiConfig.Cfg.Sekai.DBType = "postgres"
				harukiConfig.Cfg.Sekai.DBURL = "target"
				harukiConfig.Cfg.Sekai.RemoteSync.SourceDBType = "postgres"
				harukiConfig.Cfg.Sekai.RemoteSync.SourceDBURL = ""
			},
			wantDetail: "source_db_url",
		},
		{
			name: "missing target URL",
			configure: func() {
				harukiConfig.Cfg.Sekai.DBType = "postgres"
				harukiConfig.Cfg.Sekai.DBURL = ""
				harukiConfig.Cfg.Sekai.RemoteSync.SourceDBType = "postgres"
				harukiConfig.Cfg.Sekai.RemoteSync.SourceDBURL = "source"
			},
			wantDetail: "sekai.db_url",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.configure()
			_, err := buildSekaiDBSyncSettings()
			if err == nil || !strings.Contains(err.Error(), tt.wantDetail) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}

	if !isPostgresDBType(" POSTGRES ") || !isPostgresDBType("postgresql") || !isPostgresDBType("PG") || isPostgresDBType("mysql") {
		t.Fatal("PostgreSQL type normalization is incorrect")
	}
	if got := truncateCommandOutput(" \n "); got != "<no stderr>" {
		t.Fatalf("empty stderr = %q", got)
	}
	if got := truncateCommandOutput("short"); got != "short" {
		t.Fatalf("short stderr = %q", got)
	}
	long := strings.Repeat("x", 5000)
	if got := truncateCommandOutput(long); len(got) <= 4096 || !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("long stderr was not truncated: length=%d", len(got))
	}

	logStartupInfo(logger)
	if !strings.Contains(output.String(), "service starting") {
		t.Fatalf("missing startup record: %s", output.String())
	}
	closeClients(nil, nil, nil, nil, nil, nil, nil, nil)

}
