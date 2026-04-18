package handler

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/sk"
)

func TestSKDailySpeedHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.SKDailySpeedHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/日速",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-daily-speed" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

func TestSKSpeedHandleBuildsCommandRequestWithHourDefaults(t *testing.T) {
	h := sekaiHandlers{}.SKSpeedHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/时速",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-speed" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/wlsk线",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-line" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

func TestSKCheckRoomHandleDefaultsToSelfBinding(t *testing.T) {
	h := sekaiHandlers{}.SKCheckRoomHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/cf",
		ArgText:    "event101",
		Platform:   "qq",
		UserId:     "24680",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-check-room" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.UserID != nil {
		t.Fatalf("expected binding target metadata instead of direct user id, got %+v", params)
	}
	if params.TargetPlatform != "qq" || params.TargetUserID != "24680" {
		t.Fatalf("expected self target metadata, got %+v", params)
	}
	if len(params.Ranks) != 0 {
		t.Fatalf("expected no default rank list for /cf self query, got %+v", params.Ranks)
	}
	if params.EventID != 101 {
		t.Fatalf("unexpected event id: %+v", params.EventID)
	}
}

func TestSKCheckRoomLiteHandleUsesFixedDefaultRanks(t *testing.T) {
	h := sekaiHandlers{}.SKCheckRoomLiteHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/cfl",
		ArgText:    "event101",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleSK || resolved.Mode != "sk-check-room" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.DefaultRanks {
		t.Fatalf("expected default ranks flag for /cfl, got %+v", params)
	}
	if len(params.Ranks) != len(defaultSKCheckRoomLiteRanks) {
		t.Fatalf("unexpected /cfl default rank count: %d", len(params.Ranks))
	}
	for i, rank := range defaultSKCheckRoomLiteRanks {
		if params.Ranks[i] != rank {
			t.Fatalf("unexpected /cfl default ranks: %+v", params.Ranks)
		}
	}
}

func TestSKHandlersRespectWorldLinkPrefixAcrossCommands(t *testing.T) {
	cases := []struct {
		name       string
		handler    HarukiSekaiCommandHandler
		triggerCmd string
		wantMode   string
		wantWL     bool
	}{
		{name: "line total", handler: sekaiHandlers{}.SKLineHandle(), triggerCmd: "/sk线", wantMode: "sk-line", wantWL: false},
		{name: "line wl", handler: sekaiHandlers{}.SKLineHandle(), triggerCmd: "/wlsk线", wantMode: "sk-line", wantWL: true},
		{name: "query total", handler: sekaiHandlers{}.SKQueryHandle(), triggerCmd: "/sk", wantMode: "sk-query", wantWL: false},
		{name: "query wl", handler: sekaiHandlers{}.SKQueryHandle(), triggerCmd: "/wlsk", wantMode: "sk-query", wantWL: true},
		{name: "board alias total", handler: sekaiHandlers{}.SKBoardHandle(), triggerCmd: "/sk", wantMode: "sk-query", wantWL: false},
		{name: "board alias wl", handler: sekaiHandlers{}.SKBoardHandle(), triggerCmd: "/wlsk", wantMode: "sk-query", wantWL: true},
		{name: "speed total", handler: sekaiHandlers{}.SKSpeedHandle(), triggerCmd: "/时速", wantMode: "sk-speed", wantWL: false},
		{name: "speed wl", handler: sekaiHandlers{}.SKSpeedHandle(), triggerCmd: "/wl时速", wantMode: "sk-speed", wantWL: true},
		{name: "daily speed total", handler: sekaiHandlers{}.SKDailySpeedHandle(), triggerCmd: "/日速", wantMode: "sk-daily-speed", wantWL: false},
		{name: "daily speed wl", handler: sekaiHandlers{}.SKDailySpeedHandle(), triggerCmd: "/wl日速", wantMode: "sk-daily-speed", wantWL: true},
		{name: "check room total", handler: sekaiHandlers{}.SKCheckRoomHandle(), triggerCmd: "/查房", wantMode: "sk-check-room", wantWL: false},
		{name: "check room wl", handler: sekaiHandlers{}.SKCheckRoomHandle(), triggerCmd: "/wl查房", wantMode: "sk-check-room", wantWL: true},
		{name: "check room lite total", handler: sekaiHandlers{}.SKCheckRoomLiteHandle(), triggerCmd: "/cfl", wantMode: "sk-check-room", wantWL: false},
		{name: "check room lite wl", handler: sekaiHandlers{}.SKCheckRoomLiteHandle(), triggerCmd: "/wlcfl", wantMode: "sk-check-room", wantWL: true},
		{name: "csb alias total", handler: sekaiHandlers{}.CSBHandle(), triggerCmd: "/csb", wantMode: "sk-check-room", wantWL: false},
		{name: "csb alias wl", handler: sekaiHandlers{}.CSBHandle(), triggerCmd: "/wlcsb", wantMode: "sk-check-room", wantWL: true},
		{name: "player trace total", handler: sekaiHandlers{}.SKPlayerTraceHandle(), triggerCmd: "/玩家轨迹", wantMode: "sk-player-trace", wantWL: false},
		{name: "player trace wl", handler: sekaiHandlers{}.SKPlayerTraceHandle(), triggerCmd: "/wl玩家轨迹", wantMode: "sk-player-trace", wantWL: true},
		{name: "rank trace total", handler: sekaiHandlers{}.SKRankTraceHandle(), triggerCmd: "/档线轨迹", wantMode: "sk-rank-trace", wantWL: false},
		{name: "rank trace wl", handler: sekaiHandlers{}.SKRankTraceHandle(), triggerCmd: "/wl档线轨迹", wantMode: "sk-rank-trace", wantWL: true},
		{name: "predict total", handler: sekaiHandlers{}.SKPredictHandle(), triggerCmd: "/sk预测", wantMode: "sk-predict", wantWL: false},
		{name: "predict wl", handler: sekaiHandlers{}.SKPredictHandle(), triggerCmd: "/wlsk预测", wantMode: "sk-predict", wantWL: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.handler.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				TriggerCmd: tc.triggerCmd,
				ArgText:    "",
			})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			resolved := result
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.Module != parser.ModuleSK || resolved.Mode != tc.wantMode {
				t.Fatalf("unexpected command request: %+v", resolved)
			}

			var params sk.TrackerRankQuery
			if err := json.Unmarshal(resolved.Params, &params); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}

			if tc.wantWL {
				if params.WlCharacterQuery != "wl" {
					t.Fatalf("expected wl selector, got %+v", params)
				}
				return
			}
			if params.WlCharacterQuery != "" || params.WlCharacterID != nil {
				t.Fatalf("expected total-ranking request, got %+v", params)
			}
		})
	}
}
