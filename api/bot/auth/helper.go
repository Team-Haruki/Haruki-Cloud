package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"haruki-cloud/config"
	ent "haruki-cloud/database/bot"
	"haruki-cloud/utils/smtp"
	"haruki-cloud/utils/turnstile"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var errRedisClientUnavailable = errors.New("redis client is unavailable")

type redisKVStore struct {
	client *redis.Client
}

func newRedisKVStore(client *redis.Client) RedisKVStore {
	return &redisKVStore{client: client}
}

func (s *redisKVStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if s.client == nil {
		return errRedisClientUnavailable
	}
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *redisKVStore) Get(ctx context.Context, key string) (string, error) {
	if s.client == nil {
		return "", errRedisClientUnavailable
	}
	return s.client.Get(ctx, key).Result()
}

func (s *redisKVStore) Del(ctx context.Context, key string) error {
	if s.client == nil {
		return errRedisClientUnavailable
	}
	return s.client.Del(ctx, key).Err()
}

// ================= Service Constructors =================

func NewUserService(dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string) *UserService {
	cfg := config.Cfg.HarukiBotDB
	return NewUserServiceWithDependencies(
		dbClient,
		newRedisKVStore(redisClient),
		turnstile.NewClient(cfg.TurnstileSecretKey),
		smtp.NewClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom),
		authEncryptionKey,
		noiseServerPubKey,
	)
}

func NewUserServiceWithDependencies(
	dbClient *ent.Client,
	redisStore RedisKVStore,
	turnstileClient TurnstileVerifier,
	smtpClient VerificationMailer,
	authEncryptionKey []byte,
	noiseServerPubKey string,
) *UserService {
	if redisStore == nil {
		redisStore = newRedisKVStore(nil)
	}
	return &UserService{
		dbClient:          dbClient,
		redisStore:        redisStore,
		turnstileClient:   turnstileClient,
		smtpClient:        smtpClient,
		authEncryptionKey: authEncryptionKey,
		noiseServerPubKey: noiseServerPubKey,
	}
}

func NewInternalService(dbClient *ent.Client, redisClient *redis.Client) *InternalService {
	return NewInternalServiceWithStore(dbClient, newRedisKVStore(redisClient))
}

func NewInternalServiceWithStore(dbClient *ent.Client, redisStore RedisKVStore) *InternalService {
	if redisStore == nil {
		redisStore = newRedisKVStore(nil)
	}
	return &InternalService{dbClient: dbClient, redisStore: redisStore}
}

func NewStatisticsService(client *ent.Client) *StatisticsService {
	return &StatisticsService{client: client}
}

// ================= Handler Constructors =================

func NewUserHandler(svc *UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func NewInternalHandler(svc *InternalService) *InternalHandler {
	return &InternalHandler{svc: svc}
}

func NewStatisticsHandler(svc *StatisticsService) *StatisticsHandler {
	return &StatisticsHandler{svc: svc}
}

// ================= UserService Methods =================

func (s *UserService) setRedisKey(ctx context.Context, pattern string, id any, value string, ttlMinutes int) error {
	key := fmt.Sprintf(pattern, id)
	return s.redisStore.Set(ctx, key, value, time.Duration(ttlMinutes)*time.Minute)
}

func (s *UserService) getRedisKey(ctx context.Context, pattern string, id any) (string, error) {
	key := fmt.Sprintf(pattern, id)
	return s.redisStore.Get(ctx, key)
}

func (s *UserService) delRedisKey(ctx context.Context, pattern string, id any) error {
	key := fmt.Sprintf(pattern, id)
	return s.redisStore.Del(ctx, key)
}

func (s *UserService) cleanupRegistrationKeys(ctx context.Context, qqNumber int64) {
	_ = s.delRedisKey(ctx, RedisKeyVerifyCode, qqNumber)
	_ = s.delRedisKey(ctx, RedisKeyVerifyStatus, qqNumber)
}

// ================= InternalService Methods =================

func (s *InternalService) getRedisKey(ctx context.Context, pattern string, id any) (string, error) {
	key := fmt.Sprintf(pattern, id)
	return s.redisStore.Get(ctx, key)
}

// ================= Utility Functions =================

func generateVerificationCode(length int) string {
	digits := "0123456789"
	code := make([]byte, length)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		code[i] = digits[n.Int64()]
	}
	return string(code)
}

func generateBotID() (int, error) {
	// 生成 8 位数字 bot_id (10000000 - 99999999)
	n, err := rand.Int(rand.Reader, big.NewInt(90000000))
	if err != nil {
		return 0, err
	}
	return int(n.Int64() + 10000000), nil
}

func generateCredential() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// getSessionTTL 获取 session 有效期，限制在 1~30 天
func getSessionTTL() time.Duration {
	days := config.Cfg.HarukiBotDB.SessionTTLDays
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	return time.Duration(days) * 24 * time.Hour
}

// ================= Rate Limiting =================

// checkRateLimit 检查速率限制。返回 true 表示允许通过，false 表示已超限。
func (s *UserService) checkRateLimit(ctx context.Context, action string, identifier string, maxRequests int, windowMinutes int) (bool, error) {
	key := fmt.Sprintf(RedisKeyRateLimit, action, identifier)
	val, err := s.redisStore.Get(ctx, key)
	if errors.Is(err, redis.Nil) {
		// 首次访问
		return true, s.redisStore.Set(ctx, key, "1", time.Duration(windowMinutes)*time.Minute)
	}
	if err != nil {
		return false, err
	}

	count, _ := strconv.Atoi(val)
	if count >= maxRequests {
		return false, nil
	}

	return true, s.redisStore.Set(ctx, key, strconv.Itoa(count+1), time.Duration(windowMinutes)*time.Minute)
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
	err = s.redisStore.Set(ctx, nonceKey, "1", AuthTimestampMaxAge*time.Second)
	return err == nil, err
}

// ================= Credential Hashing =================

// hashCredential 使用 bcrypt 哈希 credential
func hashCredential(credential string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(credential), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// verifyCredential 验证 credential。兼容处理：先尝试 bcrypt 验证，
// 如果 stored 不是 bcrypt 格式（旧的明文记录），回退到常量时间比较。
func verifyCredential(stored, provided string) bool {
	if isBcryptHash(stored) {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(provided)) == nil
	}
	// 明文回退（兼容旧记录）
	return len(stored) > 0 && len(provided) > 0 &&
		subtle.ConstantTimeCompare([]byte(stored), []byte(provided)) == 1
}

// isBcryptHash 检查字符串是否为 bcrypt 哈希格式
func isBcryptHash(s string) bool {
	return len(s) == 60 && s[0] == '$' && s[1] == '2'
}

// ================= Session Deletion (Logout) =================

// deleteSession 从 Redis 删除指定 bot_id 的 session
func (s *UserService) deleteSession(ctx context.Context, botID string) error {
	return s.delRedisKey(ctx, RedisKeySessionToken, botID)
}
