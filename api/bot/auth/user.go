package auth

import (
	ent "haruki-cloud/database/bot"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/middleware/secure"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ================= Route Registration =================

// registerUserRoutes mounts the public bot session routes:
//
//	POST   /api/v3/bot/:bot_id/auth    Noise NK wrapped login (AuthV3)
//	DELETE /api/v3/bot/:bot_id/logout  session revocation
//
// The secure middleware performs the per-request Noise handshake for the login
// route, so the handler only ever sees the decrypted MsgPack payload and its
// response is encrypted on the way out. Nothing is mounted without a key ring:
// this Cloud line has no plaintext or shared-key login path.
func registerUserRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, opts BotAuthOptions) {
	if opts.NoiseKeys == nil {
		return
	}
	svc := NewUserService(dbClient, redisClient).
		WithGlobalBanChecker(opts.BanChecker).
		WithBuildPolicy(opts.BuildPolicy).
		WithSecurityReporter(opts.Security)
	registerAuthV3Routes(app, NewUserHandler(svc), opts.NoiseKeys)
}

// registerAuthV3Routes mounts the AuthV3 login route behind the Noise NK
// middleware and the header-authenticated logout route next to it.
func registerAuthV3Routes(app *fiber.App, h *UserHandler, noiseKeys *crypto.KeyRing) {
	public := app.Group(AuthV3RouteBase)
	public.Post("/:bot_id/auth", secure.New(secure.Config{KeyRing: noiseKeys}), h.AuthV3)
	public.Delete("/:bot_id/logout", h.Logout)
}
