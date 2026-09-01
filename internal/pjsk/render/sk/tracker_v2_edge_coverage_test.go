package sk

import (
	"errors"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestTrackerV2UserQueryEdgeCoverage(t *testing.T) {
	if _, ok, err := (&Controller{}).buildUserFromTrackerCloudV2("jp", 1, 10, nil); ok || err != nil {
		t.Fatalf("unconfigured user query = %v, %v", ok, err)
	}

	uid := "10"
	tracker := &additionalCloudTrackerSource{queryResp: &sekaiapi.CloudRankQueryResponse{
		Ranks:    []sekaiapi.CloudRankInfo{{Rank: 20, UserID: &uid}, {Rank: 10, UserID: &uid}},
		Previous: &sekaiapi.CloudRankInfo{Rank: 9},
		Next:     &sekaiapi.CloudRankInfo{Rank: 21},
	}}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	infos, previous, next, ok, err := controller.buildUserQueryFromTrackerV2("jp", 1, 10, nil, true)
	if err != nil || !ok || len(infos) != 2 || infos[0].Rank != 10 || previous == nil || next == nil {
		t.Fatalf("user query = %#v, %#v, %#v, %v, %v", infos, previous, next, ok, err)
	}

	wrongUID := "11"
	traceErr := errors.New("trace failed")
	tracker.queryResp = &sekaiapi.CloudRankQueryResponse{Ranks: []sekaiapi.CloudRankInfo{{Rank: 5, UserID: &wrongUID}}}
	tracker.traceErr = traceErr
	if _, _, _, ok, err := controller.buildUserQueryFromTrackerV2("jp", 1, 10, nil, false); !ok || !errors.Is(err, traceErr) {
		t.Fatalf("mismatched user trace error = %v, %v", ok, err)
	}
}

func TestTrackerV2CheckRoomEdgeCoverage(t *testing.T) {
	if _, _, _, ok, err := (&Controller{}).buildCheckRoomFromTrackerCloudV2("jp", 1, []int{1}, nil, nil, false); ok || err != nil {
		t.Fatalf("unconfigured check room = %v, %v", ok, err)
	}

	uid := "10"
	tracker := &additionalCloudTrackerSource{checkResp: &sekaiapi.CloudCheckRoomResponse{
		Ranks:    []sekaiapi.CloudRankInfo{{Rank: 10, UserID: &uid}},
		Previous: &sekaiapi.CloudRankInfo{Rank: 9},
		Next:     &sekaiapi.CloudRankInfo{Rank: 11},
	}}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	info, previous, next, ok, err := controller.buildCheckRoomFromTrackerCloudV2("jp", 1, []int{10}, nil, nil, false)
	if err != nil || !ok || info.Rank != 10 || previous == nil || next == nil {
		t.Fatalf("check room fallback = %#v, %#v, %#v, %v, %v", info, previous, next, ok, err)
	}

	wrongUID := "11"
	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Rank: sekaiapi.CloudRankInfo{Rank: 5, UserID: &wrongUID}}
	tracker.traceErr = errors.New("trace failed")
	userID := int64(10)
	if _, _, _, _, err := controller.buildCheckRoomFromTrackerCloudV2("jp", 1, nil, &userID, nil, false); !errors.Is(err, tracker.traceErr) {
		t.Fatalf("check room mismatched trace error = %v", err)
	}
}

func TestTrackerV2MultiCheckRoomEdgeCoverage(t *testing.T) {
	if _, _, _, ok, err := (&Controller{}).buildCheckRoomRanksFromTrackerCloudV2("jp", 1, []int{1}, nil, false); ok || err != nil {
		t.Fatalf("unconfigured multi check room = %v, %v", ok, err)
	}

	tracker := &additionalCloudTrackerSource{checkResp: &sekaiapi.CloudCheckRoomResponse{Rank: sekaiapi.CloudRankInfo{Rank: 5}}}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	infos, _, _, ok, err := controller.buildCheckRoomRanksFromTrackerCloudV2("jp", 1, []int{5}, nil, false)
	if err != nil || !ok || len(infos) != 1 || infos[0].Rank != 5 {
		t.Fatalf("single-rank fallback = %#v, %v, %v", infos, ok, err)
	}

	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Ranks: []sekaiapi.CloudRankInfo{{}, {Rank: 2}, {Rank: 1}}}
	infos, _, _, _, err = controller.buildCheckRoomRanksFromTrackerCloudV2("jp", 1, []int{1, 2}, nil, true)
	if err != nil || len(infos) != 2 || infos[0].Rank != 1 {
		t.Fatalf("skip-missing multi check room = %#v, %v", infos, err)
	}
	if _, _, _, _, err := controller.buildCheckRoomRanksFromTrackerCloudV2("jp", 1, []int{1}, nil, false); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("strict missing-rank error = %v", err)
	}

	tracker.checkResp = &sekaiapi.CloudCheckRoomResponse{Ranks: []sekaiapi.CloudRankInfo{{}}}
	if _, _, _, _, err := controller.buildCheckRoomRanksFromTrackerCloudV2("jp", 1, []int{1}, nil, true); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("all-missing rank error = %v", err)
	}
}

