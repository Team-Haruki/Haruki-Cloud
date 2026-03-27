package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
)

func TestEventDetailHandleUsesCurrentEventWhenArgsEmpty(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-detail" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		UseCurrent bool `json:"use_current"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.UseCurrent {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventDetailHandleExtractsEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "event123",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-detail" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		EventID int `json:"event_id"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID != 123 {
		t.Fatalf("unexpected params: %+v", params)
	}
}
