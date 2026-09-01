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
	"haruki-cloud/internal/testutil"
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
		testutil.Require(t, !(err != nil), "request %s: %v", path, err)

		var envelope helperTestEnvelope
		{
			err := json.NewDecoder(response.Body).Decode(&envelope)
			testutil.Require(t, !(err != nil), "decode %s: %v", path, err)
		}

		_ = response.Body.Close()
		{
			testutil.Require(t, !(response.StatusCode != wantStatus), "%s status = HTTP %d envelope %d", path, response.StatusCode, envelope.Status)
			testutil.Require(t, !(envelope.Status != wantStatus), "%s status = HTTP %d envelope %d", path, response.StatusCode, envelope.Status)
		}

	}

	for _, path := range []string{"/msgpack", "/msgpack-empty"} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		testutil.Require(t, !(err != nil), "request %s: %v", path, err)

		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		testutil.Require(t, !(err != nil), "read %s: %v", path, err)

		var envelope map[string]any
		{
			err := msgpack.Unmarshal(body, &envelope)
			testutil.Require(t, !(err != nil), "decode msgpack %s: %v", path, err)
		}

		testutil.Require(t, !(response.Header.Get("Content-Type") != ContentTypeMsgPack), "%s content type = %q", path, response.Header.Get("Content-Type"))

	}
	{
		testutil.RequireArgs(t, ValidateStringLength("你好", 2), "rune-aware string validation failed")
		testutil.RequireArgs(t, !(ValidateStringLength("你好", 1)), "rune-aware string validation failed")
	}
	{
		testutil.RequireArgs(t, !(ValidateAlias("")), "alias/server validation failed")
		testutil.RequireArgs(t, ValidateAlias("alias"), "alias/server validation failed")
		testutil.RequireArgs(t, !(ValidateServer("")), "alias/server validation failed")
		testutil.RequireArgs(t, ValidateServer("jp"), "alias/server validation failed")
	}
	testutil.RequireArgs(t, !((&CacheBypassError{}).Error() != "cache bypass"), "unexpected cache bypass error text")

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
		{
			testutil.Require(t, !(err != nil), "cached response = %#v, %v", response, err)
			testutil.Require(t, !(response.StatusCode != http.StatusOK), "cached response = %#v, %v", response, err)
		}

		_ = response.Body.Close()
	}
	testutil.Require(t, !(fetches != 1), "fetch count = %d", fetches)

	for path, want := range map[string]int{"/failure": 500, "/bypass": 404} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		{
			testutil.Require(t, !(err != nil), "%s response = %#v, %v", path, response, err)
			testutil.Require(t, !(response.StatusCode != want), "%s response = %#v, %v", path, response, err)
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
	{
		testutil.Require(t, !(err != nil), "probe response = %#v, %v", response, err)
		testutil.Require(t, !(response.StatusCode != 200), "probe response = %#v, %v", response, err)
	}

	_ = response.Body.Close()
	{
		_, err := client.Get(context.Background(), "probe:/probe:query=none").Result()
		testutil.Require(t, !(err != nil), "cached probe missing: %v", err)
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
	{
		testutil.Require(t, !(err != nil), "bad cache probe = %#v, %v", response, err)
		testutil.Require(t, !(response.StatusCode != 204), "bad cache probe = %#v, %v", response, err)
	}

	_ = response.Body.Close()
	{

		err := harukiRedis.SetCache(context.Background(), client, "typed", map[string]any{"ok": true}, time.Minute)
		testutil.Require(t, !(err != nil), "seed typed cache: %v", err)
	}

}

func signSessionTokenForTest(t *testing.T, method jwt.SigningMethod, botID string) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, jwt.MapClaims{
		"bot_id": botID,
		"exp":    time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("session-secret"))
	testutil.Require(t, !(err != nil), "sign session token: %v", err)

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
		testutil.Require(t, !(err != nil), "session request: %v", err)

		defer response.Body.Close()
		return response.StatusCode
	}

	valid := signSessionTokenForTest(t, jwt.SigningMethodHS256, "42")
	app := makeApp(client)
	{
		got := request(makeApp(nil), "42", "42", valid)
		testutil.Require(t, !(got != 503), "nil redis status = %d", got)
	}
	{

		got := request(app, "42", "", "")
		testutil.Require(t, !(got != 401), "missing headers status = %d", got)
	}
	{

		got := request(app, "42", "43", valid)
		testutil.Require(t, !(got != 403), "URL mismatch status = %d", got)
	}
	{

		got := request(app, "42", "42", "not-a-jwt")
		testutil.Require(t, !(got != 401), "invalid JWT status = %d", got)
	}

	wrongClaim := signSessionTokenForTest(t, jwt.SigningMethodHS256, "99")
	{
		got := request(app, "42", "42", wrongClaim)
		testutil.Require(t, !(got != 403), "claim mismatch status = %d", got)
	}

	hs384 := signSessionTokenForTest(t, jwt.SigningMethodHS384, "42")
	{
		got := request(app, "42", "42", hs384)
		testutil.Require(t, !(got != 401), "HS384 status = %d, want 401", got)
	}
	{

		got := request(app, "42", "42", valid)
		testutil.Require(t, !(got != 401), "missing Redis session status = %d", got)
	}

	server.Set(fmt.Sprintf(RedisKeyBotSession, "42"), "different")
	{
		got := request(app, "42", "42", valid)
		testutil.Require(t, !(got != 401), "Redis token mismatch status = %d", got)
	}

	server.Set(fmt.Sprintf(RedisKeyBotSession, "42"), valid)
	{
		got := request(app, "42", "42", valid)
		testutil.Require(t, !(got != 204), "valid session status = %d", got)
	}

	bypass := fiber.New()
	bypass.Get("/bypass", VerifyBotSessionTestBypass(), func(c fiber.Ctx) error { return c.SendStatus(204) })
	response, err := bypass.Test(httptest.NewRequest(http.MethodGet, "/bypass", nil))
	{
		testutil.Require(t, !(err != nil), "test bypass response = %#v, %v", response, err)
		testutil.Require(t, !(response.StatusCode != 204), "test bypass response = %#v, %v", response, err)
	}

	_ = response.Body.Close()
}
