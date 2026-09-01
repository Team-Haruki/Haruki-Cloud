package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"haruki-cloud/api"
	"haruki-cloud/config"
	"haruki-cloud/database/bot/user"
	"haruki-cloud/internal/cluster"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

// ================= Session Handlers =================

// Auth 登录 - AES-256-GCM 固定密钥加密（公开 API）
// 请求体: raw bytes = nonce(12) || AES-256-GCM(key, nonce, MsgPack{bot_id, credential, timestamp, client_ip, client_location})
// 响应体: raw bytes = nonce(12) || AES-256-GCM(key, nonce, MsgPack{session_token, expires_at, noise_server_pubkey})
func (h *UserHandler) Auth(c fiber.Ctx) error {
	ctx := c.Context()
	botIDStr := c.Params("bot_id")
	botID, err := strconv.Atoi(botIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthFailed)
	}

	// 速率限制: 每 bot_id 每分钟最多 10 次
	allowed, rlErr := h.svc.checkRateLimit(ctx, "auth", botIDStr, RateLimitAuth, RateLimitAuthTTL)
	if rlErr != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if !allowed {
		return c.Status(fiber.StatusTooManyRequests).SendString(ErrRateLimitExceeded)
	}

	key := h.svc.authEncryptionKey
	payload, authErr := h.decodeAuthPayload(ctx, botIDStr, c.Body(), key)
	if authErr != nil {
		return sendAuthResponseError(c, authErr)
	}
	authenticated, authErr := h.authenticateBot(ctx, botID, botIDStr, payload.Credential)
	if authErr != nil {
		return sendAuthResponseError(c, authErr)
	}

	// 生成 session token
	sessionTTL := getSessionTTL()
	expiresAt := time.Now().Add(sessionTTL).Unix()

	sessionPayload := jwt.MapClaims{
		"bot_id": botIDStr,
		"exp":    expiresAt,
	}
	sessionToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, sessionPayload).
		SignedString([]byte(config.Cfg.HarukiBotDB.SessionSignToken))
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// 存储 session 到 Redis
	if err := h.svc.setRedisKey(ctx, RedisKeySessionToken, botIDStr, sessionToken, int(sessionTTL.Minutes())); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if !cluster.IsReadOnly() {
		h.recordLogin(ctx, authenticated.userID, payload)
	}

	// 构造加密响应: MsgPack → AES-256-GCM
	resp := HarukiAuthResponse{
		SessionToken:      sessionToken,
		ExpiresAt:         expiresAt,
		NoiseServerPubKey: h.svc.noiseServerPubKey,
	}
	respBytes, err := msgpack.Marshal(resp)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	encrypted, err := EncryptRaw(respBytes, key)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	c.Set("Content-Type", "application/octet-stream")
	return c.Send(encrypted)
}

type authResponseError struct {
	status  int
	message string
}

type authenticatedBot struct {
	userID int
}

func sendAuthResponseError(c fiber.Ctx, responseErr *authResponseError) error {
	if responseErr.message == "" {
		return c.SendStatus(responseErr.status)
	}
	return c.Status(responseErr.status).SendString(responseErr.message)
}

func (h *UserHandler) decodeAuthPayload(ctx context.Context, botID string, body, key []byte) (HarukiAuthPayload, *authResponseError) {
	if len(key) == 0 {
		return HarukiAuthPayload{}, &authResponseError{status: fiber.StatusInternalServerError}
	}
	if len(body) == 0 {
		return HarukiAuthPayload{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrInvalidEncryptedData}
	}
	plaintext, err := DecryptRaw(body, key)
	if err != nil {
		return HarukiAuthPayload{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrInvalidEncryptedData}
	}
	isNew, err := h.svc.checkAndStoreNonce(ctx, plaintext)
	if err != nil {
		return HarukiAuthPayload{}, &authResponseError{status: fiber.StatusInternalServerError}
	}
	if !isNew {
		return HarukiAuthPayload{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrReplayDetected}
	}
	var payload HarukiAuthPayload
	if err := msgpack.Unmarshal(plaintext, &payload); err != nil {
		return HarukiAuthPayload{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrInvalidEncryptedData}
	}
	if payload.BotID != botID {
		return HarukiAuthPayload{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrBotIDMismatch}
	}
	now := time.Now().Unix()
	if payload.Timestamp < now-MaxAuthTimestampAge || payload.Timestamp > now+MaxAuthTimestampAge {
		return HarukiAuthPayload{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrAuthTimestampExpired}
	}
	return payload, nil
}

// authenticateBot 校验 credential JWT 并检查所有者封禁状态。AuthV2 与 AuthV3 共用。
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
	return authenticatedBot{userID: u.ID}, nil
}

func (h *UserHandler) recordLogin(ctx context.Context, userID int, payload HarukiAuthPayload) {
	loginUpdate := h.svc.dbClient.User.UpdateOneID(userID).
		SetLastLoginAt(time.Now())
	if payload.ClientIP != "" {
		loginUpdate = loginUpdate.SetLastLoginIP(payload.ClientIP)
	}
	if payload.ClientLocation != "" {
		loginUpdate = loginUpdate.SetLastLoginLocation(payload.ClientLocation)
	}
	_ = loginUpdate.Exec(ctx)
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

// Logout 注销 - 验证 session token 后删除 Redis session
func (h *UserHandler) Logout(c fiber.Ctx) error {
	ctx := c.Context()
	botIDStr := c.Params("bot_id")
	if _, err := strconv.Atoi(botIDStr); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthFailed)
	}

	// 验证请求携带的 session token 与 Redis 中存储的一致
	sessionToken := c.Get("X-Haruki-Bot-Session-Token")
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

// ================= Nonce Cache (Replay Protection) =================

// checkAndStoreNonce 检查请求是否为重放。返回 true 表示是新请求。
func (s *UserService) checkAndStoreNonce(ctx context.Context, payload []byte) (bool, error) {
	hash := sha256.Sum256(payload)
	nonceKey := fmt.Sprintf(RedisKeyNonce, hex.EncodeToString(hash[:16]))

	_, err := s.redisStore.Get(ctx, nonceKey)
	if err == nil {
		// nonce 已存在，是重放请求
		return false, nil
	}
	if !errors.Is(err, redis.Nil) {
		return false, err
	}

	// 存储 nonce，TTL 等于时间戳容忍窗口
	err = s.redisStore.Set(ctx, nonceKey, "1", MaxAuthTimestampAge*time.Second)
	return err == nil, err
}

// deleteSession 从 Redis 删除指定 bot_id 的 session
func (s *UserService) deleteSession(ctx context.Context, botID string) error {
	return s.delRedisKey(ctx, RedisKeySessionToken, botID)
}
