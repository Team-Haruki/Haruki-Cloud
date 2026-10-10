package authban

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/middleware/secure"

	"github.com/gofiber/fiber/v3"
)

// proxiedApp mimics production: the test connection's peer (0.0.0.0) is the
// trusted reverse proxy and X-Forwarded-For carries the client address.
func proxiedApp() *fiber.App {
	return fiber.New(fiber.Config{
		ProxyHeader:        fiber.HeaderXForwardedFor,
		EnableIPValidation: true,
		TrustProxy:         true,
		TrustProxyConfig:   fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0/32"}},
	})
}

type loginOutcome int

const (
	outcomeSuccess loginOutcome = iota
	outcomeAuthFailed
	outcomeHandshakeFailed
	outcomeUnmarked
)

type loginApp struct {
	app     *fiber.App
	reached atomic.Int32
	next    atomic.Int32
}

// newLoginApp mounts LoginMiddleware in front of a stand-in for the Noise
// middleware plus handler, whose verdict the test picks per request.
func newLoginApp(g *Guard) *loginApp {
	la := &loginApp{app: proxiedApp()}
	la.app.Post("/api/v3/bot/:bot_id/auth", g.LoginMiddleware("bot_id"), func(c fiber.Ctx) error {
		la.reached.Add(1)
		switch loginOutcome(la.next.Load()) {
		case outcomeHandshakeFailed:
			c.Locals(secure.LocalHandshakeFailed, true)
			return c.SendStatus(fiber.StatusBadRequest)
		case outcomeAuthFailed:
			MarkFailure(c, ReasonAuthFailed)
			return c.SendStatus(fiber.StatusBadRequest)
		case outcomeUnmarked:
			return c.SendStatus(fiber.StatusTooManyRequests)
		}
		MarkSuccess(c)
		return c.SendStatus(fiber.StatusOK)
	})
	return la
}

func (la *loginApp) post(t *testing.T, xff, bot string, o loginOutcome) *http.Response {
	t.Helper()
	la.next.Store(int32(o))
	req := httptest.NewRequest(http.MethodPost, "/api/v3/bot/"+bot+"/auth", strings.NewReader("x"))
	if xff != "" {
		req.Header.Set(fiber.HeaderXForwardedFor, xff)
	}
	resp, err := la.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestLoginMiddlewareRejectsBeforeHandler(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 3, ExemptKnownBots: true})
	la := newLoginApp(env.guard)

	la.post(t, attackerIP, botB, outcomeSuccess)
	la.post(t, attackerIP, botA, outcomeHandshakeFailed)
	la.post(t, attackerIP, botA, outcomeAuthFailed)
	la.post(t, attackerIP, botA, outcomeUnmarked) // e.g. per-bot rate limit: not counted
	if env.guard.Check(context.Background(), attackerIP, botA).Banned {
		t.Fatal("unmarked outcome was counted")
	}
	la.post(t, attackerIP, botA, outcomeAuthFailed)
	reached := la.reached.Load()

	resp := la.post(t, attackerIP, botA, outcomeSuccess)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get(fiber.HeaderRetryAfter); got != "21600" {
		t.Fatalf("Retry-After = %q, want 21600", got)
	}
	if string(body) != i18n.T("account.api.auth_banned") || strings.Contains(string(body), attackerIP) {
		t.Fatalf("body = %q", body)
	}
	if la.reached.Load() != reached {
		t.Fatal("banned request reached the Noise middleware / handler")
	}

	// The bot that logged in from this address before keeps working.
	if resp := la.post(t, attackerIP, botB, outcomeSuccess); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("known bot status = %d", resp.StatusCode)
	}
	// Other addresses are unaffected.
	if resp := la.post(t, "198.51.100.30", botA, outcomeSuccess); resp.StatusCode != fiber.StatusOK {
		t.Fatalf("other address status = %d", resp.StatusCode)
	}
}

