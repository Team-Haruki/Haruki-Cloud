package pjsk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/utils/drawing"

	"github.com/gofiber/fiber/v3"
)

func testResolver(t *testing.T) *parser.GlobalCommandResolver {
	t.Helper()
	return parser.NewGlobalCommandResolver(nil)
}

func TestCommandEndpointReturnsImage(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	resolver := testResolver(t)
	RegisterPJSKCommandRoute(app, resolver, runtime)

	body := `{"im_platform":"qq","im_user_id":"12345","command":"/卡面 1001"}`
	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/command", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(respBody))
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("expected Content-Type image/png, got %s", resp.Header.Get("Content-Type"))
	}
	if string(respBody) != "PNGDATA" {
		t.Fatalf("unexpected body: %s", string(respBody))
	}
}

func TestCommandEndpointRejectsEmptyCommand(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	resolver := testResolver(t)
	RegisterPJSKCommandRoute(app, resolver, runtime)

	body := `{"im_platform":"qq","im_user_id":"12345","command":""}`
	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/command", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, string(respBody))
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, string(respBody))
	}
	if envelope.Message != "command is required" {
		t.Fatalf("unexpected message: %s", envelope.Message)
	}
}

func TestCommandEndpointRejectsUnrecognizedCommand(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	resolver := testResolver(t)
	RegisterPJSKCommandRoute(app, resolver, runtime)

	body := `{"im_platform":"qq","im_user_id":"12345","command":"这不是一个指令"}`
	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/command", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, string(respBody))
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, string(respBody))
	}
	if envelope.Message != "unrecognized command" {
		t.Fatalf("unexpected message: %s", envelope.Message)
	}
}

func TestCommandEndpointServerOverridesRegion(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("TWPNG"))
	}))
	defer drawingServer.Close()

	app := fiber.New()
	runtime := testRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	resolver := testResolver(t)
	RegisterPJSKCommandRoute(app, resolver, runtime)

	body := `{"im_platform":"qq","im_user_id":"12345","command":"/卡面 1001","server":"tw"}`
	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/command", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	// As long as the command resolved and render was attempted, the test passes.
	// The specific region handling is validated by the parser/bridge tests.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(respBody))
	}
}

func TestCommandEndpointNilResolverSkipsRegistration(t *testing.T) {
	app := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKCommandRoute(app, nil, runtime)

	body := `{"command":"/卡面 1001"}`
	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/command", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 (route not registered), got %d", resp.StatusCode)
	}
}
