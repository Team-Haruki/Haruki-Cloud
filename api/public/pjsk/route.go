package pjsk

import (
	"haruki-cloud/database/pjsk"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func RegisterPJSKRoutes(app *fiber.App, client *pjsk.Client, redisClient *redis.Client) {
	group := app.Group("/api/v2/public/pjsk")
	// Keep alias query endpoints public; mutation/audit operations are disabled.
	registerAliasRoutes(group, client, redisClient)
}