func TestTrackerV2TraceAndLookupEdgeCoverage(t *testing.T) {
	controller := &Controller{}
	if _, ok, err := controller.buildSubjectTraceFromTrackerV2("jp", 1, "rank", "1", nil); ok || err != nil {
		t.Fatalf("unconfigured subject trace = %v, %v", ok, err)
	}
	if _, ok, err := controller.buildSpeedInfosFromTrackerV2("jp", 1, []int{1}, nil, 60, 3600, false); ok || err != nil {
		t.Fatalf("unconfigured speed query = %v, %v", ok, err)
	}
	if _, err := latestUserTraceFromTrackerV2(nil, "jp", 1, 1, nil); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("nil trace source error = %v", err)
	}

	tracker := &additionalCloudTrackerSource{traceResp: &sekaiapi.CloudTraceResponse{}}
	controller = NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	if _, ok, err := controller.buildSubjectTraceFromTrackerV2("jp", 1, "rank", "1", nil); !ok || !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("empty subject trace = %v, %v", ok, err)
	}
	tracker.traceResp = &sekaiapi.CloudTraceResponse{RankData: []sekaiapi.CloudRankInfo{{Rank: 0}}}
	if _, err := latestUserTraceFromTrackerV2(tracker, "jp", 1, 10, nil); !errors.Is(err, sekaiapi.ErrRankingNotFound) {
		t.Fatalf("invalid user trace points error = %v", err)
	}
}

func TestTrackerV2UserIDResolutionEdgeCoverage(t *testing.T) {
	if _, ok, err := (&Controller{}).resolveTrackerUserIDByRankFromCloudV2("jp", 1, 1, nil); ok || err != nil {
		t.Fatalf("unconfigured user-id lookup = %v, %v", ok, err)
	}

	tracker := &additionalCloudTrackerSource{queryErr: errors.New("query failed")}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	if _, ok, err := controller.resolveTrackerUserIDByRankFromCloudV2("jp", 1, 1, nil); !ok || !errors.Is(err, tracker.queryErr) {
		t.Fatalf("user-id query error = %v, %v", ok, err)
	}
	tracker.queryErr = nil
	tracker.queryResp = &sekaiapi.CloudRankQueryResponse{}
	if _, _, err := controller.resolveTrackerUserIDByRankFromCloudV2("jp", 1, 1, nil); err == nil {
		t.Fatal("empty user-id lookup unexpectedly succeeded")
	}
	invalid := "bad"
	tracker.queryResp = &sekaiapi.CloudRankQueryResponse{Ranks: []sekaiapi.CloudRankInfo{{UserID: &invalid}}}
	if _, _, err := controller.resolveTrackerUserIDByRankFromCloudV2("jp", 1, 1, nil); err == nil {
		t.Fatal("invalid user-id lookup unexpectedly succeeded")
	}

	if cloudRankInfoMatchesUser(0, sekaiapi.CloudRankInfo{}) {
		t.Fatal("non-positive user unexpectedly matched")
	}
	if !cloudRankInfoMatchesUser(10, sekaiapi.CloudRankInfo{}) {
		t.Fatal("missing upstream user id should remain compatible")
	}
}

func TestTrackerV2MetricGuardEdgeCoverage(t *testing.T) {
	if hasRankInfoRoundMetrics(nil) {
		t.Fatal("nil rank info unexpectedly has metrics")
	}
	info := drawing.RankInfo{}
	(*Controller)(nil).enrichRankInfoFromCloudV2Trace("jp", 1, nil, sekaiapi.CloudRankInfo{}, &info)
	(&Controller{}).enrichRankInfoFromCloudV2Trace("jp", 1, nil, sekaiapi.CloudRankInfo{}, &info)

	tracker := &additionalCloudTrackerSource{traceErr: errors.New("trace failed")}
	controller := NewController(nil)
	controller.SetTrackerIntegration(tracker, nil, nil)
	if controller.applyCloudV2TraceMetrics(tracker, "jp", 1, nil, "rank", "1", &info) {
		t.Fatal("failed trace unexpectedly produced metrics")
	}
}

func TestTrackerGrowthPointHelperEdgeCoverage(t *testing.T) {
	earlierScore := 200
	earlierTime := int64(2_000)
	point := sekaiapi.ScoreGrowthPoint{ScoreLatest: 100, ScoreEarlier: &earlierScore, TimestampLatest: 1_000, TimestampEarlier: &earlierTime}
	if trackerPointGrowth(point) != nil {
		t.Fatal("negative derived growth unexpectedly accepted")
	}
	if trackerPointTimeDiff(point) != nil {
		t.Fatal("negative derived time difference unexpectedly accepted")
	}
	if got := latestTrackerPointScore(sekaiapi.ScoreGrowthPoint{ScoreEarlier: &earlierScore}); got != earlierScore {
		t.Fatalf("fallback score = %d", got)
	}
	if got := latestTrackerPointTimestamp(sekaiapi.ScoreGrowthPoint{TimestampEarlier: &earlierTime}); got != earlierTime {
		t.Fatalf("fallback timestamp = %d", got)
	}
}
