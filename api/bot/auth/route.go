package auth

import (
	ent "haruki-cloud/database/bot"
	"haruki-cloud/internal/core/crypto"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// RegisterBotRoutes registers the internal and statistics routes only. The
// public AuthV3 routes need the Noise key ring; use
// RegisterBotRoutesWithBanChecker to mount them.
func RegisterBotRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client) {
	RegisterBotRoutesWithBanChecker(app, dbClient, redisClient, nil, nil)
}

// RegisterBotRoutesWithBanChecker registers every bot auth route. The public
// AuthV3 login/logout routes are mounted only when noiseKeys is non-nil.
func RegisterBotRoutesWithBanChecker(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, noiseKeys *crypto.KeyRing, checker GlobalBanChecker) {
	registerUserRoutes(app, dbClient, redisClient, noiseKeys, checker)
	registerInternalRoutes(app, dbClient, redisClient, checker)
	registerStatisticsRoutes(app, dbClient)
}
