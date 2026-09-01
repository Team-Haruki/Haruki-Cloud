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
	"haruki-cloud/internal/testutil"
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
	{
		got := normalizeTrackerNameForCompare("  My Event！2026_测试 ")
		testutil.Require(t, !(got != "myevent2026测试"), "normalized tracker name = %q", got)
	}
	{

		got := normalizeTrackerNameForCompare("  !!! ")
		testutil.Require(t, !(got != ""), "punctuation-only tracker name = %q", got)
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
		{
			got := isTrackerEventTitleFuzzyMatch(tt.name, tt.meta)
			testutil.Check(t, !(got != tt.match), "isTrackerEventTitleFuzzyMatch(%q, %q) = %v", tt.name, tt.meta, got)
		}

	}
	{

		got := (*Controller)(nil).eventTitleForNameCheck("jp", 1)
		testutil.Require(t, !(got != ""), "nil controller event title = %q", got)
	}
	{

		got := NewController(nil).eventTitleForNameCheck("jp", 0)
		testutil.Require(t, !(got != ""), "invalid event ID title = %q", got)
	}
	{

		got := (&Controller{}).eventTitleForNameCheck("invalid", 1)
		testutil.Require(t, !(got != ""), "missing event source title = %q", got)
	}

	controller := NewController(nil)
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP, err: errors.New("missing")})
	{
		got := controller.eventTitleForNameCheck("invalid", 1)
		testutil.Require(t, !(got != ""), "failed event lookup title = %q", got)
	}

	controller = NewController(nil)
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP})
	{
		got := controller.eventTitleForNameCheck("jp", 1)
		testutil.Require(t, !(got != ""), "nil event title = %q", got)
	}

	controller = NewController(nil)
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP, event: &masterdata.Event{ID: 1, Name: "  Long Event 2026  "}})
	{
		got := controller.eventTitleForNameCheck("jp", 1)
		testutil.Require(t, !(got != "Long Event 2026"), "resolved event title = %q", got)
	}
	testutil.RequireArgs(t, !(controller.isTrackerEventTitleName("jp", 1, "")), "empty tracker name matched event title")
	testutil.RequireArgs(t, controller.isTrackerEventTitleName("jp", 1, "long-event-2026"), "normalized tracker name did not match event title")
	testutil.RequireArgs(t, !((&Controller{}).isTrackerEventTitleName("jp", 1, "name")), "tracker name matched without event metadata")

}

