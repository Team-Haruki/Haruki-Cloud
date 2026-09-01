package handler

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"
	"slices"
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleSK), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "sk-daily-speed"), "unexpected command request: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.SpeedUnit != "d"), "unexpected speed unit: %+v", params)
	testutil.Require(t, !(params.SpeedPeriodSecs != 24*60*60), "unexpected speed period: %+v", params)
	testutil.Require(t, slices.Equal(params.Ranks, defaultSKRanksNormal), "unexpected default speed ranks len: %+v", params)
	testutil.Require(t, params.DefaultRanks, "expected default ranks flag: %+v", params)

}

func TestSKSpeedHandleBuildsCommandRequestWithHourDefaults(t *testing.T) {
	h := sekaiHandlers{}.SKSpeedHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/时速",
		ArgText:    "",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleSK), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "sk-speed"), "unexpected command request: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.SpeedUnit != "h"), "unexpected speed unit: %+v", params)
	testutil.Require(t, !(params.SpeedPeriodSecs != 60*60), "unexpected speed period: %+v", params)
	testutil.Require(t, slices.Equal(params.Ranks, defaultSKRanksNormal), "unexpected default speed ranks len: %+v", params)

}

func TestSKSpeedHandleTreatsArgumentAsMinutePeriod(t *testing.T) {
	h := sekaiHandlers{}.SKSpeedHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/时速",
		ArgText:    "30",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params sk.TrackerRankQuery
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.SpeedUnit != "h"), "unexpected speed params: %+v", params)
		testutil.Require(t, !(params.SpeedPeriodSecs != 30*60), "unexpected speed params: %+v", params)
	}
	testutil.Require(t, slices.Equal(params.Ranks, defaultSKRanksNormal), "unexpected speed ranks: %+v", params.Ranks)

}

func TestSKDailySpeedHandleTreatsArgumentAsDayPeriod(t *testing.T) {
	h := sekaiHandlers{}.SKDailySpeedHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/日速",
		ArgText:    "2",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	var params sk.TrackerRankQuery
	{
		err := json.Unmarshal(result.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.SpeedUnit != "d"), "unexpected speed params: %+v", params)
		testutil.Require(t, !(params.SpeedPeriodSecs != 2*24*60*60), "unexpected speed params: %+v", params)
	}
	testutil.Require(t, slices.Equal(params.Ranks, defaultSKRanksNormal), "unexpected speed ranks: %+v", params.Ranks)

}

func TestSKLineHandleBuildsWorldLinkCurrentChapterSelector(t *testing.T) {
	h := sekaiHandlers{}.SKLineHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/wlsk线",
		ArgText:    "",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleSK), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "sk-line"), "unexpected command request: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.WlCharacterQuery != "wl"), "expected wl selector, got %+v", params)
	{
		testutil.Require(t, !(len(params.Ranks) == 0), "expected default wl ranks, got %+v", params)
		testutil.Require(t, params.DefaultRanks, "expected default wl ranks, got %+v", params)
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
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleSK), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "sk-check-room"), "unexpected command request: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, !(params.UserID != nil), "expected binding target metadata instead of direct user id, got %+v", params)
	{
		testutil.Require(t, !(params.TargetPlatform != "qq"), "expected self target metadata, got %+v", params)
		testutil.Require(t, !(params.TargetUserID != "24680"), "expected self target metadata, got %+v", params)
	}
	testutil.Require(t, !(len(params.Ranks) != 0), "expected no default rank list for /cf self query, got %+v", params.Ranks)
	testutil.Require(t, !(params.EventID != 101), "unexpected event id: %+v", params.EventID)

}

func TestSKBoardHandleWorldLinkEmptyDefaultsToSelf(t *testing.T) {
	h := sekaiHandlers{}.SKBoardHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/wlsk",
		ArgText:    "",
		Platform:   "qq",
		UserId:     "24680",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleSK), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "sk-query"), "unexpected command request: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}
	{

		testutil.Require(t, !(params.TargetPlatform != "qq"), "expected self target metadata for /wlsk, got %+v", params)
		testutil.Require(t, !(params.TargetUserID != "24680"), "expected self target metadata for /wlsk, got %+v", params)
	}
	testutil.Require(t, !(params.WlCharacterQuery != "wl"), "expected wl selector, got %+v", params)
	{
		testutil.Require(t, !(len(params.Ranks) != 0), "expected no default ranks for /wlsk self query, got %+v", params)
		testutil.Require(t, !(params.DefaultRanks), "expected no default ranks for /wlsk self query, got %+v", params)
	}

}

func TestSKCheckRoomLiteHandleUsesFixedDefaultRanks(t *testing.T) {
	h := sekaiHandlers{}.SKCheckRoomLiteHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/cfl",
		ArgText:    "event101",
	})
	testutil.Require(t, !(err != nil), "Handle() error = %v", err)

	resolved := result
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleSK), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "sk-check-room"), "unexpected command request: %+v", resolved)
	}

	var params sk.TrackerRankQuery
	{
		err := json.Unmarshal(resolved.Params, &params)
		testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
	}

	testutil.Require(t, params.DefaultRanks, "expected default ranks flag for /cfl, got %+v", params)
	testutil.Require(t, !(len(params.Ranks) != len(defaultSKCheckRoomLiteRanks)), "unexpected /cfl default rank count: %d", len(params.Ranks))

	for i, rank := range defaultSKCheckRoomLiteRanks {
		testutil.Require(t, !(params.Ranks[i] != rank), "unexpected /cfl default ranks: %+v", params.Ranks)

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
		{name: "csb alias total", handler: sekaiHandlers{}.CSBHandle(), triggerCmd: "/csb", wantMode: "sk-csb", wantWL: false},
		{name: "csb alias wl", handler: sekaiHandlers{}.CSBHandle(), triggerCmd: "/wlcsb", wantMode: "sk-csb", wantWL: true},
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
			testutil.Require(t, !(err != nil), "Handle() error = %v", err)

			resolved := result
			testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
			{

				testutil.Require(t, !(resolved.Module != parser.ModuleSK), "unexpected command request: %+v", resolved)
				testutil.Require(t, !(resolved.Mode != tc.wantMode), "unexpected command request: %+v", resolved)
			}

			var params sk.TrackerRankQuery
			{
				err := json.Unmarshal(resolved.Params, &params)
				testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
			}

			if tc.wantWL {
				testutil.Require(t, !(params.WlCharacterQuery != "wl"), "expected wl selector, got %+v", params)

				return
			}
			{
				testutil.Require(t, !(params.WlCharacterQuery != ""), "expected total-ranking request, got %+v", params)
				testutil.Require(t, !(params.WlCharacterID != nil), "expected total-ranking request, got %+v", params)
			}

		})
	}
}
