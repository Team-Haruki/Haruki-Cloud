package auth

import (
	ent "haruki-cloud/database/bot"
	"haruki-cloud/internal/core/buildpolicy"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/core/secevent"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// RegisterBotRoutes registers the internal and statistics routes only. The
// public AuthV3 routes need the Noise key ring; use
// RegisterBotRoutesWithBanChecker to mount them.
func RegisterBotRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client) {
	RegisterBotRoutesWithBanChecker(app, dbClient, redisClient, nil, nil)
}

// BotAuthOptions carries the optional collaborators of the public AuthV3
// routes. Every field is nil-safe: a nil BuildPolicy means policy off, a nil
// Security reporter drops events.
type BotAuthOptions struct {
	NoiseKeys   *crypto.KeyRing
	BanChecker  GlobalBanChecker
	BuildPolicy *buildpolicy.Store
	Security    secevent.Reporter
}

// RegisterBotRoutesWithBanChecker registers every bot auth route. The public
// AuthV3 login/logout routes are mounted only when noiseKeys is non-nil.
func RegisterBotRoutesWithBanChecker(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, noiseKeys *crypto.KeyRing, checker GlobalBanChecker) {
	RegisterBotRoutesWithOptions(app, dbClient, redisClient, BotAuthOptions{NoiseKeys: noiseKeys, BanChecker: checker})
}

// RegisterBotRoutesWithOptions registers every bot auth route with the full
// set of collaborators.
func RegisterBotRoutesWithOptions(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, opts BotAuthOptions) {
	registerUserRoutes(app, dbClient, redisClient, opts)
	registerInternalRoutes(app, dbClient, redisClient, opts.BanChecker)
	registerStatisticsRoutes(app, dbClient)
}
