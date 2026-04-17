package auth

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

// HarukiAuthPayload 解密后的登录载荷（MsgPack 编码）
// 请求体格式: nonce(12) || AES-256-GCM(key, nonce, MsgPack(HarukiAuthPayload))
type HarukiAuthPayload struct {
	BotID          string `msgpack:"bot_id"`
	Credential     string `msgpack:"credential"`      // JWT 签名的 credential
	Timestamp      int64  `msgpack:"timestamp"`       // 防重放攻击
	ClientIP       string `msgpack:"client_ip"`       // 客户端自报 IP（来自 myip.ipip.net）
	ClientLocation string `msgpack:"client_location"` // 客户端自报地理位置
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

// HarukiAuthResponse 登录成功响应（MsgPack 编码，AES-256-GCM 加密返回）
type HarukiAuthResponse struct {
	SessionToken      string `msgpack:"session_token"`
	ExpiresAt         int64  `msgpack:"expires_at"`          // Unix 时间戳
	NoiseServerPubKey string `msgpack:"noise_server_pubkey"` // hex 编码的 X25519 公钥
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
	RedisKeyNonce        = "hdb:bot:nonce:%s"         // payload hash
	RedisKeyRateLimit    = "hdb:bot:rl:%s:%s"         // action:identifier
)

// ================= Cache Settings =================

const (
	VerifyCodeTTLMinutes = 10
	// MaxAuthTimestampAge 认证时间戳最大偏差（秒），防重放攻击
	MaxAuthTimestampAge = 300

	// Rate limit settings
	RateLimitSendMail    = 5  // 每 QQ 号每小时最多发送 5 次验证码
	RateLimitRegister    = 5  // 每 QQ 号每验证码最多尝试 5 次验证
	RateLimitAuth        = 10 // 每 bot_id 每分钟最多认证 10 次
	RateLimitSendMailTTL = 60 // 分钟
	RateLimitRegisterTTL = 10 // 分钟
	RateLimitAuthTTL     = 1  // 分钟
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
	ErrRateLimitExceeded       = "rate limit exceeded, please try again later"
	ErrReplayDetected          = "duplicate request detected"
	ErrRegistrationDisabled    = "registration is currently disabled"
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
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

type UserService struct {
	dbClient          *ent.Client
	redisStore        RedisKVStore
	turnstileClient   TurnstileVerifier
	smtpClient        VerificationMailer
	authEncryptionKey []byte // 32-byte AES-256 key for auth payload encryption
	noiseServerPubKey string // hex-encoded server Noise NK public key
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
