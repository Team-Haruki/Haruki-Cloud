package drawing

import (
	"testing"
	"time"
)

func TestResolveRenderCacheRuleUsesOneDayTTLByDefault(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/profile")
	if rule.TTL != renderCacheTTLOneDay {
		t.Fatalf("expected default ttl %s, got %s", renderCacheTTLOneDay, rule.TTL)
	}
}

func TestResolveRenderCacheRuleUsesHalfDayTTLForSelectedEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"/api/pjsk/deck/recommend",
		"/api/pjsk/music/rewards/basic",
		"/api/pjsk/music/rewards/detail",
		"/api/pjsk/mysekai/map",
		"/api/pjsk/mysekai/resource",
		"/api/pjsk/mysekai/talk-list",
	} {
		rule := resolveRenderCacheRule(endpoint)
		if rule.TTL != renderCacheTTLHalfDay {
			t.Fatalf("%s ttl = %s, want %s", endpoint, rule.TTL, renderCacheTTLHalfDay)
		}
	}
}

func TestBuildRenderCachePolicyAliasListIsInfiniteAndIgnoresDT(t *testing.T) {
	reqA := map[string]any{
		"title":        "角色别名",
		"entity_label": "角色ID",
		"entity_id":    5,
		"entity_name":  "花里みのり",
		"aliases":      []any{"花里", "花里みのり", "minori"},
		"dt":           int64(1713852000000), // 2024-04-23 12:00:00 UTC
	}
	reqB := map[string]any{
		"title":        "角色别名",
		"entity_label": "角色ID",
		"entity_id":    5,
		"entity_name":  "花里みのり",
		"aliases":      []any{"花里", "花里みのり", "minori"},
		"dt":           int64(1713852660000), // +11m
	}

	policyA, err := buildRenderCachePolicy("/api/pjsk/misc/alias-list", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/misc/alias-list", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("alias-list key should ignore dt when aliases are unchanged: %s != %s", keyA, keyB)
	}
	if !policyA.Infinite {
		t.Fatalf("expected alias-list cache policy to be infinite")
	}
	if policyA.TTL != 0 {
		t.Fatalf("expected infinite alias-list cache ttl to be 0, got %v", policyA.TTL)
	}
}

func TestResolveRenderCacheRuleUsesInfiniteTTLForStaticEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"/api/pjsk/card/detail",
		"/api/pjsk/card/list",
		"/api/pjsk/mysekai/fixture-list",
		"/api/pjsk/mysekai/fixture-detail",
	} {
		rule := resolveRenderCacheRule(endpoint)
		if !rule.Infinite {
			t.Fatalf("%s should use infinite ttl", endpoint)
		}
		if rule.TTL != 0 {
			t.Fatalf("%s ttl = %s, want 0 for infinite ttl", endpoint, rule.TTL)
		}
	}
}

