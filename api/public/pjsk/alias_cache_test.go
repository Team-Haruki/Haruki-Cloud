package pjsk

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"haruki-cloud/config"
	pjskalias "haruki-cloud/internal/pjsk/alias"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func TestPublicAliasCacheNotFoundAndInvalidation(t *testing.T) {
	original := config.Cfg
	config.Cfg.Backend.APICacheTTL = time.Minute
	config.Cfg.Backend.AliasAPICacheTTL = 12 * time.Hour
	config.Cfg.Backend.AliasAPINotFoundCacheTTL = time.Hour
	t.Cleanup(func() { config.Cfg = original })

	ctx := context.Background()
	client := openPJSKTestClient(t)
	defer func() { _ = client.Close() }()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	server := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = redisClient.Close() }()

	app := fiber.New()
	RegisterPJSKRoutes(app, client, redisClient)

	const alias = "a+b c&d"
	byIDPath := "/api/v2/public/pjsk/alias/music/4001"
	byAliasPath := "/api/v2/public/pjsk/alias/music/by-alias?" + url.Values{"alias": {alias}}.Encode()

	for _, path := range []string{byIDPath, byAliasPath} {
		if got := requestPJSK(t, app, http.MethodGet, path, ""); got.Status != fiber.StatusNotFound {
			t.Fatalf("%s before approval: status %d", path, got.Status)
		}
	}
	if keys := server.Keys(); len(keys) != 2 {
		t.Fatalf("both 404s should be cached, keys = %v", keys)
	}
	for _, key := range server.Keys() {
		if ttl := server.TTL(key); ttl != time.Hour {
			t.Fatalf("404 TTL for %s = %v, want 1h", key, ttl)
		}
	}

	// The alias is approved: the row appears, but the cached 404s would hide
	// it for up to an hour without invalidation.
	client.Alias.Create().SetAliasType("music").SetAliasTypeID(4001).SetAlias(alias).SaveX(ctx)
	if got := requestPJSK(t, app, http.MethodGet, byIDPath, ""); got.Status != fiber.StatusNotFound {
		t.Fatalf("expected the cached 404 before invalidation, got %d", got.Status)
	}

	AliasCacheInvalidator(redisClient)(ctx, []pjskalias.AliasChange{{AliasType: "music", AliasTypeID: 4001, Alias: alias}})
	if keys := server.Keys(); len(keys) != 0 {
		t.Fatalf("invalidation should clear both keys, left %v", keys)
	}
	for _, path := range []string{byIDPath, byAliasPath} {
		if got := requestPJSK(t, app, http.MethodGet, path, ""); got.Status != fiber.StatusOK {
			t.Fatalf("%s after approval: status %d message %s", path, got.Status, got.Message)
		}
	}
	for _, key := range server.Keys() {
		if ttl := server.TTL(key); ttl != 12*time.Hour {
			t.Fatalf("200 TTL for %s = %v, want 12h", key, ttl)
		}
	}

	// Deletion clears the positive entries the same way.
	client.Alias.Delete().ExecX(ctx)
	InvalidateAliasCache(ctx, redisClient, []pjskalias.AliasChange{{AliasType: "music", AliasTypeID: 4001, Alias: alias}})
	if got := requestPJSK(t, app, http.MethodGet, byIDPath, ""); got.Status != fiber.StatusNotFound {
		t.Fatalf("after delete: status %d", got.Status)
	}
}

func TestAliasCacheInvalidatorNilRedis(t *testing.T) {
	if AliasCacheInvalidator(nil) != nil {
		t.Fatal("no redis means no listener")
	}
	InvalidateAliasCache(context.Background(), nil, []pjskalias.AliasChange{{AliasType: "music"}})
}
