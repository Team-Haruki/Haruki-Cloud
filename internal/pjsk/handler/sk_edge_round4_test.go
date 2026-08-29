package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type skRound4Tracker struct {
	outsideRange bool
}

func (s *skRound4Tracker) GetCloudSKQuery(_ string, eventID int, _ *int, ranks []int, userID *int64, includeAdjacent, _ bool, _ int64) (*sekaiapi.CloudRankQueryResponse, error) {
	if s.outsideRange && userID != nil && !includeAdjacent {
		return &sekaiapi.CloudRankQueryResponse{}, nil
	}
	rank := 1
	if len(ranks) > 0 {
		rank = ranks[0]
	}
	uid := "123456789"
	if userID != nil {
		uid = fmt.Sprintf("%d", *userID)
	}
	item := sekaiapi.CloudRankInfo{
		Rank:      rank,
		UserID:    &uid,
		Name:      "round4-user",
		Score:     123456,
		Timestamp: time.Now().Add(-10 * time.Minute).Unix(),
	}
	return &sekaiapi.CloudRankQueryResponse{
		Meta:  sekaiapi.LeaderboardMeta{Server: "jp", EventID: eventID},
		Ranks: []sekaiapi.CloudRankInfo{item},
	}, nil
}

func (*skRound4Tracker) GetCloudSKCheckRoom(_ string, eventID int, _ *int, ranks []int, userID *int64, _ bool, _ int64) (*sekaiapi.CloudCheckRoomResponse, error) {
	rank := 1
	if len(ranks) > 0 {
		rank = ranks[0]
	}
	uid := "123456789"
	if userID != nil {
		uid = fmt.Sprintf("%d", *userID)
	}
	item := sekaiapi.CloudRankInfo{Rank: rank, UserID: &uid, Name: "round4-user", Score: 123456, Timestamp: time.Now().Unix()}
	return &sekaiapi.CloudCheckRoomResponse{
		Meta: sekaiapi.LeaderboardMeta{Server: "jp", EventID: eventID},
		Rank: item,
	}, nil
}

func (*skRound4Tracker) GetCloudSKLine(_ string, eventID int, _ *int, ranks []int, _ *int64, _ bool, _ int64) (*sekaiapi.CloudLineResponse, error) {
	items := make([]sekaiapi.CloudRankInfo, 0, len(ranks))
	for _, rank := range ranks {
		items = append(items, sekaiapi.CloudRankInfo{Rank: rank, Score: rank * 1000, Timestamp: time.Now().Unix()})
	}
	return &sekaiapi.CloudLineResponse{Meta: sekaiapi.LeaderboardMeta{Server: "jp", EventID: eventID}, Ranks: items}, nil
}

func (*skRound4Tracker) GetCloudSKSpeed(_ string, eventID int, _ *int, ranks []int, _ int64, _ int64, _ bool) (*sekaiapi.CloudSpeedResponse, error) {
	speed := 100
	items := make([]sekaiapi.CloudRankInfo, 0, len(ranks))
	for _, rank := range ranks {
		items = append(items, sekaiapi.CloudRankInfo{Rank: rank, Score: rank * 1000, Speed: &speed, Timestamp: time.Now().Unix()})
	}
	return &sekaiapi.CloudSpeedResponse{
		Meta:            sekaiapi.LeaderboardMeta{Server: "jp", EventID: eventID},
		Speeds:          items,
		IntervalSeconds: 3600,
		UnitSeconds:     3600,
	}, nil
}

func (*skRound4Tracker) GetCloudSKTrace(_ string, eventID int, _ *int, subjectType, subject string, _ int) (*sekaiapi.CloudTraceResponse, error) {
	uid := subject
	if subjectType == "rank" {
		uid = "123456789"
	}
	now := time.Now().Unix()
	return &sekaiapi.CloudTraceResponse{
		Meta:    sekaiapi.LeaderboardMeta{Server: "jp", EventID: eventID},
		Subject: sekaiapi.SubjectTraceMeta{SubjectType: subjectType, Subject: subject},
		RankData: []sekaiapi.CloudRankInfo{
			{Rank: 1, UserID: &uid, Name: "round4-user", Score: 100000, Timestamp: now - 60},
			{Rank: 1, UserID: &uid, Name: "round4-user", Score: 101000, Timestamp: now},
		},
	}, nil
}

