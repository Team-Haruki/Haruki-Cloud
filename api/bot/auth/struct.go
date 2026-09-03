package auth

import (
	"context"
	"haruki-cloud/internal/core/buildpolicy"
	"haruki-cloud/internal/core/secevent"
	"time"

	"haruki-cloud/api"
	ent "haruki-cloud/database/bot"
)

// ================= Request Types =================

// AuthPayloadV3 是 AuthV3 解密后的登录载荷（MsgPack 编码）。
// 请求体格式: Noise NK Message 1，payload = MsgPack(AuthPayloadV3)。
// 载荷通过 Noise 通道传输，不再依赖二进制内置的共享 AES 密钥。
type AuthPayloadV3 struct {
	BotID      string `msgpack:"bot_id"`
	Credential string `msgpack:"credential"` // JWT 签名的 credential
	Timestamp  int64  `msgpack:"timestamp"`  // Unix 秒，服务端校验时间窗口
	// Nonce 为 16 字节随机数的 hex 编码（32 个 hex 字符），
	// 服务端按 bot_id + nonce 一次性消费。
	Nonce         string `msgpack:"nonce"`
	ClientVersion string `msgpack:"client_version"`
	BuildID       string `msgpack:"build_id"`
	// Target / BinarySHA256 是客户端自报的构建平台与二进制哈希，可选；
	// 存在时与构建许可清单比对（自报值不是远程证明，只抬高篡改成本）。
	Target       string `msgpack:"target"`
	BinarySHA256 string `msgpack:"binary_sha256"`
	// Method / Path 绑定请求上下文，防止密文被搬到其他接口重放。
	Method string `msgpack:"method"`
	Path   string `msgpack:"path"`
	// NoiseKeyID 为客户端本次握手使用的服务端公钥 ID；为空时不校验。
	NoiseKeyID string `msgpack:"noise_key_id"`
}

// InternalVerifyRequest 内部服务验证请求
type InternalVerifyRequest struct {
	BotID        string `json:"bot_id"`
	SessionToken string `json:"session_token"`
}

// ================= Response Types =================

// AuthResponseV3 是 AuthV3 登录成功响应（MsgPack 编码，经 Noise NK Message 2 加密返回）。
type AuthResponseV3 struct {
	SessionToken string `msgpack:"session_token"`
	ExpiresAt    int64  `msgpack:"expires_at"` // Unix 秒
	// EchoNonce 回显请求中的 nonce（hex），供客户端确认响应与请求匹配。
	EchoNonce       string `msgpack:"echo_nonce"`
	SessionID       string `msgpack:"session_id"`
	ServerTime      int64  `msgpack:"server_time"` // Unix 秒
	AcceptedBuildID string `msgpack:"accepted_build_id"`
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
	RedisKeyNonceV3      = "hdb:bot:nonce:v3:%s:%s" // bot_id:nonce hex
	RedisKeyRateLimit    = "hdb:bot:rl:%s:%s"       // action:identifier
)

// ================= Cache Settings =================

const (
	// MaxAuthTimestampAge 认证时间戳最大偏差（秒），防重放攻击
	MaxAuthTimestampAge = 300

	// AuthV3RouteBase 是 AuthV3 公开路由前缀；完整路径为 <base>/:bot_id/auth。
	AuthV3RouteBase = "/api/v3/bot"
	// AuthV3NonceSize 是 AuthV3 nonce 的原始字节数（hex 编码后为 2 倍长度）。
	AuthV3NonceSize = 16
	// DefaultAuthV3SessionTTL 是 AuthV3 session 的默认有效期。
	DefaultAuthV3SessionTTL = time.Hour
	// MinAuthV3SessionTTL / MaxAuthV3SessionTTL 限定配置值的合法范围。
	MinAuthV3SessionTTL = time.Minute
	MaxAuthV3SessionTTL = 30 * 24 * time.Hour

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
	ErrSecureChannelMissing = "认证请求必须经由 Noise 安全通道"
	ErrRequestBindingBroken = "请求上下文不匹配"
	ErrNoiseKeyMismatch     = "Noise 公钥标识不匹配"
	ErrInvalidNonce         = "nonce 无效"
	// ErrClientNotAuthorized covers every build-policy rejection (unknown or
	// revoked build, revoked version/bot, blocked source) with one message so
	// the response does not reveal which rule matched.
	ErrClientNotAuthorized = "客户端未获授权"
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
	dbClient         *ent.Client
	redisStore       RedisKVStore
	globalBanChecker GlobalBanChecker
	buildPolicy      *buildpolicy.Store
	security         secevent.Reporter
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
