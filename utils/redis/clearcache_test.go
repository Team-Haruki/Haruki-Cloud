package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redisclient "github.com/redis/go-redis/v9"
)

func TestCanonicalQueryHashUsesSHA256(t *testing.T) {
	const canonicalQuery = "a=1&a=3&b=2"
	const want = "4213fc4aacf338cd7ed11bb8456789925a6e37056cc2d1d8cd69662c87b9172c"
	if got := CanonicalQueryHash(canonicalQuery); got != want {
		t.Fatalf("CanonicalQueryHash() = %q, want %q", got, want)
	}
}

func TestClearCacheDeletesCanonicalSHA256Key(t *testing.T) {
	server := miniredis.RunT(t)
	client := redisclient.NewClient(&redisclient.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	const namespace = "test"
	const path = "/cache"
	query := "b=2&a=3&a=1"
	const key = "test:/cache:query=4213fc4aacf338cd7ed11bb8456789925a6e37056cc2d1d8cd69662c87b9172c"
	ctx := context.Background()
	if err := client.Set(ctx, key, "cached", 0).Err(); err != nil {
		t.Fatalf("seed cache key: %v", err)
	}

	if err := ClearCache(ctx, client, namespace, path, &query); err != nil {
		t.Fatalf("ClearCache: %v", err)
	}
	if exists, err := client.Exists(ctx, key).Result(); err != nil || exists != 0 {
		t.Fatalf("cache key still exists: exists=%d err=%v", exists, err)
	}
}
