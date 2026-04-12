package auth

import (
	"errors"
	"strconv"
	"time"

	"haruki-cloud/api"
	"haruki-cloud/config"
	ent "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/user"
	"haruki-cloud/utils/crypto"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

// ================= Public API Handlers =================

// SendMail 发送验证码 - 验证 Turnstile 并发送验证码到 QQ 邮箱（公开 API，无加密）
func (h *UserHandler) SendMail(c fiber.Ctx) error {
	ctx := c.Context()

	var req SendMailRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	if req.QQNumber == 0 {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrMissingQQNumber)
	}
	if req.TurnstileToken == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrMissingTurnstileToken)
	}

	// 验证 Turnstile
	remoteIP := c.IP()
	valid, err := h.svc.turnstileClient.VerifyToken(req.TurnstileToken, remoteIP)
	if err != nil || !valid {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrTurnstileVerifyFailed)
	}

	// 检查是否已注册
	exists, _ := h.svc.dbClient.User.Query().
		Where(user.OwnerUserIDEQ(req.QQNumber)).
		Exist(ctx)
	if exists {
		return api.JSONResponse(c, fiber.StatusConflict, ErrBotAlreadyRegistered)
	}

	// 生成验证码
	code := generateVerificationCode(6)

	// 存储验证码到 Redis
	if err := h.svc.setRedisKey(ctx, RedisKeyVerifyCode, req.QQNumber, code, VerifyCodeTTLMinutes); err != nil {
		return api.InternalError(c)
	}

	// 发送邮件
	if err := h.svc.smtpClient.SendVerificationCode(req.QQNumber, code); err != nil {
		return api.JSONResponse(c, fiber.StatusInternalServerError, ErrSendEmailFailed)
	}

	return api.JSONResponse(c, fiber.StatusOK, "验证码已发送到您的 QQ 邮箱，有效期 10 分钟")
}

// Register 注册 - 验证邮箱验证码，创建用户（公开 API，无加密）
func (h *UserHandler) Register(c fiber.Ctx) error {
	ctx := c.Context()

	var req RegisterRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	if req.QQNumber == 0 {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrMissingQQNumber)
	}
	if req.VerificationCode == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrMissingVerificationCode)
	}

	// 检查验证码
	storedCode, err := h.svc.getRedisKey(ctx, RedisKeyVerifyCode, req.QQNumber)
	if errors.Is(err, redis.Nil) {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrVerifyCodeNotFound)
	}
	if err != nil {
		return api.InternalError(c)
	}
	if storedCode != req.VerificationCode {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrVerifyCodeInvalid)
	}

	// 再次检查是否已注册（防止并发）
	exists, _ := h.svc.dbClient.User.Query().
		Where(user.OwnerUserIDEQ(req.QQNumber)).
		Exist(ctx)
	if exists {
		h.svc.cleanupRegistrationKeys(ctx, req.QQNumber)
		return api.JSONResponse(c, fiber.StatusConflict, ErrBotAlreadyRegistered)
	}

	// 生成 bot_id
	var botID int
	for i := 0; i < 10; i++ {
		id, err := generateBotID()
		if err != nil {
			return api.InternalError(c)
		}
		exists, _ := h.svc.dbClient.User.Query().
			Where(user.BotIDEQ(id)).
			Exist(ctx)
		if !exists {
			botID = id
			break
		}
	}
	if botID == 0 {
		return api.InternalError(c)
	}

	// 生成 credential
	credential, err := generateCredential()
	if err != nil {
		return api.InternalError(c)
	}

	// 创建用户
	_, err = h.svc.dbClient.User.
		Create().
		SetOwnerUserID(req.QQNumber).
		SetBotID(botID).
		SetCredential(credential).
		Save(ctx)
	if err != nil {
		return api.InternalError(c)
	}

	// 清理 Redis 键
	h.svc.cleanupRegistrationKeys(ctx, req.QQNumber)

	// 签名 credential 为 JWT
	payload := jwt.MapClaims{
		"bot_id":     strconv.Itoa(botID),
		"credential": credential,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).
		SignedString([]byte(config.Cfg.HarukiBotDB.CredentialSignToken))
	if err != nil {
		return api.InternalError(c)
	}

	return api.JSONResponse(c, fiber.StatusCreated, "注册成功", CredentialResponse{
		BotID:      strconv.Itoa(botID),
		Credential: token,
	})
}

// Auth 登录 - AES-256-GCM 固定密钥加密（公开 API）
// 请求体: raw bytes = nonce(12) || AES-256-GCM(key, nonce, MsgPack{bot_id, credential, timestamp})
// 响应体: raw bytes = nonce(12) || AES-256-GCM(key, nonce, MsgPack{session_token, expires_at, noise_server_pubkey})
func (h *UserHandler) Auth(c fiber.Ctx) error {
	ctx := c.Context()
	botIDStr := c.Params("bot_id")
	botID, err := strconv.Atoi(botIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthFailed)
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

	plaintext, err := crypto.DecryptRaw(body, key)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrInvalidEncryptedData)
	}

	// MsgPack 解码载荷
	var payload AuthPayload
	if err := msgpack.Unmarshal(plaintext, &payload); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(ErrInvalidEncryptedData)
	}

	// 验证 bot_id 一致性
	if payload.BotID != botIDStr {
		return c.Status(fiber.StatusBadRequest).SendString(ErrBotIDMismatch)
	}

	// 验证时间戳（防重放）
	now := time.Now().Unix()
	if payload.Timestamp < now-AuthTimestampMaxAge || payload.Timestamp > now+AuthTimestampMaxAge {
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

	if u.Credential != tokenCredential {
		return c.Status(fiber.StatusBadRequest).SendString(ErrAuthFailed)
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
	_ = h.svc.setRedisKey(ctx, RedisKeySessionToken, botIDStr, sessionToken, int(sessionTTL.Minutes()))

	// 构造加密响应: MsgPack → AES-256-GCM
	resp := AuthResponse{
		SessionToken:      sessionToken,
		ExpiresAt:         expiresAt,
		NoiseServerPubKey: h.svc.noiseServerPubKey,
	}
	respBytes, err := msgpack.Marshal(resp)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	encrypted, err := crypto.EncryptRaw(respBytes, key)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	c.Set("Content-Type", "application/octet-stream")
	return c.Send(encrypted)
}

// ================= Route Registration =================

func registerUserRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string) {
	svc := NewUserService(dbClient, redisClient, authEncryptionKey, noiseServerPubKey)
	h := NewUserHandler(svc)

	// 公开 API（无需鉴权，暴露到公网）
	public := app.Group("/bot")

	public.Post("/send-mail", h.SendMail) // 发送验证码
	public.Post("/register", h.Register)  // 注册
	public.Post("/:bot_id/auth", h.Auth)  // 登录（AES-256-GCM 固定密钥加密）
}
