package redis

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func ClearCache(ctx context.Context, redisClient *redis.Client, namespace, path string, queryParams *string) error {
	queryHash := "none"
	if queryParams != nil {
		canonicalQuery := CanonicalizeQueryString(*queryParams)
		sum := md5.Sum([]byte(canonicalQuery))
		queryHash = hex.EncodeToString(sum[:])
	}
	if err := DeleteCache(ctx, redisClient, fmt.Sprintf("%s:%s:query=%s", namespace, path, queryHash)); err != nil {
		return fmt.Errorf("clear redis cache failed: %w", err)
	}
	return nil
}
