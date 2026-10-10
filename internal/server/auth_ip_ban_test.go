package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/authban"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// TestFiberConfigClientIP pins how c.IP() resolves the client address, which
// the login IP ban and the security events rely on.
func TestFiberConfigClientIP(t *testing.T) {
	original := harukiConfig.Cfg
	t.Cleanup(func() { harukiConfig.Cfg = original })

	for _, tc := range []struct {
		name    string
		trusted []string
		xff     string
		want    string
	}{
		{"trusted proxy, single entry", []string{"0.0.0.0/32"}, "203.0.113.5", "203.0.113.5"},
		{"trusted proxy appends to a spoofed header", []string{"0.0.0.0/32"}, "192.0.2.1, 203.0.113.5", "203.0.113.5"},
		{"two trusted hops", []string{"0.0.0.0/32", "100.64.0.0/10"}, "192.0.2.1, 203.0.113.5, 100.100.1.1", "203.0.113.5"},
		{"junk entries are skipped", []string{"0.0.0.0/32"}, "203.0.113.5, not-an-ip", "203.0.113.5"},
		{"untrusted peer: header ignored", []string{"10.0.0.0/8"}, "203.0.113.5", "0.0.0.0"},
		{"trusted proxy without header", []string{"0.0.0.0/32"}, "", "0.0.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harukiConfig.Cfg.Backend.EnableTrustProxy = true
			harukiConfig.Cfg.Backend.TrustProxies = tc.trusted
			harukiConfig.Cfg.Backend.ProxyHeader = fiber.HeaderXForwardedFor
			app := fiber.New(fiberConfig())
			app.Get("/ip", func(c fiber.Ctx) error { return c.SendString(c.IP()) })
			req := httptest.NewRequest(http.MethodGet, "/ip", nil)
			if tc.xff != "" {
				req.Header.Set(fiber.HeaderXForwardedFor, tc.xff)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != tc.want {
				t.Fatalf("c.IP() = %q, want %q", body, tc.want)
			}
		})
	}
}

func TestBuildAuthIPBan(t *testing.T) {
	original := harukiConfig.Cfg
	t.Cleanup(func() { harukiConfig.Cfg = original })
	logger := harukiLogger.NewLogger("Test", "ERROR", io.Discard)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	off, on := false, true
	harukiConfig.Cfg.Security.AuthIPBan = harukiConfig.AuthIPBanConfig{Enabled: &off}
	if g, err := buildAuthIPBan(ctx, logger, fiber.New(), rdb, nil); g != nil || err != nil {
		t.Fatalf("disabled: %v %v", g, err)
	}
	// Unset enabled: the ban ships dark.
	harukiConfig.Cfg.Security.AuthIPBan = harukiConfig.AuthIPBanConfig{}
	if g, err := buildAuthIPBan(ctx, logger, fiber.New(), rdb, nil); g != nil || err != nil {
		t.Fatalf("default config enabled the ban: %v %v", g, err)
	}
	harukiConfig.Cfg.Security.AuthIPBan = harukiConfig.AuthIPBanConfig{Enabled: &on}
	if g, err := buildAuthIPBan(ctx, logger, fiber.New(), nil, nil); g != nil || err != nil {
		t.Fatalf("without redis: %v %v", g, err)
	}
	harukiConfig.Cfg.Security.AuthIPBan = harukiConfig.AuthIPBanConfig{Enabled: &on, NeverBanCIDRs: []string{"bogus"}}
	if _, err := buildAuthIPBan(ctx, logger, fiber.New(), rdb, nil); err == nil {
		t.Fatal("invalid never_ban_cidrs accepted")
	}

	// Enabled with the other defaults: bot routes left alone.
	harukiConfig.Cfg.Security.AuthIPBan = harukiConfig.AuthIPBanConfig{Enabled: &on}
	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor, EnableIPValidation: true, TrustProxy: true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0/32"}}})
	guard := initAuthIPBan(ctx, logger, app, rdb, nil)
	if guard == nil {
		t.Fatal("enabled: true did not enable the ban")
	}
	cfg := guard.Config()
	if cfg.Threshold != 10 || !cfg.CountBuildRejected || !cfg.ExemptKnownBots || cfg.BlockBotRoutes {
		t.Fatalf("effective defaults = %+v", cfg)
	}
	ban(t, guard)
	if got := commandStatus(t, app); got != fiber.StatusOK {
		t.Fatalf("bot route from a banned address = %d, want 200 by default", got)
	}

	// block_bot_routes guards the bot routes registered after it.
	harukiConfig.Cfg.Security.AuthIPBan = harukiConfig.AuthIPBanConfig{Enabled: &on, BlockBotRoutes: true}
	blocking := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor, EnableIPValidation: true, TrustProxy: true,
		TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0/32"}}})
	if g, err := buildAuthIPBan(ctx, logger, blocking, rdb, nil); err != nil || !g.BlocksBotRoutes() {
		t.Fatalf("block_bot_routes: %v %v", g, err)
	}
	if got := commandStatus(t, blocking); got != fiber.StatusTooManyRequests {
		t.Fatalf("bot route with block_bot_routes = %d, want 429", got)
	}
}

const bannedTestIP = "203.0.113.60"

func ban(t *testing.T, guard *authban.Guard) {
	t.Helper()
	for range guard.Config().Threshold {
		guard.RecordFailure(context.Background(), bannedTestIP, "", authban.ReasonAuthFailed)
	}
	if !guard.Check(context.Background(), bannedTestIP, "").Banned {
		t.Fatal("setup: address not banned")
	}
}

func commandStatus(t *testing.T, app *fiber.App) int {
	t.Helper()
	app.Post("/api/v2/bot/:botId/pjsk/*", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bot/123/pjsk/profile", strings.NewReader("x"))
	req.Header.Set(fiber.HeaderXForwardedFor, bannedTestIP)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}
