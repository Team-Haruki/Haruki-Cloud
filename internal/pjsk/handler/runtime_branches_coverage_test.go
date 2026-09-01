package handler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderprovider "haruki-cloud/internal/pjsk/render/provider"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
)

func TestSKHandlerFactoriesAndTrackerExecutionBranches(t *testing.T) {
	ctx := HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{
			Context:    context.Background(),
			Platform:   "qq",
			UserId:     "100",
			TriggerCmd: "/sk",
			ArgText:    "100",
		},
		region: renderregion.JP,
		flags:  map[string]bool{},
	}
	handlers := []HarukiSekaiCommandHandler{
		(sekaiHandlers{}).SKLineHandle(),
		(sekaiHandlers{}).SKQueryHandle(),
		(sekaiHandlers{}).SKSpeedHandle(),
		(sekaiHandlers{}).SKCheckRoomHandle(),
		(sekaiHandlers{}).SKCheckRoomLiteHandle(),
		(sekaiHandlers{}).SKPlayerTraceHandle(),
		(sekaiHandlers{}).SKRankTraceHandle(),
		(sekaiHandlers{}).SKDailySpeedHandle(),
		(sekaiHandlers{}).SKPredictHandle(),
		(sekaiHandlers{}).SKBoardHandle(),
		(sekaiHandlers{}).CSBHandle(),
		(sekaiHandlers{}).WinratePredictHandle(),
	}
	for _, handler := range handlers {
		testutil.RequireArgs(t, !(handler.handleFunc == nil), "SK handler has nil handle function")

		request, err := handler.handleFunc(ctx)
		testutil.Check(t, !(err != nil || request == nil), "SK handler %v returned request=%v err=%v", handler.Commands, request, err)

	}

	app, _ := newExecutionCoverageApp(t)
	params, err := json.Marshal(rendersk.TrackerRankQuery{Region: "jp", Ranks: []int{100}})
	testutil.RequireArgs(t, !(err != nil), err)

	for _, mode := range []string{"sk-line", "sk-query", "sk-check-room", "sk-csb", "sk-speed", "sk-daily-speed", "sk-player-trace", "sk-rank-trace", "sk-predict"} {
		rc := &RequestContext{
			Ctx: context.Background(),
			Cmd: &CommandRequest{
				Mode:              mode,
				Region:            "jp",
				RequesterPlatform: "qq",
				RequesterUserID:   "100",
				Params:            params,
			},
			App:            app,
			Region:         renderregion.JP,
			RegionStr:      "jp",
			Platform:       "qq",
			PlatformUserID: "100",
		}
		_, _ = executeSKMode(rc, app.SK)
	}
	{

		_, err := skImageResult(nil, errors.New("boom"))
		testutil.RequireArgs(t, !(err == nil), "skImageResult dropped error")
	}

	selfRC := &RequestContext{Cmd: &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "1"}}
	uid := int64(123)
	for _, tt := range []struct {
		req  rendersk.TrackerRankQuery
		want bool
	}{
		{rendersk.TrackerRankQuery{}, true},
		{rendersk.TrackerRankQuery{TargetPlatform: "QQ", TargetUserID: "1"}, true},
		{rendersk.TrackerRankQuery{TargetPlatform: "qq", TargetUserID: "2"}, false},
		{rendersk.TrackerRankQuery{UserID: &uid}, false},
		{rendersk.TrackerRankQuery{Ranks: []int{1}}, false},
	} {
		{
			got := isSKSelfTrackerQuery(selfRC, tt.req)
			testutil.Check(t, !(got != tt.want), "isSKSelfTrackerQuery(%+v) = %v", tt.req, got)
		}

	}
	testutil.RequireArgs(t, !(isSKSelfTrackerQuery(nil, rendersk.TrackerRankQuery{})), "nil context unexpectedly treated as self")
	testutil.RequireArgs(t, !(normalizeSKSelfRankingNotFoundError(false, "jp", sekaiapi.ErrRankingNotFound) != sekaiapi.ErrRankingNotFound), "non-self ranking error changed")
	testutil.RequireArgs(t, !(normalizeSKSelfRankingNotFoundError(true, "jp", nil) != nil), "nil ranking error changed")
	{

		err := normalizeSKSelfRankingNotFoundError(true, "jp", sekaiapi.ErrRankingNotFound)
		{
			testutil.Require(t, !(err == nil), "self ranking error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "JP"), "self ranking error = %v", err)
		}
	}
	{

		query, ok := trackerRankQueryFromParams(nil)
		{
			testutil.RequireArgs(t, !(ok), "nil tracker params unexpectedly accepted")
			testutil.RequireArgs(t, !(len(query.Ranks) != 0), "nil tracker params unexpectedly accepted")
		}
	}
	{

		query, ok := trackerRankQueryFromParams(&CommandRequest{Params: []byte("bad")})
		{
			testutil.RequireArgs(t, !(ok), "invalid tracker params unexpectedly accepted")
			testutil.RequireArgs(t, !(len(query.Ranks) != 0), "invalid tracker params unexpectedly accepted")
		}
	}

	empty, _ := json.Marshal(rendersk.TrackerRankQuery{})
	{
		_, ok := trackerRankQueryFromParams(&CommandRequest{Params: empty})
		testutil.RequireArgs(t, !(ok), "empty tracker query unexpectedly accepted")
	}

	request := rendersk.TrackerRankQuery{WlCharacterID: drawing.IntPtr(-1)}
	{
		err := resolveTrackerCharacterSelection(context.Background(), nil, &request)
		{
			testutil.Require(t, !(err != nil), "invalid WL id was not cleared: %+v, %v", request, err)
			testutil.Require(t, !(request.WlCharacterID != nil), "invalid WL id was not cleared: %+v, %v", request, err)
		}
	}
	{

		err := resolveTrackerCharacterSelection(context.Background(), nil, nil)
		testutil.Require(t, !(err != nil), "nil tracker selection failed: %v", err)
	}

	chapter := &struct {
		start int64
		end   int64
	}{start: 100, end: 200}
	_ = chapter
	applyTrackerWorldBloomChapterTiming(nil, nil)
	{
		err := resolveTrackerTargetUser(context.Background(), nil, nil, "", "")
		testutil.Require(t, !(err != nil), "nil tracker target failed: %v", err)
	}

	request = rendersk.TrackerRankQuery{UserID: &uid}
	{
		err := resolveTrackerTargetUser(context.Background(), nil, &request, "", "")
		testutil.Require(t, !(err != nil), "resolved tracker target failed: %v", err)
	}

	request = rendersk.TrackerRankQuery{TargetPlatform: "qq", TargetUserID: "1"}
	{
		err := resolveTrackerTargetUser(context.Background(), nil, &request, "", "")
		testutil.Require(t, errors.Is(err, accountdata.ErrBindingServiceUnavailable), "unconfigured binding target error = %v", err)
	}

}

