package sekai

import (
	"context"
	"encoding/json"
	"strings"
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

func TestEventListHandleUsesFullRangeWhenArgsEmpty(t *testing.T) {
	h := sekaiHandlers{}.EventHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		IncludePast   bool `json:"include_past"`
		IncludeFuture bool `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventDetailHandleFallsBackToListForFilterQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "紫 25h",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		Attr          string `json:"attr"`
		Unit          string `json:"unit"`
		IncludePast   bool   `json:"include_past"`
		IncludeFuture bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Attr != "mysterious" || params.Unit != "school_refusal" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventDetailHandleTreatsBare25AsEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "25",
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
	if params.EventID != 25 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventListHandleFallsBackToDetailForSingleEventQuery(t *testing.T) {
	h := sekaiHandlers{}.EventHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "mnr1",
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
		BanCharID int `json:"ban_char_id"`
		BanSeq    int `json:"ban_seq"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.BanCharID != 5 || params.BanSeq != 1 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventListHandleTreatsBare25AsUnitFilter(t *testing.T) {
	h := sekaiHandlers{}.EventHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		Unit          string `json:"unit"`
		IncludePast   bool   `json:"include_past"`
		IncludeFuture bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Unit != "school_refusal" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventHandleReturnsCombinedHelpOnInvalidQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	_, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "???",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "【查单个活动格式】") || !strings.Contains(err.Error(), "【查多个活动格式】") {
		t.Fatalf("unexpected error: %v", err)
	}
}
