package groupguard

import (
	"encoding/json"
	sonic "github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/config"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"github.com/gofiber/fiber/v3"
)

type testEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func TestCheckBindingReturnsBoundResult(t *testing.T) {
	app := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/private/game-binding" {
			t.Fatalf("unexpected path: %s", got)
		}
		if got := r.URL.Query().Get("platform"); got != "qq" {
			t.Fatalf("unexpected platform: %q", got)
		}
		if got := r.URL.Query().Get("platform_user_id"); got != "123" {
			t.Fatalf("unexpected platform_user_id: %q", got)
		}
		_ = sonic.ConfigDefault.NewEncoder(w).Encode([]map[string]string{
			{"server": "jp", "gameUserId": "111"},
		})
	}))

	resp := sendGroupGuardRequest(t, app, "/api/internal/group-guard/binding/check", `{"platform":"qq","platform_user_id":"123"}`)
	if resp.Status != fiber.StatusOK || resp.Message != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	var data BindingCheckResult
	if err := sonic.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if !data.Bound || data.Banned || len(data.Bindings) != 1 || data.Bindings[0].Server != "jp" || data.Bindings[0].GameUserID != "111" {
		t.Fatalf("unexpected data: %+v", data)
	}
}

func TestCheckBindingTreatsInvalidPlatformAsUnbound(t *testing.T) {
	app := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden: invalid platform or platform_user_id for this user"}`))
	}))

	resp := sendGroupGuardRequest(t, app, "/api/internal/group-guard/binding/check", `{"platform":"qq","platform_user_id":"123"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected response: %+v", resp)
	}
	var data BindingCheckResult
	if err := sonic.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.Bound || data.Banned || len(data.Bindings) != 0 {
		t.Fatalf("unexpected data: %+v", data)
	}
}

func TestCheckBindingTreatsBannedAsNonBound(t *testing.T) {
	app := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden: account owner is banned"}`))
	}))

	resp := sendGroupGuardRequest(t, app, "/api/internal/group-guard/binding/check", `{"platform":"qq","platform_user_id":"123"}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected response: %+v", resp)
	}
	var data BindingCheckResult
	if err := sonic.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.Bound || !data.Banned || len(data.Bindings) != 0 {
		t.Fatalf("unexpected data: %+v", data)
	}
}

func TestCheckBindingBatchReturnsResults(t *testing.T) {
	app := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		platformUserID := r.URL.Query().Get("platform_user_id")
		switch platformUserID {
		case "123":
			_ = sonic.ConfigDefault.NewEncoder(w).Encode([]map[string]string{
				{"server": "jp", "gameUserId": "111"},
			})
		case "456":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"forbidden: invalid platform or platform_user_id for this user"}`))
		default:
			t.Fatalf("unexpected platform_user_id: %q", platformUserID)
		}
	}))

	resp := sendGroupGuardRequest(t, app, "/api/internal/group-guard/binding/check-batch", `{"platform":"qq","platform_user_ids":["123","456","123"]}`)
	if resp.Status != fiber.StatusOK {
		t.Fatalf("unexpected response: %+v", resp)
	}
	var data BindingCheckBatchResponse
	if err := sonic.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data.Results) != 2 {
		t.Fatalf("unexpected result count: %+v", data.Results)
	}
	if item := data.Results["123"]; !item.Bound || len(item.Bindings) != 1 {
		t.Fatalf("unexpected bound result: %+v", item)
	}
	if item := data.Results["456"]; item.Bound || item.Banned || len(item.Bindings) != 0 {
		t.Fatalf("unexpected unbound result: %+v", item)
	}
}

func TestCheckBindingReturnsServiceUnavailableWhenToolboxMissing(t *testing.T) {
	prev := config.Cfg
	config.Cfg = config.Config{}
	config.Cfg.Backend.AllowInsecureInternalAPI = true
	t.Cleanup(func() { config.Cfg = prev })

	app := fiber.New()
	RegisterGroupGuardRoutes(app, nil)

	resp := sendGroupGuardRequest(t, app, "/api/internal/group-guard/binding/check", `{"platform":"qq","platform_user_id":"123"}`)
	if resp.Status != fiber.StatusServiceUnavailable {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Message != "绑定查询服务未就绪，请稍后再试" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
}

func TestCheckBindingDoesNotExposeToolboxErrorDetails(t *testing.T) {
	app := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"upstream failed: https://production-game-api.sekai.colorfulpalette.org/api/jp/user/123/profile?token=secret"}`))
	}))

	resp := sendGroupGuardRequest(t, app, "/api/internal/group-guard/binding/check", `{"platform":"qq","platform_user_id":"123"}`)
	if resp.Status != fiber.StatusBadGateway {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Message != "查询绑定状态失败，请稍后再试" {
		t.Fatalf("unexpected message: %s", resp.Message)
	}
	if strings.Contains(resp.Message, "http://") || strings.Contains(resp.Message, "https://") || strings.Contains(resp.Message, "token") {
		t.Fatalf("message leaked upstream detail: %s", resp.Message)
	}
}

func newTestApp(t *testing.T, handler http.Handler) *fiber.App {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	prev := config.Cfg
	config.Cfg = config.Config{}
	config.Cfg.Backend.AllowInsecureInternalAPI = true
	t.Cleanup(func() { config.Cfg = prev })

	app := fiber.New()
	RegisterGroupGuardRoutes(app, sekaiapi.NewToolboxClient(&config.ToolboxConfig{
		BaseURL: server.URL,
	}))
	return app
}

func sendGroupGuardRequest(t *testing.T, app *fiber.App, path string, body string) testEnvelope {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	var envelope testEnvelope
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope
}