func TestLoginMiddlewareUsesTrustedClientAddress(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 1})
	la := newLoginApp(env.guard)

	// A client-supplied entry left of the real address is ignored: the
	// trusted proxy appended the address it saw.
	la.post(t, "192.0.2.66, "+attackerIP, botA, outcomeAuthFailed)
	if !env.guard.Check(context.Background(), attackerIP, botA).Banned {
		t.Fatal("real client address not banned")
	}
	if env.guard.Check(context.Background(), "192.0.2.66", botA).Banned {
		t.Fatal("spoofed X-Forwarded-For entry was banned")
	}

	// From an untrusted peer the header is ignored altogether; the peer
	// itself is the client (0.0.0.0 here, which is never banned).
	untrusted := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor, EnableIPValidation: true, TrustProxy: true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"10.0.0.1/32"}}})
	untrusted.Post("/api/v3/bot/:bot_id/auth", env.guard.LoginMiddleware("bot_id"), func(c fiber.Ctx) error {
		MarkFailure(c, ReasonAuthFailed)
		return c.SendStatus(fiber.StatusBadRequest)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v3/bot/"+botA+"/auth", nil)
	req.Header.Set(fiber.HeaderXForwardedFor, "198.51.100.77")
	if _, err := untrusted.Test(req); err != nil {
		t.Fatal(err)
	}
	if env.guard.Check(context.Background(), "198.51.100.77", botA).Banned {
		t.Fatal("X-Forwarded-For from an untrusted peer was trusted")
	}
}

func TestNilGuardMiddlewaresPassThrough(t *testing.T) {
	var g *Guard
	la := newLoginApp(g)
	if resp := la.post(t, attackerIP, botA, outcomeAuthFailed); resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	app := fiber.New()
	app.Use(g.BotRoutesMiddleware())
	app.Get("/api/v2/bot/1/command/manifests", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v2/bot/1/command/manifests", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("nil bot-route guard: %v %v", resp, err)
	}
}

func TestBotRoutesMiddleware(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 1, ExemptKnownBots: true, BlockBotRoutes: true})
	if !env.guard.BlocksBotRoutes() {
		t.Fatal("BlocksBotRoutes = false")
	}
	ctx := context.Background()
	env.guard.RecordSuccess(ctx, attackerIP, botB)
	env.guard.RecordFailure(ctx, attackerIP, botA, ReasonAuthFailed)

	app := proxiedApp()
	app.Use(env.guard.BotRoutesMiddleware())
	ok := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	app.Post("/api/v2/bot/:botId/pjsk/*", ok)
	app.Get("/api/v2/bot/:botId/command/manifests", ok)
	app.Delete("/api/v3/bot/:bot_id/logout", ok)
	app.Post("/api/v3/bot/:bot_id/auth", ok)
	app.Get("/api/v2/public/pjsk/alias/x", ok)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/api/v2/bot/" + botA + "/pjsk/profile", fiber.StatusTooManyRequests},
		{http.MethodGet, "/api/v2/bot/" + botA + "/command/manifests", fiber.StatusTooManyRequests},
		{http.MethodDelete, "/api/v3/bot/" + botA + "/logout", fiber.StatusTooManyRequests},
		{http.MethodPost, "/api/v2/bot/" + botB + "/pjsk/profile", fiber.StatusOK}, // known bot
		{http.MethodPost, "/api/v3/bot/" + botA + "/auth", fiber.StatusOK},         // left to LoginMiddleware
		{http.MethodGet, "/api/v2/public/pjsk/alias/x", fiber.StatusOK},            // not a bot route
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set(fiber.HeaderXForwardedFor, attackerIP)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != tc.want {
			t.Fatalf("%s %s = %d, want %d", tc.method, tc.path, resp.StatusCode, tc.want)
		}
		if tc.want == fiber.StatusTooManyRequests && resp.Header.Get(fiber.HeaderRetryAfter) == "" {
			t.Fatalf("%s %s: no Retry-After", tc.method, tc.path)
		}
	}
}

func TestBotRouteBotID(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v2/bot/123/pjsk/x":      "123",
		"/api/v2/bot/123":             "123",
		"/api/v3/bot/123/logout":      "123",
		"/api/v3/bot/123/auth":        "",
		"/api/v3/bot/123/auth/":       "",
		"/api/v2/bot/123/auth":        "123",
		"/api/v2/bot//pjsk":           "",
		"/api/v2/public/pjsk/alias":   "",
		"/internal/bot/auth-bans":     "",
		"/api/v2/botanist/123/pjsk/x": "",
		"/api/v3/bot/":                "",
	} {
		got, ok := botRouteBotID(path)
		if got != want || ok != (want != "") {
			t.Fatalf("botRouteBotID(%q) = %q, %v; want %q", path, got, ok, want)
		}
	}
}

func TestRejectRoundsRetryAfterUp(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error { return reject(c, 1500*time.Millisecond) })
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil || resp.Header.Get(fiber.HeaderRetryAfter) != "2" {
		t.Fatalf("Retry-After = %q, %v", resp.Header.Get(fiber.HeaderRetryAfter), err)
	}
}
