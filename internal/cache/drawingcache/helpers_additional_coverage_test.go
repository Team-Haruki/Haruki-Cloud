package drawingcache

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestCachePathAndConfigHelperBranches(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"", "png"}, {" .JPG ", "jpg"}, {"webp", "webp"}, {"bad/ext", "png"}, {"..", "png"},
	} {
		if got := normalizeFileExt(tc.input); got != tc.want {
			t.Fatalf("normalizeFileExt(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	if got := sanitizeStoragePath("/API/PJSK/Card List/"); got != filepath.Join("api", "pjsk", "card-list") {
		t.Fatalf("unexpected storage path: %q", got)
	}
	if got := sanitizeStoragePath(" "); got != "" {
		t.Fatalf("blank storage path = %q", got)
	}

	root := t.TempDir()
	child := filepath.Join(root, "child")
	if !isPathUnderBase(root, child) || isPathUnderBase(root, root) || isPathUnderBase(root, filepath.Dir(root)) {
		t.Fatal("path containment helpers returned unexpected result")
	}
	if exists, err := fileExists(child); err != nil || exists {
		t.Fatalf("missing file = %v,%v", exists, err)
	}
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if exists, err := fileExists(child); err != nil || !exists {
		t.Fatalf("existing directory = %v,%v", exists, err)
	}
}

func TestCacheErrorAndConfigHelperBranches(t *testing.T) {
	if drawingCacheErrorType(nil) != "" {
		t.Fatal("nil cache error type should be blank")
	}
	if got := drawingCacheErrorType(errors.New("boom")); got != "*errors.errorString" {
		t.Fatalf("unexpected cache error type: %q", got)
	}
	if shared, err := fileReferencedByLiveRecord(nil, "path", "key"); err != nil || shared {
		t.Fatalf("nil db shared reference = %v,%v", shared, err)
	}

	cfg := normalizeConfig(Config{StorageDir: " /tmp/cache ", GCInterval: -time.Second})
	if cfg.StorageDir != "/tmp/cache" || cfg.DBPath != "/tmp/cache/cache.db" || cfg.GCInterval != 0 {
		t.Fatalf("unexpected normalized config: %+v", cfg)
	}
	cfg = normalizeConfig(Config{StorageDir: "/tmp/cache", DBPath: " custom.db ", GCInterval: time.Second})
	if cfg.DBPath != "custom.db" || cfg.GCInterval != time.Second {
		t.Fatalf("explicit config changed: %+v", cfg)
	}
	if cfg := normalizeConfig(Config{}); cfg.GCInterval != DefaultGCInterval || cfg.DBPath != "" {
		t.Fatalf("default config = %+v", cfg)
	}
}

func TestCacheServiceAndRouteGuardBranches(t *testing.T) {
	var service *Service
	if err := service.Close(); err != nil || service.Config() != (Config{}) {
		t.Fatalf("nil service helpers = %+v,%v", service.Config(), err)
	}
	service.RegisterRoutes(fiber.New())
	(&Service{}).RegisterRoutes(fiber.New())
	if _, err := NewService(context.Background(), Config{}); err == nil {
		t.Fatal("empty storage service unexpectedly succeeded")
	}

	api := NewAPI(nil, " ")
	if api.storageDir != "./cache_images" || api.now == nil || api.stats == nil {
		t.Fatalf("unexpected default API: %+v", api)
	}
	app := fiber.New()
	api.RegisterRoutes(app)
	for _, path := range []string{"/cache", "/cache/stats"} {
		req, err := http.NewRequest(http.MethodDelete, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusMethodNotAllowed {
			t.Fatalf("DELETE %s status = %d", path, resp.StatusCode)
		}
	}

	StartGCWorker(context.Background(), nil, "", time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	StartGCWorker(ctx, nil, "", 0)
}
