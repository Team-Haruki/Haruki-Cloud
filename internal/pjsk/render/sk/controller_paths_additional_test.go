package sk

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type additionalCloudTrackerSource struct {
	queryResp *sekaiapi.CloudRankQueryResponse
	queryErr  error
	checkResp *sekaiapi.CloudCheckRoomResponse
	checkErr  error
	lineResp  *sekaiapi.CloudLineResponse
	lineErr   error
	speedResp *sekaiapi.CloudSpeedResponse
	speedErr  error
	traceResp *sekaiapi.CloudTraceResponse
	traceErr  error
}

func (s *additionalCloudTrackerSource) GetCloudSKQuery(string, int, *int, []int, *int64, bool, bool, int64) (*sekaiapi.CloudRankQueryResponse, error) {
	return s.queryResp, s.queryErr
}

func (s *additionalCloudTrackerSource) GetCloudSKCheckRoom(string, int, *int, []int, *int64, bool, int64) (*sekaiapi.CloudCheckRoomResponse, error) {
	return s.checkResp, s.checkErr
}

func (s *additionalCloudTrackerSource) GetCloudSKLine(string, int, *int, []int, *int64, bool, int64) (*sekaiapi.CloudLineResponse, error) {
	return s.lineResp, s.lineErr
}

func (s *additionalCloudTrackerSource) GetCloudSKSpeed(string, int, *int, []int, int64, int64, bool) (*sekaiapi.CloudSpeedResponse, error) {
	return s.speedResp, s.speedErr
}

func (s *additionalCloudTrackerSource) GetCloudSKTrace(string, int, *int, string, string, int) (*sekaiapi.CloudTraceResponse, error) {
	return s.traceResp, s.traceErr
}

type additionalEventSource struct {
	region renderregion.Value
	event  *masterdata.Event
	err    error
}

func (s *additionalEventSource) DefaultRegion() renderregion.Value { return s.region }
func (s *additionalEventSource) GetEventByID(int) (*masterdata.Event, error) {
	return s.event, s.err
}
func (s *additionalEventSource) GetEvents() []*masterdata.Event {
	if s.event == nil {
		return nil
	}
	return []*masterdata.Event{s.event}
}

func additionalStringPtr(value string) *string { return &value }

func TestTrackerEventNameHelpersAdditional(t *testing.T) {
	if got := normalizeTrackerNameForCompare("  My Event！2026_测试 "); got != "myevent2026测试" {
		t.Fatalf("normalized tracker name = %q", got)
	}
	if got := normalizeTrackerNameForCompare("  !!! "); got != "" {
		t.Fatalf("punctuation-only tracker name = %q", got)
	}

	fuzzyCases := []struct {
		name  string
		meta  string
		match bool
	}{
		{"", "event", false},
		{"!!!", "event", false},
		{"My Event", "my-event", true},
		{"Long Event 2026 extra", "Long Event 2026", true},
		{"Long Event 2026", "Long Event 2026 extra", true},
		{"short", "sho", false},
		{"different event", "another title", false},
	}
	for _, tt := range fuzzyCases {
		if got := isTrackerEventTitleFuzzyMatch(tt.name, tt.meta); got != tt.match {
			t.Errorf("isTrackerEventTitleFuzzyMatch(%q, %q) = %v", tt.name, tt.meta, got)
		}
	}

	if got := (*Controller)(nil).eventTitleForNameCheck("jp", 1); got != "" {
		t.Fatalf("nil controller event title = %q", got)
	}
	if got := NewController(nil).eventTitleForNameCheck("jp", 0); got != "" {
		t.Fatalf("invalid event ID title = %q", got)
	}
	if got := (&Controller{}).eventTitleForNameCheck("invalid", 1); got != "" {
		t.Fatalf("missing event source title = %q", got)
	}

	controller := NewController(nil)
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP, err: errors.New("missing")})
	if got := controller.eventTitleForNameCheck("invalid", 1); got != "" {
		t.Fatalf("failed event lookup title = %q", got)
	}
	controller = NewController(nil)
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP})
	if got := controller.eventTitleForNameCheck("jp", 1); got != "" {
		t.Fatalf("nil event title = %q", got)
	}
	controller = NewController(nil)
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP, event: &masterdata.Event{ID: 1, Name: "  Long Event 2026  "}})
	if got := controller.eventTitleForNameCheck("jp", 1); got != "Long Event 2026" {
		t.Fatalf("resolved event title = %q", got)
	}
	if controller.isTrackerEventTitleName("jp", 1, "") {
		t.Fatal("empty tracker name matched event title")
	}
	if !controller.isTrackerEventTitleName("jp", 1, "long-event-2026") {
		t.Fatal("normalized tracker name did not match event title")
	}
	if (&Controller{}).isTrackerEventTitleName("jp", 1, "name") {
		t.Fatal("tracker name matched without event metadata")
	}
}

