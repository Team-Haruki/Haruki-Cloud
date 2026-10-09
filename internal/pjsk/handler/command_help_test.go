package handler

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestCommandHelpFlagShortCircuitsValidation(t *testing.T) {
	server, calls := newCommandHelpDrawingServer(t, "music")
	defer server.Close()

	h := sekaiHandlers{}.SongHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	resolved, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查曲",
		ArgText:    "-help",
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

func TestCommandHelpPlainHelpIsQueryText(t *testing.T) {
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
			if resolved == nil || resolved.IsHelp {
				t.Fatalf("expected normal command request, got %+v", resolved)
			}
			if resolved.Query != arg {
				t.Fatalf("query = %q, want %q", resolved.Query, arg)
			}
		})
	}
}

func TestCommandHelpDeckGenericTriggerUsesEventDeckDoc(t *testing.T) {
	h := sekaiHandlers{}.EventDeckHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	resolved, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/组卡",
		ArgText:    "-help",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if resolved == nil || !resolved.IsHelp {
		t.Fatalf("expected help command request, got %+v", resolved)
	}
	if path := commandHelpRequestPath(resolved); path != "deck/event" {
		t.Fatalf("commandHelpRequestPath() = %q, want deck/event", path)
	}
	md, err := commandHelpMarkdown(resolved)
	if err != nil {
		t.Fatalf("commandHelpMarkdown() error = %v", err)
	}
	doc, _, _ := i18n.HelpDoc(i18n.DefaultLocale, "deck_event")
	if !strings.HasPrefix(md, doc) {
		t.Fatalf("expected event deck markdown, got %q", md)
	}
}

func TestCommandHelpMysekaiBlueprintTriggerUsesBlueprintDoc(t *testing.T) {
	h := sekaiHandlers{}.MysekaiBlueprintHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	resolved, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msb",
		ArgText:    "-help",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if resolved == nil || !resolved.IsHelp {
		t.Fatalf("expected help command request, got %+v", resolved)
	}
	if path := commandHelpRequestPath(resolved); path != "mysekai/blueprint" {
		t.Fatalf("commandHelpRequestPath() = %q, want mysekai/blueprint", path)
	}
}

func TestCommandHelpFallsBackToTextWhenDrawingUnavailable(t *testing.T) {
	resolved := &CommandRequest{
		IsHelp:         true,
		CommandPath:    "profile/unbind",
		TriggerCommand: "/解绑",
	}

	message, err := ExecuteCommandRequest(context.Background(), resolved, &renderapp.App{})
	if err != nil {
		t.Fatalf("ExecuteCommandRequest() error = %v", err)
	}
	if len(message) != 1 || message[0].Type != onebot11.TypeText {
		t.Fatalf("expected single text help message, got %+v", message)
	}
	text, ok := message[0].Data.(onebot11.TextData)
	if !ok || !strings.Contains(text.Text, "/解绑") {
		t.Fatalf("expected unbind help text, got %+v", message[0].Data)
	}
	if strings.ContainsAny(text.Text, "#`") {
		t.Fatalf("text help still has Markdown markup: %q", text.Text)
	}
}

