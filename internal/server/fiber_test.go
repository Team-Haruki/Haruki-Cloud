package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/middleware/secure"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

func TestCreateFiberAppUsesBoundedGlobalRequestBodyLimit(t *testing.T) {
	app := createFiberApp(harukiLogger.NewLogger("Test", "ERROR", io.Discard))
	if got := app.Config().BodyLimit; got != globalRequestBodyLimit {
		t.Fatalf("BodyLimit = %d, want %d", got, globalRequestBodyLimit)
	}
}

func TestRequestBodyLimitForPath(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{path: "/api/v2/public/chunithm/music/query-batch", want: defaultRequestBodyLimit},
		{path: "/api/v2/bot/66666666/auth", want: cacheControlRequestBodyLimit},
		{path: "/api/v2/bot/66666666/pjsk/card/box", want: botCommandRequestBodyLimit},
		{path: "/api/v2/bot/66666666/pjsk/profile/settings", want: botCommandRequestBodyLimit},
		{path: "/internal/subscription-events/mysekai-birthday", want: birthdayEventRequestBodyLimit},
		{path: "/internal/subscription-events/mysekai-birthday/", want: birthdayEventRequestBodyLimit},
		{path: "/cache", want: cacheControlRequestBodyLimit},
		{path: "/cache/", want: cacheControlRequestBodyLimit},
		{path: "/cache/stats", want: cacheControlRequestBodyLimit},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			if got := requestBodyLimitForPath(test.path); got != test.want {
				t.Fatalf("limit = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRequestBodyLimitMiddlewareBoundary(t *testing.T) {
	app := fiber.New(fiber.Config{BodyLimit: globalRequestBodyLimit})
	app.Use(requestBodyLimitMiddleware())
	handled := 0
	app.Post("/api/v2/public/test", func(c fiber.Ctx) error {
		handled++
		return c.SendStatus(fiber.StatusNoContent)
	})

	request := func(size int) *http.Response {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/public/test", bytes.NewReader(make([]byte, size)))
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second, FailOnTimeout: true})
		if err != nil {
			t.Fatalf("request size %d failed: %v", size, err)
		}
		return resp
	}

	allowed := request(defaultRequestBodyLimit)
	_ = allowed.Body.Close()
	if allowed.StatusCode != fiber.StatusNoContent || handled != 1 {
		t.Fatalf("boundary request status=%d handled=%d", allowed.StatusCode, handled)
	}

	rejected := request(defaultRequestBodyLimit + 1)
	_ = rejected.Body.Close()
	if rejected.StatusCode != fiber.StatusRequestEntityTooLarge || handled != 1 {
		t.Fatalf("oversized request status=%d handled=%d", rejected.StatusCode, handled)
	}
}

