package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"haruki-cloud/config"
	ent "haruki-cloud/database/bot"

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

func (s *redisKVStore) Incr(ctx context.Context, key string) (int64, error) {
	if s.client == nil {
		return 0, errRedisClientUnavailable
	}
	return s.client.Incr(ctx, key).Result()
}

func (s *redisKVStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if s.client == nil {
		return errRedisClientUnavailable
	}
	return s.client.Expire(ctx, key, ttl).Err()
}

// ================= Service Constructors =================

func NewUserService(dbClient *ent.Client, redisClient *redis.Client, authEncryptionKey []byte, noiseServerPubKey string) *UserService {
	cfg := config.Cfg.HarukiBotDB
	return NewUserServiceWithDependencies(
		dbClient,
		newRedisKVStore(redisClient),
		newTurnstileClient(cfg.TurnstileSecretKey),
		newSMTPClient(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom),
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

// ================= Rate Limiting =================

// checkRateLimit 检查速率限制。返回 true 表示允许通过，false 表示已超限。
// 使用 INCR + EXPIRE 模式：仅在首次创建 key 时设置 TTL，后续递增不会重置窗口。
func (s *UserService) checkRateLimit(ctx context.Context, action string, identifier string, maxRequests int, windowMinutes int) (bool, error) {
	key := fmt.Sprintf(RedisKeyRateLimit, action, identifier)
	count, err := s.redisStore.Incr(ctx, key)
	if err != nil {
		return false, err
	}

	// 首次创建 key（count == 1）时设置 TTL
	if count == 1 {
		_ = s.redisStore.Expire(ctx, key, time.Duration(windowMinutes)*time.Minute)
	}

	return count <= int64(maxRequests), nil
}

