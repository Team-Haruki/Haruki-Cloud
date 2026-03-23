package auth

import (
	ent "haruki-cloud/database/bot"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func RegisterBotRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client) {
	registerUserRoutes(app, dbClient, redisClient)
	registerInternalRoutes(app, dbClient, redisClient)
	registerStatisticsRoutes(app, dbClient)
}
