package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	json "github.com/bytedance/sonic"

	"haruki-cloud/config"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestEventDetailHandleUsesCurrentEventWhenArgsEmpty(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-detail" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "紫 25h",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-detail" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "mnr1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-detail" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
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

func TestEventListHandleSupportsSharedUnitAndAttrAliases(t *testing.T) {
	h := sekaiHandlers{}.EventHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "v 粉花",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
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
	if params.Attr != "cute" || params.Unit != "piapro" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventDetailHandleParsesOnlyUnitFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "仅mmj",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Unit          string `json:"unit"`
		OnlyUnit      bool   `json:"only_unit"`
		IncludePast   bool   `json:"include_past"`
		IncludeFuture bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Unit != "idol" || !params.OnlyUnit {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventDetailHandleParsesWorldBloomTurnAndCharacterFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "wl3 miku",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		EventType      string `json:"event_type"`
		WorldBloomTurn int    `json:"world_bloom_turn"`
		CharacterID    int    `json:"character_id"`
		IncludePast    bool   `json:"include_past"`
		IncludeFuture  bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventType != "world_bloom" || params.WorldBloomTurn != 3 || params.CharacterID != 21 {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventDetailHandleKeepsBareWorldBloomFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "wl",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	var params struct {
		EventType      string `json:"event_type"`
		WorldBloomTurn int    `json:"world_bloom_turn"`
	}
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventType != "world_bloom" || params.WorldBloomTurn != 0 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventHandleReturnsCombinedHelpOnInvalidQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	_, err := h.Handle(&PjskHandlerContext{
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

func TestEventRecordHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.EventRecordHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/活动记录",
		ArgText:    "u2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-record" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
		Selector       string `json:"selector"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventPlannerHandleAllowsDefaultRegion(t *testing.T) {
	h := sekaiHandlers{}.EventPlannerHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动规划",
		ArgText:    "pt500w 当前",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil || result.Module != parser.ModuleEvent || result.Mode != "event-planner" {
		t.Fatalf("unexpected command request: %+v", result)
	}
	if result.RegionExplicit {
		t.Fatalf("expected implicit region, got %+v", result)
	}

	var params eventPlannerCommandParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.TargetPoint != 5_000_000 || !params.Deck.UseCurrentDeck {
		t.Fatalf("unexpected planner params: %+v", params)
	}
}

func TestEventPlannerHandleParsesPlannerParams(t *testing.T) {
	h := sekaiHandlers{}.EventPlannerHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/cn活动规划",
		ArgText:    "event154 当前pt320w pt1200w #123 456 789 101 112 歌 虾ex 龙mas 5火",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil || result.Module != parser.ModuleEvent || result.Mode != "event-planner" {
		t.Fatalf("unexpected command request: %+v", result)
	}

	var params eventPlannerCommandParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID != 154 || params.TargetPoint != 12_000_000 || params.CurrentPoint != 3_200_000 {
		t.Fatalf("unexpected point params: %+v", params)
	}
	if len(params.Deck.FixedCards) != 5 || params.Deck.FixedCards[0] != 123 {
		t.Fatalf("unexpected deck params: %+v", params)
	}
	if len(params.Boosts) != 1 || params.Boosts[0] != 5 {
		t.Fatalf("unexpected boosts: %+v", params.Boosts)
	}
	if len(params.Songs) != 2 || params.Songs[0].Query != "虾" || params.Songs[0].Difficulty != "expert" ||
		params.Songs[1].Query != "龙" || params.Songs[1].Difficulty != "master" {
		t.Fatalf("unexpected songs: %+v", params.Songs)
	}
}

func TestEventPlannerDefaultDragonUsesLostAndFound(t *testing.T) {
	params, err := parseEventPlannerParams("pt1200w", "/cn活动规划")
	if err != nil {
		t.Fatalf("parseEventPlannerParams() error = %v", err)
	}
	songs := eventPlannerSongsForRequest(params, buildEventPlannerBaseDeckQuery(renderregion.CN, params.Deck))
	if len(songs) != 3 || songs[1].Query != "龙" || songs[1].MusicID != eventPlannerLostAndFoundMusicID ||
		songs[2].Query != "野车" || songs[2].MusicID != eventPlannerOmakaseMusicID {
		t.Fatalf("unexpected default songs: %+v", songs)
	}
}

func TestEventPlannerOmakaseSongDoesNotRequireMusicController(t *testing.T) {
	deckCtrl := newHandlerTestDeckController(t)
	baseQuery := buildEventPlannerBaseDeckQuery(renderregion.JP, deckAutoQueryParams{MaxProfile: true})
	req, err := buildEventPlannerSongDeckRequest(
		deckCtrl,
		&renderapp.App{},
		renderregion.JP,
		7,
		baseQuery,
		eventPlannerSongSelection{
			Query:      "野车",
			Difficulty: "master",
			MusicID:    eventPlannerOmakaseMusicID,
		},
	)
	if err != nil {
		t.Fatalf("buildEventPlannerSongDeckRequest() error = %v", err)
	}
	if req.MusicID == nil || *req.MusicID != eventPlannerOmakaseMusicID {
		t.Fatalf("unexpected music id: %+v", req.MusicID)
	}
	if req.MusicTitle == nil || !strings.Contains(*req.MusicTitle, "おまかせ") {
		t.Fatalf("unexpected music title: %+v", req.MusicTitle)
	}
	if req.MusicCoverPath == nil || *req.MusicCoverPath != "static_images/omakase.png" {
		t.Fatalf("unexpected music cover path: %+v", req.MusicCoverPath)
	}
	if len(req.DeckData) != 1 {
		t.Fatalf("unexpected deck data: %+v", req.DeckData)
	}
}

func TestEventPlannerDefaultsToRLAlgorithm(t *testing.T) {
	params, err := parseEventPlannerParams("pt1200w", "/cn活动规划")
	if err != nil {
		t.Fatalf("parseEventPlannerParams() error = %v", err)
	}
	query := buildEventPlannerBaseDeckQuery(renderregion.CN, params.Deck)
	if query.Algorithm != "rl" {
		t.Fatalf("unexpected default algorithm: %q", query.Algorithm)
	}
}

func TestEventPlannerKeepsExplicitAlgorithm(t *testing.T) {
	params, err := parseEventPlannerParams("pt1200w dfs", "/cn活动规划")
	if err != nil {
		t.Fatalf("parseEventPlannerParams() error = %v", err)
	}
	query := buildEventPlannerBaseDeckQuery(renderregion.CN, params.Deck)
	if query.Algorithm != "dfs" {
		t.Fatalf("unexpected explicit algorithm: %q", query.Algorithm)
	}
}

func TestEventPlannerBoostMultiplierMatchesDeckBoostDisplay(t *testing.T) {
	tests := []struct {
		boost int
		want  int64
	}{
		{boost: 0, want: 1},
		{boost: 1, want: 5},
		{boost: 2, want: 10},
		{boost: 3, want: 15},
		{boost: 4, want: 20},
		{boost: 5, want: 25},
		{boost: 6, want: 27},
		{boost: 7, want: 29},
		{boost: 8, want: 31},
		{boost: 9, want: 33},
		{boost: 10, want: 35},
	}
	for _, tc := range tests {
		if got := eventPlannerBoostMultiplier(tc.boost); got != tc.want {
			t.Fatalf("eventPlannerBoostMultiplier(%d) = %d, want %d", tc.boost, got, tc.want)
		}
	}
}

func TestEventPlannerDailyPointUsesRemainingEventTimeWhenCurrentPointKnown(t *testing.T) {
	dayMillis := int64(24 * time.Hour / time.Millisecond)
	startAt := int64(1_000_000)
	aggregateAt := startAt + 10*dayMillis
	now := startAt + 4*dayMillis

	got := eventPlannerDailyPoint(10_000_000, 1_000_000, startAt, aggregateAt, now, true)
	want := int64(1_500_000)
	if got != want {
		t.Fatalf("eventPlannerDailyPoint() = %d, want %d", got, want)
	}
}

func TestEventPlannerDailyPointUsesFullEventTimeWhenCurrentPointUnknown(t *testing.T) {
	dayMillis := int64(24 * time.Hour / time.Millisecond)
	startAt := int64(1_000_000)
	aggregateAt := startAt + 10*dayMillis
	now := startAt + 4*dayMillis

	got := eventPlannerDailyPoint(10_000_000, 1_000_000, startAt, aggregateAt, now, false)
	want := int64(1_000_000)
	if got != want {
		t.Fatalf("eventPlannerDailyPoint() = %d, want %d", got, want)
	}
}

func TestEventPlannerFixedCardIDsDoNotBecomeTargetPoint(t *testing.T) {
	_, err := parseEventPlannerParams("#12345 23456 34567 45678 56789 歌 虾 5火", "/cn活动规划")
	if err == nil || !strings.Contains(err.Error(), "需要提供目标 pt") {
		t.Fatalf("expected missing target error, got %v", err)
	}
	if strings.Contains(err.Error(), "活动规划用法") {
		t.Fatalf("expected concise missing target error, got %v", err)
	}
}

func TestEventPlannerParsesDeckLikeOptions(t *testing.T) {
	params, err := parseEventPlannerParams("pt1000w 当前pt100w wl3 mzk 歌 野车 10火 队友综合25w 队友实效200", "/jp活动规划")
	if err != nil {
		t.Fatalf("parseEventPlannerParams() error = %v", err)
	}
	if params.TargetPoint != 10_000_000 || params.CurrentPoint != 1_000_000 || !params.CurrentPointSet {
		t.Fatalf("unexpected point params: %+v", params)
	}
	if params.Deck.WorldBloomEventTurn == nil || *params.Deck.WorldBloomEventTurn != 3 || params.Deck.WorldBloomCharacterID == nil || *params.Deck.WorldBloomCharacterID <= 0 {
		t.Fatalf("unexpected wl params: %+v", params.Deck)
	}
	if params.Deck.MultiLiveTeammatePower == nil || *params.Deck.MultiLiveTeammatePower != 250_000 {
		t.Fatalf("unexpected teammate power: %+v", params.Deck.MultiLiveTeammatePower)
	}
	if params.Deck.MultiLiveTeammateScoreUp == nil || *params.Deck.MultiLiveTeammateScoreUp != 200 {
		t.Fatalf("unexpected teammate score up: %+v", params.Deck.MultiLiveTeammateScoreUp)
	}
	if len(params.Boosts) != 1 || params.Boosts[0] != 10 {
		t.Fatalf("unexpected boosts: %+v", params.Boosts)
	}
	if len(params.Songs) != 1 || params.Songs[0].Query != "野车" || params.Songs[0].MusicID != eventPlannerOmakaseMusicID {
		t.Fatalf("unexpected parsed songs: %+v", params.Songs)
	}
	songs := eventPlannerSongsForRequest(params, buildEventPlannerBaseDeckQuery(renderregion.JP, params.Deck))
	if len(songs) != 1 || songs[0].MusicID != eventPlannerOmakaseMusicID {
		t.Fatalf("unexpected resolved songs: %+v", songs)
	}
}

func TestEventPlannerParsesTotalRankingOption(t *testing.T) {
	params, err := parseEventPlannerParams("t100 总榜 wl3 mzk", "/jp活动规划")
	if err != nil {
		t.Fatalf("parseEventPlannerParams() error = %v", err)
	}
	if params.TargetRank != 100 || !params.TotalRanking {
		t.Fatalf("unexpected total ranking params: %+v", params)
	}
	if params.Deck.WorldBloomEventTurn == nil || *params.Deck.WorldBloomEventTurn != 3 || params.Deck.WorldBloomCharacterID == nil || *params.Deck.WorldBloomCharacterID <= 0 {
		t.Fatalf("unexpected wl params: %+v", params.Deck)
	}
}

func TestEventPlannerCurrentPointUsesWorldBloomTracker(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{"server":"jp","eventId":170,"scope":"world_bloom","characterId":17,"fetchedAt":1},"ranks":[{"rank":42,"userId":"12345678901234","name":"tester","score":765432,"timestamp":1}]}`))
	}))
	defer server.Close()

	point, known, warning := resolveEventPlannerCurrentPoint(
		eventPlannerTrackerTestContext(server.URL),
		&accountdata.ResolvedBinding{PJSKUserID: "12345678901234"},
		renderregion.JP,
		&masterdata.Event{ID: 170, EventType: "world_bloom"},
		renderdeck.AutoQuery{WorldBloomCharacterID: drawing.IntPtr(17)},
		eventPlannerCommandParams{},
	)
	if gotPath != "/api/v2/cloud/events/jp/170/leaderboards/world-bloom/17/sk/query" {
		t.Fatalf("unexpected tracker path: %s", gotPath)
	}
	if point != 765432 || !known || warning != "" {
		t.Fatalf("unexpected current point result: point=%d known=%v warning=%q", point, known, warning)
	}
}