func (*skRound4Tracker) GetEventStatus(string, int) (*sekaiapi.EventStatusResponse, error) {
	return &sekaiapi.EventStatusResponse{Status: 1, StatusDesc: "healthy"}, nil
}

func skRound4Params(t *testing.T, req rendersk.TrackerRankQuery) []byte {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal tracker request: %v", err)
	}
	return raw
}

func skRound4Context(t *testing.T, app *renderapp.App, mode string, req rendersk.TrackerRankQuery) *RequestContext {
	t.Helper()
	return &RequestContext{
		Ctx: context.Background(),
		Cmd: &CommandRequest{
			Mode:              mode,
			Region:            "jp",
			RequesterPlatform: "qq",
			RequesterUserID:   "7",
			Params:            skRound4Params(t, req),
		},
		App:            app,
		Region:         renderregion.JP,
		RegionStr:      "jp",
		Platform:       "qq",
		PlatformUserID: "7",
	}
}

func TestSKRound4TrackerSuccessBranches(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)
	app.SK.SetTrackerIntegration(&skRound4Tracker{}, nil, nil)
	uid := int64(123456789)
	rankQuery := rendersk.TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}}
	selfQuery := rendersk.TrackerRankQuery{
		Region:         "jp",
		EventID:        1,
		UserID:         &uid,
		TargetPlatform: "QQ",
		TargetUserID:   "7",
	}

	cases := []struct {
		mode string
		req  rendersk.TrackerRankQuery
	}{
		{mode: "sk-line", req: rankQuery},
		{mode: "sk-query", req: selfQuery},
		{mode: "sk-check-room", req: selfQuery},
		{mode: "sk-csb", req: selfQuery},
		{mode: "sk-speed", req: rankQuery},
		{mode: "sk-player-trace", req: selfQuery},
		{mode: "sk-rank-trace", req: rankQuery},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			result, err := executeSKMode(skRound4Context(t, app, tc.mode, tc.req), app.SK)
			if err != nil {
				t.Fatalf("executeSKMode(%s): %v", tc.mode, err)
			}
			if len(result.image) == 0 {
				t.Fatalf("executeSKMode(%s) returned no image", tc.mode)
			}
		})
	}
}

func TestSKRound4PlayerTraceFallbackAndWarning(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)
	app.SK.SetTrackerIntegration(&skRound4Tracker{}, nil, nil)
	raw, err := json.Marshal(drawing.PlayerTraceRequest{
		EventID: 1,
		Region:  "jp",
		Ranks:   []drawing.RankInfo{{Rank: 1, Score: drawing.IntPtr(100)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rc := &RequestContext{
		Ctx: context.Background(),
		Cmd: &CommandRequest{Mode: "sk-player-trace", Region: "jp", Params: raw},
		App: app,
	}
	data, err := executeSKPlayerTrace(rc, app.SK)
	if err != nil || len(data) == 0 {
		t.Fatalf("fallback player trace = %q, %v", data, err)
	}

	warningTracker := &skRound4Tracker{outsideRange: true}
	app.SK.SetTrackerIntegration(warningTracker, nil, nil)
	uid := int64(123456789)
	selfQuery := rendersk.TrackerRankQuery{
		Region:         "jp",
		EventID:        1,
		UserID:         &uid,
		TargetPlatform: "qq",
		TargetUserID:   "7",
	}
	message, err := executeSK(skRound4Context(t, app, "sk-query", selfQuery))
	if err != nil {
		t.Fatalf("executeSK warning query: %v", err)
	}
	if len(message) < 2 {
		t.Fatalf("warning query message = %#v", message)
	}
}

func TestSKRound4PrepareErrorsAcrossModes(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)
	app.SK.SetTrackerIntegration(&skRound4Tracker{}, nil, nil)
	req := rendersk.TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}, WlCharacterQuery: "wl"}
	for _, mode := range []string{
		"sk-line", "sk-query", "sk-check-room", "sk-csb", "sk-speed",
		"sk-player-trace", "sk-rank-trace", "sk-predict",
	} {
		t.Run(mode, func(t *testing.T) {
			if _, err := executeSKMode(skRound4Context(t, app, mode, req), app.SK); err == nil {
				t.Fatalf("executeSKMode(%s) unexpectedly succeeded", mode)
			}
		})
	}
}