func TestEventPlannerExecutionAndRequestBranches(t *testing.T) {
	ctx := HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{Context: context.Background(), TriggerCmd: "/活动规划", ArgText: "pt100w"},
		region:             renderregion.JP,
		flags:              map[string]bool{},
	}
	handler := (sekaiHandlers{}).EventPlannerHandle()
	{
		request, err := handler.handleFunc(ctx)
		{
			testutil.Require(t, !(err != nil), "event planner handler = %+v, %v", request, err)
			testutil.Require(t, !(request == nil), "event planner handler = %+v, %v", request, err)
			testutil.Require(t, !(request.Mode != "event-planner"), "event planner handler = %+v, %v", request, err)
		}
	}

	ctx.flags["is_help"] = true
	{
		request, err := handler.handleFunc(ctx)
		{
			testutil.Require(t, !(err != nil), "event planner help handler = %+v, %v", request, err)
			testutil.Require(t, !(request == nil), "event planner help handler = %+v, %v", request, err)
			testutil.Require(t, !(request.Mode != "event-planner-help"), "event planner help handler = %+v, %v", request, err)
		}
	}

	rc := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{Mode: "event-planner", Region: "jp"}}
	{
		_, err := executeEventPlanner(rc)
		testutil.RequireArgs(t, !(err == nil), "nil planner app unexpectedly succeeded")
	}

	rc.App = &renderapp.App{Decks: &renderdeck.Controller{}}
	{
		_, err := executeEventPlanner(rc)
		testutil.RequireArgs(t, !(err == nil), "nil planner music unexpectedly succeeded")
	}

	baseApp, _ := newExecutionCoverageApp(t)
	rc.App = &renderapp.App{Decks: baseApp.Decks, Music: baseApp.Music}
	{
		_, err := executeEventPlanner(rc)
		testutil.RequireArgs(t, !(err == nil), "nil planner drawing unexpectedly succeeded")
	}

	rc.App = baseApp
	{
		_, err := executeEventPlanner(rc)
		testutil.RequireArgs(t, !(err == nil), "missing planner snapshot unexpectedly succeeded")
	}

	rc.Platform, rc.PlatformUserID = "qq", "1"
	rc.bindingErr = accountdata.ErrNoBinding
	rc.bindingOnce.Do(func() {})
	{
		_, err := executeEventPlanner(rc)
		testutil.RequireArgs(t, !(err == nil), "missing planner binding unexpectedly succeeded")
	}

	rc = &RequestContext{
		Ctx:            context.Background(),
		Cmd:            &CommandRequest{Mode: "event-planner", Region: "jp"},
		App:            baseApp,
		Region:         renderregion.JP,
		RegionStr:      "jp",
		Platform:       "qq",
		PlatformUserID: "1",
		binding:        &accountdata.ResolvedBinding{PJSKUserID: "1234567890", SuiteVisible: false},
	}
	rc.bindingOnce.Do(func() {})
	{
		_, err := executeEventPlanner(rc)
		testutil.RequireArgs(t, !(err == nil), "hidden planner suite unexpectedly succeeded")
	}

	eventInfo := &masterdata.Event{
		ID:              7,
		Name:            "Planner Event",
		AssetBundleName: "event_7",
		StartAt:         time.Now().Add(-24 * time.Hour).UnixMilli(),
		AggregateAt:     time.Now().Add(6 * 24 * time.Hour).UnixMilli(),
	}
	{
		_, err := buildEventPlannerDrawingRequest(rc, renderregion.JP, nil, &runtimeSnapshotStub{}, renderdeck.AutoQuery{}, eventPlannerCommandParams{}, nil, 100, "input", 0, true)
		testutil.RequireArgs(t, !(err == nil), "nil planner event unexpectedly accepted")
	}
	{

		_, err := buildEventPlannerDrawingRequest(rc, renderregion.JP, eventInfo, &runtimeSnapshotStub{}, renderdeck.AutoQuery{}, eventPlannerCommandParams{}, nil, 100, "input", 200, true)
		testutil.RequireArgs(t, !(err == nil), "empty planner songs unexpectedly accepted")
	}

	params := eventPlannerCommandParams{Boosts: []int{1, 10}}
	selection := []eventPlannerSongSelection{{Query: "野车", Difficulty: "master", MusicID: eventPlannerOmakaseMusicID}}
	{
		_, err := buildEventPlannerDrawingRequest(rc, renderregion.JP, eventInfo, &runtimeSnapshotStub{}, renderdeck.AutoQuery{}, params, selection, 100, "input", 0, true)
		testutil.RequireArgs(t, !(err == nil), "unconfigured planner deck unexpectedly succeeded")
	}

	base := buildEventPlannerBaseDeckQuery(renderregion.TW, deckAutoQueryParams{UseCurrentDeck: true})
	{
		testutil.Require(t, base.UseExactCardState, "planner base defaults = %+v", base)
		testutil.Require(t, !(base.Algorithm != "rl"), "planner base defaults = %+v", base)
		testutil.Require(t, !(base.LiveType != "multi"), "planner base defaults = %+v", base)
		testutil.Require(t, !(base.Target != "score"), "planner base defaults = %+v", base)
	}

	for _, songs := range [][]eventPlannerSongSelection{
		eventPlannerSongsForRequest(eventPlannerCommandParams{Songs: []eventPlannerSongSelection{{Query: "虾"}}}, renderdeck.AutoQuery{}),
		eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{MusicID: drawing.IntPtr(123), MusicQuery: "song"}),
		eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{MusicQuery: "野车"}),
		eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{}),
	} {
		testutil.RequireArgs(t, !(len(songs) == 0), "planner song source returned no songs")

	}
	{
		testutil.RequireArgs(t, !(eventPlannerEventBannerPath(nil, renderregion.JP, eventInfo) != ""), "nil planner banner returned a path")
		testutil.RequireArgs(t, !(eventPlannerEventBannerPath(baseApp, renderregion.JP, nil) != ""), "nil planner banner returned a path")
	}

	for _, tt := range []struct {
		target, current, start, aggregate, now int64
		known                                  bool
	}{
		{0, 0, 0, 1, 0, false},
		{100, 0, 10, 10, 10, false},
		{100, 200, 0, 100, 50, true},
		{100, 0, 0, 100, 200, true},
	} {
		_ = eventPlannerDailyPoint(tt.target, tt.current, tt.start, tt.aggregate, tt.now, tt.known)
	}
}