func TestEventPlannerCurrentPointTotalRankingUsesNormalTracker(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{"server":"jp","eventId":170,"scope":"total","fetchedAt":1},"ranks":[{"rank":88,"userId":"12345678901234","name":"tester","score":654321,"timestamp":1}]}`))
	}))
	defer server.Close()

	point, known, warning := resolveEventPlannerCurrentPoint(
		eventPlannerTrackerTestContext(server.URL),
		&accountdata.ResolvedBinding{PJSKUserID: "12345678901234"},
		renderregion.JP,
		&masterdata.Event{ID: 170, EventType: "world_bloom"},
		renderdeck.AutoQuery{WorldBloomCharacterID: drawing.IntPtr(17)},
		eventPlannerCommandParams{TotalRanking: true},
	)
	if gotPath != "/api/v2/cloud/events/jp/170/leaderboards/total/sk/query" {
		t.Fatalf("unexpected tracker path: %s", gotPath)
	}
	if point != 654321 || !known || warning != "" {
		t.Fatalf("unexpected current point result: point=%d known=%v warning=%q", point, known, warning)
	}
}

func TestEventPlannerCurrentPointFallsBackToZeroWhenTrackerMisses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	point, known, warning := resolveEventPlannerCurrentPoint(
		eventPlannerTrackerTestContext(server.URL),
		&accountdata.ResolvedBinding{PJSKUserID: "12345678901234"},
		renderregion.JP,
		&masterdata.Event{ID: 170},
		renderdeck.AutoQuery{},
		eventPlannerCommandParams{},
	)
	if point != 0 || !known || !strings.Contains(warning, "前100") {
		t.Fatalf("unexpected current point fallback: point=%d known=%v warning=%q", point, known, warning)
	}
}

func TestEventPlannerTargetRankUsesWorldBloomRankingByDefault(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{"server":"jp","eventId":170,"scope":"world_bloom","characterId":17,"fetchedAt":1},"ranks":[{"rank":100,"score":456789,"timestamp":1}]}`))
	}))
	defer server.Close()

	point, source, err := resolveEventPlannerTargetPoint(
		eventPlannerTrackerTestContext(server.URL),
		renderregion.JP,
		&masterdata.Event{ID: 170, EventType: "world_bloom"},
		renderdeck.AutoQuery{WorldBloomCharacterID: drawing.IntPtr(17)},
		eventPlannerCommandParams{TargetRank: 100},
	)
	if err != nil {
		t.Fatalf("resolveEventPlannerTargetPoint() error = %v", err)
	}
	if gotPath != "/api/v2/cloud/events/jp/170/leaderboards/world-bloom/17/sk/line" {
		t.Fatalf("unexpected tracker path: %s", gotPath)
	}
	if point != 456789 || !strings.Contains(source, "WL章节") {
		t.Fatalf("unexpected target result: point=%d source=%q", point, source)
	}
}

