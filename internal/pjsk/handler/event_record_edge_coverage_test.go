package handler

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

func TestEventRecordPureEdgeBranches(t *testing.T) {
	if _, err := buildEventRecordFromSnapshot(nil, renderregion.JP); err == nil {
		t.Fatal("nil event-record runtime unexpectedly succeeded")
	}
	if hasEventRecordHistorySource(nil) || hasEventRecordHistorySource(&rendersnapshot.RawUserData{}) {
		t.Fatal("empty snapshot unexpectedly has event history")
	}
	if !hasEventRecordHistorySource(&rendersnapshot.RawUserData{UserEventResults: []rendersnapshot.RawUserEventResult{{EventID: 1}}}) {
		t.Fatal("event result should count as history")
	}

	if eventRecordRankBoundaries(json.RawMessage("{")) != nil {
		t.Fatal("invalid rank ranges should return nil")
	}
	rawRanges := json.RawMessage(`[
		{"fromRank":1,"toRank":10},
		{"fromRank":20,"toRank":0}
	]`)
	boundaries := eventRecordRankBoundaries(rawRanges)
	for _, rank := range []int{1, 10, 19, 20} {
		if _, ok := boundaries[rank]; !ok {
			t.Fatalf("missing rank boundary %d: %#v", rank, boundaries)
		}
	}

	if eventRecordHonorTiers(json.RawMessage("{"), nil) != nil {
		t.Fatal("invalid honor tiers should return nil")
	}
	honorRanges := json.RawMessage(`[
		{"toRank":0,"eventRankingRewardDetails":[{"resourceType":"honor","resourceId":1}]},
		{"toRank":100,"eventRankingRewardDetails":[{"resourceType":"honor","resourceId":0},{"resource_type":"HONOR","resource_id":7}]},
		{"toRank":50,"event_ranking_rewards":[{"resource_box_id":9}]}
	]`)
	tiers := eventRecordHonorTiers(honorRanges, map[int][]int{9: {7, 8}})
	if tiers[7] != 50 || tiers[8] != 50 {
		t.Fatalf("honor tiers = %#v", tiers)
	}

	ids := eventRecordHonorIDsFromRewardRange(eventRecordRankingRewardRange{
		EventRankingRewardDetails2: []eventRecordRankingRewardDetail{{ResourceType2: "honor", ResourceID2: 3}},
		EventRankingRewards:        []eventRecordRankingReward{{ResourceBoxID: 0, ResourceBoxID2: 4}, {ResourceBoxID: 0}},
	}, map[int][]int{4: {5}})
	if !reflect.DeepEqual(ids, []int{3, 5}) {
		t.Fatalf("reward honor ids = %#v", ids)
	}

	if eventRecordResourceBoxHonorIDsForPurpose(nil, renderregion.JP, "x") != nil {
		t.Fatal("nil resource-box runtime should return nil")
	}
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}}
	if eventRecordResourceBoxHonorIDsForPurpose(rc, renderregion.JP, "x") != nil {
		t.Fatal("unconfigured resource-box runtime should return nil")
	}
	if buildEventRecordWorldBloomChapterHonorRankDisplays(nil, renderregion.JP, nil, nil) != nil {
		t.Fatal("nil world-bloom data should return nil")
	}
}