func TestQueryAndWinRateRequestValidationAdditional(t *testing.T) {
	controller := NewController(nil)
	{
		_, err := controller.BuildQueryRequest(drawing.SKRequest{})
		testutil.RequireArgs(t, !(err == nil), "query without ranks unexpectedly succeeded")
	}

	query := drawing.SKRequest{Ranks: []drawing.RankInfo{{Rank: 1}}}
	{
		got, err := controller.BuildQueryRequest(query)
		{
			testutil.Require(t, !(err != nil), "valid query request = %+v, %v", got, err)
			testutil.Require(t, reflect.DeepEqual(got, &query), "valid query request = %+v, %v", got, err)
		}
	}
	{

		_, err := (*Controller)(nil).RenderQuery(query)
		testutil.RequireArgs(t, !(err == nil), "nil query renderer unexpectedly succeeded")
	}

	drawingController := &Controller{drawing: &drawing.HarukiDrawingClient{}}
	{
		_, err := drawingController.RenderQuery(drawing.SKRequest{})
		testutil.RequireArgs(t, !(err == nil), "invalid rendered query unexpectedly succeeded")
	}
	{

		_, err := controller.BuildCheckRoomRequest(drawing.CFRequest{})
		testutil.RequireArgs(t, !(err == nil), "check-room request without ranks unexpectedly succeeded")
	}
	{

		_, err := controller.BuildCheckRoomRequest(drawing.CFRequest{Ranks: []drawing.RankInfo{{Rank: 101}}})
		testutil.RequireArgs(t, !(err == nil), "out-of-range check-room rank unexpectedly succeeded")
	}

	check := drawing.CFRequest{Ranks: []drawing.RankInfo{{Rank: 100}}}
	{
		got, err := controller.BuildCheckRoomRequest(check)
		{
			testutil.Require(t, !(err != nil), "valid check-room request = %+v, %v", got, err)
			testutil.Require(t, reflect.DeepEqual(got, &check), "valid check-room request = %+v, %v", got, err)
		}
	}
	{

		_, err := (*Controller)(nil).RenderCheckRoom(check)
		testutil.RequireArgs(t, !(err == nil), "nil check-room renderer unexpectedly succeeded")
	}
	{

		_, err := drawingController.RenderCheckRoom(drawing.CFRequest{})
		testutil.RequireArgs(t, !(err == nil), "invalid rendered check-room request unexpectedly succeeded")
	}
	{

		_, err := controller.BuildCSBRequest(drawing.CSBRequest{})
		testutil.RequireArgs(t, !(err == nil), "CSB request without ranks unexpectedly succeeded")
	}
	{

		_, err := controller.BuildCSBRequest(drawing.CSBRequest{Ranks: []drawing.RankInfo{{Rank: 10}, {Rank: 101}}})
		testutil.RequireArgs(t, !(err == nil), "out-of-range final CSB rank unexpectedly succeeded")
	}

	csb := drawing.CSBRequest{Ranks: []drawing.RankInfo{{Rank: 100}}}
	{
		got, err := controller.BuildCSBRequest(csb)
		{
			testutil.Require(t, !(err != nil), "valid CSB request = %+v, %v", got, err)
			testutil.Require(t, reflect.DeepEqual(got, &csb), "valid CSB request = %+v, %v", got, err)
		}
	}
	{

		_, err := (*Controller)(nil).RenderCSB(csb)
		testutil.RequireArgs(t, !(err == nil), "nil CSB renderer unexpectedly succeeded")
	}
	{

		_, err := drawingController.RenderCSB(drawing.CSBRequest{})
		testutil.RequireArgs(t, !(err == nil), "invalid rendered CSB request unexpectedly succeeded")
	}
	{

		err := validateSKCheckRoomSupportedRanks([]drawing.RankInfo{{Rank: 1}, {Rank: 101}})
		testutil.RequireArgs(t, !(err == nil), "rank-list validation unexpectedly succeeded")
	}
	{

		err := validateSKCheckRoomSupportedRank(100)
		testutil.Require(t, !(err != nil), "rank 100 rejected: %v", err)
	}
	{

		_, err := controller.BuildWinRateRequest(drawing.WinRateRequest{})
		testutil.RequireArgs(t, !(err == nil), "win-rate request without teams unexpectedly succeeded")
	}

	winRate := drawing.WinRateRequest{TeamInfo: []drawing.TeamInfo{{TeamID: 1}}}
	{
		got, err := controller.BuildWinRateRequest(winRate)
		{
			testutil.Require(t, !(err != nil), "valid win-rate request = %+v, %v", got, err)
			testutil.Require(t, reflect.DeepEqual(got, &winRate), "valid win-rate request = %+v, %v", got, err)
		}
	}
	{

		_, err := (*Controller)(nil).RenderWinRate(winRate)
		testutil.RequireArgs(t, !(err == nil), "nil win-rate renderer unexpectedly succeeded")
	}
	{

		_, err := drawingController.RenderWinRate(drawing.WinRateRequest{})
		testutil.RequireArgs(t, !(err == nil), "invalid rendered win-rate request unexpectedly succeeded")
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
		testutil.Check(t, !(prev != tt.prev || next != tt.next || hasPrevious != tt.hasPrevious || hasNext != tt.hasNext), "queryAdjacentSKLineRanks(%d, %v) = %d, %d, %v, %v", tt.rank, tt.wl, prev, next, hasPrevious, hasNext)

	}
}

