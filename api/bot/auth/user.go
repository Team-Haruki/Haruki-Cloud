package auth

import (
	ent "haruki-cloud/database/bot"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/middleware/secure"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ================= Route Registration =================

func registerUserRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string, noiseKeys *crypto.KeyRing, checker GlobalBanChecker) {
	svc := NewUserService(dbClient, redisClient, authEncryptionKey, noiseServerPubKey).WithGlobalBanChecker(checker)
	h := NewUserHandler(svc)

	// 公开 API（无需鉴权，暴露到公网）
	public := app.Group("/api/v2/bot")

	public.Post("/:bot_id/auth", h.Auth)       // 登录（AES-256-GCM 固定密钥加密，AuthV2）
	public.Delete("/:bot_id/logout", h.Logout) // 注销

	registerAuthV3Routes(app, h, noiseKeys)
}

// registerAuthV3Routes mounts the Noise NK wrapped AuthV3 endpoint. The secure
// middleware performs the per-request handshake, so the handler only ever sees
// the decrypted MsgPack payload and its response is encrypted on the way out.
func registerAuthV3Routes(app *fiber.App, h *UserHandler, noiseKeys *crypto.KeyRing) {
	if noiseKeys == nil {
		return
	}
	v3 := app.Group(AuthV3RouteBase)
	v3.Post("/:bot_id/auth", secure.New(secure.Config{KeyRing: noiseKeys}), h.AuthV3)
}
