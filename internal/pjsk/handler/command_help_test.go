package handler

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestCommandHelpFlagShortCircuitsValidation(t *testing.T) {
	for _, arg := range []string{"-help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			server, calls := newCommandHelpDrawingServer(t, "music")
			defer server.Close()

			h := sekaiHandlers{}.SongHandle()
			h.Regions = []renderregion.Value{renderregion.JP}

			resolved, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				TriggerCmd: "/查曲",
				ArgText:    arg,
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
		})
	}
}

func TestCommandHelpLegacyTokensAreNormalArgs(t *testing.T) {
	for _, arg := range []string{"help", "帮助", "--help"} {
		t.Run(arg, func(t *testing.T) {
			h := sekaiHandlers{}.SongHandle()
			h.Regions = []renderregion.Value{renderregion.JP}
			resolved, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				TriggerCmd: "/查曲",
				ArgText:    arg,
			})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.IsHelp || resolved.Module == parser.ModuleHelp {
				t.Fatalf("expected normal command, got help request: %+v", resolved)
			}
			if resolved.Query != arg {
				t.Fatalf("query = %q, want %q", resolved.Query, arg)
			}
		})
	}
}

func TestCommandHelpDoesNotCaptureSongTitleHelpToken(t *testing.T) {
	tests := []struct {
		name       string
		handler    HarukiSekaiCommandHandler
		trigger    string
		args       string
		wantMode   string
		wantQuery  string
		wantSkill  bool
		wantModule parser.TargetModule
	}{
		{
			name:       "song",
			handler:    sekaiHandlers{}.SongHandle(),
			trigger:    "/song",
			args:       "Help me, ERINNNNNN!!",
			wantMode:   "music-detail",
			wantQuery:  "Help me, ERINNNNNN!!",
			wantModule: parser.ModuleMusic,
		},
		{
			name:       "chart",
			handler:    sekaiHandlers{}.ChartHandle(),
			trigger:    "/谱面预览",
			args:       "Help me, ERINNNNNN!! expert",
			wantMode:   "music-chart",
			wantQuery:  "Help me, ERINNNNNN!! expert",
			wantModule: parser.ModuleMusic,
		},
		{
			name:       "bpm",
			handler:    sekaiHandlers{}.BPMHandle(),
			trigger:    "/查BPM",
			args:       "Help me, ERINNNNNN!! master",
			wantMode:   "music-bpm-detail",
			wantQuery:  "Help me, ERINNNNNN!!",
			wantModule: parser.ModuleMusic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.handler
			h.Regions = []renderregion.Value{renderregion.JP}
			resolved, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				TriggerCmd: tt.trigger,
				ArgText:    tt.args,
			})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.IsHelp || resolved.Module == parser.ModuleHelp {
				t.Fatalf("expected normal command, got help request: %+v", resolved)
			}
			if resolved.Module != tt.wantModule || resolved.Mode != tt.wantMode || resolved.Query != tt.wantQuery {
				t.Fatalf("unexpected request: %+v", resolved)
			}
		})
	}
}

func TestCommandHelpDeckGenericTriggerUsesDeckOverview(t *testing.T) {
	generic := &CommandRequest{CommandPath: "deck/event", TriggerCommand: "/组卡"}
	if got := commandHelpRequestPath(generic); got != "deck" {
		t.Fatalf("generic deck request path = %q, want deck", got)
	}
	md, err := commandHelpMarkdown(generic)
	if err != nil {
		t.Fatalf("commandHelpMarkdown(generic) error = %v", err)
	}
	if !strings.Contains(md, "# 组卡与队伍推荐") {
		t.Fatalf("expected deck overview markdown, got %q", md)
	}

	event := &CommandRequest{CommandPath: "deck/event", TriggerCommand: "/活动组卡"}
	if got := commandHelpRequestPath(event); got != "deck/event" {
		t.Fatalf("event deck request path = %q, want deck/event", got)
	}
	md, err = commandHelpMarkdown(event)
	if err != nil {
		t.Fatalf("commandHelpMarkdown(event) error = %v", err)
	}
	if !strings.Contains(md, "# 活动组卡") {
		t.Fatalf("expected event deck markdown, got %q", md)
	}
}

