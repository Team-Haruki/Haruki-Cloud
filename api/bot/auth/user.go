package auth

import (
	ent "haruki-cloud/database/bot"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// ================= Route Registration =================

func registerUserRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string) {
	svc := NewUserService(dbClient, redisClient, authEncryptionKey, noiseServerPubKey)
	h := NewUserHandler(svc)

	// 公开 API（无需鉴权，暴露到公网）
	public := app.Group("/api/v2/bot")

	public.Post("/:bot_id/auth", h.Auth)       // 登录（AES-256-GCM 固定密钥加密）
	public.Delete("/:bot_id/logout", h.Logout) // 注销
}
