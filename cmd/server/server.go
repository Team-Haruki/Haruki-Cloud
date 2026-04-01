package main

import (
	"fmt"
	"os"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	botDB "haruki-cloud/database/bot"
	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	usersDB "haruki-cloud/database/users"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

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