func TestUpdatedHelpExamplesParse(t *testing.T) {
	tests := []struct {
		name     string
		handler  HarukiSekaiCommandHandler
		trigger  string
		args     string
		module   parser.TargetModule
		mode     string
		query    string
		paramKey string
	}{
		{
			name:    "song help token title",
			handler: sekaiHandlers{}.SongHandle(),
			trigger: "/查歌",
			args:    "Help me, ERINNNNNN!!",
			module:  parser.ModuleMusic,
			mode:    "music-detail",
			query:   "Help me, ERINNNNNN!!",
		},
		{
			name:    "chart help token title",
			handler: sekaiHandlers{}.ChartHandle(),
			trigger: "/谱面预览",
			args:    "Help me, ERINNNNNN!! expert",
			module:  parser.ModuleMusic,
			mode:    "music-chart",
			query:   "Help me, ERINNNNNN!! expert",
		},
		{
			name:    "bpm song query",
			handler: sekaiHandlers{}.BPMHandle(),
			trigger: "/查BPM",
			args:    "Help me, ERINNNNNN!! master",
			module:  parser.ModuleMusic,
			mode:    "music-bpm-detail",
			query:   "Help me, ERINNNNNN!!",
		},
		{
			name:    "bpm numeric search",
			handler: sekaiHandlers{}.BPMSearchHandle(),
			trigger: "/bpms",
			args:    "200 expert",
			module:  parser.ModuleMusic,
			mode:    "music-bpm",
			query:   "200",
		},
		{
			name:    "character mission flower tree",
			handler: sekaiHandlers{}.CharacterMissionHandle(),
			trigger: "/cr任务",
			args:    "miku all 花树",
			module:  parser.ModuleEducation,
			mode:    "education-character-mission",
			query:   "miku all 花树",
		},
		{
			name:    "generic deck current event",
			handler: sekaiHandlers{}.EventDeckHandle(),
			trigger: "/组卡",
			args:    "",
			module:  parser.ModuleDeck,
			mode:    "deck-event",
			query:   "",
		},
		{
			name:    "event deck simulated bonus",
			handler: sekaiHandlers{}.EventDeckHandle(),
			trigger: "/活动组卡",
			args:    "mmj 蓝",
			module:  parser.ModuleDeck,
			mode:    "deck-event",
			query:   "mmj 蓝",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.handler
			h.Regions = []renderregion.Value{renderregion.JP}
			resolved, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				Platform:   "qq",
				UserId:     "42",
				TriggerCmd: tt.trigger,
				ArgText:    tt.args,
			})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.IsHelp {
				t.Fatalf("expected normal command, got help: %+v", resolved)
			}
			if resolved.Module != tt.module || resolved.Mode != tt.mode || resolved.Query != tt.query {
				t.Fatalf("unexpected parsed example: %+v", resolved)
			}
		})
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

func TestCommandHelpMarkdownHasExactDocsForRegisteredRoutes(t *testing.T) {
	EnsureCommandHandlersRegistered()
	routes := corehandler.ListBotRoutes()
	if len(routes) == 0 {
		t.Fatal("expected registered bot routes")
	}

	for _, route := range routes {
		t.Run(strings.ReplaceAll(route.Path, "/", "_"), func(t *testing.T) {
			key := commandHelpDocKey(route.Path)
			md, ok, err := readCommandHelpMarkdown(key)
			if err != nil {
				t.Fatalf("readCommandHelpMarkdown(%q) error = %v", key, err)
			}
			if !ok || strings.TrimSpace(md) == "" {
				t.Fatalf("missing exact command help markdown for %q (helpdocs/%s.md)", route.Path, key)
			}
		})
	}
}

func TestCommandHelpMarkdownMentionsRawCommandAliases(t *testing.T) {
	EnsureCommandHandlersRegistered()
	handlersVal := reflect.ValueOf(sekaiHandlers{})
	handlersTyp := handlersVal.Type()
	configTyp := reflect.TypeOf(HarukiSekaiCommandHandler{})
	for i := 0; i < handlersVal.NumMethod(); i++ {
		methodVal := handlersVal.Method(i)
		methodTyp := methodVal.Type()
		if methodTyp.NumIn() != 0 || methodTyp.NumOut() != 1 || methodTyp.Out(0) != configTyp {
			continue
		}

		methodName := handlersTyp.Method(i).Name
		handler := methodVal.Call(nil)[0].Interface().(HarukiSekaiCommandHandler)
		if handler.IsDisabled() || strings.TrimSpace(handler.Path) == "" {
			continue
		}
		md, err := commandHelpMarkdown(&CommandRequest{CommandPath: handler.Path})
		if err != nil {
			t.Fatalf("%s commandHelpMarkdown(%q) error = %v", methodName, handler.Path, err)
		}
		for _, command := range handler.Commands {
			command = strings.TrimSpace(command)
			if command == "" {
				continue
			}
			if !strings.Contains(md, command) {
				t.Errorf("%s helpdocs/%s.md does not mention command alias %q", methodName, commandHelpDocKey(handler.Path), command)
			}
		}
	}
}

func TestCommandHelpMarkdownCommandExamplesAreRegistered(t *testing.T) {
	EnsureCommandHandlersRegistered()
	codeSpanRE := regexp.MustCompile("`([^`]+)`")
	entries, err := commandHelpDocs.ReadDir("helpdocs")
	if err != nil {
		t.Fatalf("ReadDir(helpdocs) error = %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		file := "helpdocs/" + entry.Name()
		data, err := commandHelpDocs.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", file, err)
		}
		for _, match := range codeSpanRE.FindAllStringSubmatch(string(data), -1) {
			if len(match) != 2 {
				continue
			}
			commandText := strings.TrimSpace(match[1])
			if !strings.HasPrefix(commandText, "/") || commandHelpDocCommandSpanIgnored(commandText) {
				continue
			}
			if matched := corehandler.MatchCommandHandler(commandText); matched.Handler == nil {
				t.Errorf("%s mentions unregistered command example %q", file, commandText)
			}
		}
	}
}

func commandHelpDocCommandSpanIgnored(commandText string) bool {
	switch strings.TrimSpace(strings.ToLower(commandText)) {
	case "/jp", "/cn", "/tw", "/kr", "/en":
		return true
	default:
		return false
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
	md, err := commandHelpMarkdown(&CommandRequest{CommandPath: "music/unknown-helper"})
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
