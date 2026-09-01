package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/cluster"
	"haruki-cloud/internal/middleware/secure"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/shamaton/msgpack/v3"
)

// AuthV3 登录 - Noise NK 通道认证（公开 API）
//
// 路由挂载在 secure 中间件之后：请求体是 Noise NK Message 1，中间件完成握手并把
// 解密后的 MsgPack(AuthPayloadV3) 写回 body；响应写出 MsgPack(AuthResponseV3)，
// 由同一中间件用本次握手的密钥封装成 Noise NK Message 2。
//
// 与 AuthV2 的区别：
//   - 不再使用二进制内置的共享 AES 密钥，客户端只需预置服务端 Noise 公钥；
//   - nonce 由客户端显式提供，服务端按 bot_id + nonce 一次性消费；
//   - method / path 绑定请求上下文，密文无法搬到其他接口重放；
//   - session 有效期缩短为 auth_v3_session_ttl（默认 1 小时）。
func (h *UserHandler) AuthV3(c fiber.Ctx) error {
	ctx := c.Context()
	botIDStr := c.Params("bot_id")
	botID, err := strconv.Atoi(botIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthFailed)
	}

	// 防御性检查：即使路由被错误挂载到 Noise 之外，也拒绝处理明文请求。
	if wrapped, _ := c.Locals(secure.LocalSecureNoise).(bool); !wrapped {
		return c.Status(fiber.StatusBadRequest).SendString(ErrSecureChannelMissing)
	}

	// 速率限制与 AuthV2 共用同一计数桶，避免攻击者叠加两条通道的预算。
	allowed, rlErr := h.svc.checkRateLimit(ctx, "auth", botIDStr, RateLimitAuth, RateLimitAuthTTL)
	if rlErr != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if !allowed {
		return c.Status(fiber.StatusTooManyRequests).SendString(ErrRateLimitExceeded)
	}

	payload, authErr := h.decodeAuthPayloadV3(ctx, c, botIDStr)
	if authErr != nil {
		return sendAuthResponseError(c, authErr)
	}
	authenticated, authErr := h.authenticateBot(ctx, botID, botIDStr, payload.Credential)
	if authErr != nil {
		return sendAuthResponseError(c, authErr)
	}

	now := time.Now()
	sessionTTL := getAuthV3SessionTTL()
	expiresAt := now.Add(sessionTTL).Unix()
	sessionID, err := randomHex(AuthV3NonceSize)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	sessionToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"bot_id": botIDStr,
		"sid":    sessionID,
		"iat":    now.Unix(),
		"exp":    expiresAt,
		"ver":    3,
	}).SignedString([]byte(config.Cfg.HarukiBotDB.SessionSignToken))
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if err := h.svc.redisStore.Set(ctx, fmt.Sprintf(RedisKeySessionToken, botIDStr), sessionToken, sessionTTL); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if !cluster.IsReadOnly() {
		h.recordLoginV3(ctx, authenticated.userID, c.IP())
	}
	slog.InfoContext(ctx, "bot auth v3 succeeded",
		"bot_id", botIDStr,
		"client_version", payload.ClientVersion,
		"build_id", payload.BuildID,
		"noise_key_id", c.Locals(secure.LocalNoiseKeyID),
	)

	respBytes, err := msgpack.Marshal(AuthResponseV3{
		SessionToken:    sessionToken,
		ExpiresAt:       expiresAt,
		EchoNonce:       payload.Nonce,
		SessionID:       sessionID,
		ServerTime:      now.Unix(),
		AcceptedBuildID: payload.BuildID,
	})
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	c.Set("Content-Type", "application/msgpack")
	return c.Send(respBytes)
}

