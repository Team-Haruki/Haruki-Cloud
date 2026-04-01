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

func initRedis(mainLogger *harukiLogger.Logger) *redis.Client {
	redisClient := harukiRedis.NewRedisClient(harukiConfig.Cfg.Redis)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		mainLogger.Errorf("Failed to connect Redis: %v", err)
		os.Exit(1)
	}
	return redisClient
}

func initChunithmIfEnabled(mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) (*chunithmMainDB.Client, *chunithmMusicDB.Client) {
	if !harukiConfig.Cfg.Chunithm.Enabled {
		return nil, nil
	}

	chunithmMainClient, err := chunithmMainDB.Open(harukiConfig.Cfg.Chunithm.BindingDBType, harukiConfig.Cfg.Chunithm.BindingDBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Chunithm main DB: %v", err)
		os.Exit(1)
	}
	if err := chunithmMainClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Chunithm main DB: %v", err)
		os.Exit(1)
	}

	chunithmMusicClient, err := chunithmMusicDB.Open(harukiConfig.Cfg.Chunithm.MusicDBType, harukiConfig.Cfg.Chunithm.MusicDBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Chunithm music DB: %v", err)
		os.Exit(1)
	}
	if err := chunithmMusicClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Chunithm music DB: %v", err)
		os.Exit(1)
	}

	publicChunithm.RegisterChunithmRoutes(app, chunithmMainClient, chunithmMusicClient, redisClient)
	return chunithmMainClient, chunithmMusicClient
}

func initUsers(mainLogger *harukiLogger.Logger) *usersDB.Client {
	if strings.TrimSpace(harukiConfig.Cfg.UsersDB.DBType) == "" || strings.TrimSpace(harukiConfig.Cfg.UsersDB.DBURL) == "" {
		mainLogger.Warnf("Users DB is not configured; profile binding commands will be unavailable")
		return nil
	}

	client, err := usersDB.Open(harukiConfig.Cfg.UsersDB.DBType, harukiConfig.Cfg.UsersDB.DBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Users DB: %v", err)
		os.Exit(1)
	}
	if err := client.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Users DB: %v", err)
		os.Exit(1)
	}
	return client
}

func initPJSKIfEnabled(mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) *pjskDB.Client {
	if !harukiConfig.Cfg.PJSK.Enabled {
		return nil
	}

	pjskClient, err := pjskDB.Open(harukiConfig.Cfg.PJSK.DBType, harukiConfig.Cfg.PJSK.DBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to PJSK DB: %v", err)
		os.Exit(1)
	}
	if err := pjskClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for PJSK DB: %v", err)
		os.Exit(1)
	}

	publicPJSK.RegisterPJSKRoutes(app, pjskClient, redisClient)
	return pjskClient
}

func initSekaiIfEnabled(mainLogger *harukiLogger.Logger) *sekaiDB.Client {
	if !harukiConfig.Cfg.Sekai.Enabled {
		return nil
	}

	sekaiClient, err := sekaiDB.Open(harukiConfig.Cfg.Sekai.DBType, harukiConfig.Cfg.Sekai.DBURL)
	if err != nil {
		mainLogger.Errorf("Failed to connect to Sekai DB: %v", err)
		os.Exit(1)
	}
	if err := sekaiClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Sekai DB: %v", err)
		os.Exit(1)
	}

	return sekaiClient
}

func initBot(mainLogger *harukiLogger.Logger, app *fiber.App, redisClient *redis.Client) *botDB.Client {
	botDBClient, err := botDB.Open(harukiConfig.Cfg.HarukiBotDB.DBType, harukiConfig.Cfg.HarukiBotDB.DBURL)
	if err != nil {
		mainLogger.Errorf("Failed to initialize Bot entgo client: %v", err)
		os.Exit(1)
	}
	if err := botDBClient.Schema.Create(context.Background()); err != nil {
		mainLogger.Errorf("Failed to create schema for Bot DB: %v", err)
		os.Exit(1)
	}

	botAuth.RegisterBotRoutes(app, botDBClient, redisClient)
	return botDBClient
}
