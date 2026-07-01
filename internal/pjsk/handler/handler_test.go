package handler

import (
	"context"
	"fmt"
	json "github.com/bytedance/sonic"
	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"
	"log"
	"strings"
	"testing"
)

func dispatchForTest(ctx context.Context, event Event) (*CommandRequest, error) {
	handlerContext, err := BuildContext(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("构建命令上下文失败: %w", err)
	}

	matched := corehandler.MatchCommandHandler(handlerContext.GetArgs())
	if matched.Handler == nil || matched.Handler.IsDisabled() {
		return nil, nil
	}

	executable, ok := matched.Handler.(CommandHandler)
	if !ok {
		return nil, fmt.Errorf("registered handler does not implement pjsk command handler: %T", matched.Handler)
	}

	handlerContext.ArgText = strings.TrimSpace(string(matched.ArgText))
	handlerContext.TriggerCmd = matched.Command
	return executable.Handle(handlerContext)
}

func TestRegisterCommandHandler(t *testing.T) {

	RegisterSekaiCommandHandler()

	corehandler.PrintTree()
	v, e := dispatchForTest(context.Background(), Event{
		Message: onebot11.Message{
			{Type: "text", Data: map[string]string{"text": "/cn查谱面 虾"}},
		},
	})
	log.Println(v, e)
	v, e = dispatchForTest(context.Background(), Event{
		Message: onebot11.Message{
			{Type: "text", Data: map[string]string{"text": "/card 1"}},
		},
	})
	log.Println(v, e)
}

func TestSekaiHandlerParsesUIDArgFromArgsAndAt(t *testing.T) {
	skh := HarukiSekaiCommandHandler{
		ParseUIDArg: commandBoolPtr(true),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if ctx.UIDArg() != "@987654321" {
				t.Fatalf("uidArg = %q", ctx.UIDArg())
			}
			if ctx.GetArgs() != "剩余参数" {
				t.Fatalf("args = %q", ctx.GetArgs())
			}
			return makeCommandRequest(ctx, parser.ModuleProfile, "test"), nil
		},
	}

	baseCtx := &PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/sk",
		ArgText:    "u2 12345678901234 @123456789 剩余参数",
		AtIds:      []string{"987654321"},
	}

	if _, err := skh.Handle(baseCtx); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
}

func TestSekaiHandlerCanDisableUIDArgParsing(t *testing.T) {
	skh := HarukiSekaiCommandHandler{
		ParseUIDArg: commandBoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if ctx.UIDArg() != "" {
				t.Fatalf("uidArg = %q", ctx.UIDArg())
			}
			if ctx.GetArgs() != "u2 12345678901234 @123456789 剩余参数" {
				t.Fatalf("args = %q", ctx.GetArgs())
			}
			return makeCommandRequest(ctx, parser.ModuleProfile, "test"), nil
		},
	}

	baseCtx := &PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/绑定",
		ArgText:    "u2 12345678901234 @123456789 剩余参数",
		AtIds:      []string{"987654321"},
	}

	if _, err := skh.Handle(baseCtx); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
}

func TestDispatchKeepsCustomChartIDWithEmbeddedUSelector(t *testing.T) {
	EnsureCommandHandlersRegistered()

	const scoreID = "7ao-at6p85d-g9jvnqu5f-pvekg3"
	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/jp谱面预览 " + scoreID}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMusic || resolved.Mode != "music-chart" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
	if resolved.Region != "jp" {
		t.Fatalf("unexpected region: %s", resolved.Region)
	}
	if resolved.Query != scoreID {
		t.Fatalf("query = %q, want %q", resolved.Query, scoreID)
	}
}

func TestDispatchSupportsRegionPrefixedSKCommandWithMapSegments(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/cnsk event101 100"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-query" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
	if resolved.Region != "cn" {
		t.Fatalf("unexpected region: %s", resolved.Region)
	}
}

func TestDispatchSupportsRegionPrefixedCheckRoomWithoutWhitespace(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/cncf1"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-check-room" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
	if resolved.Region != "cn" {
		t.Fatalf("unexpected region: %s", resolved.Region)
	}

	var params map[string]any
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	ranks, ok := params["ranks"].([]any)
	if !ok || len(ranks) != 1 || int(ranks[0].(float64)) != 1 {
		t.Fatalf("unexpected ranks payload: %#v", params["ranks"])
	}
}

