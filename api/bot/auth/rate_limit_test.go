package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type expiryFailingRedisStore struct {
	*memoryRedisStore
	expireErr error
	deleted   bool
}

func (s *expiryFailingRedisStore) Expire(context.Context, string, time.Duration) error {
	return s.expireErr
}

func (s *expiryFailingRedisStore) Del(ctx context.Context, key string) error {
	s.deleted = true
	return s.memoryRedisStore.Del(ctx, key)
}

func TestCheckRateLimitCleansUpCounterWhenExpiryFails(t *testing.T) {
	wantErr := errors.New("expiry unavailable")
	store := &expiryFailingRedisStore{
		memoryRedisStore: newMemoryRedisStore(),
		expireErr:        wantErr,
	}
	service := NewUserServiceWithDependencies(nil, store, nil, "")

	allowed, err := service.checkRateLimit(context.Background(), "auth", "123", 10, 1)
	if allowed {
		t.Fatal("checkRateLimit() allowed request when expiry setup failed")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("checkRateLimit() error = %v, want wrapped %v", err, wantErr)
	}
	if !store.deleted {
		t.Fatal("checkRateLimit() did not clean up the counter")
	}
	if _, ok := store.value["haruki:ratelimit:auth:123"]; ok {
		t.Fatal("rate-limit counter remained after expiry failure")
	}
}
