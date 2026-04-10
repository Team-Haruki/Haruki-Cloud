package sekai

import (
	"context"
	"encoding/json"
	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"log"
	"testing"
)

func TestRegisterCommandHandler(t *testing.T) {

	RegisterSekaiCommandHandler()

	handler.PrintTree()
	v, e := handler.Dispatch(context.Background(), handler.Event{
		Message: onebot11.Message{
			{Type: "text", Data: map[string]string{"text": "/cn查谱面 虾"}},
		},
	})
	log.Println(v, e)
	v, e = handler.Dispatch(context.Background(), handler.Event{
		Message: onebot11.Message{
			{Type: "text", Data: map[string]string{"text": "/card 1"}},
		},
	})
	log.Println(v, e)
}

func TestSekaiHandlerParsesUIDArgFromArgsAndAt(t *testing.T) {
	skh := SekaiCommandHandler{
		ParseUIDArg: boolPtr(true),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			if ctx.UIDArg() != "@987654321" {
				t.Fatalf("uidArg = %q", ctx.UIDArg())
			}
			if ctx.GetArgs() != "剩余参数" {
				t.Fatalf("args = %q", ctx.GetArgs())
			}
			return ctx, nil
		},
	}

	baseCtx := &handler.HandlerContext{
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
	skh := SekaiCommandHandler{
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			if ctx.UIDArg() != "" {
				t.Fatalf("uidArg = %q", ctx.UIDArg())
			}
			if ctx.GetArgs() != "u2 12345678901234 @123456789 剩余参数" {
				t.Fatalf("args = %q", ctx.GetArgs())
			}
			return ctx, nil
		},
	}

	baseCtx := &handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/绑定",
		ArgText:    "u2 12345678901234 @123456789 剩余参数",
		AtIds:      []string{"987654321"},
	}

	if _, err := skh.Handle(baseCtx); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
}

func TestDispatchSupportsRegionPrefixedSKCommandWithMapSegments(t *testing.T) {
	EnsureCommandHandlersRegistered()

	result, err := handler.Dispatch(context.Background(), handler.Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/cnsk event101 100"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok || resolved == nil {
		t.Fatalf("expected resolved command, got %#v", result)
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-query" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
	if resolved.Region != "cn" {
		t.Fatalf("unexpected region: %s", resolved.Region)
	}
}

func TestDispatchSupportsRegionPrefixedWorldBloomSKLineWithCharacterOnly(t *testing.T) {
	EnsureCommandHandlersRegistered()

	result, err := handler.Dispatch(context.Background(), handler.Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/cnwlsk线冬弥"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok || resolved == nil {
		t.Fatalf("expected resolved command, got %#v", result)
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

	result, err := handler.Dispatch(context.Background(), handler.Event{
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

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok || resolved == nil {
		t.Fatalf("expected resolved command, got %#v", result)
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

	result, err := handler.Dispatch(context.Background(), handler.Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/skp event101 100"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok || resolved == nil {
		t.Fatalf("expected resolved command, got %#v", result)
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-predict" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}
}
