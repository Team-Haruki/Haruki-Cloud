package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/identity"
	pjskalias "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/chardata"
	sekaiHandler "haruki-cloud/internal/pjsk/handler/sekai"
	"haruki-cloud/internal/pjsk/meta"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
	harukiLogger "haruki-cloud/utils/logger"
	harukiRedis "haruki-cloud/utils/redis"
	sekaiAPI "haruki-cloud/utils/sekai"

	botAuth "haruki-cloud/api/bot/auth"
	botPJSK "haruki-cloud/api/bot/pjsk"
	legacyPJSK "haruki-cloud/api/legacy/pjsk"
	publicChunithm "haruki-cloud/api/public/chunithm"
	publicPJSK "haruki-cloud/api/public/pjsk"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	botDB "haruki-cloud/database/bot"
	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	usersDB "haruki-cloud/database/users"

	"github.com/bytedance/sonic"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

var Version = "2.0.0-dev"

func main() {
	loggerWriter := setupLogging()
	mainLogger := harukiLogger.NewLogger("Main", harukiConfig.Cfg.Backend.LogLevel, loggerWriter)
	logStartupInfo(mainLogger)
	redisClient := initRedis(mainLogger)
	app := createFiberApp(mainLogger)
	usersClient := initUsers(mainLogger)
	chunithmMainClient, chunithmMusicClient := initChunithmIfEnabled(mainLogger, app, redisClient)
	pjskClient := initPJSKIfEnabled(mainLogger, app, redisClient)
	sekaiClient := initSekaiIfEnabled(mainLogger)
	renderRuntime := initPJSKRenderIfEnabled(mainLogger, sekaiClient, pjskClient)
	configureSekaiRuntime(mainLogger, renderRuntime, pjskClient, usersClient)
	legacyPJSK.RegisterPJSKRenderRoutes(app, renderRuntime)
	pjskResolver := initPJSKParserIfEnabled(mainLogger, sekaiClient)
	legacyPJSK.RegisterPJSKCommandRoute(app, pjskResolver, renderRuntime)
	botDBClient := initBot(mainLogger, app, redisClient)
	botPJSK.RegisterPJSKBotRoutes(app, renderRuntime, redisClient, botDBClient)

	defer closeClients(usersClient, chunithmMainClient, chunithmMusicClient, pjskClient, sekaiClient, botDBClient)

	if renderRuntime != nil {
		mainLogger.Infof("PJSK render runtime initialized; internal render routes registered")
	}

	startServer(mainLogger, app)
}

func setupLogging() io.Writer {
	harukiConfig.LoadConfig("haruki-db-configs.yaml")
	loggerWriter := io.Writer(os.Stdout)

	if harukiConfig.Cfg.Backend.MainLogFile != "" {
		logFile, err := harukiLogger.OpenLogFile(harukiConfig.Cfg.Backend.MainLogFile)
		if err != nil {
			tmpLogger := harukiLogger.NewLogger("Main", harukiConfig.Cfg.Backend.LogLevel, os.Stdout)
			tmpLogger.Errorf("failed to open main log file: %v", err)
			os.Exit(1)
		}
		loggerWriter = harukiLogger.NewMultiWriter(os.Stdout, logFile)
	}

	harukiLogger.SetGlobalLogLevel(harukiConfig.Cfg.Backend.LogLevel)
	harukiLogger.SetGlobalFileWriter(loggerWriter)
	return loggerWriter
}

func logStartupInfo(mainLogger *harukiLogger.Logger) {
	mainLogger.Infof("========================= Haruki Database Backend %s =========================", Version)
	mainLogger.Infof("Powered By Haruki Dev Team")
	mainLogger.Infof("Haruki Suite Backend Main Access Log Level: %s", harukiConfig.Cfg.Backend.LogLevel)
	mainLogger.Infof("Haruki Suite Backend Main Access Log Save Path: %s", harukiConfig.Cfg.Backend.MainLogFile)
	mainLogger.Infof("Go Fiber Access Log Save Path: %s", harukiConfig.Cfg.Backend.AccessLogPath)
}

