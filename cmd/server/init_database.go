package main

import (
	"context"
	"os"
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
// On failure it logs the error and calls os.Exit(1).
func initDBClient[T interface {
	Close() error
}](ctx context.Context, logger *harukiLogger.Logger, name string, openFn func() (T, error), schemaFn func(T, context.Context) error) T {
	client, err := openFn()
	if err != nil {
		logger.Errorf("Failed to connect to %s DB: %v", name, err)
		os.Exit(1)
	}
	if err := schemaFn(client, ctx); err != nil {
		logger.Errorf("Failed to create schema for %s DB: %v", name, err)
		os.Exit(1)
	}
	return client
}

func initRedis(ctx context.Context, mainLogger *harukiLogger.Logger) *redis.Client {
	ctx = ensureContext(ctx)
	redisClient := harukiRedis.NewRedisClient(harukiConfig.Cfg.Redis)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		mainLogger.Errorf("Failed to connect Redis: %v", err)
		os.Exit(1)
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
		mainLogger.Warnf("Users DB is not configured; profile binding commands will be unavailable")
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

	return initDBClient(ctx, mainLogger, "Sekai",
		func() (*sekaiDB.Client, error) {
			return sekaiDB.Open(harukiConfig.Cfg.Sekai.DBType, harukiConfig.Cfg.Sekai.DBURL)
		},
		func(c *sekaiDB.Client, ctx context.Context) error { return c.Schema.Create(ctx) },
	)
}

func initBot(ctx context.Context, mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) *botDB.Client {
	ctx = ensureContext(ctx)

	botDBClient := initDBClient(ctx, mainLogger, "Bot",
		func() (*botDB.Client, error) {
			return botDB.Open(harukiConfig.Cfg.HarukiBotDB.DBType, harukiConfig.Cfg.HarukiBotDB.DBURL)
		},
		func(c *botDB.Client, ctx context.Context) error { return c.Schema.Create(ctx) },
	)

	botAuth.RegisterBotRoutes(app, botDBClient, redisClient)
	return botDBClient
}