func TestTrackerIdentityAdditional(t *testing.T) {
	{
		_, _, _, err := (&Controller{}).buildSingleRankBaseFromTracker("jp", 1, 100, nil)
		testutil.RequireArgs(t, !(err == nil), "rank lookup without tracker unexpectedly succeeded")
	}
	{

		_, err := (&Controller{}).buildSingleRankFromTracker("jp", 1, 100, nil)
		testutil.RequireArgs(t, !(err == nil), "rank-name lookup without tracker unexpectedly succeeded")
	}
	{

		_, err := (&Controller{}).buildSingleUserBaseFromTracker("jp", 1, 123, nil)
		testutil.RequireArgs(t, !(err == nil), "user lookup without tracker unexpectedly succeeded")
	}

	queryErr := errors.New("query failed")
	controller := NewController(nil)
	controller.SetTrackerIntegration(&additionalCloudTrackerSource{queryErr: queryErr}, nil, nil)
	{
		_, _, _, err := controller.buildSingleRankBaseFromTracker("jp", 1, 100, nil)
		testutil.Require(t, errors.Is(err, queryErr), "rank query error = %v", err)
	}
	{

		_, err := controller.buildSingleUserFromTracker("jp", 1, 123, nil)
		testutil.Require(t, errors.Is(err, queryErr), "user query error = %v", err)
	}

	controller = NewController(nil)
	controller.SetTrackerIntegration(&additionalCloudTrackerSource{queryResp: &sekaiapi.CloudRankQueryResponse{}}, nil, nil)
	{
		_, _, _, err := controller.buildSingleRankBaseFromTracker("jp", 1, 100, nil)
		testutil.Require(t, errors.Is(err, sekaiapi.ErrRankingNotFound), "empty rank query error = %v", err)
	}
	{

		_, err := controller.buildSingleUserBaseFromTracker("jp", 1, 123, nil)
		testutil.Require(t, errors.Is(err, sekaiapi.ErrRankingNotFound), "empty user query error = %v", err)
	}

	uid := "123"
	rankItem := sekaiapi.CloudRankInfo{Rank: 100, UserID: &uid, Name: "Long Event 2026", Score: 9000, Timestamp: 1_700_000_000}
	controller = NewController(nil)
	controller.SetTrackerIntegration(&additionalCloudTrackerSource{queryResp: &sekaiapi.CloudRankQueryResponse{Ranks: []sekaiapi.CloudRankInfo{rankItem}}}, nil, nil)
	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP, event: &masterdata.Event{ID: 1, Name: "Long Event 2026"}})
	info, resolvedUID, hasUID, err := controller.buildSingleRankBaseFromTracker("jp", 1, 100, nil)
	{
		testutil.Require(t, !(err != nil), "rank identity = %+v, %d, %v, %v", info, resolvedUID, hasUID, err)
		testutil.Require(t, !(info.Rank != 100), "rank identity = %+v, %d, %v, %v", info, resolvedUID, hasUID, err)
		testutil.Require(t, !(resolvedUID != 123), "rank identity = %+v, %d, %v, %v", info, resolvedUID, hasUID, err)
		testutil.Require(t, hasUID, "rank identity = %+v, %d, %v, %v", info, resolvedUID, hasUID, err)
	}

	info, err = controller.buildSingleRankLatestFromTracker("jp", 1, 100, nil)
	{
		testutil.Require(t, !(err != nil), "latest rank identity = %+v, %v", info, err)
		testutil.Require(t, !(info.Name != "Long Event 2026"), "latest rank identity = %+v, %v", info, err)
	}

	info, err = controller.buildSingleRankFromTracker("jp", 1, 100, nil)
	{
		testutil.Require(t, !(err != nil), "sanitized rank identity = %+v, %v", info, err)
		testutil.Require(t, !(info.Name != "Rank 100"), "sanitized rank identity = %+v, %v", info, err)
	}

	info, err = controller.buildSingleUserBaseFromTracker("jp", 1, 123, nil)
	{
		testutil.Require(t, !(err != nil), "user identity = %+v, %v", info, err)
		testutil.Require(t, !(info.Rank != 100), "user identity = %+v, %v", info, err)
	}

}