func TestEventPlannerProviderTargetAndCurrentPointBranches(t *testing.T) {
	ctx := context.Background()
	{
		_, _, err := resolveEventPlannerEvent(ctx, nil, renderregion.JP, 0)
		testutil.RequireArgs(t, !(err == nil), "nil planner provider unexpectedly resolved")
	}

	now := time.Now()
	events := &bridgeDeckTestEventProvider{events: []*masterdata.Event{
		{ID: 1, Name: "Old", StartAt: now.Add(-2 * time.Hour).UnixMilli(), AggregateAt: now.Add(time.Hour).UnixMilli()},
		{ID: 2, Name: "Current", StartAt: now.Add(-time.Hour).UnixMilli(), AggregateAt: now.Add(2 * time.Hour).UnixMilli()},
		nil,
	}}
	provider := bridgeDeckTestMasterProvider{region: renderregion.JP, events: events}
	app := &renderapp.App{
		Provider: provider,
		Providers: map[renderregion.Value]renderprovider.MasterDataProvider{
			renderregion.JP: provider,
		},
	}
	{
		event, warning, err := resolveEventPlannerEvent(ctx, app, renderregion.JP, 2)
		{
			testutil.Require(t, !(err != nil), "explicit planner event = %+v, %q, %v", event, warning, err)
			testutil.Require(t, !(event == nil), "explicit planner event = %+v, %q, %v", event, warning, err)
			testutil.Require(t, !(event.ID != 2), "explicit planner event = %+v, %q, %v", event, warning, err)
			testutil.Require(t, !(warning != ""), "explicit planner event = %+v, %q, %v", event, warning, err)
		}
	}
	{

		_, _, err := resolveEventPlannerEvent(ctx, app, renderregion.JP, 999)
		testutil.RequireArgs(t, !(err == nil), "missing planner event unexpectedly resolved")
	}
	{

		event, warning, err := resolveEventPlannerEvent(ctx, app, renderregion.JP, 0)
		{
			testutil.Require(t, !(err != nil), "current planner event = %+v, %q, %v", event, warning, err)
			testutil.Require(t, !(event == nil), "current planner event = %+v, %q, %v", event, warning, err)
			testutil.Require(t, !(event.ID != 2), "current planner event = %+v, %q, %v", event, warning, err)
			testutil.Require(t, !(warning == ""), "current planner event = %+v, %q, %v", event, warning, err)
		}
	}

	closedProvider := bridgeDeckTestMasterProvider{region: renderregion.JP, events: &bridgeDeckTestEventProvider{events: []*masterdata.Event{{ID: 3, StartAt: now.Add(-2 * time.Hour).UnixMilli(), AggregateAt: now.Add(-time.Hour).UnixMilli()}}}}
	{
		_, _, err := resolveEventPlannerEvent(ctx, &renderapp.App{Provider: closedProvider}, renderregion.JP, 0)
		testutil.RequireArgs(t, !(err == nil), "closed planner event unexpectedly current")
	}

	turn := 3
	simulated, warning, err := resolveEventPlannerEventFromQuery(ctx, nil, renderregion.JP, renderdeck.AutoQuery{WorldBloomEventTurn: &turn})
	{
		testutil.Require(t, !(err != nil), "simulated WL planner event = %+v, %q, %v", simulated, warning, err)
		testutil.Require(t, !(simulated == nil), "simulated WL planner event = %+v, %q, %v", simulated, warning, err)
		testutil.Require(t, !(simulated.EventType != "world_bloom"), "simulated WL planner event = %+v, %q, %v", simulated, warning, err)
		testutil.Require(t, !(warning == ""), "simulated WL planner event = %+v, %q, %v", simulated, warning, err)
	}
	{
		testutil.RequireArgs(t, !(eventPlannerSimulatedEventName(renderdeck.AutoQuery{}) != "模拟活动"), "regular simulated planner labels mismatch")
		testutil.RequireArgs(t, !(eventPlannerSimulatedEventType(renderdeck.AutoQuery{}) != ""), "regular simulated planner labels mismatch")
	}
	testutil.RequireArgs(t, !(eventPlannerProvider(nil, renderregion.JP) != nil), "nil event planner provider unexpectedly resolved")
	{

		got := eventPlannerProvider(app, renderregion.Unknown)
		{
			testutil.Require(t, !(got == nil), "fallback planner provider = %#v", got)
			testutil.Require(t, !(got.Region() != renderregion.JP), "fallback planner provider = %#v", got)
		}
	}

	rc := &RequestContext{Ctx: ctx, App: &renderapp.App{}}
	eventInfo := &masterdata.Event{ID: 7, EventType: "world_bloom"}
	params := eventPlannerCommandParams{TargetPoint: 123}
	{
		point, source, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, params)
		{
			testutil.Require(t, !(err != nil), "direct planner target = %d, %q, %v", point, source, err)
			testutil.Require(t, !(point != 123), "direct planner target = %d, %q, %v", point, source, err)
			testutil.Require(t, !(source == ""), "direct planner target = %d, %q, %v", point, source, err)
		}
	}
	{

		_, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, eventPlannerCommandParams{})
		testutil.RequireArgs(t, !(err == nil), "missing planner target unexpectedly accepted")
	}
	{

		_, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, &masterdata.Event{}, renderdeck.AutoQuery{}, eventPlannerCommandParams{TargetRank: 100})
		testutil.RequireArgs(t, !(err == nil), "simulated rank target unexpectedly accepted")
	}
	{

		_, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, eventPlannerCommandParams{TargetRank: 100})
		testutil.RequireArgs(t, !(err == nil), "rank target without tracker unexpectedly accepted")
	}
	{

		point, known, warning := resolveEventPlannerCurrentPoint(nil, nil, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, eventPlannerCommandParams{CurrentPointSet: true, CurrentPoint: 50})
		{
			testutil.Require(t, !(point != 50), "explicit current point = %d, %v, %q", point, known, warning)
			testutil.Require(t, known, "explicit current point = %d, %v, %q", point, known, warning)
			testutil.Require(t, !(warning != ""), "explicit current point = %d, %v, %q", point, known, warning)
		}
	}
	{

		_, _, warning := resolveEventPlannerCurrentPoint(rc, nil, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, eventPlannerCommandParams{})
		testutil.RequireArgs(t, !(warning == ""), "missing tracker current-point warning omitted")
	}

	for _, binding := range []*accountdata.ResolvedBinding{nil, {PJSKUserID: "bad"}, {PJSKUserID: "1234567890"}} {
		_, _ = eventPlannerBindingUID(binding)
	}
	metadataID := 22
	for _, query := range []renderdeck.AutoQuery{{}, {WorldBloomCharacterID: drawing.IntPtr(21)}, {MetadataWorldBloomCharacterID: &metadataID}} {
		_, _ = eventPlannerWorldBloomCharacterID(query)
	}
	_ = eventPlannerCurrentPointTrackerWarning(sekaiapi.ErrRankingNotFound, false)
	_ = eventPlannerCurrentPointTrackerWarning(errors.New("boom"), true)
	{
		testutil.RequireArgs(t, eventPlannerUseWorldBloomRanking(eventInfo, eventPlannerCommandParams{}), "planner WL ranking selection mismatch")
		testutil.RequireArgs(t, !(eventPlannerUseWorldBloomRanking(eventInfo, eventPlannerCommandParams{TotalRanking: true})), "planner WL ranking selection mismatch")
	}

}