func TestSKRound4RenderAndPlayerEdges(t *testing.T) {
	uid := int64(123456789)
	selfQuery := rendersk.TrackerRankQuery{
		Region:         "jp",
		EventID:        1,
		UserID:         &uid,
		TargetPlatform: "qq",
		TargetUserID:   "7",
	}
	controller := rendersk.NewController(nil)
	controller.SetTrackerIntegration(&skRound4Tracker{}, nil, nil)
	app := &renderapp.App{SK: controller}
	for _, mode := range []string{"sk-query", "sk-check-room", "sk-csb", "sk-player-trace"} {
		rc := skRound4Context(t, app, mode, selfQuery)
		if _, err := executeSKMode(rc, controller); err == nil {
			t.Fatalf("%s with missing drawing client unexpectedly succeeded", mode)
		}
	}

	explicitTarget := rendersk.TrackerRankQuery{
		Region:         "jp",
		EventID:        1,
		Ranks:          []int{1},
		TargetPlatform: "qq",
		TargetUserID:   "missing",
	}
	if _, err := executeSKPlayerTrace(skRound4Context(t, app, "sk-player-trace", explicitTarget), controller); !errors.Is(err, accountdata.ErrBindingServiceUnavailable) {
		t.Fatalf("explicit target error = %v", err)
	}

	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(context.Background(), "qq", "7", "12345678901234"); err != nil {
		t.Fatalf("bind fallback requester: %v", err)
	}
	drawingApp, _ := newExecutionCoverageApp(t)
	drawingApp.Bindings = service
	drawingApp.SK.SetTrackerIntegration(&skRound4Tracker{}, nil, nil)
	rc := &RequestContext{
		Ctx: context.Background(),
		Cmd: &CommandRequest{
			Mode:              "sk-player-trace",
			RequesterPlatform: "qq",
			RequesterUserID:   "7",
		},
		App:            drawingApp,
		Platform:       "qq",
		PlatformUserID: "7",
	}
	if _, err := executeSKPlayerTrace(rc, drawingApp.SK); err == nil {
		t.Fatal("requester fallback without an inferable event unexpectedly succeeded")
	}
}

func TestSKRound4HandlerBuildErrors(t *testing.T) {
	ctx := HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{
			Context: context.Background(),
			ArgText: "not-a-rank",
		},
		region: renderregion.JP,
		flags:  map[string]bool{},
	}
	for _, handler := range []HarukiSekaiCommandHandler{
		(sekaiHandlers{}).SKLineHandle(),
		(sekaiHandlers{}).SKQueryHandle(),
		(sekaiHandlers{}).SKCheckRoomHandle(),
		(sekaiHandlers{}).SKCheckRoomLiteHandle(),
		(sekaiHandlers{}).SKPlayerTraceHandle(),
		(sekaiHandlers{}).SKRankTraceHandle(),
		(sekaiHandlers{}).SKPredictHandle(),
		(sekaiHandlers{}).SKBoardHandle(),
		(sekaiHandlers{}).CSBHandle(),
	} {
		if request, err := handler.handleFunc(ctx); err == nil || request != nil {
			t.Errorf("handler %v returned request=%#v err=%v", handler.Commands, request, err)
		}
	}
}