func TestEventRecordHonorAndTimingBranches(t *testing.T) {
	userHonors := map[int]struct{}{7: {}}
	if eventRecordUserHasAnyHonor(userHonors, []int{0, 8}) || !eventRecordUserHasAnyHonor(userHonors, []int{0, 7}) {
		t.Fatal("honor membership mismatch")
	}
	if eventRecordWorldBloomChapterClosed(nil, 10) {
		t.Fatal("nil chapter should not be closed")
	}
	characterID := 1
	chapter := &masterdata.WorldBloom{GameCharacterID: &characterID, AggregateAt: 10}
	if eventRecordWorldBloomChapterClosed(chapter, 9) || !eventRecordWorldBloomChapterClosed(chapter, 10) {
		t.Fatal("aggregate fallback close time mismatch")
	}

	out := map[int][]int{}
	eventRecordAddHonorDetails(out, 0, []eventRecordRankingRewardDetail{{ResourceType: "honor", ResourceID: 1}})
	eventRecordAddHonorDetails(out, 2, []eventRecordRankingRewardDetail{
		{ResourceType: "material", ResourceID: 9},
		{ResourceType2: " HONOR ", ResourceID2: 7},
	})
	if !reflect.DeepEqual(out[2], []int{7}) {
		t.Fatalf("added honor details = %#v", out)
	}
	if eventRecordDetailResourceType(eventRecordRankingRewardDetail{ResourceType2: " HONOR "}) != "honor" ||
		eventRecordDetailResourceID(eventRecordRankingRewardDetail{ResourceID2: 4}) != 4 {
		t.Fatal("legacy reward detail fallback mismatch")
	}

	if buildEventRecordHonorRankDisplays(nil, nil, nil) != nil {
		t.Fatal("nil honor rank display input should return nil")
	}
	raw := &rendersnapshot.RawUserData{
		Now:               100,
		UserHonors:        []rendersnapshot.RawUserHonor{{HonorID: 7}, {HonorID: 0}},
		UserProfileHonors: []rendersnapshot.RawUserProfileHonor{{HonorID: 8, HonorId2: 9}, {HonorID: 0, HonorId2: 0}},
	}
	displays := buildEventRecordHonorRankDisplays(raw,
		map[int]eventMasterEntry{1: {"aggregateAt": int64(90)}, 2: {"closedAt": float64(200)}},
		map[int]map[int]int{1: {7: 100, 8: 50, 99: 0}, 2: {7: 1}},
	)
	if displays[1].tier != 50 {
		t.Fatalf("rank displays = %#v", displays)
	}
	if collectEventRecordUserHonorIDs(nil) != nil {
		t.Fatal("nil user honors should return nil")
	}
	if eventRecordRankingSettled(map[string]any{"closedAt": json.Number("9")}, 9) != true {
		t.Fatal("closedAt fallback should be settled")
	}
	if eventRecordRankNote(renderregion.JP) != nil || eventRecordRankNote(renderregion.CN) == nil {
		t.Fatal("rank note region classification mismatch")
	}
}

func TestEventRecordSortingAndConversionEdges(t *testing.T) {
	ranks := map[int]int{1: 999, 2: 1000, 3: 2000}
	dropUntrustedEventRecordSnapshotRanks(renderregion.CN, ranks, map[int]map[int]struct{}{
		2: {1000: {}}, 3: {1999: {}},
	})
	if _, ok := ranks[2]; ok || ranks[1] != 999 || ranks[3] != 2000 {
		t.Fatalf("filtered ranks = %#v", ranks)
	}
	dropUntrustedEventRecordSnapshotRanks(renderregion.JP, ranks, map[int]map[int]struct{}{3: {2000: {}}})
	if ranks[3] != 2000 {
		t.Fatal("JP ranks should remain trusted")
	}

	if buildEventHistoryFromMaster(nil, 1, nil, nil, "jp") != nil {
		t.Fatal("empty event master should return nil")
	}
	rankDisplay := "T20"
	rankTier := 10
	point := 5
	items := []drawing.EventHistory{
		{ID: 1, RankDisplay: &rankDisplay, EventPoint: &point, StartAt: float64(1)},
		{ID: 2, RankTier: &rankTier, StartAt: float64(2)},
		{ID: 3, EventPoint: nil, StartAt: float64(3)},
	}
	sortEventHistory(items)
	if items[0].ID != 2 || eventHistoryPoint(items[2]) != 0 {
		t.Fatalf("sorted event history = %#v", items)
	}
	for _, text := range []string{"", "rank1", "T0", "Tbad"} {
		if _, ok := eventHistorySortRankDisplay(text); ok {
			t.Fatalf("invalid rank display %q accepted", text)
		}
	}
	if rank, ok := eventHistorySortRankDisplay(" t12 "); !ok || rank != 12 {
		t.Fatalf("rank display parse = %d, %v", rank, ok)
	}

	if stringVal(map[string]any{"x": 1}, "x") != "" || intVal(map[string]any{"x": json.Number("4")}, "x") != 4 ||
		int64Val(map[string]any{"x": json.Number("5")}, "x") != 5 || int64Val(map[string]any{"x": "bad"}, "x") != 0 {
		t.Fatal("event master conversion mismatch")
	}
}
