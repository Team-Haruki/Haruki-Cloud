package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	json "haruki-cloud/internal/jsonutil"
)

type CachePath struct {
	Namespace   string
	Path        string
	QueryString string
}

func CanonicalizeQueryString(queryString string) string {
	if queryString == "" {
		return ""
	}
	values, err := url.ParseQuery(queryString)
	if err != nil {
		// Fallback to original if parsing fails, though unlikely with valid requests
		return queryString
	}

	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		vals := values[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return strings.Join(parts, "&")
}

// CanonicalQueryHash returns a stable cache-key digest for an already
// canonicalized query string.
func CanonicalQueryHash(canonicalQuery string) string {
	sum := sha256.Sum256([]byte(canonicalQuery))
	return hex.EncodeToString(sum[:])
}

func SetCache(ctx context.Context, client *redis.Client, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return client.Set(ctx, key, data, ttl).Err()
}

func GetCache(ctx context.Context, client *redis.Client, key string, out any) (bool, error) {
	val, err := client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(val), out)
}

func DeleteCache(ctx context.Context, client *redis.Client, key string) error {
	return client.Del(ctx, key).Err()
}