func TestCommandHelpFallsBackToTextWhenDrawingFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing help renderer", http.StatusNotFound)
	}))
	defer server.Close()

	resolved := &CommandRequest{
		IsHelp:         true,
		CommandPath:    "music",
		TriggerCommand: "/查曲",
	}

	app := &renderapp.App{Drawing: drawing.NewHarukiDrawingClient(server.URL)}
	message, err := ExecuteCommandRequest(context.Background(), resolved, app)
	if err != nil {
		t.Fatalf("ExecuteCommandRequest() error = %v", err)
	}
	if len(message) != 1 || message[0].Type != onebot11.TypeText {
		t.Fatalf("expected single text help message, got %+v", message)
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

func TestCommandHelpExactMarkdownAvailableForRegisteredRoutes(t *testing.T) {
	EnsureCommandHandlersRegistered()
	routes := corehandler.ListBotRoutes()
	if len(routes) == 0 {
		t.Fatal("expected registered bot routes")
	}

	seen := map[string]struct{}{}
	for _, route := range routes {
		key := commandHelpDocKey(route.Path)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		t.Run(key, func(t *testing.T) {
			md, ok, err := readCommandHelpMarkdown(key)
			if err != nil {
				t.Fatalf("readCommandHelpMarkdown(%q) error = %v", key, err)
			}
			if !ok {
				t.Fatalf("missing exact help markdown for route path %q", route.Path)
			}
			if strings.TrimSpace(md) == "" {
				t.Fatalf("exact help markdown for route path %q is empty", route.Path)
			}
		})
	}

	t.Run("mysekai_blueprint", func(t *testing.T) {
		md, ok, err := readCommandHelpMarkdown("mysekai/blueprint")
		if err != nil {
			t.Fatalf("readCommandHelpMarkdown(mysekai/blueprint) error = %v", err)
		}
		if !ok || strings.TrimSpace(md) == "" {
			t.Fatal("missing exact help markdown for /msb blueprint trigger")
		}
	})
}

func TestCommandHelpMarkdownPrefersExactFile(t *testing.T) {
	md, err := commandHelpMarkdown(&CommandRequest{CommandPath: "music/bpm"})
	if err != nil {
		t.Fatalf("commandHelpMarkdown() error = %v", err)
	}
	doc, _, _ := i18n.HelpDoc(i18n.DefaultLocale, "music_bpm")
	if !strings.HasPrefix(md, doc) {
		t.Fatalf("expected the music_bpm document first, got %q", md)
	}
}

func TestCommandHelpGeneratesRegionSection(t *testing.T) {
	EnsureCommandHandlersRegistered()
	md, err := commandHelpMarkdown(&CommandRequest{CommandPath: "music"})
	if err != nil {
		t.Fatal(err)
	}
	heading := "## " + i18n.T("usage.help.section_region")
	prefix := i18n.T("usage.help.region_prefix", i18n.Data{"Codes": "`jp` `cn` `tw` `kr` `en`", "Example": "`/jp查曲`"})
	fallback := i18n.T("usage.help.region_default", i18n.Data{"Region": i18n.RegionLabel("jp")})
	if !strings.Contains(md, heading+"\n- "+prefix+"\n- "+fallback) {
		t.Fatalf("music help lacks the generated region section:\n%s", md)
	}

	md, err = commandHelpMarkdown(&CommandRequest{CommandPath: "sk/winrate"})
	if err != nil {
		t.Fatal(err)
	}
	if only := i18n.T("usage.help.region_only", i18n.Data{"Region": i18n.RegionLabel("jp")}); !strings.Contains(md, heading+"\n- "+only) {
		t.Fatalf("JP-only help lacks the region-only line:\n%s", md)
	}

	for _, path := range []string{"admin/kill", "alias/music", "profile/timezone"} {
		md, err := commandHelpMarkdown(&CommandRequest{CommandPath: path})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(md, heading) {
			t.Fatalf("%s help should not explain regions:\n%s", path, md)
		}
	}
}

func TestCommandHelpRegionExamplesAreRegistered(t *testing.T) {
	EnsureCommandHandlersRegistered()
	for _, route := range corehandler.ListBotRoutes() {
		md, err := commandHelpMarkdown(&CommandRequest{CommandPath: route.Path})
		if err != nil {
			t.Fatal(err)
		}
		section := commandHelpRegionSection(i18n.DefaultLocale, md, route.Path)
		if isRegionAgnosticHelpRoute(route.Path) {
			continue
		}
		if section == "" {
			t.Errorf("%s: no region section", route.Path)
			continue
		}
		for _, span := range helpDocCodeSpans(section) {
			if strings.HasPrefix(span, "/") && !helpDocTriggerResolves(span) {
				t.Errorf("%s: region example %q is not registered", route.Path, span)
			}
		}
	}
}

func TestCommandHelpAliasSectionSkipsShownAndPrefixedSpellings(t *testing.T) {
	EnsureCommandHandlersRegistered()
	md, err := commandHelpMarkdown(&CommandRequest{CommandPath: "sk/line"})
	if err != nil {
		t.Fatal(err)
	}
	_, aliases, _ := strings.Cut(md, "## "+i18n.T("usage.help.section_aliases"))
	if aliases == "" {
		t.Fatalf("sk/line help lacks the alias section:\n%s", md)
	}
	if strings.Contains(aliases, "/wl") {
		t.Fatalf("alias section lists wl-prefixed spellings:\n%s", aliases)
	}
	doc, _, _ := i18n.HelpDoc(i18n.DefaultLocale, "sk_line")
	for _, span := range helpDocCodeSpans(doc) {
		if command := helpDocSpanCommand(span); command != "" && strings.Contains(aliases, "`"+command+"`") {
			t.Fatalf("alias section repeats %q shown in the document", command)
		}
	}
}

func TestCommandHelpBlueprintAliasesStayInTheirDocument(t *testing.T) {
	EnsureCommandHandlersRegistered()
	blueprint := commandHelpAliases(mysekaiBlueprintHelpPath)
	talk := commandHelpAliases("mysekai/talk-list")
	if len(blueprint) == 0 || len(talk) == 0 {
		t.Fatalf("aliases: blueprint=%v talk=%v", blueprint, talk)
	}
	for _, alias := range blueprint {
		if !isMysekaiBlueprintHelpTrigger(alias) || slices.Contains(talk, alias) {
			t.Fatalf("blueprint alias %q leaks: blueprint=%v talk=%v", alias, blueprint, talk)
		}
	}
}

func TestCommandHelpPlainTextDropsMarkdown(t *testing.T) {
	md := "# 查曲\n\n查询歌曲。\n\n## 用法\n- `/查曲 <歌曲>`\n### 难度\n- **必填**：`master`\n```text\n/添加歌曲别名\ntyw\n```"
	got := commandHelpPlainText(i18n.DefaultLocale, md)
	for _, marker := range []string{"#", "`", "**", "- "} {
		if strings.Contains(got, marker) {
			t.Fatalf("plain help keeps %q:\n%s", marker, got)
		}
	}
	want := "查曲\n\n查询歌曲。\n\n" +
		i18n.T("usage.help.plain_heading", i18n.Data{"Heading": "用法"}) + "\n· /查曲 <歌曲>\n" +
		i18n.T("usage.help.plain_heading", i18n.Data{"Heading": "难度"}) + "\n· 必填：master\n/添加歌曲别名\ntyw"
	if got != want {
		t.Fatalf("plain help =\n%s\nwant\n%s", got, want)
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

// The generated 区服 section of a MySekai route that CN rejects leaves cn
// out and says so; the routes open to CN keep it.
func TestMySekaiHelpRegionSectionLeavesOutCN(t *testing.T) {
	EnsureCommandHandlersRegistered()
	note := i18n.M("usage.help.region_no_cn_mysekai").String()
	for path, blocked := range map[string]bool{
		"mysekai/map":                true,
		"mysekai/resource":           true,
		"profile/check-data-mysekai": true,
		"mysekai/housing-sk":         false,
		"deck/mysekai":               false,
	} {
		markdown, ok, err := i18n.HelpDoc(i18n.DefaultLocale, commandHelpDocKey(path))
		if err != nil || !ok {
			t.Fatalf("HelpDoc(%s) = %v, %v", path, ok, err)
		}
		section := commandHelpRegionSection(i18n.DefaultLocale, markdown, path)
		if section == "" {
			t.Fatalf("%s: no region section", path)
		}
		if got := strings.Contains(section, "`cn`"); got == blocked {
			t.Errorf("%s: region section lists cn = %v:\n%s", path, got, section)
		}
		if got := strings.Contains(section, note); got != blocked {
			t.Errorf("%s: region section has the CN note = %v:\n%s", path, got, section)
		}
	}
}