func TestTrackerLineFastPathsAdditional(t *testing.T) {
	controller := &Controller{}
	{
		_, err := controller.buildSingleRankLineFromTracker("jp", 1, 100, nil)
		testutil.RequireArgs(t, !(err == nil), "rank line without tracker unexpectedly succeeded")
	}
	{

		_, err := controller.buildSingleUserLineFromTracker("jp", 1, 123, nil)
		testutil.RequireArgs(t, !(err == nil), "user line without tracker unexpectedly succeeded")
	}
	{

		got, err := controller.buildLineRanksFromTracker("jp", 1, nil, nil, false)
		{
			testutil.Require(t, !(err != nil), "empty fallback line ranks = %#v, %v", got, err)
			testutil.Require(t, !(len(got) != 0), "empty fallback line ranks = %#v, %v", got, err)
		}
	}
	{

		_, err := controller.buildLineRanksFromTracker("jp", 1, []int{100}, nil, false)
		testutil.RequireArgs(t, !(err == nil), "fallback line rank without tracker unexpectedly succeeded")
	}

	lineErr := errors.New("line failed")
	tracker := &additionalCloudTrackerSource{lineErr: lineErr}
	controller = NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	{
		_, err := controller.buildSingleRankLineFromTracker("jp", 1, 100, nil)
		testutil.Require(t, errors.Is(err, lineErr), "rank line error = %v", err)
	}
	{

		_, err := controller.buildSingleUserLineFromTracker("jp", 1, 123, nil)
		testutil.Require(t, errors.Is(err, lineErr), "user line error = %v", err)
	}
	{

		_, err := controller.buildLineRanksOrUserFromTracker("jp", 1, nil, additionalInt64Ptr(123), nil, false)
		{
			testutil.Require(t, !(err == nil), "wrapped user-line error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "tracker user query failed"), "wrapped user-line error = %v", err)
		}
	}

	tracker.lineErr = nil
	tracker.lineResp = &sekaiapi.CloudLineResponse{}
	{
		_, err := controller.buildSingleRankLineFromTracker("jp", 1, 100, nil)
		testutil.Require(t, errors.Is(err, sekaiapi.ErrRankingNotFound), "empty rank line error = %v", err)
	}
	{

		_, err := controller.buildSingleUserLineFromTracker("jp", 1, 123, nil)
		testutil.Require(t, errors.Is(err, sekaiapi.ErrRankingNotFound), "empty user line error = %v", err)
	}

	uid := "123"
	tracker.lineResp = &sekaiapi.CloudLineResponse{Ranks: []sekaiapi.CloudRankInfo{{Rank: 50, UserID: &uid, Name: "Visible", Score: 5000, Timestamp: 1_700_000_000}}}
	info, err := controller.buildSingleRankLineFromTracker("jp", 1, 50, nil)
	{
		testutil.Require(t, !(err != nil), "rank line = %+v, %v", info, err)
		testutil.Require(t, !(info.Rank != 50), "rank line = %+v, %v", info, err)
	}

	info, err = controller.buildSingleUserLineFromTracker("jp", 1, 123, nil)
	{
		testutil.Require(t, !(err != nil), "user line = %+v, %v", info, err)
		testutil.Require(t, !(info.Rank != 50), "user line = %+v, %v", info, err)
		testutil.Require(t, !(info.Name != ""), "user line = %+v, %v", info, err)
	}

	infos, err := controller.buildLineRanksOrUserFromTracker("jp", 1, nil, additionalInt64Ptr(123), nil, false)
	{
		testutil.Require(t, !(err != nil), "user line dispatch = %#v, %v", infos, err)
		testutil.Require(t, !(len(infos) != 1), "user line dispatch = %#v, %v", infos, err)
		testutil.Require(t, !(infos[0].Rank != 50), "user line dispatch = %#v, %v", infos, err)
	}

	infos, err = controller.buildLineRanksOrUserFromTracker("jp", 1, []int{50}, nil, nil, false)
	{
		testutil.Require(t, !(err != nil), "rank line dispatch = %#v, %v", infos, err)
		testutil.Require(t, !(len(infos) != 1), "rank line dispatch = %#v, %v", infos, err)
		testutil.Require(t, !(infos[0].Rank != 50), "rank line dispatch = %#v, %v", infos, err)
	}

	wrongUID := "999"
	tracker.lineResp = &sekaiapi.CloudLineResponse{Ranks: []sekaiapi.CloudRankInfo{{Rank: 60, UserID: &wrongUID, Name: "Wrong", Score: 6000}}}
	tracker.traceErr = errors.New("trace failed")
	{
		_, err := controller.buildSingleUserLineFromTracker("jp", 1, 123, nil)
		testutil.Require(t, errors.Is(err, tracker.traceErr), "mismatched-user trace error = %v", err)
	}

	tracker.traceErr = nil
	tracker.traceResp = &sekaiapi.CloudTraceResponse{RankData: []sekaiapi.CloudRankInfo{{Rank: 55, UserID: &uid, Name: "Trace", Score: 5500, Timestamp: 1_700_000_001}}}
	info, err = controller.buildSingleUserLineFromTracker("jp", 1, 123, nil)
	{
		testutil.Require(t, !(err != nil), "mismatched-user trace fallback = %+v, %v", info, err)
		testutil.Require(t, !(info.Rank != 55), "mismatched-user trace fallback = %+v, %v", info, err)
		testutil.Require(t, !(info.Name != ""), "mismatched-user trace fallback = %+v, %v", info, err)
	}

	points := []sekaiapi.RankDataPoint{{Score: 1, Timestamp: 2}, {Score: 3, Timestamp: 4}}
	{
		got := rankTraceSamples(points)
		testutil.Require(t, reflect.DeepEqual(got, []trackerScoreSample{{score: 1, timestamp: 2}, {score: 3, timestamp: 4}}), "rank trace samples = %#v", got)
	}

}

func TestTrackerQueryRequestBranchesAdditional(t *testing.T) {
	{
		_, err := (*Controller)(nil).BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}})
		testutil.RequireArgs(t, !(err == nil), "nil tracker query controller unexpectedly succeeded")
	}

	queryErr := errors.New("query failed")
	tracker := &additionalCloudTrackerSource{queryErr: queryErr}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	{
		_, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}})
		testutil.Require(t, errors.Is(err, queryErr), "rank query request error = %v", err)
	}
	{

		_, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, UserID: additionalInt64Ptr(123)})
		testutil.Require(t, errors.Is(err, queryErr), "user query request error = %v", err)
	}

	tracker.queryErr = nil
	tracker.queryResp = &sekaiapi.CloudRankQueryResponse{}
	{
		_, err := controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}})
		testutil.RequireArgs(t, !(err == nil), "empty tracker query unexpectedly succeeded")
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
	{
		testutil.Require(t, !(err != nil), "rank query payload = %+v, %v", payload, err)
		testutil.Require(t, !(len(payload.Ranks) != 1), "rank query payload = %+v, %v", payload, err)
		testutil.Require(t, !(payload.PrevRanks == nil), "rank query payload = %+v, %v", payload, err)
		testutil.Require(t, !(payload.NextRanks == nil), "rank query payload = %+v, %v", payload, err)
	}

	payload, err = controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, UserID: additionalInt64Ptr(123)})
	{
		testutil.Require(t, !(err != nil), "user query payload = %+v, %v", payload, err)
		testutil.Require(t, !(len(payload.Ranks) != 1), "user query payload = %+v, %v", payload, err)
		testutil.Require(t, !(payload.Ranks[0].Rank != 100), "user query payload = %+v, %v", payload, err)
	}

	wlCharacterID := 21
	payload, err = controller.BuildQueryRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}, WlCharacterID: &wlCharacterID})
	{
		testutil.Require(t, !(err != nil), "world-link query payload = %+v, %v", payload, err)
		testutil.Require(t, !(payload.WlCharaIconPath == nil), "world-link query payload = %+v, %v", payload, err)
		testutil.Require(t, !(payload.CharaIconPath == nil), "world-link query payload = %+v, %v", payload, err)
	}

	checkErr := errors.New("check failed")
	tracker.checkErr = checkErr
	{
		_, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}})
		testutil.Require(t, errors.Is(err, checkErr), "check-room tracker error = %v", err)
	}

	tracker.checkErr = nil
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{}
	{
		_, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}})
		testutil.Require(t, errors.Is(err, sekaiapi.ErrRankingNotFound), "empty check-room tracker error = %v", err)
	}

	averageRound := 1
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{
		Rank:     sekaiapi.CloudRankInfo{Rank: 100, UserID: &uid, Score: 1000, AverageRound: &averageRound},
		Previous: &previous,
		Next:     &next,
	}
	checkPayload, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{100}})
	{
		testutil.Require(t, !(err != nil), "check-room payload = %+v, %v", checkPayload, err)
		testutil.Require(t, !(len(checkPayload.Ranks) != 1), "check-room payload = %+v, %v", checkPayload, err)
		testutil.Require(t, !(checkPayload.PrevRank == nil), "check-room payload = %+v, %v", checkPayload, err)
		testutil.Require(t, !(checkPayload.NextRank == nil), "check-room payload = %+v, %v", checkPayload, err)
	}

	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Rank: sekaiapi.CloudRankInfo{Rank: 101, UserID: &uid, AverageRound: &averageRound}}
	{
		_, err := controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{101}})
		testutil.RequireArgs(t, !(err == nil), "out-of-range tracker check-room unexpectedly succeeded")
	}

	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Ranks: []sekaiapi.CloudRankInfo{
		{Rank: 20, UserID: additionalStringPtr("20"), AverageRound: &averageRound},
		{Rank: 10, UserID: additionalStringPtr("10"), AverageRound: &averageRound},
	}}
	checkPayload, err = controller.BuildCheckRoomRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{10, 20}})
	{
		testutil.Require(t, !(err != nil), "multi-rank check-room payload = %+v, %v", checkPayload, err)
		testutil.Require(t, !(len(checkPayload.Ranks) != 2), "multi-rank check-room payload = %+v, %v", checkPayload, err)
		testutil.Require(t, !(checkPayload.Ranks[0].Rank != 10), "multi-rank check-room payload = %+v, %v", checkPayload, err)
	}
	{

		_, err := controller.BuildCSBRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1, 2}})
		testutil.RequireArgs(t, !(err == nil), "multi-rank CSB tracker request unexpectedly succeeded")
	}

}

