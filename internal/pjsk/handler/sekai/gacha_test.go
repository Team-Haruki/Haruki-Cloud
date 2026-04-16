package sekai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
)

func TestGachaHandleUsesPastInclusiveListWhenArgsEmpty(t *testing.T) {
	h := sekaiHandlers{}.GachaHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡池",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Module != parser.ModuleGacha || resolved.Mode != "gacha-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		IncludePast bool `json:"include_past"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.IncludePast {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestGachaHandleUsesDetailForDirectID(t *testing.T) {
	h := sekaiHandlers{}.GachaHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡池",
		ArgText:    "123",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Mode != "gacha-detail" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		GachaID int `json:"gacha_id"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.GachaID != 123 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestGachaHandleUsesDetailForNegativeIndex(t *testing.T) {
	h := sekaiHandlers{}.GachaHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡池",
		ArgText:    "-2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Mode != "gacha-detail" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		NegIndex int `json:"neg_index"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.NegIndex != -2 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestGachaHandleUsesDetailForEventSelector(t *testing.T) {
	h := sekaiHandlers{}.GachaHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡池",
		ArgText:    "event123",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Mode != "gacha-detail" {
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

func TestGachaHandleParsesListFilters(t *testing.T) {
	h := sekaiHandlers{}.GachaHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡池",
		ArgText:    "当前 25年 card123 p2 复刻 回响",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if resolved.Mode != "gacha-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		OnlyCurrent bool `json:"only_current"`
		Year        int  `json:"year"`
		CardID      int  `json:"card_id"`
		Page        int  `json:"page"`
		IsRerelease bool `json:"is_rerelease"`
		IsRecall    bool `json:"is_recall"`
		IncludePast bool `json:"include_past"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.OnlyCurrent || params.Year != 2025 || params.CardID != 123 || params.Page != 2 || !params.IsRerelease || !params.IsRecall || !params.IncludePast {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestGachaHandleReturnsCombinedHelpOnInvalidQuery(t *testing.T) {
	h := sekaiHandlers{}.GachaHandle()

	_, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/卡池",
		ArgText:    "???",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "【查单个卡池格式】") || !strings.Contains(err.Error(), "【查多个卡池格式】") {
		t.Fatalf("unexpected error: %v", err)
	}
}