func TestEventPlannerTargetRankTotalRankingUsesNormalRanking(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{"server":"jp","eventId":170,"scope":"total","fetchedAt":1},"ranks":[{"rank":100,"score":987654,"timestamp":1}]}`))
	}))
	defer server.Close()

	point, source, err := resolveEventPlannerTargetPoint(
		eventPlannerTrackerTestContext(server.URL),
		renderregion.JP,
		&masterdata.Event{ID: 170, EventType: "world_bloom"},
		renderdeck.AutoQuery{WorldBloomCharacterID: drawing.IntPtr(17)},
		eventPlannerCommandParams{TargetRank: 100, TotalRanking: true},
	)
	if err != nil {
		t.Fatalf("resolveEventPlannerTargetPoint() error = %v", err)
	}
	if gotPath != "/api/v2/cloud/events/jp/170/leaderboards/total/sk/line" {
		t.Fatalf("unexpected tracker path: %s", gotPath)
	}
	if point != 987654 || strings.Contains(source, "WL章节") {
		t.Fatalf("unexpected target result: point=%d source=%q", point, source)
	}
}

func TestEventPlannerExplicitCurrentPointSkipsTracker(t *testing.T) {
	point, known, warning := resolveEventPlannerCurrentPoint(
		nil,
		nil,
		renderregion.JP,
		&masterdata.Event{ID: 170, EventType: "world_bloom"},
		renderdeck.AutoQuery{WorldBloomCharacterID: drawing.IntPtr(17)},
		eventPlannerCommandParams{CurrentPoint: 123456, CurrentPointSet: true},
	)
	if point != 123456 || !known || warning != "" {
		t.Fatalf("unexpected explicit current point: point=%d known=%v warning=%q", point, known, warning)
	}
}

func eventPlannerTrackerTestContext(baseURL string) *RequestContext {
	return &RequestContext{
		Ctx: context.Background(),
		App: &renderapp.App{
			Tracker: sekaiapi.NewTrackerClient(&config.TrackerConfig{BaseURL: baseURL}),
		},
	}
}

func TestEventPlannerKeepsLiveTypeAfterSongMarker(t *testing.T) {
	params, err := parseEventPlannerParams("pt1000w 歌 虾ex solo", "/jp活动规划")
	if err != nil {
		t.Fatalf("parseEventPlannerParams() error = %v", err)
	}
	if params.Deck.LiveType != "solo" {
		t.Fatalf("expected solo live type, got %+v", params.Deck)
	}
	if len(params.Songs) != 1 || params.Songs[0].Query != "虾" || params.Songs[0].Difficulty != "expert" {
		t.Fatalf("unexpected songs: %+v", params.Songs)
	}
}

func TestExecuteEventRecordReturnsBindingErrorBeforeSuiteMessage(t *testing.T) {
	_, err := executeEvent(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleEvent,
		Mode:              "event-record",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Events:   renderevent.NewController(nil, nil, nil),
		Bindings: newHandlerTestBindingService(t),
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	var replyErr onebot11.ReplayError
	if !errors.As(WrapDomainError(err), &replyErr) {
		t.Fatalf("expected ReplayError, got %T (%v)", err, err)
	}
	if string(replyErr) != ErrMsgBindingNotFound {
		t.Fatalf("unexpected replay error: %q", replyErr)
	}
}

func TestExecuteEventRecordReturnsContextualSuiteMessageWhenSnapshotMissing(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	_, err := executeEvent(NewRequestContext(ctx, &CommandRequest{
		Module:            parser.ModuleEvent,
		Mode:              "event-record",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Events:   renderevent.NewController(nil, nil, nil),
		Bindings: service,
	}))
	if err == nil || err.Error() != buildPrivateDataNotFoundMessage("suite", &accountdata.ResolvedBinding{
		Server:     "jp",
		PJSKUserID: "12345678901234",
		Visible:    false,
	}) {
		t.Fatalf("unexpected error: %v", err)
	}
}
