package auth

import (
	ent "haruki-cloud/database/bot"
	"haruki-cloud/internal/core/crypto"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// RegisterBotRoutes registers the legacy AuthV2 public routes plus the
// internal and statistics routes. It does not mount AuthV3 because that
// endpoint needs the Noise key ring; use RegisterBotRoutesWithBanChecker.
func RegisterBotRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string) {
	RegisterBotRoutesWithBanChecker(app, dbClient, redisClient, authEncryptionKey, noiseServerPubKey, nil, nil)
}

// RegisterBotRoutesWithBanChecker registers every bot auth route. When
// noiseKeys is non-nil the Noise-wrapped AuthV3 endpoint is mounted as well.
func RegisterBotRoutesWithBanChecker(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string, noiseKeys *crypto.KeyRing, checker GlobalBanChecker) {
	registerUserRoutes(app, dbClient, redisClient, authEncryptionKey, noiseServerPubKey, noiseKeys, checker)
	registerInternalRoutes(app, dbClient, redisClient, checker)
	registerStatisticsRoutes(app, dbClient)
}