func initRedis(mainLogger *harukiLogger.Logger) *redis.Client {
	redisClient := harukiRedis.NewRedisClient(harukiConfig.Cfg.Redis)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		mainLogger.Errorf("Failed to connect Redis: %v", err)
		os.Exit(1)
	}
	return redisClient
}

func createFiberApp(mainLogger *harukiLogger.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit:   30 * 1024 * 1024,
		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,
		ProxyHeader: harukiConfig.Cfg.Backend.ProxyHeader,
		TrustProxy:  harukiConfig.Cfg.Backend.EnableTrustProxy,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: harukiConfig.Cfg.Backend.TrustProxies,
		},
	})

	if harukiConfig.Cfg.Backend.AccessLog != "" {
		loggerConfig := logger.Config{Format: harukiConfig.Cfg.Backend.AccessLog}
		if harukiConfig.Cfg.Backend.AccessLogPath != "" {
			accessLogFile, err := harukiLogger.OpenLogFile(harukiConfig.Cfg.Backend.AccessLogPath)
			if err != nil {
				mainLogger.Errorf("Failed to open access log file: %v", err)
				os.Exit(1)
			}
			loggerConfig.Stream = accessLogFile
		}
		app.Use(logger.New(loggerConfig))
	}
	return app
}

func initChunithmIfEnabled(mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) (*chunithmMainDB.Client, *chunithmMusicDB.Client) {
	if !harukiConfig.Cfg.Chunithm.Enabled {
		return nil, nil
	}

	chunithmMainClient, err := chunithmMainDB.Open(harukiConfig.Cfg.Chunithm.BindingDBType, harukiConfig.Cfg.Chunithm.BindingDBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Chunithm main DB: %v", err)
		os.Exit(1)
	}
	if err := chunithmMainClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Chunithm main DB: %v", err)
		os.Exit(1)
	}

	chunithmMusicClient, err := chunithmMusicDB.Open(harukiConfig.Cfg.Chunithm.MusicDBType, harukiConfig.Cfg.Chunithm.MusicDBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Chunithm music DB: %v", err)
		os.Exit(1)
	}
	if err := chunithmMusicClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Chunithm music DB: %v", err)
		os.Exit(1)
	}

	publicChunithm.RegisterChunithmRoutes(app, chunithmMainClient, chunithmMusicClient, redisClient)
	return chunithmMainClient, chunithmMusicClient
}

func initUsers(mainLogger *harukiLogger.Logger) *usersDB.Client {
	if strings.TrimSpace(harukiConfig.Cfg.UsersDB.DBType) == "" || strings.TrimSpace(harukiConfig.Cfg.UsersDB.DBURL) == "" {
		mainLogger.Warnf("Users DB is not configured; profile binding commands will be unavailable")
		return nil
	}

	client, err := usersDB.Open(harukiConfig.Cfg.UsersDB.DBType, harukiConfig.Cfg.UsersDB.DBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Users DB: %v", err)
		os.Exit(1)
	}
	if err := client.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Users DB: %v", err)
		os.Exit(1)
	}
	return client
}

func initPJSKIfEnabled(mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) *pjskDB.Client {
	if !harukiConfig.Cfg.PJSK.Enabled {
		return nil
	}

	pjskClient, err := pjskDB.Open(harukiConfig.Cfg.PJSK.DBType, harukiConfig.Cfg.PJSK.DBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to PJSK DB: %v", err)
		os.Exit(1)
	}
	if err := pjskClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for PJSK DB: %v", err)
		os.Exit(1)
	}

	publicPJSK.RegisterPJSKRoutes(app, pjskClient, redisClient)
	return pjskClient
}

func configureSekaiRuntime(mainLogger *harukiLogger.Logger, renderRuntime *renderapp.App, pjskClient *pjskDB.Client, usersClient *usersDB.Client) {
	if renderRuntime == nil || pjskClient == nil {
		return
	}

	var resolver *identity.Resolver
	if usersClient != nil {
		resolver = identity.NewResolver(usersClient)
		renderRuntime.Bindings = userdata.NewBindingService(
			pjskClient,
			resolver,
			sekaiAPI.GetSekaiAPIClient(),
		)
		renderRuntime.Bindings.SetFastVerificationProvider(sekaiAPI.GetToolboxClient())
		if renderRuntime.Assets != nil {
			renderRuntime.Bindings.SetProfileBGStorage(userdata.NewLocalProfileBGStore(renderRuntime.Assets.Primary()))
		}
		renderRuntime.BanChecker = userdata.NewBanService(usersClient)
	}

	renderRuntime.Aliases = pjskalias.NewService(renderRuntime.Sekai, pjskClient, resolver)
	mainLogger.Infof("Sekai runtime services configured")
}

