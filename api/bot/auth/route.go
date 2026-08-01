package auth

import (
	ent "haruki-cloud/database/bot"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func RegisterBotRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string) {
	RegisterBotRoutesWithBanChecker(app, dbClient, redisClient, authEncryptionKey, noiseServerPubKey, nil)
}

func RegisterBotRoutesWithBanChecker(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string, checker GlobalBanChecker) {
	registerUserRoutes(app, dbClient, redisClient, authEncryptionKey, noiseServerPubKey, checker)
	registerInternalRoutes(app, dbClient, redisClient, checker)
	registerStatisticsRoutes(app, dbClient)
}
