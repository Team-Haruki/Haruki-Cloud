package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"
	harukiRedis "haruki-cloud/utils/redis"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/shamaton/msgpack/v3"
)

func TestResponseHelpersAndValidation(t *testing.T) {
	app := fiber.New()
	app.Get("/json", func(c fiber.Ctx) error { return JSONResponse(c, 201, "created", fiber.Map{"id": 1}) })
	app.Get("/json-empty", func(c fiber.Ctx) error { return ErrorResponse(c, 400, "bad") })
	app.Get("/internal", func(c fiber.Ctx) error { return InternalError(c) })
	app.Get("/msgpack", func(c fiber.Ctx) error { return MsgPackResponse(c, 202, "packed", "value") })
	app.Get("/msgpack-empty", func(c fiber.Ctx) error { return MsgPackResponse(c, 203, "empty") })

	for path, wantStatus := range map[string]int{"/json": 201, "/json-empty": 400, "/internal": 500} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		var envelope helperTestEnvelope
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != wantStatus || envelope.Status != wantStatus {
			t.Fatalf("%s status = HTTP %d envelope %d", path, response.StatusCode, envelope.Status)
		}
	}

	for _, path := range []string{"/msgpack", "/msgpack-empty"} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var envelope map[string]any
		if err := msgpack.Unmarshal(body, &envelope); err != nil {
			t.Fatalf("decode msgpack %s: %v", path, err)
		}
		if response.Header.Get("Content-Type") != ContentTypeMsgPack {
			t.Fatalf("%s content type = %q", path, response.Header.Get("Content-Type"))
		}
	}

	if !ValidateStringLength("你好", 2) || ValidateStringLength("你好", 1) {
		t.Fatal("rune-aware string validation failed")
	}
	if ValidateAlias("") || !ValidateAlias("alias") || ValidateServer("") || !ValidateServer("jp") {
		t.Fatal("alias/server validation failed")
	}
	if (&CacheBypassError{}).Error() != "cache bypass" {
		t.Fatal("unexpected cache bypass error text")
	}
}

func TestCacheQueryAndWithCachePaths(t *testing.T) {
	original := config.Cfg
	config.Cfg.Backend.APICacheTTL = time.Minute
	t.Cleanup(func() { config.Cfg = original })

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	fetches := 0
	app := fiber.New()
	app.Get("/cached/:haruki_user_id", func(c fiber.Ctx) error {
		if got := GetHarukiUserIDFromPath(c); got != 42 {
			return fmt.Errorf("path id = %d", got)
		}
		if got := GetHarukiUserIDFromQuery(c); got != 7 {
			return fmt.Errorf("query id = %d", got)
		}
		return WithCache(c, client, "additional", func(_ string) (any, error) {
			fetches++
			return fiber.Map{"value": "fresh"}, nil
		})
	})
	app.Get("/failure", func(c fiber.Ctx) error {
		return WithCache(c, nil, "failure", func(_ string) (any, error) { return nil, errors.New("fetch failed") })
	})
	app.Get("/bypass", func(c fiber.Ctx) error {
		return WithCache(c, nil, "bypass", func(_ string) (any, error) {
			return nil, &CacheBypassError{Response: JSONResponse(c, http.StatusNotFound, "missing")}
		})
	})

	for range 2 {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, "/cached/42?haruki_user_id=7", nil))
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("cached response = %#v, %v", response, err)
		}
		_ = response.Body.Close()
	}
	if fetches != 1 {
		t.Fatalf("fetch count = %d", fetches)
	}
	for path, want := range map[string]int{"/failure": 500, "/bypass": 404} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil || response.StatusCode != want {
			t.Fatalf("%s response = %#v, %v", path, response, err)
		}
		_ = response.Body.Close()
	}

	probe := fiber.New()
	probe.Get("/probe", func(c fiber.Ctx) error {
		key, cached, found, err := CacheQuery(context.Background(), c, nil, "probe")
		if err != nil || found || cached != nil || !strings.Contains(key, "probe:/probe") {
			return fmt.Errorf("nil cache result = %q %#v %t %v", key, cached, found, err)
		}
		return CachedJSONResponse(context.Background(), c, client, time.Minute, key, 200, "ok", "data")
	})
	response, err := probe.Test(httptest.NewRequest(http.MethodGet, "/probe", nil))
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("probe response = %#v, %v", response, err)
	}
	_ = response.Body.Close()
	if _, err := client.Get(context.Background(), "probe:/probe:query=none").Result(); err != nil {
		t.Fatalf("cached probe missing: %v", err)
	}

	badQuery := fiber.New()
	badQuery.Get("/bad", func(c fiber.Ctx) error {
		closedClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
		_ = closedClient.Close()
		_, _, _, err := CacheQuery(context.Background(), c, closedClient, "bad")
		if err == nil {
			return errors.New("expected cache error")
		}
		return c.SendStatus(204)
	})
	response, err = badQuery.Test(httptest.NewRequest(http.MethodGet, "/bad", nil))
	if err != nil || response.StatusCode != 204 {
		t.Fatalf("bad cache probe = %#v, %v", response, err)
	}
	_ = response.Body.Close()

	if err := harukiRedis.SetCache(context.Background(), client, "typed", map[string]any{"ok": true}, time.Minute); err != nil {
		t.Fatalf("seed typed cache: %v", err)
	}
}