func TestQueryAndWinRateRequestValidationAdditional(t *testing.T) {
	controller := NewController(nil)
	if _, err := controller.BuildQueryRequest(drawing.SKRequest{}); err == nil {
		t.Fatal("query without ranks unexpectedly succeeded")
	}
	query := drawing.SKRequest{Ranks: []drawing.RankInfo{{Rank: 1}}}
	if got, err := controller.BuildQueryRequest(query); err != nil || !reflect.DeepEqual(got, &query) {
		t.Fatalf("valid query request = %+v, %v", got, err)
	}
	if _, err := (*Controller)(nil).RenderQuery(query); err == nil {
		t.Fatal("nil query renderer unexpectedly succeeded")
	}
	drawingController := &Controller{drawing: &drawing.HarukiDrawingClient{}}
	if _, err := drawingController.RenderQuery(drawing.SKRequest{}); err == nil {
		t.Fatal("invalid rendered query unexpectedly succeeded")
	}

	if _, err := controller.BuildCheckRoomRequest(drawing.CFRequest{}); err == nil {
		t.Fatal("check-room request without ranks unexpectedly succeeded")
	}
	if _, err := controller.BuildCheckRoomRequest(drawing.CFRequest{Ranks: []drawing.RankInfo{{Rank: 101}}}); err == nil {
		t.Fatal("out-of-range check-room rank unexpectedly succeeded")
	}
	check := drawing.CFRequest{Ranks: []drawing.RankInfo{{Rank: 100}}}
	if got, err := controller.BuildCheckRoomRequest(check); err != nil || !reflect.DeepEqual(got, &check) {
		t.Fatalf("valid check-room request = %+v, %v", got, err)
	}
	if _, err := (*Controller)(nil).RenderCheckRoom(check); err == nil {
		t.Fatal("nil check-room renderer unexpectedly succeeded")
	}
	if _, err := drawingController.RenderCheckRoom(drawing.CFRequest{}); err == nil {
		t.Fatal("invalid rendered check-room request unexpectedly succeeded")
	}

	if _, err := controller.BuildCSBRequest(drawing.CSBRequest{}); err == nil {
		t.Fatal("CSB request without ranks unexpectedly succeeded")
	}
	if _, err := controller.BuildCSBRequest(drawing.CSBRequest{Ranks: []drawing.RankInfo{{Rank: 10}, {Rank: 101}}}); err == nil {
		t.Fatal("out-of-range final CSB rank unexpectedly succeeded")
	}
	csb := drawing.CSBRequest{Ranks: []drawing.RankInfo{{Rank: 100}}}
	if got, err := controller.BuildCSBRequest(csb); err != nil || !reflect.DeepEqual(got, &csb) {
		t.Fatalf("valid CSB request = %+v, %v", got, err)
	}
	if _, err := (*Controller)(nil).RenderCSB(csb); err == nil {
		t.Fatal("nil CSB renderer unexpectedly succeeded")
	}
	if _, err := drawingController.RenderCSB(drawing.CSBRequest{}); err == nil {
		t.Fatal("invalid rendered CSB request unexpectedly succeeded")
	}

	if err := validateSKCheckRoomSupportedRanks([]drawing.RankInfo{{Rank: 1}, {Rank: 101}}); err == nil {
		t.Fatal("rank-list validation unexpectedly succeeded")
	}
	if err := validateSKCheckRoomSupportedRank(100); err != nil {
		t.Fatalf("rank 100 rejected: %v", err)
	}

	if _, err := controller.BuildWinRateRequest(drawing.WinRateRequest{}); err == nil {
		t.Fatal("win-rate request without teams unexpectedly succeeded")
	}
	winRate := drawing.WinRateRequest{TeamInfo: []drawing.TeamInfo{{TeamID: 1}}}
	if got, err := controller.BuildWinRateRequest(winRate); err != nil || !reflect.DeepEqual(got, &winRate) {
		t.Fatalf("valid win-rate request = %+v, %v", got, err)
	}
	if _, err := (*Controller)(nil).RenderWinRate(winRate); err == nil {
		t.Fatal("nil win-rate renderer unexpectedly succeeded")
	}
	if _, err := drawingController.RenderWinRate(drawing.WinRateRequest{}); err == nil {
		t.Fatal("invalid rendered win-rate request unexpectedly succeeded")
	}
}

