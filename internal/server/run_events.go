package server

import (
	"context"
	"fmt"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/api"
	harukiConfig "haruki-cloud/config"
	pjskDB "haruki-cloud/database/pjsk"
	"haruki-cloud/internal/cluster"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/realtime"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	json "haruki-cloud/internal/jsonutil"
)

// Process roles. The API role is the default; the events role serves only
// the realtime SSE gateway (`haruki-server events` or HARUKI_ROLE=events).
const (
	RoleAPI    = "api"
	RoleEvents = "events"
)

// eventsBodyLimit caps request bodies on the events role; ingest and close
// bodies are a few hundred bytes.
const eventsBodyLimit = 64 << 10

// ResolveRole picks the process role from the first command-line argument,
// falling back to the HARUKI_ROLE value.
func ResolveRole(args []string, env string) string {
	if len(args) > 0 {
		if role := strings.ToLower(strings.TrimSpace(args[0])); role == RoleEvents || role == RoleAPI {
			return role
		}
	}
	if strings.EqualFold(strings.TrimSpace(env), RoleEvents) {
		return RoleEvents
	}
	return RoleAPI
}

// RunEvents boots the events role. It opens only the PJSK database: no
// schema migration (the API role owns migrations, so it deploys first), no
// Redis, render runtime or master data.
func RunEvents(ctx context.Context) {
	ctx = ensureContext(ctx)
	loggerWriter := setupLogging()
	mainLogger := harukiLogger.NewLogger("Events", harukiConfig.Cfg.Backend.LogLevel, loggerWriter)
	defer closeMainLogFile(mainLogger)
	logStartupInfo(mainLogger)

	pjskClient := openEventsPJSKClient(mainLogger)
	defer func() { _ = pjskClient.Close() }()

	service := newEventsService(pjskClient, mainLogger)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := service.Store().Ping(pingCtx); err != nil {
		mainLogger.Warn("realtime_events table is not reachable; deploy the API role first so it migrates the schema",
			"event", "realtime_schema_missing", "error_type", fmt.Sprintf("%T", err))
	}
	cancel()

	app := createEventsApp(mainLogger)
	service.Register(app, api.VerifyAPIAuthorization())
	go service.Run(ctx)
	startEventsServer(ctx, mainLogger, app, service, eventsListenAddr(harukiConfig.Cfg.Events.WithDefaults()))
}

func openEventsPJSKClient(mainLogger *harukiLogger.Logger) *pjskDB.Client {
	cfg := harukiConfig.Cfg.PJSK
	if !cfg.Enabled || strings.TrimSpace(cfg.DBURL) == "" {
		fatalStartup(mainLogger, "events role needs the PJSK database (pjsk.enabled and pjsk.db_url)")
	}
	drv, err := openEntDriver("pjsk", cfg.DBType, cfg.DBURL, cfg.DBPool())
	if err != nil {
		fatalStartup(mainLogger, "failed to connect to database", "database", "PJSK", "error_type", fmt.Sprintf("%T", err))
	}
	client := pjskDB.NewClient(pjskDB.Driver(drv))
	installEntTracing(client)
	return client
}

func newEventsService(pjskClient *pjskDB.Client, logger *harukiLogger.Logger) *realtime.Service {
	return realtime.NewService(pjskClient, harukiConfig.Cfg.Events, realtime.Options{
		Logger:   logger,
		ReadOnly: cluster.IsReadOnly,
	})
}

// createEventsApp is the events role's own app: no access log or per-route
// body limits, no compression, and no read/write/idle timeouts, which would
// cut long-lived streams.
func createEventsApp(mainLogger *harukiLogger.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit:   eventsBodyLimit,
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
	})
	app.Use(recover.New(recover.Config{PanicHandler: func(c fiber.Ctx, recovered any) error {
		attrs := []any{"panic_type", fmt.Sprintf("%T", recovered)}
		if !harukiConfig.Cfg.Profile.IsProduction() {
			attrs = append(attrs, "stack", string(debug.Stack()))
		}
		mainLogger.ErrorContext(c.Context(), "request panic recovered", attrs...)
		return fiber.ErrInternalServerError
	}}))
	return app
}

func eventsListenAddr(cfg harukiConfig.EventsConfig) string {
	return net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
}

// startEventsServer serves until ctx ends, then ends the streams with a
// retry hint before shutting the listener down.
func startEventsServer(ctx context.Context, mainLogger *harukiLogger.Logger, app *fiber.App, service *realtime.Service, addr string) {
	mainLogger.Info("events server starting", "listen_addr", addr)
	go func() {
		<-ctx.Done()
		service.Shutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), harukiConfig.DefaultEventsShutdownTimeout)
		defer cancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			mainLogger.Error("graceful events shutdown failed", "error_type", fmt.Sprintf("%T", err))
		}
	}()
	if err := app.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		if ctx.Err() != nil {
			mainLogger.Info("events server stopped")
			return
		}
		fatalStartup(mainLogger, "failed to start events server", "error_type", fmt.Sprintf("%T", err))
	}
}

// initEmbeddedEvents mounts the realtime routes on the main app when
// events.embedded is set. It must run before any /internal group is
// registered: the group's shared-token middleware would otherwise reject
// the ingest token.
func initEmbeddedEvents(ctx context.Context, mainLogger *harukiLogger.Logger, app *fiber.App, pjskClient *pjskDB.Client) *realtime.Service {
	if !harukiConfig.Cfg.Events.Embedded {
		return nil
	}
	if pjskClient == nil {
		mainLogger.Warn("events.embedded needs the PJSK database; realtime routes are not mounted", "event", "realtime_embedded_disabled")
		return nil
	}
	service := newEventsService(pjskClient, mainLogger)
	service.Register(app, api.VerifyAPIAuthorization())
	go service.Run(ensureContext(ctx))
	mainLogger.Info("realtime routes mounted on the main app", "event", "realtime_embedded")
	return service
}

// eventStreamCloser is how the API role closes the streams of a replaced
// or cancelled subscription: in process when embedded, else over HTTP to
// the events role, else not at all.
func eventStreamCloser(embedded *realtime.Service) realtime.Closer {
	if embedded != nil {
		return embedded
	}
	closer := realtime.NewHTTPCloser(harukiConfig.Cfg.Events.InternalBaseURL, api.InternalAPIAuthorization(), internalCallerUserAgent())
	if closer == nil {
		return nil
	}
	return closer
}

// internalCallerUserAgent satisfies backend.accept_user_agent when set.
func internalCallerUserAgent() string {
	if ua := strings.TrimSpace(harukiConfig.Cfg.Backend.AcceptUserAgent); ua != "" {
		return ua
	}
	if ua := strings.TrimSpace(harukiConfig.Cfg.HMES.UserAgent); ua != "" {
		return ua
	}
	return "Haruki-Cloud"
}

func wireEventStreams(renderRuntime *renderapp.App, closer realtime.Closer) {
	if renderRuntime != nil {
		renderRuntime.EventStreams = closer
	}
}
