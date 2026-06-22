package server

import (
	"context"
	"fmt"
	"os"
	"time"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	botDB "haruki-cloud/database/bot"
	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	usersDB "haruki-cloud/database/users"
	"haruki-cloud/utils/censor"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/redis/go-redis/v9"
)

var accessLogFileHandle *os.File

func createFiberApp(mainLogger *harukiLogger.Logger) *fiber.App {
	accessLogFileHandle = nil
	app := fiber.New(fiber.Config{
		BodyLimit:   300 * 1024 * 1024,
		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,
		ProxyHeader: harukiConfig.Cfg.Backend.ProxyHeader,
		TrustProxy:  harukiConfig.Cfg.Backend.EnableTrustProxy,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: harukiConfig.Cfg.Backend.TrustProxies,
		},
	})

	app.Use(recover.New(recover.Config{
		EnableStackTrace: !harukiConfig.Cfg.Profile.IsProduction(),
	}))

	if harukiConfig.Cfg.Backend.AccessLog != "" {
		loggerConfig := logger.Config{Format: harukiConfig.Cfg.Backend.AccessLog}
		if harukiConfig.Cfg.Backend.AccessLogPath != "" {
			accessLogFile, err := harukiLogger.OpenLogFile(harukiConfig.Cfg.Backend.AccessLogPath)
			if err != nil {
				mainLogger.Errorf("Failed to open access log file: %v", err)
				os.Exit(1)
			}
			accessLogFileHandle = accessLogFile
			loggerConfig.Stream = accessLogFile
		}
		app.Use(logger.New(loggerConfig))
	}
	registerReadinessRoute(app)
	return app
}

func registerReadinessRoute(app *fiber.App) {
	app.Get("/readyz", func(c fiber.Ctx) error {
		// Minimal readiness signal only. Deployment profile, build version and
		// node name/role are intentionally NOT exposed here — this endpoint is
		// public/unauthenticated, and that metadata aids targeting/recon. Expose
		// it via an authenticated endpoint if monitoring needs it.
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":  fiber.StatusOK,
			"message": "ok",
		})
	})
}

func closeAccessLogFile(mainLogger *harukiLogger.Logger) {
	if accessLogFileHandle == nil {
		return
	}
	if err := accessLogFileHandle.Close(); err != nil && mainLogger != nil {
		mainLogger.Warnf("failed to close access log file: %v", err)
	}
	accessLogFileHandle = nil
}

func closeClients(
	redisClient *redis.Client,
	censorService *censor.Service,
	usersClient *usersDB.Client,
	chunithmMainClient *chunithmMainDB.Client,
	chunithmMusicClient *chunithmMusicDB.Client,
	pjskClient *pjskDB.Client,
	sekaiClient *sekaiDB.Client,
	botDBClient *botDB.Client,
) {
	if redisClient != nil {
		_ = redisClient.Close()
	}
	if censorService != nil {
		_ = censorService.Close()
	}
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

func startServer(ctx context.Context, mainLogger *harukiLogger.Logger, app *fiber.App) {
	addr := fmt.Sprintf("%s:%d", harukiConfig.Cfg.Backend.Host, harukiConfig.Cfg.Backend.Port)
	listenConfig := fiber.ListenConfig{
		DisableStartupMessage: true,
	}
	ctx = ensureContext(ctx)
	if harukiConfig.Cfg.Backend.SSL {
		listenConfig.CertFile = harukiConfig.Cfg.Backend.SSLCert
		listenConfig.CertKeyFile = harukiConfig.Cfg.Backend.SSLKey
		mainLogger.Infof("SSL enabled, starting HTTPS server at %s", addr)
	} else {
		mainLogger.Infof("Starting HTTP server at %s", addr)
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			mainLogger.Errorf("Failed graceful shutdown: %v", err)
		}
	}()

	if err := app.Listen(addr, listenConfig); err != nil {
		if ctx.Err() != nil {
			mainLogger.Infof("HTTP server stopped")
			return
		}
		mainLogger.Errorf("Failed to start HTTP server: %v", err)
		os.Exit(1)
	}
}