func TestQueryAdjacentRanksAdditional(t *testing.T) {
	cases := []struct {
		rank                 int
		wl                   bool
		prev, next           int
		hasPrevious, hasNext bool
	}{
		{0, false, 0, 0, false, false},
		{1, false, 0, 2, false, true},
		{20, false, 10, 30, true, true},
		{15, false, 10, 20, true, true},
		{400000, false, 300000, 0, true, false},
		{7000, true, 5000, 10000, true, true},
	}
	for _, tt := range cases {
		prev, next, hasPrevious, hasNext := queryAdjacentSKLineRanks(tt.rank, tt.wl)
		if prev != tt.prev || next != tt.next || hasPrevious != tt.hasPrevious || hasNext != tt.hasNext {
			t.Errorf("queryAdjacentSKLineRanks(%d, %v) = %d, %d, %v, %v", tt.rank, tt.wl, prev, next, hasPrevious, hasNext)
		}
	}
}

func TestTrackerIdentityAdditional(t *testing.T) {
	if _, _, _, err := (&Controller{}).buildSingleRankBaseFromTracker("jp", 1, 100, nil); err == nil {
		t.Fatal("rank lookup without tracker unexpectedly succeeded")
	}
	if _, err := (&Controller{}).buildSingleRankFromTracker("jp", 1, 100, nil); err == nil {
		t.Fatal("rank-name lookup without tracker unexpectedly succeeded")
	}
	if _, err := (&Controller{}).buildSingleUserBaseFromTracker("jp", 1, 123, nil); err == nil {
		t.Fatal("user lookup without tracker unexpectedly succeeded")
	}

	queryErr := errors.New("query failed")
	controller := NewController(nil)
	controller.SetTrackerIntegration(&additionalCloudTrackerSource{queryErr: queryErr}, nil, nil)
	if _, _, _, err := controller.buildSingleRankBaseFromTracker("jp", 1, 100, nil); !errors.Is(err, queryErr) {
		t.Fatalf("rank query error = %v", err)
	}
	if _, err := controller.buildSingleUserFromTracker("jp", 1, 123, nil); !errors.Is(err, queryErr) {
		t.Fatalf("user query error = %v", err)
	}

	controller = NewController(nil)
	controller.SetTrackerIntegration(&additionalCloudTrackerSource{queryResp: &sekaiapi.CloudRankQueryResponse{}}, nil, nil)
	if _, _, _, err := controller.buildSingleRankBaseFromTracker("jp", 1, 100, nil); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("empty rank query error = %v", err)
	}
	if _, err := controller.buildSingleUserBaseFromTracker("jp", 1, 123, nil); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("empty user query error = %v", err)
	}

	uid := "123"
	rankItem := sekaiapi.CloudRankInfo{Rank: 100, UserID: &uid, Name: "Long Event 2026", Score: 9000, Timestamp: 1_700_000_000}
	controller = NewController(nil)
	controller.SetTrackerIntegration(&additionalCloudTrackerSource{queryResp: &sekaiapi.CloudRankQueryResponse{Ranks: []sekaiapi.CloudRankInfo{rankItem}}}, nil, nil)
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP, event: &masterdata.Event{ID: 1, Name: "Long Event 2026"}})
	info, resolvedUID, hasUID, err := controller.buildSingleRankBaseFromTracker("jp", 1, 100, nil)
	if err != nil || info.Rank != 100 || resolvedUID != 123 || !hasUID {
		t.Fatalf("rank identity = %+v, %d, %v, %v", info, resolvedUID, hasUID, err)
	}
	info, err = controller.buildSingleRankLatestFromTracker("jp", 1, 100, nil)
	if err != nil || info.Name != "Long Event 2026" {
		t.Fatalf("latest rank identity = %+v, %v", info, err)
	}
	info, err = controller.buildSingleRankFromTracker("jp", 1, 100, nil)
	if err != nil || info.Name != "Rank 100" {
		t.Fatalf("sanitized rank identity = %+v, %v", info, err)
	}
	info, err = controller.buildSingleUserBaseFromTracker("jp", 1, 123, nil)
	if err != nil || info.Rank != 100 {
		t.Fatalf("user identity = %+v, %v", info, err)
	}
}

