//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package server

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	usersDB "haruki-cloud/database/users"
	"haruki-cloud/internal/pjsk/accountdata"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func serverCoverageSQLiteDSN(t *testing.T, name string) string {
	t.Helper()
	return fmt.Sprintf("file:%s?_fk=1", filepath.Join(t.TempDir(), name+".db"))
}

func TestDatabaseInitializersWithSQLiteAndMiniRedis(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	var output bytes.Buffer
	mainLogger := startupTestLogger(&output)
	var redisClient *redis.Client
	redisServer := miniredis.NewMiniRedis()
	if err := redisServer.Start(); err == nil {
		defer redisServer.Close()
		host, portText, err := net.SplitHostPort(redisServer.Addr())
		if err != nil {
			t.Fatalf("split miniredis address: %v", err)
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			t.Fatalf("parse miniredis port: %v", err)
		}
		harukiConfig.Cfg.Redis.Host = host
		harukiConfig.Cfg.Redis.Port = port
		redisClient = initRedis(nil, mainLogger)
	} else {
		t.Logf("miniredis unavailable; Redis initializer branch skipped: %v", err)
	}

	harukiConfig.Cfg.Chunithm.Enabled = true
	harukiConfig.Cfg.Chunithm.BindingDBType = "sqlite3"
	harukiConfig.Cfg.Chunithm.BindingDBURL = serverCoverageSQLiteDSN(t, "chunithm-main")
	harukiConfig.Cfg.Chunithm.MusicDBType = "sqlite3"
	harukiConfig.Cfg.Chunithm.MusicDBURL = serverCoverageSQLiteDSN(t, "chunithm-music")
	chunithmMain, chunithmMusic := initChunithmIfEnabled(nil, mainLogger, fiber.New(), redisClient)
	if chunithmMain == nil || chunithmMusic == nil {
		t.Fatal("enabled Chunithm databases were not initialized")
	}

	harukiConfig.Cfg.UsersDB.DBType = "sqlite3"
	harukiConfig.Cfg.UsersDB.DBURL = serverCoverageSQLiteDSN(t, "users")
	usersClient := initUsers(nil, mainLogger)
	if usersClient == nil {
		t.Fatal("configured users database was not initialized")
	}

	harukiConfig.Cfg.PJSK.Enabled = true
	harukiConfig.Cfg.PJSK.DBType = "sqlite3"
	harukiConfig.Cfg.PJSK.DBURL = serverCoverageSQLiteDSN(t, "pjsk")
	pjskClient := initPJSKIfEnabled(nil, mainLogger, fiber.New(), redisClient)
	if pjskClient == nil {
		t.Fatal("enabled PJSK database was not initialized")
	}

	harukiConfig.Cfg.Sekai.Enabled = true
	harukiConfig.Cfg.Sekai.DBType = "sqlite3"
	harukiConfig.Cfg.Sekai.DBURL = serverCoverageSQLiteDSN(t, "sekai-migrated")
	harukiConfig.Cfg.Sekai.AutoMigrate = true
	sekaiMigrated := initSekaiIfEnabled(nil, mainLogger)
	if sekaiMigrated == nil {
		t.Fatal("auto-migrated Sekai database was not initialized")
	}
	if err := sekaiMigrated.Close(); err != nil {
		t.Fatalf("close migrated Sekai database: %v", err)
	}
	harukiConfig.Cfg.Sekai.DBURL = serverCoverageSQLiteDSN(t, "sekai-unmigrated")
	harukiConfig.Cfg.Sekai.AutoMigrate = false
	sekaiClient := initSekaiIfEnabled(nil, mainLogger)
	if sekaiClient == nil {
		t.Fatal("non-migrating Sekai database was not initialized")
	}

	harukiConfig.Cfg.HarukiBotDB.DBType = "sqlite3"
	harukiConfig.Cfg.HarukiBotDB.DBURL = serverCoverageSQLiteDSN(t, "bot")
	banChecker := accountdata.NewBanService(usersClient)
	botClient := initBot(nil, mainLogger, fiber.New(), redisClient, bytes.Repeat([]byte{1}, 32), "", nil, banChecker)
	if botClient == nil {
		t.Fatal("bot database was not initialized")
	}

	closeClients(redisClient, nil, usersClient, chunithmMain, chunithmMusic, pjskClient, sekaiClient, botClient)
}

