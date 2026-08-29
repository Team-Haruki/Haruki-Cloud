package education

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

func TestCharacterMissionQueryHelpers(t *testing.T) {
	if got := CharacterMissionShortName("play_live"); got != "队长次数" {
		t.Fatalf("known short name = %q", got)
	}
	if got := CharacterMissionShortName("future_type"); got != "future_type" {
		t.Fatalf("unknown short name = %q", got)
	}
	if got := normalizeCharacterMissionQuery("  Another Vocal（EX） "); got != "another vocal(ex)" {
		t.Fatalf("normalized query = %q", got)
	}

	allTests := []struct {
		input     string
		wantAll   bool
		wantQuery string
	}{
		{input: "全部 一歌", wantAll: true, wantQuery: "一歌"},
		{input: "一歌 all", wantAll: true, wantQuery: "一歌"},
		{input: "  一歌  ", wantQuery: "一歌"},
	}
	for _, tt := range allTests {
		gotAll, gotQuery := ExtractCharacterMissionAllFlag(tt.input)
		if gotAll != tt.wantAll || gotQuery != tt.wantQuery {
			t.Errorf("ExtractCharacterMissionAllFlag(%q) = %v, %q; want %v, %q", tt.input, gotAll, gotQuery, tt.wantAll, tt.wantQuery)
		}
	}

	typeTests := []struct {
		input     string
		wantType  string
		wantQuery string
	}{
		{input: "一歌 四星技能", wantType: "skill_level_up_rare"},
		{input: "一歌 ANVO", wantType: "collect_another_vocal"},
		{input: "  一歌  ", wantQuery: "一歌"},
	}
	for _, tt := range typeTests {
		gotType, gotQuery := ExtractCharacterMissionType(tt.input)
		if gotType != tt.wantType || gotQuery != tt.wantQuery {
			t.Errorf("ExtractCharacterMissionType(%q) = %q, %q; want %q, %q", tt.input, gotType, gotQuery, tt.wantType, tt.wantQuery)
		}
	}
}

func TestCharacterMissionCloneAndLookupHelpers(t *testing.T) {
	if cloneCharacterMissions(nil) != nil || cloneCharacterMissionParameterGroups(nil) != nil || cloneCharacterLevels(nil) != nil {
		t.Fatal("nil clone input should produce nil")
	}

	mission := &CharacterMission{ID: 1, CharacterID: 2, CharacterMissionType: "play_live", ParameterGroupID: 3, IsAchievementMission: true}
	missions := cloneCharacterMissions([]*CharacterMission{nil, mission})
	if len(missions) != 1 || missions[0] == mission || !reflect.DeepEqual(*missions[0], *mission) {
		t.Fatalf("mission clone = %#v", missions)
	}

	group := &CharacterMissionParameterGroup{GameID: 4, Seq: 2, Requirement: 30, Exp: 7, Quantity: 9}
	groups := cloneCharacterMissionParameterGroups([]*CharacterMissionParameterGroup{nil, group})
	if len(groups) != 1 || groups[0] == group || !reflect.DeepEqual(*groups[0], *group) {
		t.Fatalf("group clone = %#v", groups)
	}

	level := &CharacterLevel{Level: 5, TotalExp: 500}
	levels := cloneCharacterLevels([]*CharacterLevel{nil, level})
	if len(levels) != 1 || levels[0] == level || !reflect.DeepEqual(*levels[0], *level) {
		t.Fatalf("level clone = %#v", levels)
	}

	rawCharacters := []rendersnapshot.RawUserCharacter{{CharacterID: 1}, {CharacterID: 2, CharacterRank: 10}}
	if got := findRawUserCharacter(rawCharacters, 2); got == nil || got.CharacterRank != 10 {
		t.Fatalf("findRawUserCharacter() = %#v", got)
	}
	if got := findRawUserCharacter(rawCharacters, 9); got != nil {
		t.Fatalf("missing raw character = %#v", got)
	}

	if characterMissionStatusesForCharacter(nil, 1) != nil || characterMissionStatusesForCharacter(&rendersnapshot.RawUserData{}, 0) != nil {
		t.Fatal("invalid status lookup should return nil")
	}
	raw := &rendersnapshot.RawUserData{UserCharacterMissionV2Statuses: []rendersnapshot.RawUserCharacterMissionV2Status{
		{CharacterID: 1, MissionID: 10},
		{CharacterID: 2, MissionID: 20},
	}}
	if got := characterMissionStatusesForCharacter(raw, 1); len(got) != 1 || got[0].MissionID != 10 {
		t.Fatalf("status lookup = %#v", got)
	}

	if got := findCharacterMissionByType([]*CharacterMission{nil, mission}, "play_live"); got != mission {
		t.Fatalf("mission lookup = %#v", got)
	}
	if got := findCharacterMissionByType([]*CharacterMission{nil, mission}, "missing"); got != nil {
		t.Fatalf("missing mission lookup = %#v", got)
	}
	if got := characterMissionDisplayName(1); got != "星乃一歌" {
		t.Fatalf("known character display name = %q", got)
	}
	if got := characterMissionDisplayName(99); got != "角色99" {
		t.Fatalf("fallback character display name = %q", got)
	}
}