func TestTrackerLineFastPathsAdditional(t *testing.T) {
	controller := &Controller{}
	if _, err := controller.buildSingleRankLineFromTracker("jp", 1, 100, nil); err == nil {
		t.Fatal("rank line without tracker unexpectedly succeeded")
	}
	if _, err := controller.buildSingleUserLineFromTracker("jp", 1, 123, nil); err == nil {
		t.Fatal("user line without tracker unexpectedly succeeded")
	}
	if got, err := controller.buildLineRanksFromTracker("jp", 1, nil, nil, false); err != nil || len(got) != 0 {
		t.Fatalf("empty fallback line ranks = %#v, %v", got, err)
	}
	if _, err := controller.buildLineRanksFromTracker("jp", 1, []int{100}, nil, false); err == nil {
		t.Fatal("fallback line rank without tracker unexpectedly succeeded")
	}

	lineErr := errors.New("line failed")
	tracker := &additionalCloudTrackerSource{lineErr: lineErr}
	controller = NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	if _, err := controller.buildSingleRankLineFromTracker("jp", 1, 100, nil); !errors.Is(err, lineErr) {
		t.Fatalf("rank line error = %v", err)
	}
	if _, err := controller.buildSingleUserLineFromTracker("jp", 1, 123, nil); !errors.Is(err, lineErr) {
		t.Fatalf("user line error = %v", err)
	}
	if _, err := controller.buildLineRanksOrUserFromTracker("jp", 1, nil, additionalInt64Ptr(123), nil, false); err == nil || !strings.Contains(err.Error(), "tracker user query failed") {
		t.Fatalf("wrapped user-line error = %v", err)
	}

	tracker.lineErr = nil
	tracker.lineResp = &sekaiapi.CloudLineResponse{}
	if _, err := controller.buildSingleRankLineFromTracker("jp", 1, 100, nil); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("empty rank line error = %v", err)
	}
	if _, err := controller.buildSingleUserLineFromTracker("jp", 1, 123, nil); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("empty user line error = %v", err)
	}

	uid := "123"
	tracker.lineResp = &sekaiapi.CloudLineResponse{Ranks: []sekaiapi.CloudRankInfo{{Rank: 50, UserID: &uid, Name: "Visible", Score: 5000, Timestamp: 1_700_000_000}}}
	info, err := controller.buildSingleRankLineFromTracker("jp", 1, 50, nil)
	if err != nil || info.Rank != 50 {
		t.Fatalf("rank line = %+v, %v", info, err)
	}
	info, err = controller.buildSingleUserLineFromTracker("jp", 1, 123, nil)
	if err != nil || info.Rank != 50 || info.Name != "" {
		t.Fatalf("user line = %+v, %v", info, err)
	}
	infos, err := controller.buildLineRanksOrUserFromTracker("jp", 1, nil, additionalInt64Ptr(123), nil, false)
	if err != nil || len(infos) != 1 || infos[0].Rank != 50 {
		t.Fatalf("user line dispatch = %#v, %v", infos, err)
	}
	infos, err = controller.buildLineRanksOrUserFromTracker("jp", 1, []int{50}, nil, nil, false)
	if err != nil || len(infos) != 1 || infos[0].Rank != 50 {
		t.Fatalf("rank line dispatch = %#v, %v", infos, err)
	}

	wrongUID := "999"
	tracker.lineResp = &sekaiapi.CloudLineResponse{Ranks: []sekaiapi.CloudRankInfo{{Rank: 60, UserID: &wrongUID, Name: "Wrong", Score: 6000}}}
	tracker.traceErr = errors.New("trace failed")
	if _, err := controller.buildSingleUserLineFromTracker("jp", 1, 123, nil); !errors.Is(err, tracker.traceErr) {
		t.Fatalf("mismatched-user trace error = %v", err)
	}
	tracker.traceErr = nil
	tracker.traceResp = &sekaiapi.CloudTraceResponse{RankData: []sekaiapi.CloudRankInfo{{Rank: 55, UserID: &uid, Name: "Trace", Score: 5500, Timestamp: 1_700_000_001}}}
	info, err = controller.buildSingleUserLineFromTracker("jp", 1, 123, nil)
	if err != nil || info.Rank != 55 || info.Name != "" {
		t.Fatalf("mismatched-user trace fallback = %+v, %v", info, err)
	}

	points := []sekaiapi.RankDataPoint{{Score: 1, Timestamp: 2}, {Score: 3, Timestamp: 4}}
	if got := rankTraceSamples(points); !reflect.DeepEqual(got, []trackerScoreSample{{score: 1, timestamp: 2}, {score: 3, timestamp: 4}}) {
		t.Fatalf("rank trace samples = %#v", got)
	}
}

