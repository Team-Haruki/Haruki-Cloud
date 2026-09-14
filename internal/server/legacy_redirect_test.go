package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/urlhost"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
)

func legacyRouteTestApp(t *testing.T, cfg harukiConfig.ImageCacheConfig, hosts *urlhost.Set) (*fiber.App, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	app := fiber.New()
	registerImageCacheRoute(app, cfg, hosts, harukiLogger.NewLogger("LegacyRoute", "DEBUG", &logs))
	return app, &logs
}

func legacyRouteGet(t *testing.T, app *fiber.App, path string) *http.Response {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestLegacyImageCacheRouteServesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pjsk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pjsk", "abc.png"), []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	hosts := urlhost.Single("https://ic.example")
	// Default: legacy_redirect off serves the directory even with hosts configured.
	app, logs := legacyRouteTestApp(t, harukiConfig.ImageCacheConfig{Dir: dir}, hosts)
	resp := legacyRouteGet(t, app, "/ic/pjsk/abc.png")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "png-bytes" {
		t.Fatalf("static route = %d %q", resp.StatusCode, body)
	}
	if !strings.Contains(logs.String(), "image cache static serving enabled") {
		t.Fatalf("logs:\n%s", logs.String())
	}

	// Redirect requested without any host keeps serving the directory, with a warning.
	cfg := harukiConfig.ImageCacheConfig{Dir: dir}
	cfg.LegacyRedirect.Enabled = true
	app, logs = legacyRouteTestApp(t, cfg, nil)
	if resp := legacyRouteGet(t, app, "/ic/pjsk/abc.png"); resp.StatusCode != http.StatusOK {
		t.Fatalf("fallback static route = %d", resp.StatusCode)
	}
	if !strings.Contains(logs.String(), "legacy redirect needs image_cache.hosts") {
		t.Fatalf("logs:\n%s", logs.String())
	}

	// Neither: no route at all.
	app, _ = legacyRouteTestApp(t, harukiConfig.ImageCacheConfig{}, hosts)
	if resp := legacyRouteGet(t, app, "/ic/pjsk/abc.png"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unconfigured route = %d", resp.StatusCode)
	}
}

func TestLegacyImageCacheRouteRedirects(t *testing.T) {
	cfg := harukiConfig.ImageCacheConfig{Dir: t.TempDir()}
	cfg.LegacyRedirect.Enabled = true
	hosts, err := urlhost.New(map[string]string{"cn09": "https://ic-cn09.example/base"}, urlhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	app, logs := legacyRouteTestApp(t, cfg, hosts)
	if !strings.Contains(logs.String(), "image cache legacy route redirects") {
		t.Fatalf("logs:\n%s", logs.String())
	}

	resp := legacyRouteGet(t, app, "/ic/pjsk/abc%20def.png")
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "https://ic-cn09.example/base/pjsk/abc%20def.png" {
		t.Fatalf("Location = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != legacyRedirectCacheControl {
		t.Fatalf("Cache-Control = %q", got)
	}

	for _, bad := range []string{"/ic/", "/ic/pjsk/..%2F..%2Fetc%2Fpasswd", "/ic/a%00b"} {
		if resp := legacyRouteGet(t, app, bad); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", bad, resp.StatusCode)
		}
	}
}

func TestLegacyImageCacheRedirectRejectsBadEscape(t *testing.T) {
	app := fiber.New()
	app.Get(legacyImageCacheRoute, legacyImageCacheRedirect(urlhost.Single("https://ic.example")))
	req := httptest.NewRequest(http.MethodGet, "/ic/placeholder", nil)
	req.URL.Opaque = "/ic/bad%zzescape"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET bad escape: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestLegacyImageCacheRedirectWithoutHostIs404(t *testing.T) {
	app := fiber.New()
	app.Get(legacyImageCacheRoute, legacyImageCacheRedirect(nil))
	if resp := legacyRouteGet(t, app, "/ic/pjsk/abc.png"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRenderImageHosts(t *testing.T) {
	if renderImageHosts(nil) != nil {
		t.Fatal("nil runtime must yield nil hosts")
	}
	hosts := urlhost.Single("https://ic.example")
	if renderImageHosts(&renderapp.App{ImageHosts: hosts}) != hosts {
		t.Fatal("runtime hosts not returned")
	}
}
