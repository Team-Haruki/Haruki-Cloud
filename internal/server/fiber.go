package server

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	botDB "haruki-cloud/database/bot"
	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	usersDB "haruki-cloud/database/users"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/censor"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/redis/go-redis/v9"
	json "haruki-cloud/internal/jsonutil"
)

var accessLogFileHandle *os.File
var accessLogAsyncWriter *harukiLogger.AsyncWriter

const (
	globalRequestBodyLimit        = 16 << 20
	defaultRequestBodyLimit       = 1 << 20
	botCommandRequestBodyLimit    = 4 << 20
	birthdayEventRequestBodyLimit = 12 << 20
	cacheControlRequestBodyLimit  = 64 << 10
)

func createFiberApp(mainLogger *harukiLogger.Logger) *fiber.App {
	accessLogFileHandle = nil
	accessLogAsyncWriter = nil
	app := fiber.New(fiber.Config{
		BodyLimit:   globalRequestBodyLimit,
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
		ProxyHeader: harukiConfig.Cfg.Backend.ProxyHeader,
		TrustProxy:  harukiConfig.Cfg.Backend.EnableTrustProxy,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: harukiConfig.Cfg.Backend.TrustProxies,
		},
	})

	app.Use(requestid.New())
	if harukiConfig.Cfg.Backend.AccessLog != "" {
		accessWriter := harukiLogger.GlobalWriter()
		if harukiConfig.Cfg.Backend.AccessLogPath != "" {
			accessLogFile, err := harukiLogger.OpenLogFile(harukiConfig.Cfg.Backend.AccessLogPath)
			if err != nil {
				fatalStartup(mainLogger, "failed to open access log file", "error_type", fmt.Sprintf("%T", err))
			}
			accessLogFileHandle = accessLogFile
			accessLogAsyncWriter = harukiLogger.NewReliableAsyncWriter(accessLogFile, logQueueCapacity, reliableLogOverflowWriter)
			accessWriter = accessLogAsyncWriter
		}
		app.Use(accessLogMiddleware(harukiLogger.NewLogger("HTTP", "INFO", accessWriter)))
	}
	app.Use(recover.New(recover.Config{PanicHandler: func(c fiber.Ctx, recovered any) error {
		attrs := []any{"panic_type", fmt.Sprintf("%T", recovered)}
		if !harukiConfig.Cfg.Profile.IsProduction() {
			attrs = append(attrs, "stack", string(debug.Stack()))
		}
		mainLogger.ErrorContext(c.Context(), "request panic recovered", attrs...)
		return fiber.ErrInternalServerError
	}}))
	app.Use(requestBodyLimitMiddleware())
	registerReadinessRoute(app)
	return app
}

// requestBodyLimitMiddleware applies tighter endpoint-family limits before any
// route handler binds JSON/MsgPack or the Noise middleware decrypts a bot body.
// Fiber's global limit remains the hard pre-routing cap; these smaller limits
// keep ordinary public and control-plane requests from consuming that entire
// allowance. Compressed request bodies are rejected because Fiber can only
// enforce its global decompressed limit; accepting them here would let a small
// wire body bypass the tighter per-route limits. The only larger exception is
// the internal-token-protected birthday event payload, which carries a filtered
// MySekai snapshot.
func requestBodyLimitMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		encoding := strings.TrimSpace(c.Get(fiber.HeaderContentEncoding))
		if encoding != "" && !strings.EqualFold(encoding, "identity") {
			return fiber.ErrUnsupportedMediaType
		}
		if len(c.Request().Body()) > requestBodyLimitForPath(c.Path()) {
			return fiber.ErrRequestEntityTooLarge
		}
		return c.Next()
	}
}