func TestSKRound4WorldBloomSelectionBranches(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", "file:handler_sk_round4_wl?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	now := time.Now().UnixMilli()
	seedHandlerTestWorldBloomEvent(t, ctx, client, "jp", 404, now-1000, now+100000, []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - 500, aggregateAt: now + 50000, characterID: 21},
		{chapterNo: 2, startAt: now - 400, aggregateAt: now + 60000},
	})
	app := &renderapp.App{Sekai: client}

	explicit := rendersk.TrackerRankQuery{Region: "jp", EventID: 404, WlCharacterID: drawing.IntPtr(21), WlCharacterQuery: "ignored"}
	if err := resolveTrackerCharacterSelection(ctx, app, &explicit); err != nil {
		t.Fatalf("explicit character selection: %v", err)
	}
	if explicit.WlCharacterQuery != "" || explicit.EventStartAt == nil || explicit.EventAggregateAt == nil {
		t.Fatalf("explicit character selection result = %+v", explicit)
	}

	missing := rendersk.TrackerRankQuery{Region: "jp", EventID: 404, WlCharacterID: drawing.IntPtr(99)}
	if err := resolveTrackerCharacterSelection(ctx, app, &missing); err == nil {
		t.Fatal("missing explicit character unexpectedly resolved")
	}

	badChapter := rendersk.TrackerRankQuery{Region: "jp", EventID: 404, WlCharacterQuery: "wl99"}
	if err := resolveTrackerCharacterSelection(ctx, app, &badChapter); err == nil {
		t.Fatal("missing chapter unexpectedly resolved")
	}

	noCharacter := rendersk.TrackerRankQuery{Region: "jp", EventID: 404, WlCharacterQuery: "wl2"}
	if err := resolveTrackerCharacterSelection(ctx, app, &noCharacter); err == nil {
		t.Fatal("chapter without character unexpectedly resolved")
	}
}

func TestSKRound4TargetLookupErrorsAndRequesterUID(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	app := &renderapp.App{Bindings: service}
	cases := []rendersk.TrackerRankQuery{
		{Region: "jp", TargetPlatform: "qq", TargetUserID: "missing", TargetSelector: "u1"},
		{Region: "jp", RegionExplicit: true, TargetPlatform: "qq", TargetUserID: "missing"},
		{Region: "tw", TargetPlatform: "qq", TargetUserID: "missing"},
	}
	for _, req := range cases {
		if err := resolveTrackerTargetUser(ctx, app, &req, "qq", "requester"); err == nil {
			t.Fatalf("missing target unexpectedly resolved: %+v", req)
		}
	}

	if _, err := service.Bind(ctx, "qq", "requester", "12345678901234"); err != nil {
		t.Fatalf("bind requester: %v", err)
	}
	rc := &RequestContext{
		Ctx:            ctx,
		Cmd:            &CommandRequest{Region: "jp"},
		App:            app,
		RegionStr:      "jp",
		Platform:       "qq",
		PlatformUserID: "requester",
	}
	if uid := resolveRequesterGameUID(rc); uid != 12345678901234 {
		t.Fatalf("resolveRequesterGameUID = %d", uid)
	}

	if _, err := service.Bind(ctx, "qq", "invalid", "0"); err != nil {
		t.Fatalf("bind zero uid: %v", err)
	}
	invalidRC := &RequestContext{
		Ctx:            ctx,
		Cmd:            &CommandRequest{Region: "jp"},
		App:            app,
		RegionStr:      "jp",
		Platform:       "qq",
		PlatformUserID: "invalid",
	}
	if uid := resolveRequesterGameUID(invalidRC); uid != 0 {
		t.Fatalf("invalid requester UID = %d", uid)
	}
	invalidTarget := rendersk.TrackerRankQuery{
		Region:         "jp",
		RegionExplicit: true,
		TargetPlatform: "qq",
		TargetUserID:   "invalid",
	}
	if err := resolveTrackerTargetUser(ctx, app, &invalidTarget, "qq", "invalid"); err == nil {
		t.Fatal("zero target UID unexpectedly resolved")
	}
}

func TestSKRound4ErrorIdentity(t *testing.T) {
	err := errors.New("round4")
	if got := normalizeSKSelfRankingNotFoundError(true, "jp", err); !errors.Is(got, err) {
		t.Fatalf("normalization changed unrelated error: %v", got)
	}
}
