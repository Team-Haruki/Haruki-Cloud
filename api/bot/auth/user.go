package auth

import (
	"crypto/subtle"
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

	// 速率限制: 每 QQ 号每小时最多 5 次
	qqStr := strconv.FormatInt(req.QQNumber, 10)
	allowed, err := h.svc.checkRateLimit(ctx, "sendmail", qqStr, RateLimitSendMail, RateLimitSendMailTTL)
	if err != nil {
		return api.InternalError(c)
	}
	if !allowed {
		return api.JSONResponse(c, fiber.StatusTooManyRequests, ErrRateLimitExceeded)
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

	// 速率限制: 每 QQ 号每验证码窗口最多 5 次尝试
	qqStr := strconv.FormatInt(req.QQNumber, 10)
	allowed, err := h.svc.checkRateLimit(ctx, "register", qqStr, RateLimitRegister, RateLimitRegisterTTL)
	if err != nil {
		return api.InternalError(c)
	}
	if !allowed {
		return api.JSONResponse(c, fiber.StatusTooManyRequests, ErrRateLimitExceeded)
	}

	// 检查验证码
	storedCode, err := h.svc.getRedisKey(ctx, RedisKeyVerifyCode, req.QQNumber)
	if errors.Is(err, redis.Nil) {
		return api.JSONResponse(c, fiber.StatusBadRequest, ErrVerifyCodeNotFound)
	}
	if err != nil {
		return api.InternalError(c)
	}
	if subtle.ConstantTimeCompare([]byte(storedCode), []byte(req.VerificationCode)) != 1 {
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

	// bcrypt 哈希 credential 后存储
	hashedCredential, err := hashCredential(credential)
	if err != nil {
		return api.InternalError(c)
	}

	// 创建用户
	_, err = h.svc.dbClient.User.
		Create().
		SetOwnerUserID(req.QQNumber).
		SetBotID(botID).
		SetCredential(hashedCredential).
		Save(ctx)
	if err != nil {
		return api.InternalError(c)
	}

	// 清理 Redis 键
	h.svc.cleanupRegistrationKeys(ctx, req.QQNumber)

	// 签名 credential 为 JWT（JWT 中嵌入的是明文 credential，用于后续 auth 比对）
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

// ================= Route Registration =================

func registerUserRoutes(app *fiber.App, dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string) {
	svc := NewUserService(dbClient, redisClient, authEncryptionKey, noiseServerPubKey)
	h := NewUserHandler(svc)

	// 公开 API（无需鉴权，暴露到公网）
	public := app.Group("/api/v2/bot")

	public.Post("/send-mail", h.SendMail)      // 发送验证码
	public.Post("/register", h.Register)       // 注册
	public.Post("/:bot_id/auth", h.Auth)       // 登录（AES-256-GCM 固定密钥加密）
	public.Delete("/:bot_id/logout", h.Logout) // 注销
}
