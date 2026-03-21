package pjsk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/utils/drawing"

	"github.com/gofiber/fiber/v3"
)

func TestPJSKRenderDispatchBuildRouteReturnsBuiltPayload(t *testing.T) {
	appServer := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(appServer, runtime)

	resp := requestRenderRoute(t, appServer, http.MethodPost, "/internal/pjsk/render", `{"target":"card/detail","operation":"build","payload":{"query":"1001","region":"jp"}}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected status=%d message=%s", resp.Status, resp.Message)
	}

	var data struct {
		Endpoint string `json:"endpoint"`
		Method   string `json:"method"`
		Payload  struct {
			CardInfo struct {
				CardID int `json:"card_id"`
			} `json:"card_info"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if data.Endpoint != cardDetailDrawingEndpoint {
		t.Fatalf("unexpected endpoint: %s", data.Endpoint)
	}
	if data.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", data.Method)
	}
	if data.Payload.CardInfo.CardID != 1001 {
		t.Fatalf("unexpected payload: %+v", data.Payload)
	}
}

func TestPJSKRenderDispatchRenderRouteReturnsDrawingBytes(t *testing.T) {
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != mysekaiResourceEndpoint {
			t.Fatalf("unexpected drawing path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("DISPATCHPNG"))
	}))
	defer drawingServer.Close()

	appServer := fiber.New()
	runtime := mysekaiRenderApp(t, drawing.NewHarukiDrawingClient(drawingServer.URL))
	RegisterPJSKRenderRoutes(appServer, runtime)

	req, err := http.NewRequest(http.MethodPost, "/internal/pjsk/render", strings.NewReader(`{"target":"mysekai/resource","operation":"render","payload":{"region":"jp"}}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := appServer.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected http status: %d body=%s", resp.StatusCode, string(body))
	}
	if string(body) != "DISPATCHPNG" {
		t.Fatalf("unexpected render body: %s", string(body))
	}
}

func TestPJSKRenderDispatchRejectsUnsupportedTarget(t *testing.T) {
	appServer := fiber.New()
	runtime := testRenderApp(t, nil)
	RegisterPJSKRenderRoutes(appServer, runtime)

	resp := requestRenderRoute(t, appServer, http.MethodPost, "/internal/pjsk/render", `{"target":"legacy/render","operation":"build","payload":{}}`)
	if resp.Status != fiber.StatusBadRequest {
		t.Fatalf("expected bad request, got status=%d message=%s", resp.Status, resp.Message)
	}
	if resp.Message != "unsupported render target: legacy/render" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
}
