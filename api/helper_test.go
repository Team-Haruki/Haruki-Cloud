package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"

	"github.com/gofiber/fiber/v3"
)

type helperTestEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func TestVerifyAPIAuthorizationRejectsWhenUnconfigured(t *testing.T) {
	prev := config.Cfg
	config.Cfg = config.Config{}
	t.Cleanup(func() { config.Cfg = prev })

	app := fiber.New()
	app.Post("/internal/test", VerifyAPIAuthorization(), func(c fiber.Ctx) error {
		return JSONResponse(c, fiber.StatusOK, "ok")
	})

	resp := sendHelperTestRequest(t, app, nil)
	if resp.Status != fiber.StatusServiceUnavailable {
		t.Fatalf("expected status=%d, got=%d message=%s", fiber.StatusServiceUnavailable, resp.Status, resp.Message)
	}
	if resp.Message != "Internal API authorization is not configured" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
}

func TestVerifyAPIAuthorizationAllowsExplicitInsecureMode(t *testing.T) {
	prev := config.Cfg
	config.Cfg = config.Config{}
	config.Cfg.Backend.AllowInsecureInternalAPI = true
	t.Cleanup(func() { config.Cfg = prev })

	app := fiber.New()
	app.Post("/internal/test", VerifyAPIAuthorization(), func(c fiber.Ctx) error {
		return JSONResponse(c, fiber.StatusOK, "ok")
	})

	resp := sendHelperTestRequest(t, app, nil)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("expected status=200, got=%d message=%s", resp.Status, resp.Message)
	}
}

func TestVerifyAPIAuthorizationUsesInternalAPITokenFallback(t *testing.T) {
	prev := config.Cfg
	config.Cfg = config.Config{}
	config.Cfg.HarukiBotDB.InternalAPIToken = "fallback-token"
	t.Cleanup(func() { config.Cfg = prev })

	app := fiber.New()
	app.Post("/internal/test", VerifyAPIAuthorization(), func(c fiber.Ctx) error {
		return JSONResponse(c, fiber.StatusOK, "ok")
	})

	unauthorized := sendHelperTestRequest(t, app, map[string]string{
		"Authorization": "fallback-token",
	})
	if unauthorized.Status != fiber.StatusUnauthorized {
		t.Fatalf("expected unauthorized status, got=%d message=%s", unauthorized.Status, unauthorized.Message)
	}

	authorized := sendHelperTestRequest(t, app, map[string]string{
		"Authorization": "Bearer fallback-token",
	})
	if authorized.Status != fiber.StatusOK {
		t.Fatalf("expected status=200, got=%d message=%s", authorized.Status, authorized.Message)
	}
}

func TestVerifyAPIAuthorizationStillChecksUserAgent(t *testing.T) {
	prev := config.Cfg
	config.Cfg = config.Config{}
	config.Cfg.Backend.AcceptAuthorization = "Bearer helper-test"
	config.Cfg.Backend.AcceptUserAgent = "Haruki-Internal"
	t.Cleanup(func() { config.Cfg = prev })

	app := fiber.New()
	app.Post("/internal/test", VerifyAPIAuthorization(), func(c fiber.Ctx) error {
		return JSONResponse(c, fiber.StatusOK, "ok")
	})

	forbidden := sendHelperTestRequest(t, app, map[string]string{
		"Authorization": "Bearer helper-test",
		"User-Agent":    "Other-Client",
	})
	if forbidden.Status != fiber.StatusForbidden {
		t.Fatalf("expected forbidden status, got=%d message=%s", forbidden.Status, forbidden.Message)
	}

	okResp := sendHelperTestRequest(t, app, map[string]string{
		"Authorization": "Bearer helper-test",
		"User-Agent":    "Haruki-Internal/1.0",
	})
	if okResp.Status != fiber.StatusOK {
		t.Fatalf("expected status=200, got=%d message=%s", okResp.Status, okResp.Message)
	}
}

func TestCacheKeyFromFiberCtxUsesCanonicalSHA256QueryHash(t *testing.T) {
	app := fiber.New()
	var cacheKey string
	app.Get("/cache", func(c fiber.Ctx) error {
		cacheKey = cacheKeyFromFiberCtx(c, "test")
		return c.SendStatus(fiber.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodGet, "/cache?b=2&a=3&a=1", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	const want = "test:/cache:query=4213fc4aacf338cd7ed11bb8456789925a6e37056cc2d1d8cd69662c87b9172c"
	if cacheKey != want {
		t.Fatalf("cache key = %q, want %q", cacheKey, want)
	}
}

func TestCacheKeyFromFiberCtxUsesNoneWithoutQuery(t *testing.T) {
	app := fiber.New()
	var cacheKey string
	app.Get("/cache", func(c fiber.Ctx) error {
		cacheKey = cacheKeyFromFiberCtx(c, "test")
		return c.SendStatus(fiber.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodGet, "/cache", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	if cacheKey != "test:/cache:query=none" {
		t.Fatalf("unexpected cache key without query: %q", cacheKey)
	}
}

func sendHelperTestRequest(t *testing.T, app *fiber.App, headers map[string]string) helperTestEnvelope {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, "/internal/test", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var envelope helperTestEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response body: %v raw=%s", err, string(body))
	}
	return envelope
}
