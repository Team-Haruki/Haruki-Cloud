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
		if handler.handleFunc == nil {
			t.Fatal("SK handler has nil handle function")
		}
		request, err := handler.handleFunc(ctx)
		if err != nil || request == nil {
			t.Errorf("SK handler %v returned request=%v err=%v", handler.Commands, request, err)
		}
	}

	app, _ := newExecutionCoverageApp(t)
	params, err := json.Marshal(rendersk.TrackerRankQuery{Region: "jp", Ranks: []int{100}})
	if err != nil {
		t.Fatal(err)
	}
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

	if _, err := skImageResult(nil, errors.New("boom")); err == nil {
		t.Fatal("skImageResult dropped error")
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
		if got := isSKSelfTrackerQuery(selfRC, tt.req); got != tt.want {
			t.Errorf("isSKSelfTrackerQuery(%+v) = %v", tt.req, got)
		}
	}
	if isSKSelfTrackerQuery(nil, rendersk.TrackerRankQuery{}) {
		t.Fatal("nil context unexpectedly treated as self")
	}
	if normalizeSKSelfRankingNotFoundError(false, "jp", sekaiapi.ErrRankingNotFound) != sekaiapi.ErrRankingNotFound {
		t.Fatal("non-self ranking error changed")
	}
	if normalizeSKSelfRankingNotFoundError(true, "jp", nil) != nil {
		t.Fatal("nil ranking error changed")
	}
	if err := normalizeSKSelfRankingNotFoundError(true, "jp", sekaiapi.ErrRankingNotFound); err == nil || !strings.Contains(err.Error(), "JP") {
		t.Fatalf("self ranking error = %v", err)
	}

	if query, ok := trackerRankQueryFromParams(nil); ok || len(query.Ranks) != 0 {
		t.Fatal("nil tracker params unexpectedly accepted")
	}
	if query, ok := trackerRankQueryFromParams(&CommandRequest{Params: []byte("bad")}); ok || len(query.Ranks) != 0 {
		t.Fatal("invalid tracker params unexpectedly accepted")
	}
	empty, _ := json.Marshal(rendersk.TrackerRankQuery{})
	if _, ok := trackerRankQueryFromParams(&CommandRequest{Params: empty}); ok {
		t.Fatal("empty tracker query unexpectedly accepted")
	}

	request := rendersk.TrackerRankQuery{WlCharacterID: drawing.IntPtr(-1)}
	if err := resolveTrackerCharacterSelection(context.Background(), nil, &request); err != nil || request.WlCharacterID != nil {
		t.Fatalf("invalid WL id was not cleared: %+v, %v", request, err)
	}
	if err := resolveTrackerCharacterSelection(context.Background(), nil, nil); err != nil {
		t.Fatalf("nil tracker selection failed: %v", err)
	}
	chapter := &struct {
		start int64
		end   int64
	}{start: 100, end: 200}
	_ = chapter
	applyTrackerWorldBloomChapterTiming(nil, nil)
	if err := resolveTrackerTargetUser(context.Background(), nil, nil, "", ""); err != nil {
		t.Fatalf("nil tracker target failed: %v", err)
	}
	request = rendersk.TrackerRankQuery{UserID: &uid}
	if err := resolveTrackerTargetUser(context.Background(), nil, &request, "", ""); err != nil {
		t.Fatalf("resolved tracker target failed: %v", err)
	}
	request = rendersk.TrackerRankQuery{TargetPlatform: "qq", TargetUserID: "1"}
	if err := resolveTrackerTargetUser(context.Background(), nil, &request, "", ""); !errors.Is(err, accountdata.ErrBindingServiceUnavailable) {
		t.Fatalf("unconfigured binding target error = %v", err)
	}
}

