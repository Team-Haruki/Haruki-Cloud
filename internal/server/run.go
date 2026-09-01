package server

import (
	"context"
	"encoding/hex"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	botPJSK "haruki-cloud/api/bot/pjsk"
	groupGuardAPI "haruki-cloud/api/groupguard"
	"haruki-cloud/internal/pjsk/accountdata"

	"github.com/gofiber/fiber/v3/middleware/static"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	_ "modernc.org/sqlite"
)

func Run(ctx context.Context) {
	loggerWriter := setupLogging()
	mainLogger := harukiLogger.NewLogger("Main", harukiConfig.Cfg.Backend.LogLevel, loggerWriter)
	defer closeMainLogFile(mainLogger)
	defer closeAccessLogFile(mainLogger)
	logStartupInfo(mainLogger)
	redisClient := initRedis(ctx, mainLogger)
	app := createFiberApp(mainLogger)
	drawingCacheService := initDrawingCacheIfConfigured(ctx, mainLogger, app)
	usersClient := initUsers(ctx, mainLogger)
	banChecker := accountdata.NewBanService(usersClient)
	banChecker.SetReadOnly(harukiConfig.Cfg.Node.ReadOnly)
	banChecker.SetAdminQQIDs(harukiConfig.Cfg.Moderation.AdminQQIDs)
	chunithmMainClient, chunithmMusicClient := initChunithmIfEnabled(ctx, mainLogger, app, redisClient)
	pjskClient := initPJSKIfEnabled(ctx, mainLogger, app, redisClient)
	startSekaiDBRemoteSync(ctx, mainLogger)
	sekaiClient := initSekaiIfEnabled(ctx, mainLogger)
	renderRuntime := initPJSKRenderIfEnabled(ctx, mainLogger, sekaiClient, pjskClient)
	censorService := initCensorIfEnabled(ctx, mainLogger, renderRuntime)
	configureSekaiRuntime(mainLogger, renderRuntime, pjskClient, usersClient, banChecker, censorService)
	if renderRuntime != nil {
		groupGuardAPI.RegisterGroupGuardRoutes(app, renderRuntime.Toolbox)
	}
	validateBotAuthSecrets(mainLogger)
	noiseKeys := initNoiseKeyRing(mainLogger)
	authEncryptionKey := initAuthEncryptionKey(mainLogger)
	var noiseServerPubKeyHex string
	if noiseKeys != nil {
		noiseServerPubKeyHex = hex.EncodeToString(noiseKeys.Primary().Pair.Public)
	}
	botDBClient := initBot(ctx, mainLogger, app, redisClient, authEncryptionKey, noiseServerPubKeyHex, noiseKeys, banChecker)
	botRouteDispatchers := botPJSK.RegisterPJSKBotRoutesWithContext(ctx, app, renderRuntime, redisClient, botDBClient, noiseKeys)

	if dir := harukiConfig.Cfg.PJSKRender.ImageCache.Dir; dir != "" {
		app.Get("/ic/*", static.New(dir))
		mainLogger.Info("image cache static serving enabled", "http_route", "/ic/*")
	}

	defer closeClients(redisClient, censorService, usersClient, chunithmMainClient, chunithmMusicClient, pjskClient, sekaiClient, botDBClient)
	if botRouteDispatchers != nil {
		defer botRouteDispatchers.Close()
	}
	if drawingCacheService != nil {
		defer func() { _ = drawingCacheService.Close() }()
	}
	if renderRuntime != nil {
		defer func() { _ = renderRuntime.Close() }()
	}

	if renderRuntime != nil {
		mainLogger.Info("PJSK render runtime ready")
	}

	startServer(ctx, mainLogger, app)
}