func TestTrackerQueryRequestBranchesAdditional(t *testing.T) {
	if _, err := (*Controller)(nil).BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}}); err == nil {
		t.Fatal("nil tracker query controller unexpectedly succeeded")
	}

	queryErr := errors.New("query failed")
	tracker := &additionalCloudTrackerSource{queryErr: queryErr}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	if _, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}}); !errors.Is(err, queryErr) {
		t.Fatalf("rank query request error = %v", err)
	}
	if _, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, UserID: additionalInt64Ptr(123)}); !errors.Is(err, queryErr) {
		t.Fatalf("user query request error = %v", err)
	}

	tracker.queryErr = nil
	tracker.queryResp = &sekaiapi.CloudRankQueryResponse{}
	if _, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}}); err == nil {
		t.Fatal("empty tracker query unexpectedly succeeded")
	}

	uid := "123"
	previous := sekaiapi.CloudRankInfo{Rank: 50, UserID: additionalStringPtr("50"), Score: 500}
	next := sekaiapi.CloudRankInfo{Rank: 200, UserID: additionalStringPtr("200"), Score: 200}
	tracker.queryResp = &sekaiapi.CloudRankQueryResponse{
		Ranks:    []sekaiapi.CloudRankInfo{{Rank: 100, UserID: &uid, Name: "Player", Score: 1000}},
		Previous: &previous,
		Next:     &next,
	}
	payload, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}})
	if err != nil || len(payload.Ranks) != 1 || payload.PrevRanks == nil || payload.NextRanks == nil {
		t.Fatalf("rank query payload = %+v, %v", payload, err)
	}
	payload, err = controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, UserID: additionalInt64Ptr(123)})
	if err != nil || len(payload.Ranks) != 1 || payload.Ranks[0].Rank != 100 {
		t.Fatalf("user query payload = %+v, %v", payload, err)
	}
	wlCharacterID := 21
	payload, err = controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}, WlCharacterID: &wlCharacterID})
	if err != nil || payload.WlCharaIconPath == nil || payload.CharaIconPath == nil {
		t.Fatalf("world-link query payload = %+v, %v", payload, err)
	}

	checkErr := errors.New("check failed")
	tracker.checkErr = checkErr
	if _, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}}); !errors.Is(err, checkErr) {
		t.Fatalf("check-room tracker error = %v", err)
	}
	tracker.checkErr = nil
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{}
	if _, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}}); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("empty check-room tracker error = %v", err)
	}
	averageRound := 1
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{
		Rank:     sekaiapi.CloudRankInfo{Rank: 100, UserID: &uid, Score: 1000, AverageRound: &averageRound},
		Previous: &previous,
		Next:     &next,
	}
	checkPayload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}})
	if err != nil || len(checkPayload.Ranks) != 1 || checkPayload.PrevRank == nil || checkPayload.NextRank == nil {
		t.Fatalf("check-room payload = %+v, %v", checkPayload, err)
	}
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Rank: sekaiapi.CloudRankInfo{Rank: 101, UserID: &uid, AverageRound: &averageRound}}
	if _, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{101}}); err == nil {
		t.Fatal("out-of-range tracker check-room unexpectedly succeeded")
	}
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Ranks: []sekaiapi.CloudRankInfo{
		{Rank: 20, UserID: additionalStringPtr("20"), AverageRound: &averageRound},
		{Rank: 10, UserID: additionalStringPtr("10"), AverageRound: &averageRound},
	}}
	checkPayload, err = controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{10, 20}})
	if err != nil || len(checkPayload.Ranks) != 2 || checkPayload.Ranks[0].Rank != 10 {
		t.Fatalf("multi-rank check-room payload = %+v, %v", checkPayload, err)
	}

	if _, err := controller.BuildCSBRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1, 2}}); err == nil {
		t.Fatal("multi-rank CSB tracker request unexpectedly succeeded")
	}
}