func TestBuildRenderCachePolicyEventListUsesDynamicWindowTTL(t *testing.T) {
	now := int64(1774118400000)
	endAt := now + int64((2*time.Hour)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"dt": now,
		"event_info": []any{
			map[string]any{
				"id":       101,
				"start_at": now - int64((time.Hour)/time.Millisecond),
				"end_at":   endAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	want := 2*time.Hour + renderCacheWindowTTLBuffer
	if policy.TTL != want {
		t.Fatalf("event list ttl = %v, want %v", policy.TTL, want)
	}
	if policy.Infinite {
		t.Fatal("event list should no longer use infinite ttl")
	}
}

func TestBuildRenderCachePolicyVLiveUsesDynamicWindowTTL(t *testing.T) {
	now := int64(1774118400000)
	endAt := now + int64((90*time.Minute)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/vlive/list", map[string]any{
		"dt": now,
		"lives": []any{
			map[string]any{
				"id":     1,
				"end_at": endAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	want := 90*time.Minute + renderCacheWindowTTLBuffer
	if policy.TTL != want {
		t.Fatalf("vlive ttl = %v, want %v", policy.TTL, want)
	}
}

func TestBuildRenderCachePolicyWindowTTLUsesMinimumForPastData(t *testing.T) {
	now := int64(1774118400000)
	endAt := now - int64((time.Hour)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/event/detail", map[string]any{
		"dt": now,
		"event_info": map[string]any{
			"id":     101,
			"end_at": endAt,
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	if policy.TTL != renderCacheWindowTTLMin {
		t.Fatalf("past event detail ttl = %v, want %v", policy.TTL, renderCacheWindowTTLMin)
	}
}

func TestBuildRenderCachePolicyMarksCardListAsInfinite(t *testing.T) {
	policy, err := buildRenderCachePolicy("/api/pjsk/card/list", CardListRequest{
		Region: "JP",
		Cards: []CardBasic{
			{CardID: 1001},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	if !policy.Infinite {
		t.Fatalf("expected card list cache policy to be infinite")
	}
	if policy.TTL != 0 {
		t.Fatalf("expected infinite cache policy ttl to be 0, got %s", policy.TTL)
	}
}

func TestBuildRenderCachePolicyIgnoresEventRecordUserUpdateTime(t *testing.T) {
	reqA := EventRecordRequest{
		EventInfo: []EventHistory{{ID: 1, EventName: "Event"}},
		UserInfo: DetailedProfileCardRequest{
			ID:              "123",
			Region:          "JP",
			Nickname:        "Tester",
			Source:          "suite",
			UpdateTime:      1,
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
	}
	reqB := reqA
	reqB.UserInfo.UpdateTime = 2

	policyA, err := buildRenderCachePolicy("/api/pjsk/event/record", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/event/record", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("event record key should ignore user update time: %s != %s", keyA, keyB)
	}
	if policyA.APIPath != policyB.APIPath {
		t.Fatalf("event record api_path should stay stable: %s != %s", policyA.APIPath, policyB.APIPath)
	}
}

func TestBuildRenderCachePolicyIgnoresUnusedProfileUpdateTime(t *testing.T) {
	reqA := ProfileRequest{
		Profile: BasicProfile{
			ID:              "123",
			Region:          "JP",
			Nickname:        "Tester",
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
		UpdateTime: new(int64(1)),
	}
	reqB := reqA
	reqB.UpdateTime = new(int64(2))

	policyA, err := buildRenderCachePolicy("/api/pjsk/profile", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/profile", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("profile key should ignore unused update_time: %s != %s", keyA, keyB)
	}
	if policyA.APIPath != policyB.APIPath {
		t.Fatalf("profile api_path should stay stable: %s != %s", policyA.APIPath, policyB.APIPath)
	}
}

func TestBuildRenderCachePolicyBucketsSKWinRateUpdatedAtBy10Seconds(t *testing.T) {
	reqA := WinRateRequest{
		UpdatedAt:        1774118400000,
		EventStartAt:     10,
		EventAggregateAt: 1774118404000,
		TeamInfo: []TeamInfo{
			{TeamID: 1, TeamName: "A", WinRate: 0.5},
			{TeamID: 2, TeamName: "B", WinRate: 0.5},
		},
	}
	reqB := reqA
	reqB.UpdatedAt = 1774118409000
	reqB.EventAggregateAt = 1774118409000
	reqC := reqA
	reqC.UpdatedAt = 1774118411000
	reqC.EventAggregateAt = 1774118411000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("winrate key should bucket updated_at/event_aggregate_at within 10s: %s != %s", keyA, keyB)
	}
	if keyA == keyC {
		t.Fatalf("winrate key should change after 10s bucket boundary")
	}
	if policyA.TTL != 10*time.Second {
		t.Fatalf("expected 10s ttl for winrate cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyMusicListUsesRenderFlagsAndPublicFallback(t *testing.T) {
	req := MusicListRequest{
		UserResults: map[int]any{1: "ap"},
		MusicList: []map[string]any{
			{"id": 1, "difficulty": 32},
		},
		RequiredDifficulties: "master",
		Profile: &DetailedProfileCardRequest{
			ID:              "service",
			Region:          "JP",
			Nickname:        "Lunabot",
			Source:          "lunabot-service",
			UpdateTime:      1,
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
	}

	showPolicy, err := buildRenderCachePolicy("/api/pjsk/music/list?show_id=true&show_leak=false", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy show: %v", err)
	}
	hidePolicy, err := buildRenderCachePolicy("/api/pjsk/music/list?show_id=false&show_leak=false", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy hide: %v", err)
	}

	if showPolicy.UserID != renderCachePublic {
		t.Fatalf("expected public fallback user_id, got %s", showPolicy.UserID)
	}
	if showPolicy.APIPath != "api/pjsk/music/list" {
		t.Fatalf("unexpected api_path: %s", showPolicy.APIPath)
	}
	if showPolicy.APIPath != hidePolicy.APIPath {
		t.Fatalf("expected stable api_path across render flags")
	}

	keyShow, err := buildRenderCacheKey(showPolicy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey show: %v", err)
	}
	keyHide, err := buildRenderCacheKey(hidePolicy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey hide: %v", err)
	}
	if keyShow == keyHide {
		t.Fatalf("expected different keys for different render flags")
	}
}

func TestBuildRenderCachePolicySKQueryIgnoresTopLevelEventIDForUserID(t *testing.T) {
	req := SKRequest{
		ID:     1,
		Region: "JP",
		Name:   "Event",
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester"},
		},
	}

	policy, err := buildRenderCachePolicy("/api/pjsk/sk/query", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}

	if policy.UserID != renderCachePublic {
		t.Fatalf("expected public user_id, got %s", policy.UserID)
	}
	if policy.APIPath != "api/pjsk/sk/query" {
		t.Fatalf("unexpected api_path: %s", policy.APIPath)
	}
}

func TestBuildRenderCachePolicySKQueryOnlyChangesWhenPulledInfoChanges(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "JP",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqC := reqA
	reqC.AggregateAt = 1774118420000
	reqC.Ranks[0].Time = 1774118420000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("sk query key should stay stable when pulled info is unchanged: %s != %s", keyA, keyB)
	}
	if keyA == keyC {
		t.Fatalf("sk query key should change after pulled info changes")
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyTWSKQueryDoesNotBucketByTime(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "TW",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqB.AggregateAt = 1774118429000
	reqB.Ranks[0].Time = 1774118429000
	reqC := reqA
	reqC.AggregateAt = 1774118404000
	reqC.Ranks[0].Time = 1774118405000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA == keyB {
		t.Fatalf("tw sk query key should change when tracker timestamps differ")
	}
	if keyA != keyC {
		t.Fatalf("tw sk query key should remain identical for identical content: %s != %s", keyA, keyC)
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for tw sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyENSKQueryDoesNotBucketByTime(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "EN",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqB.AggregateAt = 1774118459000
	reqB.Ranks[0].Time = 1774118459000
	reqC := reqA
	reqC.AggregateAt = 1774118404000
	reqC.Ranks[0].Time = 1774118405000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA == keyB {
		t.Fatalf("en sk query key should change when tracker timestamps differ")
	}
	if keyA != keyC {
		t.Fatalf("en sk query key should remain identical for identical content: %s != %s", keyA, keyC)
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for en sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyIgnoresRootDT(t *testing.T) {
	policyA, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"region": "JP",
		"dt":     1774118400000,
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"region": "JP",
		"dt":     1774118700000,
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("expected dt to be ignored by cache key: %s != %s", keyA, keyB)
	}
}
