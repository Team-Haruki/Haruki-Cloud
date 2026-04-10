package auth

import (
	"errors"

	"haruki-cloud/api"
	"haruki-cloud/config"
	ent "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/user"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// VerifySession 验证 bot_id 和 session_token
func (h *InternalHandler) VerifySession(c fiber.Ctx) error {
	ctx := c.Context()

	var req InternalVerifyRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	if req.BotID == "" || req.SessionToken == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, "bot_id and session_token are required")
	}

	// 验证 JWT session token
	decoded, err := jwt.Parse(req.SessionToken, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.Cfg.HarukiBotDB.SessionSignToken), nil
	})
	if err != nil || !decoded.Valid {
		return api.JSONResponse(c, fiber.StatusOK, "ok", InternalVerifyResponse{Valid: false})
	}

	claims, ok := decoded.Claims.(jwt.MapClaims)
	if !ok {
		return api.JSONResponse(c, fiber.StatusOK, "ok", InternalVerifyResponse{Valid: false})
	}

	tokenBotID := claims["bot_id"].(string)
	if tokenBotID != req.BotID {
		return api.JSONResponse(c, fiber.StatusOK, "ok", InternalVerifyResponse{Valid: false})
	}

	// 检查 Redis 中的 session 是否存在
	storedSession, err := h.svc.getRedisKey(ctx, RedisKeySessionToken, req.BotID)
	if errors.Is(err, redis.Nil) {
		return api.JSONResponse(c, fiber.StatusOK, "ok", InternalVerifyResponse{Valid: false})
	}
	if err != nil {
		return api.InternalError(c)
	}

	// 验证 session token 是否匹配
	if storedSession != req.SessionToken {
		return api.JSONResponse(c, fiber.StatusOK, "ok", InternalVerifyResponse{Valid: false})
	}

	// 获取用户信息
	u, err := h.svc.dbClient.User.Query().
		Where(user.BotIDEQ(mustAtoi(req.BotID))).
		Only(ctx)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusOK, "ok", InternalVerifyResponse{Valid: false})
	}

	return api.JSONResponse(c, fiber.StatusOK, "ok", InternalVerifyResponse{
		Valid:       true,
		OwnerUserID: u.OwnerUserID,
		BotID:       u.BotID,
	})
}

// mustAtoi 安全转换字符串为整数，失败返回 0
func mustAtoi(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func registerInternalRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client) {
	svc := NewInternalService(dbClient, redisClient)
	h := NewInternalHandler(svc)

	// 内部 API（使用统一的 API 鉴权）
	internal := app.Group("/internal/bot", api.VerifyAPIAuthorization())

	internal.Post("/verify-session", h.VerifySession)
}