func TestTrackerValidationAndSpeedAdditional(t *testing.T) {
	controller := NewController(nil)
	{
		_, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}})
		testutil.RequireArgs(t, !(err == nil), "tracker validation without tracker unexpectedly succeeded")
	}

	tracker := &additionalCloudTrackerSource{}
	controller.SetTrackerIntegration(tracker, nil, nil)
	{
		_, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "invalid", EventID: 1, Ranks: []int{1}})
		testutil.RequireArgs(t, !(err == nil), "invalid tracker region unexpectedly succeeded")
	}
	{

		_, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "jp", EventID: 1, UserID: additionalInt64Ptr(0)})
		testutil.RequireArgs(t, !(err == nil), "empty normalized tracker target unexpectedly succeeded")
	}
	{

		_, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "jp", Ranks: []int{1}})
		testutil.RequireArgs(t, !(err == nil), "missing inferred event unexpectedly succeeded")
	}

	negativeWL := -1
	normalized, err := controller.validateTrackerQuery(TrackerRankQuery{
		Region:        " JP ",
		EventID:       1,
		Ranks:         []int{2, -1, 2, 1},
		CompareRank:   -2,
		WlCharacterID: &negativeWL,
	})
	{
		testutil.Require(t, !(err != nil), "normalized tracker query = %#v, %v", normalized, err)
		testutil.Require(t, reflect.DeepEqual(normalized.Ranks, []int{1, 2}), "normalized tracker query = %#v, %v", normalized, err)
		testutil.Require(t, !(normalized.CompareRank != 0), "normalized tracker query = %#v, %v", normalized, err)
		testutil.Require(t, !(normalized.WlCharacterID != nil), "normalized tracker query = %#v, %v", normalized, err)
	}

	controller.RegisterEventSource(&additionalEventSource{region: renderregion.JP, event: &masterdata.Event{ID: 1, EventType: "marathon"}})
	positiveWL := 21
	{
		_, err := controller.validateTrackerQuery(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}, WlCharacterID: &positiveWL})
		testutil.RequireArgs(t, !(err == nil), "world-link character on ordinary event unexpectedly succeeded")
	}

	period, unitPeriod, unit := normalizeTrackerSpeedConfig(TrackerRankQuery{SpeedUnit: "day"})
	{
		testutil.Require(t, !(period != 86_400), "daily speed config = %d, %d, %q", period, unitPeriod, unit)
		testutil.Require(t, !(unitPeriod != 86_400), "daily speed config = %d, %d, %q", period, unitPeriod, unit)
		testutil.Require(t, !(unit != "日"), "daily speed config = %d, %d, %q", period, unitPeriod, unit)
	}

	period, unitPeriod, unit = normalizeTrackerSpeedConfig(TrackerRankQuery{SpeedUnit: "unknown", SpeedPeriodSecs: 123})
	{
		testutil.Require(t, !(period != 123), "hourly speed config = %d, %d, %q", period, unitPeriod, unit)
		testutil.Require(t, !(unitPeriod != 3_600), "hourly speed config = %d, %d, %q", period, unitPeriod, unit)
		testutil.Require(t, !(unit != "时"), "hourly speed config = %d, %d, %q", period, unitPeriod, unit)
	}
	{
		testutil.RequireArgs(t, shouldSkipMissingTrackerRankError(true, sekaiapi.ErrRankingNotFound), "skip-missing tracker error predicate mismatch")
		testutil.RequireArgs(t, !(shouldSkipMissingTrackerRankError(false, sekaiapi.ErrRankingNotFound)), "skip-missing tracker error predicate mismatch")
		testutil.RequireArgs(t, !(shouldSkipMissingTrackerRankError(true, errors.New("other"))), "skip-missing tracker error predicate mismatch")
	}
	{

		got, err := controller.buildRanksFromTracker("jp", 1, nil, nil, false)
		{
			testutil.Require(t, !(err != nil), "empty tracker ranks = %#v, %v", got, err)
			testutil.Require(t, !(got != nil), "empty tracker ranks = %#v, %v", got, err)
		}
	}
	{

		_, err := (&Controller{}).buildRanksFromTracker("jp", 1, []int{1}, nil, false)
		testutil.RequireArgs(t, !(err == nil), "tracker rank build without cloud source unexpectedly succeeded")
	}
	{

		_, err := controller.BuildSpeedRequest(drawing.SpeedRequest{})
		testutil.RequireArgs(t, !(err == nil), "speed request without ranks unexpectedly succeeded")
	}

	speedRequest := drawing.SpeedRequest{Ranks: []drawing.SpeedInfo{{Rank: 1}}}
	{
		got, err := controller.BuildSpeedRequest(speedRequest)
		{
			testutil.Require(t, !(err != nil), "valid speed request = %+v, %v", got, err)
			testutil.Require(t, reflect.DeepEqual(got, &speedRequest), "valid speed request = %+v, %v", got, err)
		}
	}
	{

		_, err := (*Controller)(nil).RenderSpeed(speedRequest)
		testutil.RequireArgs(t, !(err == nil), "nil speed renderer unexpectedly succeeded")
	}
	{

		_, err := (&Controller{drawing: &drawing.HarukiDrawingClient{}}).RenderSpeed(drawing.SpeedRequest{})
		testutil.RequireArgs(t, !(err == nil), "invalid rendered speed request unexpectedly succeeded")
	}
	{

		_, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, UserID: additionalInt64Ptr(1)})
		testutil.RequireArgs(t, !(err == nil), "user speed query unexpectedly succeeded")
	}

	tracker.speedErr = errors.New("speed failed")
	{
		_, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}})
		testutil.Require(t, errors.Is(err, tracker.speedErr), "tracker speed error = %v", err)
	}

	tracker.speedErr = nil
	speed := 120
	tracker.speedResp = &sekaiapi.CloudSpeedResponse{Speeds: []sekaiapi.CloudRankInfo{{Rank: 1, Score: 1000, Speed: &speed, Timestamp: 1_700_000_000}}}
	payload, err := controller.BuildSpeedRequestFromTracker(TrackerRankQuery{Region: "jp", EventID: 1, Ranks: []int{1}, SpeedUnit: "day"})
	{
		testutil.Require(t, !(err != nil), "tracker speed payload = %+v, %v", payload, err)
		testutil.Require(t, !(len(payload.Ranks) != 1), "tracker speed payload = %+v, %v", payload, err)
		testutil.Require(t, !(payload.Ranks[0].Speed == nil), "tracker speed payload = %+v, %v", payload, err)
		testutil.Require(t, !(payload.RequestType != "日"), "tracker speed payload = %+v, %v", payload, err)
	}

}

func additionalInt64Ptr(value int64) *int64 { return &value }
