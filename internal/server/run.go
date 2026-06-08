package server

import (
	"context"
	"encoding/hex"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	botPJSK "haruki-cloud/api/bot/pjsk"
	groupGuardAPI "haruki-cloud/api/groupguard"

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
	chunithmMainClient, chunithmMusicClient := initChunithmIfEnabled(ctx, mainLogger, app, redisClient)
	pjskClient := initPJSKIfEnabled(ctx, mainLogger, app, redisClient)
	startSekaiDBRemoteSync(ctx, mainLogger)
	sekaiClient := initSekaiIfEnabled(ctx, mainLogger)
	renderRuntime := initPJSKRenderIfEnabled(ctx, mainLogger, sekaiClient, pjskClient)
	censorService := initCensorIfEnabled(ctx, mainLogger, renderRuntime)
	configureSekaiRuntime(mainLogger, renderRuntime, pjskClient, usersClient, censorService)
	if renderRuntime != nil {
		groupGuardAPI.RegisterGroupGuardRoutes(app, renderRuntime.Toolbox)
	}
	noiseKeyPair := initNoiseKeyPair(mainLogger)
	authEncryptionKey := initAuthEncryptionKey(mainLogger)
	var noiseServerPubKeyHex string
	if noiseKeyPair != nil {
		noiseServerPubKeyHex = hex.EncodeToString(noiseKeyPair.Public)
	}
	botDBClient := initBot(ctx, mainLogger, app, redisClient, authEncryptionKey, noiseServerPubKeyHex)
	botPJSK.RegisterPJSKBotRoutesWithContext(ctx, app, renderRuntime, redisClient, botDBClient, noiseKeyPair)

	if dir := harukiConfig.Cfg.PJSKRender.ImageCache.Dir; dir != "" {
		app.Get("/ic/*", static.New(dir))
		mainLogger.Infof("Image cache static serving enabled at /ic -> %s", dir)
	}

	defer closeClients(redisClient, censorService, usersClient, chunithmMainClient, chunithmMusicClient, pjskClient, sekaiClient, botDBClient)
	if drawingCacheService != nil {
		defer func() { _ = drawingCacheService.Close() }()
	}
	if renderRuntime != nil {
		defer func() { _ = renderRuntime.Close() }()
	}

	if renderRuntime != nil {
		mainLogger.Infof("PJSK render runtime initialized")
	}

	startServer(ctx, mainLogger, app)
}
