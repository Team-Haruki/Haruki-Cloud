package pjsk

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"haruki-cloud/utils/drawing"

	"github.com/gofiber/fiber/v3"
)

const testBotID = "11451419"

// testBotApp registers bot routes on a fresh Fiber instance.
func testBotApp(t *testing.T, drawingURL string) *fiber.App {
	t.Helper()
	var client *drawing.HarukiDrawingClient
	if drawingURL != "" {
		client = drawing.NewHarukiDrawingClient(drawingURL)
	}
	app := fiber.New()
	runtime := testRenderApp(t, client)
	resolver := testResolver(t)
	RegisterPJSKBotRoutes(app, resolver, runtime, nil)
	return app
}

// encodeOneBotPayload creates a Base64-encoded OneBot v11 JSON payload from command text.
func encodeOneBotPayload(command string) string {
	payload := map[string]interface{}{
		"post_type":    "message",
		"message_type": "private",
		"raw_message":  command,
		"message": []map[string]interface{}{
			{"type": "text", "data": map[string]string{"text": command}},
		},
	}
	b, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(b)
}

// botPJSKPath returns the full URL for a PJSK bot endpoint.
func botPJSKPath(path string) string {
	return "/api/v2/bot/" + testBotID + "/pjsk/" + path
}

// ── Endpoint tests ──────────────────────────────────────────────────────────

func TestBotEndpointGetReturnsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	params := url.Values{}
	params.Set("command", encodeOneBotPayload("/卡面 1001"))
	req, _ := http.NewRequest(http.MethodGet, botPJSKPath("card/detail")+"?"+params.Encode(), nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png, got %s", resp.Header.Get("Content-Type"))
	}
	if string(body) != "PNGDATA" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBotEndpointPostReturnsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGPOST"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	body := `{"im_platform":"qq","im_user_id":"12345","command":"` + encodeOneBotPayload("/卡面 1001") + `"}`
	req, _ := http.NewRequest(http.MethodPost, botPJSKPath("card/detail"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
	if string(respBody) != "PNGPOST" {
		t.Fatalf("unexpected body: %s", respBody)
	}
}

func TestBotEndpointPlainTextFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNG"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	// Plain text (not Base64) should fall back gracefully
	body := `{"command":"/卡面 1001"}`
	req, _ := http.NewRequest(http.MethodPost, botPJSKPath("card/detail"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
}

func TestBotEndpointOneBotMessageArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGSEG"))
	}))
	defer srv.Close()
	app := testBotApp(t, srv.URL)

	// OneBot payload with only message array (no raw_message)
	payload := map[string]interface{}{
		"message": []map[string]interface{}{
			{"type": "text", "data": map[string]string{"text": "/卡面 "}},
			{"type": "text", "data": map[string]string{"text": "1001"}},
		},
	}
	b, _ := json.Marshal(payload)
	encoded := base64.StdEncoding.EncodeToString(b)

	body := `{"command":"` + encoded + `"}`
	req, _ := http.NewRequest(http.MethodPost, botPJSKPath("card/detail"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}
}

func TestBotEndpointWrongCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	// /卡面 resolves to card-detail, but we send it to card/list
	body := `{"command":"` + encodeOneBotPayload("/卡面 1001") + `"}`
	req, _ := http.NewRequest(http.MethodPost, botPJSKPath("card/list"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, respBody)
	}

	var envelope renderEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, respBody)
	}
	if envelope.Message != "command does not match this endpoint" {
		t.Fatalf("unexpected message: %s", envelope.Message)
	}
}

func TestBotEndpointEmptyCommandRejects400(t *testing.T) {
	app := testBotApp(t, "")

	body := `{"command":""}`
	req, _ := http.NewRequest(http.MethodPost, botPJSKPath("card/detail"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBotManifestEndpoint(t *testing.T) {
	app := testBotApp(t, "")

	req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, respBody)
	}

	// Placeholder response — just verify it returns valid JSON with "message"
	var envelope renderEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		t.Fatalf("decode manifest: %v raw=%s", err, respBody)
	}
	if !strings.Contains(envelope.Message, "placeholder") {
		t.Fatalf("expected placeholder message, got: %s", envelope.Message)
	}
}

func TestBotNilResolverSkipsRegistration(t *testing.T) {
	app := fiber.New()
	RegisterPJSKBotRoutes(app, nil, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/v2/bot/"+testBotID+"/command/manifests", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 (no routes), got %d", resp.StatusCode)
	}
}

// ── decodeCommand unit tests ────────────────────────────────────────────────

func TestDecodeCommandOneBotRawMessage(t *testing.T) {
	payload := `{"raw_message":"/卡面 1001","message_type":"private"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	result, err := decodeCommand(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result != "/卡面 1001" {
		t.Fatalf("expected '/卡面 1001', got '%s'", result)
	}
}

func TestDecodeCommandOneBotMessageSegments(t *testing.T) {
	payload := `{"message":[{"type":"text","data":{"text":"/查卡 "}},{"type":"text","data":{"text":"初音"}}]}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	result, err := decodeCommand(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result != "/查卡 初音" {
		t.Fatalf("expected '/查卡 初音', got '%s'", result)
	}
}

func TestDecodeCommandPlainTextFallback(t *testing.T) {
	result, err := decodeCommand("/卡面 1001")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result != "/卡面 1001" {
		t.Fatalf("expected '/卡面 1001', got '%s'", result)
	}
}
