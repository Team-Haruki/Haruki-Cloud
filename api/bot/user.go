package bot

import (
	"context"
	"encoding/json"
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
)

// ================= Public API Handlers =================

// SendMail 发送验证码 - 验证 Turnstile 并发送验证码到 QQ 邮箱（公开 API，无加密）
func (h *UserHandler) SendMail(c fiber.Ctx) error {
	ctx := context.Background()

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
	ctx := context.Background()

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

// Auth 登录 - AES-256-GCM 加密验证 credential，生成 session token（公开 API，AES 加密）
func (h *UserHandler) Auth(c fiber.Ctx) error {
	ctx := context.Background()
	botIDStr := c.Params("bot_id")
	botID, err := strconv.Atoi(botIDStr)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, "invalid bot_id")
	}

	var req AuthRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	if req.EncryptedPayload == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrInvalidEncryptedData)
	}

	// 获取用户信息
	u, err := h.svc.dbClient.User.Query().
		Where(user.BotIDEQ(botID)).
		Only(ctx)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrAuthFailed)
	}

	// 使用 credential 作为解密密钥（取前 32 字节）
	keyBytes := deriveKeyFromCredential(u.Credential)

	// 解密载荷
	plaintext, err := crypto.Decrypt(req.EncryptedPayload, keyBytes)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrInvalidEncryptedData)
	}

	// 解析载荷
	var payload AuthPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrInvalidEncryptedData)
	}

	// 验证时间戳（防重放）
	now := time.Now().Unix()
	if payload.Timestamp < now-AuthTimestampMaxAge || payload.Timestamp > now+AuthTimestampMaxAge {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrAuthTimestampExpired)
	}

	// 解析并验证 JWT credential
	decoded, err := jwt.Parse(payload.Credential, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.Cfg.HarukiBotDB.CredentialSignToken), nil
	})
	if err != nil || !decoded.Valid {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrInvalidCredential)
	}

	claims, ok := decoded.Claims.(jwt.MapClaims)
	if !ok {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrInvalidCredential)
	}

	tokenBotID := claims["bot_id"].(string)
	tokenCredential := claims["credential"].(string)

	if tokenBotID != botIDStr {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrBotIDMismatch)
	}

	if u.Credential != tokenCredential {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrAuthFailed)
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
		return api.InternalError(c)
	}

	// 存储 session 到 Redis
	_ = h.svc.setRedisKey(ctx, RedisKeySessionToken, botIDStr, sessionToken, int(sessionTTL.Minutes()))

	return api.JSONResponse(c, fiber.StatusOK, "ok", AuthResponse{
		SessionToken: sessionToken,
		ExpiresAt:    expiresAt,
	})
}

// ================= Route Registration =================

func registerUserRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client) {
	svc := NewUserService(dbClient, redisClient)
	h := NewUserHandler(svc)

	// 公开 API（无需鉴权，暴露到公网）
	public := app.Group("/bot")

	public.Post("/send-mail", h.SendMail) // 发送验证码
	public.Post("/register", h.Register)  // 注册
	public.Post("/:bot_id/auth", h.Auth)  // 登录（AES-256-GCM 加密）
}
