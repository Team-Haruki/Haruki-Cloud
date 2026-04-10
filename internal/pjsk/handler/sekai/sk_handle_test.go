package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/sk"
)

func TestSKDailySpeedHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.SKDailySpeedHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/日速",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-daily-speed" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.SpeedUnit != "d" {
		t.Fatalf("unexpected speed unit: %+v", params)
	}
	if params.SpeedPeriodSecs != 24*60*60 {
		t.Fatalf("unexpected speed period: %+v", params)
	}
	if !params.DefaultRanks {
		t.Fatalf("expected default ranks flag: %+v", params)
	}
}

func TestSKSpeedHandleBuildsResolvedCommandWithHourDefaults(t *testing.T) {
	h := sekaiHandlers{}.SKSpeedHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/时速",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-speed" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.SpeedUnit != "h" {
		t.Fatalf("unexpected speed unit: %+v", params)
	}
	if params.SpeedPeriodSecs != 60*60 {
		t.Fatalf("unexpected speed period: %+v", params)
	}
}

func TestSKLineHandleBuildsWorldLinkCurrentChapterSelector(t *testing.T) {
	h := sekaiHandlers{}.SKLineHandle()

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/wlsk线",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-line" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.WlCharacterQuery != "wl" {
		t.Fatalf("expected wl selector, got %+v", params)
	}
	if len(params.Ranks) == 0 || !params.DefaultRanks {
		t.Fatalf("expected default wl ranks, got %+v", params)
	}
}