func TestRequestBodyLimitMiddlewareRejectsCompressedLimitBypass(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(bytes.Repeat([]byte("a"), defaultRequestBodyLimit+1)); err != nil {
		t.Fatalf("compress request: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close compressor: %v", err)
	}
	if compressed.Len() >= defaultRequestBodyLimit {
		t.Fatalf("compressed fixture is not below route limit: %d", compressed.Len())
	}

	app := fiber.New(fiber.Config{BodyLimit: globalRequestBodyLimit})
	app.Use(requestBodyLimitMiddleware())
	handled := false
	app.Post("/api/v2/public/test", func(c fiber.Ctx) error {
		handled = true
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/public/test", bytes.NewReader(compressed.Bytes()))
	req.Header.Set(fiber.HeaderContentEncoding, "gzip")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("compressed request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnsupportedMediaType)
	}
	if handled {
		t.Fatal("compressed request reached the route handler")
	}
}

func TestRequestBodyLimitAllowsNormalNoiseBotRequest(t *testing.T) {
	serverKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	initiator, err := crypto.NewInitiator(serverKey.Public)
	if err != nil {
		t.Fatalf("create initiator: %v", err)
	}
	plaintext := []byte(`{"matched_command":"/box","message":[{"type":"text","data":{"text":"/box"}}]}`)
	ciphertext, err := initiator.EncryptPacket(plaintext)
	if err != nil {
		t.Fatalf("encrypt request: %v", err)
	}

	app := fiber.New(fiber.Config{BodyLimit: globalRequestBodyLimit})
	app.Use(requestBodyLimitMiddleware())
	var received []byte
	app.Post("/api/v2/bot/:botId/pjsk/card/box", secure.New(secure.Config{ServerPrivateKey: serverKey}), func(c fiber.Ctx) error {
		received = append([]byte(nil), c.Body()...)
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/bot/66666666/pjsk/card/box", bytes.NewReader(ciphertext))
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second, FailOnTimeout: true})
	if err != nil {
		t.Fatalf("noise request failed: %v", err)
	}
	defer resp.Body.Close()
	if !bytes.Equal(received, plaintext) {
		t.Fatalf("decrypted body = %q, want %q", received, plaintext)
	}
	encryptedResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	decryptedResponse, err := initiator.DecryptPacket(encryptedResponse)
	if err != nil {
		t.Fatalf("decrypt response: %v", err)
	}
	if string(decryptedResponse) != "ok" {
		t.Fatalf("response = %q, want ok", decryptedResponse)
	}
}

func TestAccessLogMiddlewareUsesCanonicalFields(t *testing.T) {
	var output bytes.Buffer
	app := fiber.New()
	app.Use(requestid.New())
	app.Use(accessLogMiddleware(harukiLogger.NewLogger("HTTP", "INFO", &output)))
	app.Post("/items/:id", func(c fiber.Ctx) error {
		c.Request().SetBody([]byte("decoded-request"))
		return c.SendString("ok")
	})

	response, err := app.Test(httptest.NewRequest("POST", "/items/42", strings.NewReader("wire")))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}

	line := output.String()
	for _, field := range []string{
		"component=HTTP",
		"event=http_request",
		"http_method=POST",
		"http_route=/items/:id",
		"status_code=200",
		"server_duration_ms=",
		"duration_scope=fiber_handler",
		"request_id=",
		"request_bytes=4",
		"response_bytes=2",
	} {
		if !strings.Contains(line, field) {
			t.Errorf("access line missing %q: %s", field, line)
		}
	}
}

func TestAccessLogMiddlewareRecordsRecoveredPanic(t *testing.T) {
	var output bytes.Buffer
	app := fiber.New()
	app.Use(requestid.New())
	app.Use(accessLogMiddleware(harukiLogger.NewLogger("HTTP", "INFO", &output)))
	app.Use(recover.New())
	app.Get("/panic", func(c fiber.Ctx) error {
		panic("boom")
	})

	_, _ = app.Test(httptest.NewRequest("GET", "/panic", nil))
	line := output.String()
	if strings.Count(line, "event=http_request") != 1 {
		t.Fatalf("expected one access line: %q", line)
	}
	if !strings.Contains(line, "level=ERROR") || !strings.Contains(line, "status_code=500") {
		t.Fatalf("panic access metadata missing: %q", line)
	}
}

func TestAccessLogMiddlewareClassifiesReturned4xxAsWarning(t *testing.T) {
	var output bytes.Buffer
	app := fiber.New()
	app.Use(requestid.New())
	app.Use(accessLogMiddleware(harukiLogger.NewLogger("HTTP", "INFO", &output)))
	app.Get("/bad-request", func(c fiber.Ctx) error {
		return fiber.ErrBadRequest
	})

	response, err := app.Test(httptest.NewRequest("GET", "/bad-request", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
	line := output.String()
	for _, field := range []string{"level=WARN", "status_code=400", "error_type=*fiber.Error"} {
		if !strings.Contains(line, field) {
			t.Fatalf("4xx access log missing %q: %q", field, line)
		}
	}
}