func TestCensorInitializationSuccessDisabledAndOpenFailure(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	var output bytes.Buffer
	mainLogger := startupTestLogger(&output)
	if got := initCensorIfEnabled(nil, mainLogger, nil); got != nil {
		t.Fatal("empty censor configuration should be disabled")
	}
	harukiConfig.Cfg.Censor.CensorDBType = "missing-driver"
	harukiConfig.Cfg.Censor.CensorDBURL = "dsn"
	if got := initCensorIfEnabled(nil, mainLogger, nil); got != nil {
		t.Fatal("invalid censor driver should fail without a service")
	}
	harukiConfig.Cfg.Censor.CensorDBType = "sqlite3"
	harukiConfig.Cfg.Censor.CensorDBURL = serverCoverageSQLiteDSN(t, "censor")
	service := initCensorIfEnabled(nil, mainLogger, nil)
	if service == nil {
		t.Fatalf("configured censor service was not initialized: %s", output.String())
	}
	if err := service.Close(); err != nil {
		t.Fatalf("close censor service: %v", err)
	}
}

func TestConfigureSekaiRuntimeAndRenderInitialization(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	mainLogger := startupTestLogger(&output)

	pjskClient, err := pjskDB.Open("sqlite3", serverCoverageSQLiteDSN(t, "render-pjsk"))
	if err != nil {
		t.Fatalf("open render PJSK DB: %v", err)
	}
	defer pjskClient.Close()
	if err := pjskClient.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate render PJSK DB: %v", err)
	}
	sekaiDSN := serverCoverageSQLiteDSN(t, "render-sekai")
	sekaiClient, err := sekaiDB.Open("sqlite3", sekaiDSN)
	if err != nil {
		t.Fatalf("open render Sekai DB: %v", err)
	}
	defer sekaiClient.Close()
	if err := sekaiClient.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate render Sekai DB: %v", err)
	}
	usersClient, err := usersDB.Open("sqlite3", serverCoverageSQLiteDSN(t, "render-users"))
	if err != nil {
		t.Fatalf("open render users DB: %v", err)
	}
	defer usersClient.Close()
	if err := usersClient.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate render users DB: %v", err)
	}

	harukiConfig.Cfg.PJSKRender.Enabled = true
	harukiConfig.Cfg.PJSKRender.MusicMeta.RefreshInterval = time.Hour
	harukiConfig.Cfg.PJSKRender.MusicMeta.OutputDir = t.TempDir()
	harukiConfig.Cfg.PJSKRender.AssetDirs.Primary = t.TempDir()
	harukiConfig.Cfg.Sekai.DBType = "sqlite3"
	harukiConfig.Cfg.Sekai.DBURL = sekaiDSN
	runtime := initPJSKRenderIfEnabled(ctx, mainLogger, sekaiClient, pjskClient)
	if runtime == nil {
		t.Fatal("enabled render runtime was not initialized")
	}
	defer runtime.Close()

	configureSekaiRuntime(mainLogger, nil, pjskClient, usersClient, nil, nil)
	configureSekaiRuntime(mainLogger, runtime, nil, usersClient, nil, nil)

	harukiConfig.Cfg.Censor.CensorDBType = "sqlite3"
	harukiConfig.Cfg.Censor.CensorDBURL = serverCoverageSQLiteDSN(t, "render-censor")
	censorService := initCensorIfEnabled(ctx, mainLogger, runtime)
	if censorService == nil {
		t.Fatal("render censor service was not initialized")
	}
	defer censorService.Close()
	banChecker := accountdata.NewBanService(usersClient)
	configureSekaiRuntime(mainLogger, runtime, pjskClient, usersClient, banChecker, censorService)
	if runtime.Bindings == nil || runtime.Aliases == nil || runtime.PrivateDataCache == nil || runtime.BuiltSnapshotCache == nil || runtime.BanChecker != banChecker {
		t.Fatal("Sekai runtime services were not fully configured")
	}
}

func TestLoggingSetupAndClosePaths(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	t.Setenv("HARUKI_CONFIG_PATH", "   ")
	if got := resolveConfigPath(); got != defaultConfigPath {
		t.Fatalf("blank config override resolved to %q", got)
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "main.log")
	configPath := filepath.Join(dir, "server.yaml")
	configBody := fmt.Sprintf("profile: dev\nbackend:\n  log_level: INFO\n  main_log_file: %q\n", logPath)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write logging config: %v", err)
	}
	t.Setenv("HARUKI_CONFIG_PATH", "  "+configPath+"  ")
	if got := resolveConfigPath(); got != configPath {
		t.Fatalf("config override resolved to %q", got)
	}
	writer := setupLogging()
	mainLogger := harukiLogger.NewLogger("Main", "INFO", writer)
	mainLogger.Info("coverage logging record")
	closeMainLogFile(mainLogger)
	content, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(content), "coverage logging record") {
		t.Fatalf("main log content = %q, %v", content, err)
	}

	closed, err := os.CreateTemp(dir, "closed-main-*.log")
	if err != nil {
		t.Fatalf("create closed main log: %v", err)
	}
	mainLogFileHandle = closed
	if err := closed.Close(); err != nil {
		t.Fatalf("pre-close main log: %v", err)
	}
	closeMainLogFileHandle()
	harukiLogger.SetGlobalFileWriter(io.Discard)
	harukiLogger.SetCommandWriter(nil)
}