func TestCharacterMissionProgressHelpersEdgeCases(t *testing.T) {
	groups := []*CharacterMissionParameterGroup{
		nil,
		{Seq: 1, Requirement: 10, Exp: 2},
		{Seq: 3, Requirement: 30, Exp: 6},
	}
	if got := characterMissionRequirementBySeq(groups, 0); got != 0 {
		t.Fatalf("requirement seq 0 = %d", got)
	}
	if got := characterMissionRequirementBySeq(groups, 2); got != 10 {
		t.Fatalf("requirement seq 2 = %d", got)
	}
	if got := characterMissionGroupExp(groups, 0); got != 0 {
		t.Fatalf("group exp seq 0 = %d", got)
	}
	if got := characterMissionGroupExp(groups, 2); got != 2 {
		t.Fatalf("group exp seq 2 = %d", got)
	}
	if got := characterMissionClearedTotal(groups, 0); got != 0 {
		t.Fatalf("cleared total seq 0 = %d", got)
	}
	if got := characterMissionClearedTotal(groups, 3); got != 50 {
		t.Fatalf("cleared total seq 3 = %d", got)
	}

	if got := characterMissionUpper(nil, false); got != nil {
		t.Fatalf("empty upper = %#v", got)
	}
	if got := characterMissionUpper([]*CharacterMissionParameterGroup{nil, {Seq: 1, Requirement: 10}, {Seq: 2, Requirement: 5}}, false); got == nil || *got != 10 {
		t.Fatalf("standard upper = %#v", got)
	}
	if got := characterMissionUpper(groups, true); got == nil || *got <= 0 {
		t.Fatalf("EX upper = %#v", got)
	}

	if need, exp := characterMissionNextTarget(nil, 0, true); need != nil || exp != nil {
		t.Fatalf("empty EX next target = %#v, %#v", need, exp)
	}
	if need, exp := characterMissionNextTarget([]*CharacterMissionParameterGroup{nil, {Seq: 1, Requirement: 10, Exp: 2}}, 0, false); need == nil || *need != 10 || exp == nil || *exp != 2 {
		t.Fatalf("standard next target = %#v, %#v", need, exp)
	}
	if need, exp := characterMissionNextTarget(groups, 100, false); need != nil || exp != nil {
		t.Fatalf("completed next target = %#v, %#v", need, exp)
	}

	if got := characterMissionRequirementForRound(groups, 0); got != 0 {
		t.Fatalf("round-zero requirement = %d", got)
	}
	if got := characterMissionRequirementForRound(groups, 2); got != 10 {
		t.Fatalf("round-two requirement = %d", got)
	}
	if got := characterMissionExpForRound(groups, 0); got != 0 {
		t.Fatalf("round-zero exp = %d", got)
	}
	if got := characterMissionExpForRound(groups, 2); got != 2 {
		t.Fatalf("round-two exp = %d", got)
	}
	if got := characterMissionMaxExplicitSeq([]*CharacterMissionParameterGroup{nil, {Seq: 3}, {Seq: 2}}); got != 3 {
		t.Fatalf("max explicit seq = %d", got)
	}

	rows := []drawing.CharacterMissionAllTableRow{{Seq: 1, Requirement: 10}, {Seq: 2, Requirement: 20}}
	if got := calcCharacterMissionReachedSeq(rows, 15, false, 0); got != 1 {
		t.Fatalf("standard reached seq = %d", got)
	}
	if got := calcCharacterMissionReachedSeq(rows, 0, true, 4); got != 4 {
		t.Fatalf("EX reached seq = %d", got)
	}
}

func TestCharacterMissionSnapshotValidation(t *testing.T) {
	var nilController *Controller
	if _, err := nilController.BuildCharacterMissionOverviewRequestFromSnapshot(CharacterMissionQuery{}); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("nil controller error = %v", err)
	}

	source := &testSource{region: renderregion.JP}
	controller := NewController(nil, nil, nil, renderregion.JP)
	controller.RegisterSource(source)
	if _, err := controller.BuildCharacterMissionOverviewRequestFromSnapshot(CharacterMissionQuery{}); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("missing snapshot error = %v", err)
	}

	wantErr := errors.New("snapshot invalid")
	controller = NewController(nil, nil, &educationSnapshotStub{err: wantErr}, renderregion.JP)
	controller.RegisterSource(source)
	if _, err := controller.BuildCharacterMissionOverviewRequestFromSnapshot(CharacterMissionQuery{}); !errors.Is(err, wantErr) {
		t.Fatalf("snapshot Require() error = %v", err)
	}

	profile := &drawing.DetailedProfileCardRequest{ID: "1", Region: "JP"}
	controller = NewController(nil, nil, &educationSnapshotStub{profile: profile, raw: &rendersnapshot.RawUserData{}}, renderregion.JP)
	if _, err := controller.BuildCharacterMissionOverviewRequestFromSnapshot(CharacterMissionQuery{}); err == nil || !strings.Contains(err.Error(), "data source") {
		t.Fatalf("missing source error = %v", err)
	}

	controller.RegisterSource(source)
	if _, err := controller.BuildCharacterMissionOverviewRequestFromSnapshot(CharacterMissionQuery{}); err == nil || !strings.Contains(err.Error(), "character id") {
		t.Fatalf("missing character id error = %v", err)
	}
	if _, err := controller.BuildCharacterMissionAllRequestFromSnapshot(CharacterMissionQuery{Cid: 1}); err == nil || !strings.Contains(err.Error(), "mission type") {
		t.Fatalf("missing mission type error = %v", err)
	}

	controller = NewController(nil, nil, &educationSnapshotStub{profile: profile}, renderregion.JP)
	controller.RegisterSource(source)
	if _, err := controller.BuildCharacterMissionOverviewRequestFromSnapshot(CharacterMissionQuery{Cid: 1}); err == nil || !strings.Contains(err.Error(), "raw suite") {
		t.Fatalf("missing raw data error = %v", err)
	}
	controller = NewController(nil, nil, &educationSnapshotStub{raw: &rendersnapshot.RawUserData{}}, renderregion.JP)
	controller.RegisterSource(source)
	if _, err := controller.BuildCharacterMissionOverviewRequestFromSnapshot(CharacterMissionQuery{Cid: 1}); err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("missing profile error = %v", err)
	}
}
