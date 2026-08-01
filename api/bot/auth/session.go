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

	// 使用固定 AES-256 密钥解密请求体
	key := h.svc.authEncryptionKey
	if len(key) == 0 {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	body := c.Body()
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).SendString(ErrInvalidEncryptedData)
	}

	plaintext, err := DecryptRaw(body, key)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrInvalidEncryptedData)
	}

	// Nonce 检查（防重放）
	isNew, nonceErr := h.svc.checkAndStoreNonce(ctx, plaintext)
	if nonceErr != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if !isNew {
		return c.Status(fiber.StatusBadRequest).SendString(ErrReplayDetected)
	}

	// MsgPack 解码载荷
	var payload HarukiAuthPayload
	if err := msgpack.Unmarshal(plaintext, &payload); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrInvalidEncryptedData)
	}

	// 验证 bot_id 一致性
	if payload.BotID != botIDStr {
		return c.Status(fiber.StatusBadRequest).SendString(ErrBotIDMismatch)
	}

	// 验证时间戳（防重放）
	now := time.Now().Unix()
	if payload.Timestamp < now-MaxAuthTimestampAge || payload.Timestamp > now+MaxAuthTimestampAge {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthTimestampExpired)
	}

	// 获取用户信息
	u, err := h.svc.dbClient.User.Query().
		Where(user.BotIDEQ(botID)).
		Only(ctx)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthFailed)
	}

	// 解析并验证 JWT credential
	decoded, err := jwt.Parse(payload.Credential, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.Cfg.HarukiBotDB.CredentialSignToken), nil
	})
	if err != nil || !decoded.Valid {
		return c.Status(fiber.StatusBadRequest).SendString(ErrInvalidCredential)
	}

	claims, ok := decoded.Claims.(jwt.MapClaims)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString(ErrInvalidCredential)
	}

	tokenBotID, _ := claims["bot_id"].(string)
	tokenCredential, _ := claims["credential"].(string)

	if tokenBotID != botIDStr {
		return c.Status(fiber.StatusBadRequest).SendString(ErrBotIDMismatch)
	}

	// 验证 credential（兼容 bcrypt 哈希和旧明文记录）
	if !verifyCredential(u.Credential, tokenCredential) {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthFailed)
	}
	banned, err := ownerIsGloballyBanned(ctx, h.svc.globalBanChecker, u.OwnerUserID)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if banned {
		return c.Status(fiber.StatusForbidden).SendString(ErrOwnerBanned)
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
		// 记录客户端自报的登录 IP 和位置
		loginUpdate := h.svc.dbClient.User.UpdateOneID(u.ID).
			SetLastLoginAt(time.Now())
		if payload.ClientIP != "" {
			loginUpdate = loginUpdate.SetLastLoginIP(payload.ClientIP)
		}
		if payload.ClientLocation != "" {
			loginUpdate = loginUpdate.SetLastLoginLocation(payload.ClientLocation)
		}
		_ = loginUpdate.Exec(ctx) // 非关键操作，失败不阻塞登录
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