func TestFiberAccessLogReadinessAndFailureBranches(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	harukiConfig.Cfg.Profile = harukiConfig.ProfileDev
	dir := t.TempDir()
	accessPath := filepath.Join(dir, "access.log")
	harukiConfig.Cfg.Backend.AccessLog = "structured"
	harukiConfig.Cfg.Backend.AccessLogPath = accessPath
	var mainOutput bytes.Buffer
	harukiLogger.SetGlobalFileWriter(&mainOutput)
	mainLogger := startupTestLogger(&mainOutput)
	app := createFiberApp(mainLogger)
	app.Get("/panic", func(fiber.Ctx) error { panic("coverage panic") })

	ready, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if err != nil || ready.StatusCode != fiber.StatusOK {
		t.Fatalf("readiness response = %#v, %v", ready, err)
	}
	panicResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/panic", nil))
	if err != nil || panicResponse.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("panic response = %#v, %v", panicResponse, err)
	}
	closeAccessLogFile(mainLogger)
	content, err := os.ReadFile(accessPath)
	if err != nil || !strings.Contains(string(content), "http_request") {
		t.Fatalf("access log content = %q, %v", content, err)
	}

	var errorOutput bytes.Buffer
	errorApp := fiber.New(fiber.Config{ErrorHandler: func(fiber.Ctx, error) error {
		return errors.New("error handler failed")
	}})
	errorApp.Use(accessLogMiddleware(harukiLogger.NewLogger("HTTP", "INFO", &errorOutput)))
	errorApp.Get("/bots/:botId", func(fiber.Ctx) error { return errors.New("handler failed") })
	response, err := errorApp.Test(httptest.NewRequest(http.MethodGet, "/bots/bot-1", nil))
	if err != nil || response.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("failed error-handler response = %#v, %v", response, err)
	}
	if !strings.Contains(errorOutput.String(), "bot_id=bot-1") || !strings.Contains(errorOutput.String(), "error_type=*errors.errorString") {
		t.Fatalf("failure access log = %q", errorOutput.String())
	}

	closed, err := os.CreateTemp(dir, "closed-access-*.log")
	if err != nil {
		t.Fatalf("create closed access log: %v", err)
	}
	accessLogFileHandle = closed
	accessLogAsyncWriter = nil
	if err := closed.Close(); err != nil {
		t.Fatalf("pre-close access log: %v", err)
	}
	closeAccessLogFile(mainLogger)
	harukiLogger.SetGlobalFileWriter(io.Discard)
}

func TestDrawingCacheAuthorizationAndPositiveGC(t *testing.T) {
	preserveServerConfig(t)
	harukiConfig.Cfg = harukiConfig.Config{}
	harukiConfig.Cfg.HarukiBotDB.InternalAPIToken = "cache-token"
	harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir = t.TempDir()
	harukiConfig.Cfg.PJSKRender.DrawingCache.GCInterval = time.Hour
	harukiConfig.Cfg.PJSKRender.DrawingCache.RequireAuth = true
	var output bytes.Buffer
	app := fiber.New()
	service := initDrawingCacheIfConfigured(context.Background(), startupTestLogger(&output), app)
	if service == nil {
		t.Fatal("authorized drawing cache was not initialized")
	}
	defer service.Close()
	unauthorized, err := app.Test(httptest.NewRequest(http.MethodGet, "/cache/stats", nil))
	if err != nil || unauthorized.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("unauthorized cache response = %#v, %v", unauthorized, err)
	}
	request := httptest.NewRequest(http.MethodGet, "/cache/stats", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer cache-token")
	authorized, err := app.Test(request)
	if err != nil || authorized.StatusCode != fiber.StatusOK {
		t.Fatalf("authorized cache response = %#v, %v", authorized, err)
	}
}