func requestBodyLimitForPath(path string) int {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path != "" {
		path = "/" + path
	}
	switch path {
	case "/internal/subscription-events/mysekai-birthday":
		return birthdayEventRequestBodyLimit
	case "/cache", "/cache/stats":
		return cacheControlRequestBodyLimit
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 5 && parts[0] == "api" && parts[1] == "v2" && parts[2] == "bot" {
		if parts[4] == "pjsk" {
			return botCommandRequestBodyLimit
		}
		if len(parts) == 5 && parts[4] == "auth" {
			return cacheControlRequestBodyLimit
		}
	}
	return defaultRequestBodyLimit
}

func accessLogMiddleware(accessLogger *harukiLogger.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		startedAt := time.Now()
		requestBytes := len(c.Request().Body())
		var observedErr error
		errorType := ""
		defer func() {
			statusCode := c.Response().StatusCode()
			attrs := []any{
				"event", "http_request",
				"request_id", requestid.FromContext(c),
				"http_method", c.Method(),
				"http_route", c.Route().Path,
				"status_code", statusCode,
				"server_duration_ms", commandtrace.Milliseconds(time.Since(startedAt)),
				"duration_scope", "fiber_handler",
				"request_bytes", requestBytes,
				"response_bytes", len(c.Response().Body()),
			}
			if botID := strings.TrimSpace(c.Params("botId")); botID != "" {
				attrs = append(attrs, "bot_id", botID)
			}
			if errorType != "" {
				attrs = append(attrs, "error_type", errorType)
			}
			logCtx := context.Background()
			switch {
			case statusCode >= fiber.StatusInternalServerError || (observedErr != nil && statusCode < fiber.StatusBadRequest):
				accessLogger.ErrorContext(logCtx, "http request completed", attrs...)
			case statusCode >= fiber.StatusBadRequest:
				accessLogger.WarnContext(logCtx, "http request completed", attrs...)
			default:
				accessLogger.InfoContext(logCtx, "http request completed", attrs...)
			}
		}()
		observedErr = c.Next()
		if observedErr != nil {
			errorType = fmt.Sprintf("%T", observedErr)
			if handlerErr := c.App().ErrorHandler(c, observedErr); handlerErr != nil {
				errorType = fmt.Sprintf("%T", handlerErr)
				c.Response().SetStatusCode(fiber.StatusInternalServerError)
				c.Response().Header.SetContentType("text/plain; charset=utf-8")
				c.Response().SetBodyString("Internal Server Error")
			}
		}
		return nil
	}
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
	flushComplete := true
	if accessLogAsyncWriter != nil {
		if dropped := accessLogAsyncWriter.Dropped(); dropped > 0 && mainLogger != nil {
			mainLogger.Warn("access log queue dropped records", "event", "log_queue_drop", "records", dropped)
		}
		ctx, cancel := context.WithTimeout(context.Background(), logFlushTimeout)
		err := accessLogAsyncWriter.CloseContext(ctx)
		cancel()
		if err != nil {
			flushComplete = false
			if mainLogger != nil {
				mainLogger.Error("access log flush failed", "error_type", fmt.Sprintf("%T", err))
			}
		}
		accessLogAsyncWriter = nil
	}
	if accessLogFileHandle == nil || !flushComplete {
		return
	}
	if err := accessLogFileHandle.Close(); err != nil && mainLogger != nil {
		mainLogger.Warn("failed to close access log file", "error_type", fmt.Sprintf("%T", err))
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
		mainLogger.Info("HTTP server starting", "scheme", "https", "listen_addr", addr)
	} else {
		mainLogger.Info("HTTP server starting", "scheme", "http", "listen_addr", addr)
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			mainLogger.Error("graceful HTTP shutdown failed", "error_type", fmt.Sprintf("%T", err))
		}
	}()

	if err := app.Listen(addr, listenConfig); err != nil {
		if ctx.Err() != nil {
			mainLogger.Info("HTTP server stopped")
			return
		}
		fatalStartup(mainLogger, "failed to start HTTP server", "error_type", fmt.Sprintf("%T", err))
	}
}
