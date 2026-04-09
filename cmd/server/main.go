package main

import (
	"context"
	"os/signal"
	"syscall"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	botPJSK "haruki-cloud/api/bot/pjsk"

	"github.com/gofiber/fiber/v3/middleware/static"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	_ "modernc.org/sqlite"
)

var Version = "2.0.0-dev"

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	loggerWriter := setupLogging()
	mainLogger := harukiLogger.NewLogger("Main", harukiConfig.Cfg.Backend.LogLevel, loggerWriter)
	logStartupInfo(mainLogger)
	redisClient := initRedis(rootCtx, mainLogger)
	app := createFiberApp(mainLogger)
	usersClient := initUsers(rootCtx, mainLogger)
	chunithmMainClient, chunithmMusicClient := initChunithmIfEnabled(rootCtx, mainLogger, app, redisClient)
	pjskClient := initPJSKIfEnabled(rootCtx, mainLogger, app, redisClient)
	sekaiClient := initSekaiIfEnabled(rootCtx, mainLogger)
	renderRuntime := initPJSKRenderIfEnabled(rootCtx, mainLogger, sekaiClient, pjskClient)
	censorService := initCensorIfEnabled(rootCtx, mainLogger, renderRuntime)
	configureSekaiRuntime(mainLogger, renderRuntime, pjskClient, usersClient, censorService)
	botDBClient := initBot(rootCtx, mainLogger, app, redisClient)
	noiseKeyPair := initNoiseKeyPair(mainLogger)
	botPJSK.RegisterPJSKBotRoutesWithContext(rootCtx, app, renderRuntime, redisClient, botDBClient, noiseKeyPair)

	if dir := harukiConfig.Cfg.PJSKRender.ImageCache.Dir; dir != "" {
		app.Get("/ic/*", static.New(dir))
		mainLogger.Infof("Image cache static serving enabled at /ic -> %s", dir)
	}

	defer closeClients(redisClient, censorService, usersClient, chunithmMainClient, chunithmMusicClient, pjskClient, sekaiClient, botDBClient)
	if renderRuntime != nil {
		defer func() { _ = renderRuntime.Close() }()
	}

	if renderRuntime != nil {
		mainLogger.Infof("PJSK render runtime initialized")
	}

	startServer(rootCtx, mainLogger, app)
}
