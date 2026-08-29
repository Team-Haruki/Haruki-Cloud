package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/config"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestExecuteCheckDataSuccessfulSuiteAndMySekai(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "check-data-user", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	body := "1700000000"
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "upload_time" || r.URL.Query().Get("platform") != "qq" || r.URL.Query().Get("platform_user_id") != "check-data-user" {
			t.Errorf("unexpected toolbox request: %s", r.URL.String())
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	app := &renderapp.App{
		Bindings: service,
		Toolbox:  sekaiapi.NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL, APIToken: "test"}),
	}

	for _, mode := range []string{"suite", "mysekai"} {
		params := executionCoverageParams(t, userQueryParams{Mode: "self", Platform: "qq", PlatformUserID: "check-data-user"})
		cmd := &CommandRequest{
			Module: parser.ModuleCheckData, Mode: mode, Region: "jp", Params: params,
			RequesterPlatform: "qq", RequesterUserID: "check-data-user",
		}
		message, err := executeCheckData(NewRequestContext(ctx, cmd, app))
		if err != nil {
			t.Fatalf("executeCheckData(%s): %v", mode, err)
		}
		if len(message) != 1 || message[0].Type != onebot11.TypeText {
			t.Fatalf("executeCheckData(%s) message = %#v", mode, message)
		}
		text := message[0].Data.(onebot11.TextData).Text
		if !strings.Contains(text, "数据更新时间") || !strings.Contains(text, "2023") {
			t.Fatalf("executeCheckData(%s) text = %q", mode, text)
		}
	}

	body = "not-a-timestamp"
	cmd := &CommandRequest{Mode: "suite", Region: "jp", Params: executionCoverageParams(t, userQueryParams{
		Mode: "self", Platform: "qq", PlatformUserID: "check-data-user",
	})}
	if _, err := executeCheckData(NewRequestContext(ctx, cmd, app)); err == nil || !strings.Contains(err.Error(), "解析更新时间失败") {
		t.Fatalf("invalid upload timestamp error = %v", err)
	}

	status = http.StatusInternalServerError
	body = "failure"
	if _, err := executeCheckData(NewRequestContext(ctx, cmd, app)); err == nil {
		t.Fatal("toolbox failure unexpectedly succeeded")
	}
}

func TestProfileSettingsHandlerSuccessBranches(t *testing.T) {
	ctx := additionalProfileContext("", "")
	handlers := []HarukiSekaiCommandHandler{
		sekaiHandlers{}.ProfileHideSuiteHandle(),
		sekaiHandlers{}.ProfileShowSuiteHandle(),
		sekaiHandlers{}.ProfileHideMySekaiHandle(),
		sekaiHandlers{}.ProfileShowMySekaiHandle(),
		sekaiHandlers{}.ProfileHideIDHandle(),
		sekaiHandlers{}.ProfileShowIDHandle(),
		sekaiHandlers{}.ProfileCheckDataHandle(),
		sekaiHandlers{}.MsdHandle(),
		sekaiHandlers{}.ProfileVerifyHandle(),
		sekaiHandlers{}.ProfileVerifyListHandle(),
	}
	for _, h := range handlers {
		request, err := h.handleFunc(ctx)
		if err != nil || request == nil {
			t.Fatalf("handler %s = %#v, %v", h.Path, request, err)
		}
	}
}
