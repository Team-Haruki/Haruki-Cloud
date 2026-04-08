package main

import (
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
	censorService := initCensorIfEnabled(mainLogger, renderRuntime)
	configureSekaiRuntime(mainLogger, renderRuntime, pjskClient, usersClient, censorService)
	botDBClient := initBot(mainLogger, app, redisClient)
	noiseKeyPair := initNoiseKeyPair(mainLogger)
	botPJSK.RegisterPJSKBotRoutes(app, renderRuntime, redisClient, botDBClient, noiseKeyPair)

	if dir := harukiConfig.Cfg.PJSKRender.ImageCache.Dir; dir != "" {
		app.Get("/ic/*", static.New(dir))
		mainLogger.Infof("Image cache static serving enabled at /ic -> %s", dir)
	}

	defer closeClients(usersClient, chunithmMainClient, chunithmMusicClient, pjskClient, sekaiClient, botDBClient)

	if renderRuntime != nil {
		mainLogger.Infof("PJSK render runtime initialized")
	}

	startServer(mainLogger, app)
}