func TestEventPlannerExecutionAndRequestBranches(t *testing.T) {
	ctx := HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{Context: context.Background(), TriggerCmd: "/活动规划", ArgText: "pt100w"},
		region:             renderregion.JP,
		flags:              map[string]bool{},
	}
	handler := (sekaiHandlers{}).EventPlannerHandle()
	if request, err := handler.handleFunc(ctx); err != nil || request == nil || request.Mode != "event-planner" {
		t.Fatalf("event planner handler = %+v, %v", request, err)
	}
	ctx.flags["is_help"] = true
	if request, err := handler.handleFunc(ctx); err != nil || request == nil || request.Mode != "event-planner-help" {
		t.Fatalf("event planner help handler = %+v, %v", request, err)
	}

	rc := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{Mode: "event-planner", Region: "jp"}}
	if _, err := executeEventPlanner(rc); err == nil {
		t.Fatal("nil planner app unexpectedly succeeded")
	}
	rc.App = &renderapp.App{Decks: &renderdeck.Controller{}}
	if _, err := executeEventPlanner(rc); err == nil {
		t.Fatal("nil planner music unexpectedly succeeded")
	}
	baseApp, _ := newExecutionCoverageApp(t)
	rc.App = &renderapp.App{Decks: baseApp.Decks, Music: baseApp.Music}
	if _, err := executeEventPlanner(rc); err == nil {
		t.Fatal("nil planner drawing unexpectedly succeeded")
	}
	rc.App = baseApp
	if _, err := executeEventPlanner(rc); err == nil {
		t.Fatal("missing planner snapshot unexpectedly succeeded")
	}
	rc.Platform, rc.PlatformUserID = "qq", "1"
	rc.bindingErr = accountdata.ErrNoBinding
	rc.bindingOnce.Do(func() {})
	if _, err := executeEventPlanner(rc); err == nil {
		t.Fatal("missing planner binding unexpectedly succeeded")
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
	if _, err := executeEventPlanner(rc); err == nil {
		t.Fatal("hidden planner suite unexpectedly succeeded")
	}

	eventInfo := &masterdata.Event{
		ID:              7,
		Name:            "Planner Event",
		AssetBundleName: "event_7",
		StartAt:         time.Now().Add(-24 * time.Hour).UnixMilli(),
		AggregateAt:     time.Now().Add(6 * 24 * time.Hour).UnixMilli(),
	}
	if _, err := buildEventPlannerDrawingRequest(rc, renderregion.JP, nil, &runtimeSnapshotStub{}, renderdeck.AutoQuery{}, eventPlannerCommandParams{}, nil, 100, "input", 0, true); err == nil {
		t.Fatal("nil planner event unexpectedly accepted")
	}
	if _, err := buildEventPlannerDrawingRequest(rc, renderregion.JP, eventInfo, &runtimeSnapshotStub{}, renderdeck.AutoQuery{}, eventPlannerCommandParams{}, nil, 100, "input", 200, true); err == nil {
		t.Fatal("empty planner songs unexpectedly accepted")
	}
	params := eventPlannerCommandParams{Boosts: []int{1, 10}}
	selection := []eventPlannerSongSelection{{Query: "野车", Difficulty: "master", MusicID: eventPlannerOmakaseMusicID}}
	if _, err := buildEventPlannerDrawingRequest(rc, renderregion.JP, eventInfo, &runtimeSnapshotStub{}, renderdeck.AutoQuery{}, params, selection, 100, "input", 0, true); err == nil {
		t.Fatal("unconfigured planner deck unexpectedly succeeded")
	}

	base := buildEventPlannerBaseDeckQuery(renderregion.TW, deckAutoQueryParams{UseCurrentDeck: true})
	if !base.UseExactCardState || base.Algorithm != "rl" || base.LiveType != "multi" || base.Target != "score" {
		t.Fatalf("planner base defaults = %+v", base)
	}
	for _, songs := range [][]eventPlannerSongSelection{
		eventPlannerSongsForRequest(eventPlannerCommandParams{Songs: []eventPlannerSongSelection{{Query: "虾"}}}, renderdeck.AutoQuery{}),
		eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{MusicID: drawing.IntPtr(123), MusicQuery: "song"}),
		eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{MusicQuery: "野车"}),
		eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{}),
	} {
		if len(songs) == 0 {
			t.Fatal("planner song source returned no songs")
		}
	}
	if eventPlannerEventBannerPath(nil, renderregion.JP, eventInfo) != "" || eventPlannerEventBannerPath(baseApp, renderregion.JP, nil) != "" {
		t.Fatal("nil planner banner returned a path")
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
	if _, _, err := resolveEventPlannerEvent(ctx, nil, renderregion.JP, 0); err == nil {
		t.Fatal("nil planner provider unexpectedly resolved")
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
	if event, warning, err := resolveEventPlannerEvent(ctx, app, renderregion.JP, 2); err != nil || event == nil || event.ID != 2 || warning != "" {
		t.Fatalf("explicit planner event = %+v, %q, %v", event, warning, err)
	}
	if _, _, err := resolveEventPlannerEvent(ctx, app, renderregion.JP, 999); err == nil {
		t.Fatal("missing planner event unexpectedly resolved")
	}
	if event, warning, err := resolveEventPlannerEvent(ctx, app, renderregion.JP, 0); err != nil || event == nil || event.ID != 2 || warning == "" {
		t.Fatalf("current planner event = %+v, %q, %v", event, warning, err)
	}
	closedProvider := bridgeDeckTestMasterProvider{region: renderregion.JP, events: &bridgeDeckTestEventProvider{events: []*masterdata.Event{{ID: 3, StartAt: now.Add(-2 * time.Hour).UnixMilli(), AggregateAt: now.Add(-time.Hour).UnixMilli()}}}}
	if _, _, err := resolveEventPlannerEvent(ctx, &renderapp.App{Provider: closedProvider}, renderregion.JP, 0); err == nil {
		t.Fatal("closed planner event unexpectedly current")
	}

	turn := 3
	simulated, warning, err := resolveEventPlannerEventFromQuery(ctx, nil, renderregion.JP, renderdeck.AutoQuery{WorldBloomEventTurn: &turn})
	if err != nil || simulated == nil || simulated.EventType != "world_bloom" || warning == "" {
		t.Fatalf("simulated WL planner event = %+v, %q, %v", simulated, warning, err)
	}
	if eventPlannerSimulatedEventName(renderdeck.AutoQuery{}) != "模拟活动" || eventPlannerSimulatedEventType(renderdeck.AutoQuery{}) != "" {
		t.Fatal("regular simulated planner labels mismatch")
	}
	if eventPlannerProvider(nil, renderregion.JP) != nil {
		t.Fatal("nil event planner provider unexpectedly resolved")
	}
	if got := eventPlannerProvider(app, renderregion.Unknown); got == nil || got.Region() != renderregion.JP {
		t.Fatalf("fallback planner provider = %#v", got)
	}

	rc := &RequestContext{Ctx: ctx, App: &renderapp.App{}}
	eventInfo := &masterdata.Event{ID: 7, EventType: "world_bloom"}
	params := eventPlannerCommandParams{TargetPoint: 123}
	if point, source, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, params); err != nil || point != 123 || source == "" {
		t.Fatalf("direct planner target = %d, %q, %v", point, source, err)
	}
	if _, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, eventPlannerCommandParams{}); err == nil {
		t.Fatal("missing planner target unexpectedly accepted")
	}
	if _, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, &masterdata.Event{}, renderdeck.AutoQuery{}, eventPlannerCommandParams{TargetRank: 100}); err == nil {
		t.Fatal("simulated rank target unexpectedly accepted")
	}
	if _, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, eventPlannerCommandParams{TargetRank: 100}); err == nil {
		t.Fatal("rank target without tracker unexpectedly accepted")
	}

	if point, known, warning := resolveEventPlannerCurrentPoint(nil, nil, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, eventPlannerCommandParams{CurrentPointSet: true, CurrentPoint: 50}); point != 50 || !known || warning != "" {
		t.Fatalf("explicit current point = %d, %v, %q", point, known, warning)
	}
	if _, _, warning := resolveEventPlannerCurrentPoint(rc, nil, renderregion.JP, eventInfo, renderdeck.AutoQuery{}, eventPlannerCommandParams{}); warning == "" {
		t.Fatal("missing tracker current-point warning omitted")
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
	if !eventPlannerUseWorldBloomRanking(eventInfo, eventPlannerCommandParams{}) || eventPlannerUseWorldBloomRanking(eventInfo, eventPlannerCommandParams{TotalRanking: true}) {
		t.Fatal("planner WL ranking selection mismatch")
	}
}
