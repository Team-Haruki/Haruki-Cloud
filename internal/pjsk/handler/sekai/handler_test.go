package sekai

import (
	"context"
	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
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
