package handler

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestCommandHelpPlainHelpShortCircuitsValidation(t *testing.T) {
	server, calls := newCommandHelpDrawingServer(t, "music")
	defer server.Close()

	h := sekaiHandlers{}.SongHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	resolved, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查曲",
		ArgText:    "help",
	})
	if err != nil {
		t.Fatalf("Handle() help error = %v", err)
	}
	if resolved == nil || !resolved.IsHelp {
		t.Fatalf("expected help command request, got %+v", resolved)
	}
	if resolved.CommandPath != "music" {
		t.Fatalf("unexpected command path: %q", resolved.CommandPath)
	}

	app := &renderapp.App{Drawing: drawing.NewHarukiDrawingClient(server.URL)}
	message, err := ExecuteCommandRequest(context.Background(), resolved, app)
	if err != nil {
		t.Fatalf("ExecuteCommandRequest() help error = %v", err)
	}
	if len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("expected single image help message, got %+v", message)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("drawing API calls = %d, want 1", got)
	}
}

func TestCommandHelpMarkdownAvailableForRegisteredRoutes(t *testing.T) {
	EnsureCommandHandlersRegistered()
	routes := corehandler.ListBotRoutes()
	if len(routes) == 0 {
		t.Fatal("expected registered bot routes")
	}

	for _, route := range routes {
		t.Run(strings.ReplaceAll(route.Path, "/", "_"), func(t *testing.T) {
			md, err := commandHelpMarkdown(&CommandRequest{CommandPath: route.Path})
			if err != nil {
				t.Fatalf("commandHelpMarkdown(%q) error = %v", route.Path, err)
			}
			if strings.TrimSpace(md) == "" {
				t.Fatalf("commandHelpMarkdown(%q) returned empty markdown", route.Path)
			}
		})
	}
}

func TestCommandHelpLookupKeysPreferExactThenFamily(t *testing.T) {
	keys := commandHelpLookupKeys("music/bpm")
	want := []string{"music_bpm", "music"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("commandHelpLookupKeys() = %v, want %v", keys, want)
	}
}

func TestCommandHelpMarkdownUsesModuleFileFallback(t *testing.T) {
	md, err := commandHelpMarkdown(&CommandRequest{CommandPath: "music/bpm"})
	if err != nil {
		t.Fatalf("commandHelpMarkdown() error = %v", err)
	}
	if !strings.Contains(md, "# 音乐与乐曲") {
		t.Fatalf("expected music module markdown, got %q", md)
	}
}

func newCommandHelpDrawingServer(t *testing.T, wantPath string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/pjsk/help/render" {
			t.Errorf("unexpected drawing endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var req drawing.CommandHelpRenderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode drawing request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Path != wantPath {
			t.Errorf("request path = %q, want %q", req.Path, wantPath)
		}
		if strings.TrimSpace(req.Markdown) == "" {
			t.Errorf("expected non-empty markdown")
		}
		if strings.TrimSpace(req.Title) == "" {
			t.Errorf("expected non-empty title")
		}
		w.Header().Set("Content-Type", "image/png")
		if err := png.Encode(w, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
			t.Errorf("encode png: %v", err)
		}
	}))
	return server, &calls
}
