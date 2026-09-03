package auth

import (
	"context"
	"strconv"

	"haruki-cloud/api"
	"haruki-cloud/config"
	"haruki-cloud/database/bot/user"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

// ================= Shared Auth Helpers =================

type authResponseError struct {
	status  int
	message string
}

type authenticatedBot struct {
	userID int
	// Previous-login facts used for anomaly reporting.
	lastLoginIP       string
	lastClientVersion string
	lastBuildID       string
}

func sendAuthResponseError(c fiber.Ctx, responseErr *authResponseError) error {
	if responseErr.message == "" {
		return c.SendStatus(responseErr.status)
	}
	return c.Status(responseErr.status).SendString(responseErr.message)
}

// authenticateBot 校验 credential JWT 并检查所有者封禁状态。
func (h *UserHandler) authenticateBot(ctx context.Context, botID int, botIDString string, credential string) (authenticatedBot, *authResponseError) {
	u, err := h.svc.dbClient.User.Query().
		Where(user.BotIDEQ(botID)).
		Only(ctx)
	if err != nil {
		return authenticatedBot{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrAuthFailed}
	}
	decoded, err := parseCredentialJWT(credential, config.Cfg.HarukiBotDB.CredentialSignToken)
	if err != nil || !decoded.Valid {
		return authenticatedBot{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrInvalidCredential}
	}
	claims, ok := decoded.Claims.(jwt.MapClaims)
	if !ok {
		return authenticatedBot{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrInvalidCredential}
	}
	tokenBotID, _ := claims["bot_id"].(string)
	tokenCredential, _ := claims["credential"].(string)
	if tokenBotID != botIDString {
		return authenticatedBot{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrBotIDMismatch}
	}
	if !verifyCredential(u.Credential, tokenCredential) {
		return authenticatedBot{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrAuthFailed}
	}
	banned, err := ownerIsGloballyBanned(ctx, h.svc.globalBanChecker, u.OwnerUserID)
	if err != nil {
		return authenticatedBot{}, &authResponseError{status: fiber.StatusInternalServerError}
	}
	if banned {
		return authenticatedBot{}, &authResponseError{status: fiber.StatusForbidden, message: ErrOwnerBanned}
	}
	return authenticatedBot{
		userID:            u.ID,
		lastLoginIP:       u.LastLoginIP,
		lastClientVersion: u.LastClientVersion,
		lastBuildID:       u.LastBuildID,
	}, nil
}

func parseCredentialJWT(rawCredential string, signingToken string) (*jwt.Token, error) {
	return jwt.Parse(
		rawCredential,
		func(_ *jwt.Token) (any, error) {
			return []byte(signingToken), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
}

// ================= Logout =================

// Logout 注销 - 验证 session token 后删除 Redis session
func (h *UserHandler) Logout(c fiber.Ctx) error {
	ctx := c.Context()
	botIDStr := c.Params("bot_id")
	if _, err := strconv.Atoi(botIDStr); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthFailed)
	}

	// 验证请求携带的 session token 与 Redis 中存储的一致
	sessionToken := c.Get(api.HeaderBotSessionToken)
	if sessionToken == "" {
		return api.JSONResponse(c, fiber.StatusUnauthorized, "缺少注销所需的会话令牌")
	}
	stored, err := h.svc.getRedisKey(ctx, RedisKeySessionToken, botIDStr)
	if err != nil || stored != sessionToken {
		return api.JSONResponse(c, fiber.StatusUnauthorized, ErrSessionExpired)
	}

	if err := h.svc.deleteSession(ctx, botIDStr); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	return api.JSONResponse(c, fiber.StatusOK, "已注销")
}

// deleteSession 从 Redis 删除指定 bot_id 的 session
func (s *UserService) deleteSession(ctx context.Context, botID string) error {
	return s.delRedisKey(ctx, RedisKeySessionToken, botID)
}
