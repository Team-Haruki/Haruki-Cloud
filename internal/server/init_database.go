package server

import (
	"context"
	"fmt"
	"strings"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"
	harukiRedis "haruki-cloud/utils/redis"

	botAuth "haruki-cloud/api/bot/auth"
	publicChunithm "haruki-cloud/api/public/chunithm"
	publicPJSK "haruki-cloud/api/public/pjsk"

	botDB "haruki-cloud/database/bot"
	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"
	pjskDB "haruki-cloud/database/pjsk"
	sekaiDB "haruki-cloud/database/sekai"
	usersDB "haruki-cloud/database/users"
	"haruki-cloud/internal/observability/commandtrace"

	"entgo.io/ent"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ensureContext returns ctx if non-nil, otherwise returns context.Background().
func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// initDBClient opens a database connection and creates its schema.
// On failure it emits a synchronous fatal record and terminates after a
// bounded best-effort log flush.
func initDBClient[T interface {
	Close() error
}](ctx context.Context, logger *harukiLogger.Logger, name string, openFn func() (T, error), schemaFn func(T, context.Context) error) T {
	client, err := openFn()
	if err != nil {
		fatalStartup(logger, "failed to connect to database", "database", name, "error_type", fmt.Sprintf("%T", err))
	}
	if err := schemaFn(client, ctx); err != nil {
		fatalStartup(logger, "failed to create database schema", "database", name, "error_type", fmt.Sprintf("%T", err))
	}
	installEntTracing(client)
	return client
}

type traceableEntClient interface {
	Intercept(...ent.Interceptor)
	Use(...ent.Hook)
}

func installEntTracing(client any) {
	if traced, ok := client.(traceableEntClient); ok {
		traced.Intercept(commandtrace.EntQueryInterceptor())
		traced.Use(commandtrace.EntMutationHook())
	}
}

func initRedis(ctx context.Context, mainLogger *harukiLogger.Logger) *redis.Client {
	ctx = ensureContext(ctx)
	redisClient := harukiRedis.NewRedisClient(harukiConfig.Cfg.Redis)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		fatalStartup(mainLogger, "failed to connect Redis", "error_type", fmt.Sprintf("%T", err))
	}
	return redisClient
}

func initChunithmIfEnabled(ctx context.Context, mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) (*chunithmMainDB.Client, *chunithmMusicDB.Client) {
	if !harukiConfig.Cfg.Chunithm.Enabled {
		return nil, nil
	}
	ctx = ensureContext(ctx)

	chunithmMainClient := initDBClient(ctx, mainLogger, "Chunithm main",
		func() (*chunithmMainDB.Client, error) {
			return chunithmMainDB.Open(harukiConfig.Cfg.Chunithm.BindingDBType, harukiConfig.Cfg.Chunithm.BindingDBURL)
		},
		func(c *chunithmMainDB.Client, ctx context.Context) error { return c.Schema.Create(ctx) },
	)

	chunithmMusicClient := initDBClient(ctx, mainLogger, "Chunithm music",
		func() (*chunithmMusicDB.Client, error) {
			return chunithmMusicDB.Open(harukiConfig.Cfg.Chunithm.MusicDBType, harukiConfig.Cfg.Chunithm.MusicDBURL)
		},
		func(c *chunithmMusicDB.Client, ctx context.Context) error { return c.Schema.Create(ctx) },
	)

	publicChunithm.RegisterChunithmRoutes(app, chunithmMainClient, chunithmMusicClient, redisClient)
	return chunithmMainClient, chunithmMusicClient
}

func initUsers(ctx context.Context, mainLogger *harukiLogger.Logger) *usersDB.Client {
	if strings.TrimSpace(harukiConfig.Cfg.UsersDB.DBType) == "" || strings.TrimSpace(harukiConfig.Cfg.UsersDB.DBURL) == "" {
		mainLogger.Warn("users database is not configured", "profile_binding_available", false)
		return nil
	}
	ctx = ensureContext(ctx)

	return initDBClient(ctx, mainLogger, "Users",
		func() (*usersDB.Client, error) {
			return usersDB.Open(harukiConfig.Cfg.UsersDB.DBType, harukiConfig.Cfg.UsersDB.DBURL)
		},
		func(c *usersDB.Client, ctx context.Context) error { return c.Schema.Create(ctx) },
	)
}

func initPJSKIfEnabled(ctx context.Context, mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) *pjskDB.Client {
	if !harukiConfig.Cfg.PJSK.Enabled {
		return nil
	}
	ctx = ensureContext(ctx)

	pjskClient := initDBClient(ctx, mainLogger, "PJSK",
		func() (*pjskDB.Client, error) {
			return pjskDB.Open(harukiConfig.Cfg.PJSK.DBType, harukiConfig.Cfg.PJSK.DBURL)
		},
		func(c *pjskDB.Client, ctx context.Context) error { return c.Schema.Create(ctx) },
	)

	publicPJSK.RegisterPJSKRoutes(app, pjskClient, redisClient)
	return pjskClient
}

func initSekaiIfEnabled(ctx context.Context, mainLogger *harukiLogger.Logger) *sekaiDB.Client {
	if !harukiConfig.Cfg.Sekai.Enabled {
		return nil
	}
	ctx = ensureContext(ctx)

	client, err := sekaiDB.Open(harukiConfig.Cfg.Sekai.DBType, harukiConfig.Cfg.Sekai.DBURL)
	if err != nil {
		fatalStartup(mainLogger, "failed to connect to Sekai DB", "error_type", fmt.Sprintf("%T", err))
	}
	if harukiConfig.Cfg.Sekai.AutoMigrate {
		if err := client.Schema.Create(ctx); err != nil {
			fatalStartup(mainLogger, "failed to create Sekai DB schema", "error_type", fmt.Sprintf("%T", err))
		}
	} else {
		mainLogger.Info("Sekai DB auto-migrate disabled")
	}
	installEntTracing(client)
	return client
}

func initBot(ctx context.Context, mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client, authOpts botAuth.BotAuthOptions) *botDB.Client {
	ctx = ensureContext(ctx)

	botDBClient := initDBClient(ctx, mainLogger, "Bot",
		func() (*botDB.Client, error) {
			return botDB.Open(harukiConfig.Cfg.HarukiBotDB.DBType, harukiConfig.Cfg.HarukiBotDB.DBURL)
		},
		func(c *botDB.Client, ctx context.Context) error { return c.Schema.Create(ctx) },
	)

	botAuth.RegisterBotRoutesWithOptions(app, botDBClient, redisClient, authOpts)
	return botDBClient
}