// decodeAuthPayloadV3 解析并校验 Noise 解密后的 AuthV3 载荷。
// 校验顺序：结构 → bot_id → 请求上下文绑定 → Noise 公钥 ID → 时间窗口 → nonce 消费。
// nonce 消费放在最后，避免格式错误的请求也占用一次性 nonce。
func (h *UserHandler) decodeAuthPayloadV3(ctx context.Context, c fiber.Ctx, botID string) (AuthPayloadV3, *authResponseError) {
	body := c.Body()
	if len(body) == 0 {
		return AuthPayloadV3{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrInvalidEncryptedData}
	}
	var payload AuthPayloadV3
	if err := msgpack.Unmarshal(body, &payload); err != nil {
		return AuthPayloadV3{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrInvalidEncryptedData}
	}
	if payload.BotID != botID {
		return AuthPayloadV3{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrBotIDMismatch}
	}
	if payload.Method != c.Method() || payload.Path != c.Path() {
		return AuthPayloadV3{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrRequestBindingBroken}
	}
	if payload.NoiseKeyID != "" {
		if usedKeyID, _ := c.Locals(secure.LocalNoiseKeyID).(string); usedKeyID != payload.NoiseKeyID {
			return AuthPayloadV3{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrNoiseKeyMismatch}
		}
	}
	now := time.Now().Unix()
	if payload.Timestamp < now-MaxAuthTimestampAge || payload.Timestamp > now+MaxAuthTimestampAge {
		return AuthPayloadV3{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrAuthTimestampExpired}
	}
	nonce, err := hex.DecodeString(payload.Nonce)
	if err != nil || len(nonce) != AuthV3NonceSize {
		return AuthPayloadV3{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrInvalidNonce}
	}
	isNew, err := h.svc.consumeNonceV3(ctx, botID, hex.EncodeToString(nonce))
	if err != nil {
		return AuthPayloadV3{}, &authResponseError{status: fiber.StatusInternalServerError}
	}
	if !isNew {
		return AuthPayloadV3{}, &authResponseError{status: fiber.StatusBadRequest, message: ErrReplayDetected}
	}
	return payload, nil
}

// consumeNonceV3 以 bot_id + nonce 为键做一次性消费。使用 INCR 判定首次出现，
// 判定与写入是同一条原子操作，并发重放不会同时通过。
func (s *UserService) consumeNonceV3(ctx context.Context, botID string, nonceHex string) (bool, error) {
	key := fmt.Sprintf(RedisKeyNonceV3, botID, nonceHex)
	count, err := s.redisStore.Incr(ctx, key)
	if err != nil {
		return false, err
	}
	if count != 1 {
		return false, nil
	}
	if err := s.redisStore.Expire(ctx, key, MaxAuthTimestampAge*time.Second); err != nil {
		// 没有 TTL 的 nonce 会永久占位；清掉它让客户端下次重试能通过。
		_ = s.redisStore.Del(ctx, key)
		return false, fmt.Errorf("set nonce expiry: %w", err)
	}
	return true, nil
}

// recordLoginV3 记录登录时间和服务端观察到的客户端 IP。
// AuthV3 不再接受客户端自报的 IP / 地理位置。
func (h *UserHandler) recordLoginV3(ctx context.Context, userID int, clientIP string) {
	loginUpdate := h.svc.dbClient.User.UpdateOneID(userID).SetLastLoginAt(time.Now())
	if clientIP != "" {
		loginUpdate = loginUpdate.SetLastLoginIP(clientIP)
	}
	_ = loginUpdate.Exec(ctx)
}

// getAuthV3SessionTTL 读取 AuthV3 session 有效期，限制在 [1m, 30d]，默认 1h。
func getAuthV3SessionTTL() time.Duration {
	ttl := config.Cfg.HarukiBotDB.AuthV3SessionTTL
	switch {
	case ttl <= 0:
		return DefaultAuthV3SessionTTL
	case ttl < MinAuthV3SessionTTL:
		return MinAuthV3SessionTTL
	case ttl > MaxAuthV3SessionTTL:
		return MaxAuthV3SessionTTL
	}
	return ttl
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
