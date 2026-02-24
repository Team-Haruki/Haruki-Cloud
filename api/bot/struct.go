package bot

import (
	"context"
	"time"

	ent "haruki-cloud/database/bot"
)

// ================= Request Types =================

// SendMailRequest 发送验证码请求（公开 API）
type SendMailRequest struct {
	QQNumber       int64  `json:"qq_number"`       // QQ 号码
	TurnstileToken string `json:"turnstile_token"` // Turnstile 验证 token
}

// RegisterRequest 注册请求（公开 API）
type RegisterRequest struct {
	QQNumber         int64  `json:"qq_number"`
	VerificationCode string `json:"verification_code"`
}

// AuthRequest 登录请求（公开 API，AES-256-GCM 加密）
type AuthRequest struct {
	EncryptedPayload string `json:"encrypted_payload"` // base64(nonce || ciphertext || tag)
}

// AuthPayload 解密后的登录载荷
type AuthPayload struct {
	Credential string `json:"credential"` // JWT 签名的 credential
	Timestamp  int64  `json:"timestamp"`  // 防重放攻击
}

// InternalVerifyRequest 内部服务验证请求
type InternalVerifyRequest struct {
	BotID        string `json:"bot_id"`
	SessionToken string `json:"session_token"`
}

// ================= Response Types =================

// CredentialResponse 注册成功响应
type CredentialResponse struct {
	BotID      string `json:"bot_id"`
	Credential string `json:"credential"` // JWT 签名的 credential
}

// AuthResponse 登录成功响应
type AuthResponse struct {
	SessionToken string `json:"session_token"`
	ExpiresAt    int64  `json:"expires_at"` // Unix 时间戳
}

// InternalVerifyResponse 内部服务验证响应
type InternalVerifyResponse struct {
	Valid       bool  `json:"valid"`
	OwnerUserID int64 `json:"owner_user_id,omitempty"`
	BotID       int   `json:"bot_id,omitempty"`
}

// ================= Redis Key Prefixes =================

const (
	RedisKeyVerifyCode   = "hdb:bot:verify_code:%d"   // QQ号码
	RedisKeyVerifyStatus = "hdb:bot:verify_status:%d" // QQ号码
	RedisKeySessionToken = "hdb:bot:session:%s"       // bot_id
)

// ================= Cache Settings =================

const (
	VerifyCodeTTLMinutes = 10
	// AuthTimestampMaxAge 认证时间戳最大偏差（秒），防重放攻击
	AuthTimestampMaxAge = 300
)

// ================= Error Messages =================

const (
	ErrMissingQQNumber         = "missing qq_number"
	ErrMissingTurnstileToken   = "missing turnstile_token"
	ErrTurnstileVerifyFailed   = "turnstile verification failed"
	ErrVerifyCodeNotFound      = "verification code not found or expired"
	ErrVerifyCodeInvalid       = "verification code is invalid"
	ErrBotAlreadyRegistered    = "bot already registered for this QQ"
	ErrInvalidCredential       = "invalid credential"
	ErrInvalidEncryptedData    = "invalid encrypted payload"
	ErrAuthTimestampExpired    = "auth request expired"
	ErrBotIDMismatch           = "bot_id mismatch"
	ErrAuthFailed              = "authentication failed"
	ErrSessionExpired          = "session expired or invalid"
	ErrMissingVerificationCode = "missing verification_code"
	ErrSendEmailFailed         = "failed to send verification email"
)

// ================= Service Structs =================

type TurnstileVerifier interface {
	VerifyToken(token, remoteIP string) (bool, error)
}

type VerificationMailer interface {
	SendVerificationCode(qqNumber int64, code string) error
}

type RedisKVStore interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
}

type UserService struct {
	dbClient        *ent.Client
	redisStore      RedisKVStore
	turnstileClient TurnstileVerifier
	smtpClient      VerificationMailer
}

type InternalService struct {
	dbClient   *ent.Client
	redisStore RedisKVStore
}

type StatisticsService struct {
	client *ent.Client
}

// ================= Handler Structs =================

type UserHandler struct {
	svc *UserService
}

type InternalHandler struct {
	svc *InternalService
}

type StatisticsHandler struct {
	svc *StatisticsService
}
