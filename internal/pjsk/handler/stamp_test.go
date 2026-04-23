package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
)

func TestStampHandleParsesCharacterWithPage(t *testing.T) {
	h := sekaiHandlers{}.StampHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/stamp",
		ArgText:    "mzk page 2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleStamp || resolved.Mode != "stamp-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != "mzk" {
		t.Fatalf("unexpected query: %q", resolved.Query)
	}

	var params struct {
		Page int `json:"page"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Page != 2 {
		t.Fatalf("unexpected page: %d", params.Page)
	}
}

func TestStampHandleParsesPageBeforeCharacter(t *testing.T) {
	h := sekaiHandlers{}.StampHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/stamp",
		ArgText:    "page 3 mzk",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved.Query != "mzk" {
		t.Fatalf("unexpected query: %q", resolved.Query)
	}
	var params struct {
		Page int `json:"page"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Page != 3 {
		t.Fatalf("unexpected page: %d", params.Page)
	}
}

func TestStampHandleParsesPurePage(t *testing.T) {
	h := sekaiHandlers{}.StampHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/stamp",
		ArgText:    "p 2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved.Query != "" {
		t.Fatalf("unexpected query: %q", resolved.Query)
	}
	var params struct {
		Page int `json:"page"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Page != 2 {
		t.Fatalf("unexpected page: %d", params.Page)
	}
}