func initSekaiIfEnabled(mainLogger *harukiLogger.Logger) *sekaiDB.Client {
	if !harukiConfig.Cfg.Sekai.Enabled {
		return nil
	}

	sekaiClient, err := sekaiDB.Open(harukiConfig.Cfg.Sekai.DBType, harukiConfig.Cfg.Sekai.DBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Sekai DB: %v", err)
		os.Exit(1)
	}
	if err := sekaiClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Sekai DB: %v", err)
		os.Exit(1)
	}

	return sekaiClient
}

func initPJSKRenderIfEnabled(mainLogger *harukiLogger.Logger, sekaiClient *sekaiDB.Client, pjskClient *pjskDB.Client) *renderapp.App {
	if !harukiConfig.Cfg.PJSKRender.Enabled {
		return nil
	}
	if sekaiClient == nil {
		mainLogger.Errorf("PJSK render runtime requires sekai.enabled=true")
		os.Exit(1)
	}

	metaRefreshInterval := harukiConfig.Cfg.PJSKRender.MusicMeta.RefreshInterval
	if metaRefreshInterval <= 0 {
		metaRefreshInterval = 30 * time.Minute
	}
	metaLoader := meta.NewLoader(harukiLogger.NewLoggerFromGlobal("MusicMeta"))
	if err := metaLoader.LoadAll(context.Background()); err != nil {
		mainLogger.Warnf("music meta initial load partially failed: %v", err)
	}
	metaLoader.StartBackgroundRefresh(context.Background(), metaRefreshInterval)
	mainLogger.Infof("Music meta loader started (refresh=%s)", metaRefreshInterval)

	runtime := renderapp.New(sekaiClient, pjskClient, renderapp.Config{
		DrawingBaseURL:    harukiConfig.Cfg.PJSKRender.DrawingBaseURL,
		DrawingTimeout:    harukiConfig.Cfg.PJSKRender.DrawingTimeout,
		DrawingRetryCount: harukiConfig.Cfg.PJSKRender.DrawingRetryCount,
		DrawingCache: drawing.RenderCacheConfig{
			BaseURL:    harukiConfig.Cfg.PJSKRender.DrawingCache.BaseURL,
			StorageDir: harukiConfig.Cfg.PJSKRender.DrawingCache.StorageDir,
			TTL:        harukiConfig.Cfg.PJSKRender.DrawingCache.TTL,
		},
		ImageCacheURI:   harukiConfig.Cfg.PJSKRender.ImageCache.URI,
		ImageCacheDir:   harukiConfig.Cfg.PJSKRender.ImageCache.Dir,
		AssetPrimaryDir: harukiConfig.Cfg.PJSKRender.AssetDirs.Primary,
		AssetLegacyDirs: harukiConfig.Cfg.PJSKRender.AssetDirs.Legacy,
		LocalMasterdata: renderapp.LocalMasterdataConfig{
			Enabled: harukiConfig.Cfg.PJSKRender.LocalMasterdata.Enabled,
			Dir:     harukiConfig.Cfg.PJSKRender.LocalMasterdata.Dir,
		},
		UserSnapshot: renderapp.UserSnapshotConfig{
			Provider:      harukiConfig.Cfg.PJSKRender.UserSnapshot.Provider,
			UserJSON:      harukiConfig.Cfg.PJSKRender.UserSnapshot.UserJSON,
			MusicMetaJSON: harukiConfig.Cfg.PJSKRender.UserSnapshot.MusicMetaJSON,
			MySekaiJSON:   harukiConfig.Cfg.PJSKRender.UserSnapshot.MySekaiJSON,
		},
		MetaLoader: metaLoader,
		DeckRecommend: renderapp.DeckRecommendConfig{
			Enabled:        harukiConfig.Cfg.PJSKRender.DeckRecommend.Enabled,
			UseLocalEngine: harukiConfig.Cfg.PJSKRender.DeckRecommend.UseLocalEngine,
			Timeout:        harukiConfig.Cfg.PJSKRender.DeckRecommend.Timeout,
			DefaultAlgs:    harukiConfig.Cfg.PJSKRender.DeckRecommend.DefaultAlgs,
		},
	})

	if runtime.Drawing == nil {
		mainLogger.Warnf("PJSK render runtime initialized without drawing_base_url; build-only mode")
	}
	mainLogger.Infof("PJSK render asset roots: %v", runtime.AssetRoots())
	return runtime
}

