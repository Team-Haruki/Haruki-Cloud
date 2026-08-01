package auth

import (
	"context"
	"time"

	"haruki-cloud/api"
	ent "haruki-cloud/database/bot"
)

// ================= Request Types =================

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
	RedisKeySessionToken = api.RedisKeyBotSession
	RedisKeyNonce        = "hdb:bot:nonce:%s" // payload hash
	RedisKeyRateLimit    = "hdb:bot:rl:%s:%s" // action:identifier
)

// ================= Cache Settings =================

const (
	// MaxAuthTimestampAge 认证时间戳最大偏差（秒），防重放攻击
	MaxAuthTimestampAge = 300

	// Rate limit settings
	RateLimitAuth    = 10 // 每 bot_id 每分钟最多认证 10 次
	RateLimitAuthTTL = 1  // 分钟
)

// ================= Error Messages =================

const (
	ErrInvalidCredential    = "凭证无效"
	ErrInvalidEncryptedData = "加密载荷无效"
	ErrAuthTimestampExpired = "认证请求已过期"
	ErrBotIDMismatch        = "bot_id 不匹配"
	ErrAuthFailed           = "认证失败"
	ErrSessionExpired       = "会话已过期或无效"
	ErrRateLimitExceeded    = "请求过于频繁，请稍后再试"
	ErrReplayDetected       = "检测到重复请求"
	ErrOwnerBanned          = "Bot 所有者已被全局封禁"
)

// ================= Service Structs =================

type RedisKVStore interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

type GlobalBanChecker interface {
	IsGloballyBanned(ctx context.Context, platform, userID string) (bool, error)
}

type UserService struct {
	dbClient          *ent.Client
	redisStore        RedisKVStore
	authEncryptionKey []byte // 32-byte AES-256 key for auth payload encryption
	noiseServerPubKey string // hex-encoded server Noise NK public key
	globalBanChecker  GlobalBanChecker
}

type InternalService struct {
	dbClient         *ent.Client
	redisStore       RedisKVStore
	globalBanChecker GlobalBanChecker
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
