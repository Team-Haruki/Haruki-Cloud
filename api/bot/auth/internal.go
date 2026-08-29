package auth

import (
	"errors"
	"strconv"

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
		return api.JSONResponse(c, fiber.StatusBadRequest, "请求格式错误")
	}

	if req.BotID == "" || req.SessionToken == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, "缺少 bot_id 或 session_token")
	}

	// 验证 JWT session token
	decoded, err := jwt.Parse(req.SessionToken, func(_ *jwt.Token) (any, error) {
		return []byte(config.Cfg.HarukiBotDB.SessionSignToken), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !decoded.Valid {
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{Valid: false})
	}

	claims, ok := decoded.Claims.(jwt.MapClaims)
	if !ok {
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{Valid: false})
	}

	tokenBotID, ok := claims["bot_id"].(string)
	if !ok || tokenBotID != req.BotID {
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{Valid: false})
	}

	// 检查 Redis 中的 session 是否存在
	storedSession, err := h.svc.getRedisKey(ctx, RedisKeySessionToken, req.BotID)
	if errors.Is(err, redis.Nil) {
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{Valid: false})
	}
	if err != nil {
		return api.InternalError(c)
	}

	// 验证 session token 是否匹配
	if storedSession != req.SessionToken {
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{Valid: false})
	}

	// 获取用户信息
	botID, err := strconv.Atoi(req.BotID)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{Valid: false})
	}

	u, err := h.svc.dbClient.User.Query().
		Where(user.BotIDEQ(botID)).
		Only(ctx)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{Valid: false})
	}
	banned, err := ownerIsGloballyBanned(ctx, h.svc.globalBanChecker, u.OwnerUserID)
	if err != nil {
		return api.InternalError(c)
	}
	if banned {
		return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{Valid: false})
	}

	return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, InternalVerifyResponse{
		Valid:       true,
		OwnerUserID: u.OwnerUserID,
		BotID:       u.BotID,
	})
}

func registerInternalRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, checker GlobalBanChecker) {
	svc := NewInternalService(dbClient, redisClient).WithGlobalBanChecker(checker)
	h := NewInternalHandler(svc)

	// 内部 API（使用统一的 API 鉴权）
	internal := app.Group("/internal/bot", api.VerifyAPIAuthorization())

	internal.Post("/verify-session", h.VerifySession)
}