func initPJSKParserIfEnabled(mainLogger *harukiLogger.Logger, sekaiClient *sekaiDB.Client) *parser.GlobalCommandResolver {
	if !harukiConfig.Cfg.PJSK.Enabled || sekaiClient == nil {
		return nil
	}

	parserCfg := harukiConfig.Cfg.PJSK.Parser
	region := parserCfg.ChardataRegion
	if region == "" {
		region = "jp"
	}
	refreshInterval := parserCfg.ChardataRefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = time.Hour
	}

	loader := chardata.NewLoader(sekaiClient, region, harukiLogger.NewLoggerFromGlobal("Chardata"))
	if err := loader.Load(context.Background()); err != nil {
		mainLogger.Warnf("chardata initial load failed (parser will use empty nicknames): %v", err)
	}
	loader.StartBackgroundRefresh(context.Background(), refreshInterval)

	sekaiHandler.EnsureCommandHandlersRegistered(loader.Nicknames())
	resolver := parser.NewGlobalCommandResolver(loader.Nicknames())
	mainLogger.Infof("PJSK parser initialized (chardata_region=%s, refresh=%s)", region, refreshInterval)
	return resolver
}

func initBot(mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) *botDB.Client {
	botDBClient, err := botDB.Open(harukiConfig.Cfg.HarukiBotDB.DBType, harukiConfig.Cfg.HarukiBotDB.DBURL)
	if err != nil {
		mainLogger.Errorf("Failed to initialize Bot entgo client: %v", err)
		os.Exit(1)
	}
	if err := botDBClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Bot DB: %v", err)
		os.Exit(1)
	}

	botAuth.RegisterBotRoutes(app, botDBClient, redisClient)
	return botDBClient
}

func closeClients(usersClient *usersDB.Client, chunithmMainClient *chunithmMainDB.Client, chunithmMusicClient *chunithmMusicDB.Client,
	pjskClient *pjskDB.Client, sekaiClient *sekaiDB.Client, botDBClient *botDB.Client) {
	if usersClient != nil {
		_ = usersClient.Close()
	}
	if chunithmMainClient != nil {
		_ = chunithmMainClient.Close()
	}
	if chunithmMusicClient != nil {
		_ = chunithmMusicClient.Close()
	}
	if pjskClient != nil {
		_ = pjskClient.Close()
	}
	if sekaiClient != nil {
		_ = sekaiClient.Close()
	}
	if botDBClient != nil {
		_ = botDBClient.Close()
	}
}

func startServer(mainLogger *harukiLogger.Logger, app *fiber.App) {
	addr := fmt.Sprintf("%s:%d", harukiConfig.Cfg.Backend.Host, harukiConfig.Cfg.Backend.Port)
	listenConfig := fiber.ListenConfig{
		DisableStartupMessage: true,
	}
	if harukiConfig.Cfg.Backend.SSL {
		listenConfig.CertFile = harukiConfig.Cfg.Backend.SSLCert
		listenConfig.CertKeyFile = harukiConfig.Cfg.Backend.SSLKey
		mainLogger.Infof("SSL enabled, starting HTTPS server at %s", addr)
	} else {
		mainLogger.Infof("Starting HTTP server at %s", addr)
	}
	if err := app.Listen(addr, listenConfig); err != nil {
		mainLogger.Errorf("Failed to start HTTP server: %v", err)
		os.Exit(1)
	}
}
