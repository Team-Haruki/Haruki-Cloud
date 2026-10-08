package api

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"
)

func cacheOptionsApp(t *testing.T, opts CacheOptions, fetch func(string) (any, error)) (*fiber.App, *miniredis.Miniredis) {
	t.Helper()
	original := config.Cfg
	config.Cfg.Backend.APICacheTTL = time.Minute
	t.Cleanup(func() { config.Cfg = original })
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	app.Get("/item", func(c fiber.Ctx) error {
		return WithCacheOptions(c, client, "opts", opts, fetch)
	})
	return app, server
}

func getItem(t *testing.T, app *fiber.App) (int, string) {
	t.Helper()
	response, err := app.Test(httptest.NewRequest("GET", "/item", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(raw)
}

func TestWithCacheOptionsCachesNotFound(t *testing.T) {
	fetches := 0
	app, server := cacheOptionsApp(t, CacheOptions{TTL: 12 * time.Hour, NotFoundTTL: time.Hour}, func(string) (any, error) {
		fetches++
		return nil, &CacheableStatusError{Status: fiber.StatusNotFound, Message: "not found"}
	})
	status, first := getItem(t, app)
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d", status)
	}
	status, second := getItem(t, app)
	if status != fiber.StatusNotFound || second != first {
		t.Fatalf("cached 404 = %d %s, want 404 %s", status, second, first)
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1 (404 should be cached)", fetches)
	}
	var envelope struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(first), &envelope); err != nil || envelope.Status != 404 || envelope.Message != "not found" {
		t.Fatalf("404 body = %s (%v)", first, err)
	}
	if ttl := server.TTL("opts:/item:query=none"); ttl != time.Hour {
		t.Fatalf("404 TTL = %v, want 1h", ttl)
	}
}

func TestWithCacheOptionsUsesPositiveTTL(t *testing.T) {
	fetches := 0
	app, server := cacheOptionsApp(t, CacheOptions{TTL: 12 * time.Hour, NotFoundTTL: time.Hour}, func(string) (any, error) {
		fetches++
		return map[string]int{"id": 1}, nil
	})
	for range 2 {
		if status, _ := getItem(t, app); status != fiber.StatusOK {
			t.Fatalf("status = %d", status)
		}
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d", fetches)
	}
	if ttl := server.TTL("opts:/item:query=none"); ttl != 12*time.Hour {
		t.Fatalf("TTL = %v, want 12h", ttl)
	}
}

func TestWithCacheOptionsNotFoundUncachedWithoutTTL(t *testing.T) {
	fetches := 0
	app, server := cacheOptionsApp(t, CacheOptions{}, func(string) (any, error) {
		fetches++
		return nil, &CacheableStatusError{Status: fiber.StatusNotFound, Message: "missing"}
	})
	for range 2 {
		if status, _ := getItem(t, app); status != fiber.StatusNotFound {
			t.Fatalf("status = %d", status)
		}
	}
	if fetches != 2 || server.Exists("opts:/item:query=none") {
		t.Fatalf("fetches = %d, cached = %v; 404 must not be cached without NotFoundTTL", fetches, server.Exists("opts:/item:query=none"))
	}
	if ttl := server.TTL("opts:/item:query=none"); ttl != 0 {
		t.Fatalf("unexpected TTL %v", ttl)
	}
}

func TestCachedResponseStatus(t *testing.T) {
	cases := map[string]int{
		`{"data":{"status":404},"message":"success","status":200}`: 200,
		`{"status":404,"message":"x","data":null}`:                 404,
		`{"status":"bad"}`:            200,
		`{"status":42,"message":"x"}`: 200,
		`null`:                        200,
	}
	for body, want := range cases {
		if got := cachedResponseStatus([]byte(body)); got != want {
			t.Fatalf("cachedResponseStatus(%s) = %d, want %d", body, got, want)
		}
	}
	if (&CacheableStatusError{Message: "m"}).Error() != "m" {
		t.Fatal("Error() should return the message")
	}
}