func TestSekaiRemoteSyncInitialAndBackgroundBranches(t *testing.T) {
	preserveServerConfig(t)
	truePath, err := filepath.Abs("/usr/bin/true")
	if err != nil {
		t.Fatalf("resolve true path: %v", err)
	}
	if _, err := os.Stat(truePath); err != nil {
		t.Skip("/usr/bin/true is unavailable")
	}
	falsePath := "/usr/bin/false"
	var output lockedBuffer
	mainLogger := startupTestLogger(&output)
	base := func() {
		harukiConfig.Cfg = harukiConfig.Config{}
		harukiConfig.Cfg.Sekai.Enabled = true
		harukiConfig.Cfg.Sekai.DBType = "postgres"
		harukiConfig.Cfg.Sekai.DBURL = "target"
		harukiConfig.Cfg.Sekai.RemoteSync.Enabled = true
		harukiConfig.Cfg.Sekai.RemoteSync.SourceDBType = "postgres"
		harukiConfig.Cfg.Sekai.RemoteSync.SourceDBURL = "source"
		harukiConfig.Cfg.Sekai.RemoteSync.Timeout = time.Second
		harukiConfig.Cfg.Sekai.RemoteSync.PgDumpPath = truePath
		harukiConfig.Cfg.Sekai.RemoteSync.PgRestorePath = truePath
	}

	base()
	harukiConfig.Cfg.Sekai.RemoteSync.Initial = true
	harukiConfig.Cfg.Sekai.RemoteSync.Interval = -1
	startSekaiDBRemoteSync(nil, mainLogger)

	if _, err := os.Stat(falsePath); err == nil {
		base()
		harukiConfig.Cfg.Sekai.RemoteSync.Initial = true
		harukiConfig.Cfg.Sekai.RemoteSync.Interval = -1
		harukiConfig.Cfg.Sekai.RemoteSync.PgDumpPath = falsePath
		startSekaiDBRemoteSync(context.Background(), mainLogger)
	}

	base()
	harukiConfig.Cfg.Sekai.RemoteSync.Interval = 5 * time.Millisecond
	backgroundCtx, cancel := context.WithCancel(context.Background())
	startSekaiDBRemoteSync(backgroundCtx, mainLogger)
	time.Sleep(25 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)
	if !strings.Contains(output.String(), "remote sync") {
		t.Fatalf("missing remote sync logs: %s", output.String())
	}
}

func TestRunMinimalServerLifecycle(t *testing.T) {
	preserveServerConfig(t)
	redisServer := miniredis.NewMiniRedis()
	if err := redisServer.Start(); err != nil {
		t.Skipf("loopback listeners unavailable: %v", err)
	}
	defer redisServer.Close()
	host, redisPortText, err := net.SplitHostPort(redisServer.Addr())
	if err != nil {
		t.Fatalf("split Redis address: %v", err)
	}
	redisPort, err := strconv.Atoi(redisPortText)
	if err != nil {
		t.Fatalf("parse Redis port: %v", err)
	}

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("HTTP listeners unavailable: %v", err)
	}
	serverPort := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatalf("close port probe: %v", err)
	}

	dir := t.TempDir()
	staticDir := filepath.Join(dir, "static")
	if err := os.MkdirAll(staticDir, 0o700); err != nil {
		t.Fatalf("create static directory: %v", err)
	}
	keyHex := hex.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))
	configPath := filepath.Join(dir, "run.yaml")
	configBody := fmt.Sprintf(`profile: dev
backend:
  host: 127.0.0.1
  port: %d
  log_level: ERROR
chunithm:
  enabled: false
pjsk:
  enabled: false
sekai:
  enabled: false
pjsk_render:
  enabled: false
  image_cache:
    dir: %q
redis:
  host: %q
  port: %d
haruki_bot:
  db_type: sqlite3
  db_url: %q
  credential_sign_token: credential-secret
  session_sign_token: session-secret
  internal_api_token: internal-secret
  noise_private_key: %q
  auth_encryption_key: %q
users_db:
  db_type: sqlite3
  db_url: %q
`, serverPort, staticDir, host, redisPort, serverCoverageSQLiteDSN(t, "run-bot"), keyHex, keyHex, serverCoverageSQLiteDSN(t, "run-users"))
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write Run config: %v", err)
	}
	t.Setenv("HARUKI_CONFIG_PATH", configPath)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx)
		close(done)
	}()

	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(serverPort))
	deadline := time.Now().Add(15 * time.Second)
	for {
		connection, dialErr := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			break
		}
		select {
		case <-done:
			t.Fatal("Run returned before the HTTP server became ready")
		default:
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("timed out waiting for Run HTTP listener")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
	harukiLogger.SetGlobalFileWriter(io.Discard)
	harukiLogger.SetCommandWriter(nil)
}