func TestDispatchSupportsRegionPrefixedWorldBloomSKLineWithCharacterOnly(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/cnwlsk线冬弥"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-line" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
	if resolved.Region != "cn" {
		t.Fatalf("unexpected region: %s", resolved.Region)
	}

	var params map[string]any
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if got, ok := params["wl_character_query"].(string); !ok || got != "冬弥" {
		t.Fatalf("unexpected wl_character_query: %#v", params["wl_character_query"])
	}
	ranks, ok := params["ranks"].([]any)
	if !ok || len(ranks) == 0 {
		t.Fatalf("expected default wl ranks, got %#v", params["ranks"])
	}
	if got, ok := params["default_ranks"].(bool); !ok || !got {
		t.Fatalf("unexpected default_ranks: %#v", params["default_ranks"])
	}
}

func TestDispatchSupportsAtMentionFromMapSegmentsInSK(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/sk event101 "}},
			{Type: "at", Data: map[string]any{"qq": 67890}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}

	var params map[string]any
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if got, _ := params["target_user_id"].(string); got != "67890" {
		t.Fatalf("unexpected target_user_id: %#v", params["target_user_id"])
	}
}

func TestDispatchSupportsSKPredictMode(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/skp event101 100"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-predict" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
}

func TestDispatchSupportsCSBMode(t *testing.T) {
	EnsureCommandHandlersRegistered()

	cases := []struct {
		message  string
		wantMode string
	}{
		{message: "/csb 1", wantMode: "sk-csb"},
		{message: "/查水表 1", wantMode: "sk-csb"},
		{message: "/停车时间 1", wantMode: "sk-csb"},
	}

	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			matched := corehandler.MatchCommandHandler(tc.message)
			if matched.Handler == nil {
				t.Fatalf("expected matched handler, got nil")
			}
			resolved, err := dispatchForTest(context.Background(), Event{
				Platform: "qq",
				Message: onebot11.Message{
					{Type: "text", Data: map[string]any{"text": tc.message}},
				},
				UserId: "12345",
			})
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.Module != parser.ModuleSK || resolved.Mode != tc.wantMode {
				t.Fatalf("unexpected resolved target: module=%v mode=%s matched=%s", resolved.Module, resolved.Mode, matched.Command)
			}
		})
	}
}

func TestMatchCommandHandlerPrefersArrestDifficultyOverArrest(t *testing.T) {
	EnsureCommandHandlersRegistered()

	matched := corehandler.MatchCommandHandler("/逮捕难度 master关闭")
	if matched.Handler == nil {
		t.Fatal("expected matched handler, got nil")
	}
	if matched.Command != "/逮捕难度" {
		t.Fatalf("unexpected matched command: %s", matched.Command)
	}
}

func TestRegisteredCommandAliasesDoNotConsumeArgumentPrefix(t *testing.T) {
	EnsureCommandHandlersRegistered()

	for _, route := range corehandler.ListBotRoutes() {
		for _, command := range route.Commands {
			message := command + " saki"
			matched := corehandler.MatchCommandHandler(message)
			if matched.Handler == nil {
				t.Fatalf("%q did not match any command", message)
			}
			if gotPath := matched.Handler.GetPath(); gotPath != route.Path {
				t.Fatalf("%q matched %q (%s), want route %s", message, matched.Command, gotPath, route.Path)
			}
			if gotArg := strings.TrimSpace(string(matched.ArgText)); gotArg != "saki" {
				t.Fatalf("%q arg text = %q, want saki (matched %q)", message, gotArg, matched.Command)
			}
		}
	}
}

func TestDispatchSupportsMysekaiOverviewAlias(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/msam"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-resource-map" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
}

func TestDispatchSupportsMysekaiResourceAliasWithoutMapMode(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/msa all"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-resource" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
}

func TestDispatchSupportsRegionPrefixedProfileUID(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/jp查uid"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleProfile || resolved.Mode != accountdata.ProfileModeQueryUID {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}

	var params accountdata.ProfileBindingCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Server != "jp" {
		t.Fatalf("unexpected server: %q", params.Server)
	}
}