func TestTrackerValidationAndSpeedAdditional(t *testing.T) {
	controller := NewController(nil)
	if _, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}}); err == nil {
		t.Fatal("tracker validation without tracker unexpectedly succeeded")
	}
	tracker := &additionalCloudTrackerSource{}
	controller.SetTrackerIntegration(tracker, nil, nil)
	if _, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "invalid", EventID: 1, Ranks: []int{1}}); err == nil {
		t.Fatal("invalid tracker region unexpectedly succeeded")
	}
	if _, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "jp", EventID: 1, UserID: additionalInt64Ptr(0)}); err == nil {
		t.Fatal("empty normalized tracker target unexpectedly succeeded")
	}
	if _, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "jp", Ranks: []int{1}}); err == nil {
		t.Fatal("missing inferred event unexpectedly succeeded")
	}
	negativeWL := -1
	normalized, err := controller.validateTrackerQuery(TrackerRankQuery{
		Region:        " JP ",
		EventID:       1,
		Ranks:         []int{2, -1, 2, 1},
		CompareRank:   -2,
		WlCharacterID: &negativeWL,
	})
	if err != nil || !reflect.DeepEqual(normalized.Ranks, []int{1, 2}) || normalized.CompareRank != 0 || normalized.WlCharacterID != nil {
		t.Fatalf("normalized tracker query = %#v, %v", normalized, err)
	}
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP, event: &masterdata.Event{ID: 1, EventType: "marathon"}})
	positiveWL := 21
	if _, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}, WlCharacterID: &positiveWL}); err == nil {
		t.Fatal("world-link character on ordinary event unexpectedly succeeded")
	}

	period, unitPeriod, unit := normalizeTrackerSpeedConfig(TrackerRankQuery{SpeedUnit: "day"})
	if period != 86_400 || unitPeriod != 86_400 || unit != "日" {
		t.Fatalf("daily speed config = %d, %d, %q", period, unitPeriod, unit)
	}
	period, unitPeriod, unit = normalizeTrackerSpeedConfig(TrackerRankQuery{SpeedUnit: "unknown", SpeedPeriodSecs: 123})
	if period != 123 || unitPeriod != 3_600 || unit != "时" {
		t.Fatalf("hourly speed config = %d, %d, %q", period, unitPeriod, unit)
	}
	if !shouldSkipMissingTrackerRankError(true, sekaiapi.ErrRankingNotFound) || shouldSkipMissingTrackerRankError(false, sekaiapi.ErrRankingNotFound) || shouldSkipMissingTrackerRankError(true, errors.New("other")) {
		t.Fatal("skip-missing tracker error predicate mismatch")
	}
	if got, err := controller.buildRanksFromTracker("jp", 1, nil, nil, false); err != nil || got != nil {
		t.Fatalf("empty tracker ranks = %#v, %v", got, err)
	}
	if _, err := (&Controller{}).buildRanksFromTracker("jp", 1, []int{1}, nil, false); err == nil {
		t.Fatal("tracker rank build without cloud source unexpectedly succeeded")
	}

	if _, err := controller.BuildSpeedRequest(drawing.SpeedRequest{}); err == nil {
		t.Fatal("speed request without ranks unexpectedly succeeded")
	}
	speedRequest := drawing.SpeedRequest{Ranks: []drawing.SpeedInfo{{Rank: 1}}}
	if got, err := controller.BuildSpeedRequest(speedRequest); err != nil || !reflect.DeepEqual(got, &speedRequest) {
		t.Fatalf("valid speed request = %+v, %v", got, err)
	}
	if _, err := (*Controller)(nil).RenderSpeed(speedRequest); err == nil {
		t.Fatal("nil speed renderer unexpectedly succeeded")
	}
	if _, err := (&Controller{drawing: &drawing.HarukiDrawingClient{}}).RenderSpeed(drawing.SpeedRequest{}); err == nil {
		t.Fatal("invalid rendered speed request unexpectedly succeeded")
	}
	if _, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, UserID: additionalInt64Ptr(1)}); err == nil {
		t.Fatal("user speed query unexpectedly succeeded")
	}
	tracker.speedErr = errors.New("speed failed")
	if _, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}}); !errors.Is(err, tracker.speedErr) {
		t.Fatalf("tracker speed error = %v", err)
	}
	tracker.speedErr = nil
	speed := 120
	tracker.speedResp = &sekaiapi.CloudSpeedResponse{Speeds: []sekaiapi.CloudRankInfo{{Rank: 1, Score: 1000, Speed: &speed, Timestamp: 1_700_000_000}}}
	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}, SpeedUnit: "day"})
	if err != nil || len(payload.Ranks) != 1 || payload.Ranks[0].Speed == nil || payload.RequestType != "日" {
		t.Fatalf("tracker speed payload = %+v, %v", payload, err)
	}
}

func additionalInt64Ptr(value int64) *int64 { return &value }
