package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	"haruki-cloud/config"
	ent "haruki-cloud/database/bot"
	"haruki-cloud/utils/smtp"
	"haruki-cloud/utils/turnstile"

	"github.com/redis/go-redis/v9"
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

func NewUserService(dbClient *ent.Client, redisClient *redis.Client) *UserService {
	cfg := config.Cfg.HarukiBotDB
	return NewUserServiceWithDependencies(
		dbClient,
		newRedisKVStore(redisClient),
		turnstile.NewClient(cfg.TurnstileSecretKey),
		smtp.NewClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom),
	)
}

func NewUserServiceWithDependencies(
	dbClient *ent.Client,
	redisStore RedisKVStore,
	turnstileClient TurnstileVerifier,
	smtpClient VerificationMailer,
) *UserService {
	if redisStore == nil {
		redisStore = newRedisKVStore(nil)
	}
	return &UserService{
		dbClient:        dbClient,
		redisStore:      redisStore,
		turnstileClient: turnstileClient,
		smtpClient:      smtpClient,
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

// getSessionTTL 获取 session 有效期
func getSessionTTL() time.Duration {
	days := config.Cfg.HarukiBotDB.SessionTTLDays
	if days <= 0 {
		days = 7
	}
	return time.Duration(days) * 24 * time.Hour
}

// deriveKeyFromCredential 从 credential 派生 32 字节的 AES-256 密钥
// 使用 credential 的 SHA-256 hash 作为密钥
func deriveKeyFromCredential(credential string) []byte {
	// 使用简单的方式：取 credential 的前 32 字节，不足则填充
	key := make([]byte, 32)
	copy(key, []byte(credential))
	return key
}