func signSessionTokenForTest(t *testing.T, method jwt.SigningMethod, botID string) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, jwt.MapClaims{
		"bot_id": botID,
		"exp":    time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("session-secret"))
	if err != nil {
		t.Fatalf("sign session token: %v", err)
	}
	return token
}

func TestVerifyBotSessionAllBranches(t *testing.T) {
	original := config.Cfg
	config.Cfg.HarukiBotDB.SessionSignToken = "session-secret"
	t.Cleanup(func() { config.Cfg = original })

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	makeApp := func(redisClient *redis.Client) *fiber.App {
		app := fiber.New()
		app.Get("/bots/:botId", VerifyBotSession(redisClient), func(c fiber.Ctx) error { return c.SendStatus(204) })
		return app
	}
	request := func(app *fiber.App, botID, headerID, token string) int {
		req := httptest.NewRequest(http.MethodGet, "/bots/"+botID, nil)
		if headerID != "" {
			req.Header.Set(HeaderBotID, headerID)
		}
		if token != "" {
			req.Header.Set(HeaderBotSessionToken, token)
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatalf("session request: %v", err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}

	valid := signSessionTokenForTest(t, jwt.SigningMethodHS256, "42")
	app := makeApp(client)
	if got := request(makeApp(nil), "42", "42", valid); got != 503 {
		t.Fatalf("nil redis status = %d", got)
	}
	if got := request(app, "42", "", ""); got != 401 {
		t.Fatalf("missing headers status = %d", got)
	}
	if got := request(app, "42", "43", valid); got != 403 {
		t.Fatalf("URL mismatch status = %d", got)
	}
	if got := request(app, "42", "42", "not-a-jwt"); got != 401 {
		t.Fatalf("invalid JWT status = %d", got)
	}
	wrongClaim := signSessionTokenForTest(t, jwt.SigningMethodHS256, "99")
	if got := request(app, "42", "42", wrongClaim); got != 403 {
		t.Fatalf("claim mismatch status = %d", got)
	}
	hs384 := signSessionTokenForTest(t, jwt.SigningMethodHS384, "42")
	if got := request(app, "42", "42", hs384); got != 401 {
		t.Fatalf("HS384 status = %d, want 401", got)
	}
	if got := request(app, "42", "42", valid); got != 401 {
		t.Fatalf("missing Redis session status = %d", got)
	}
	server.Set(fmt.Sprintf(RedisKeyBotSession, "42"), "different")
	if got := request(app, "42", "42", valid); got != 401 {
		t.Fatalf("Redis token mismatch status = %d", got)
	}
	server.Set(fmt.Sprintf(RedisKeyBotSession, "42"), valid)
	if got := request(app, "42", "42", valid); got != 204 {
		t.Fatalf("valid session status = %d", got)
	}

	bypass := fiber.New()
	bypass.Get("/bypass", VerifyBotSessionTestBypass(), func(c fiber.Ctx) error { return c.SendStatus(204) })
	response, err := bypass.Test(httptest.NewRequest(http.MethodGet, "/bypass", nil))
	if err != nil || response.StatusCode != 204 {
		t.Fatalf("test bypass response = %#v, %v", response, err)
	}
	_ = response.Body.Close()
}
