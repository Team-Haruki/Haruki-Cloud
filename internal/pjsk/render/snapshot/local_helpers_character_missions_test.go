package snapshot

import (
	"reflect"
	"testing"
)

func TestResolveCharacterMissionV2StatusesMergesStandardAndCompact(t *testing.T) {
	standard := []RawUserCharacterMissionV2Status{{
		CharacterID: 1, ParameterGroupID: 10, Seq: 1, MissionID: 100, MissionStatus: "cleared",
	}}
	raw := &RawUserData{
		UserCharacterMissionV2Statuses: standard,
		CompactUserCharacterMissionV2Statuses: []byte(`{
			"__ENUM__":{"missionStatus":["progress","reset","cleared"]},
			"characterId":[1,2,3,4,5],
			"parameterGroupId":[10,20,30,40,0],
			"seq":[1,2,3,4,5],
			"missionId":[101,202,303,404,505],
			"missionStatus":[2,0," COMPLETE ",9,0]
		}`),
	}

	got := ResolveCharacterMissionV2Statuses(raw)
	want := []RawUserCharacterMissionV2Status{
		{CharacterID: 1, ParameterGroupID: 10, Seq: 1, MissionID: 100, MissionStatus: "cleared"},
		{CharacterID: 2, ParameterGroupID: 20, Seq: 2, MissionID: 202, MissionStatus: "progress"},
		{CharacterID: 3, ParameterGroupID: 30, Seq: 3, MissionID: 303, MissionStatus: "complete"},
		{CharacterID: 4, ParameterGroupID: 40, Seq: 4, MissionID: 404, MissionStatus: "9"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved statuses = %#v, want %#v", got, want)
	}

	got[0].MissionID = 999
	if standard[0].MissionID != 100 {
		t.Fatal("standard status slice was not cloned")
	}
}

func TestDecodeCompactCharacterMissionV2StatusesRejectsInvalidRows(t *testing.T) {
	for name, raw := range map[string][]byte{
		"empty":        nil,
		"null":         []byte(" null "),
		"invalid json": []byte("{"),
		"no rows":      []byte(`{"__ENUM__":{}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if got := DecodeCompactCharacterMissionV2Statuses(raw); got != nil {
				t.Fatalf("DecodeCompactCharacterMissionV2Statuses() = %#v", got)
			}
		})
	}

	raw := []byte(`{
		"characterId":[0,1,2,3,4],
		"parameterGroupId":[1,0,2,3,4],
		"seq":[1,2,0,3,4],
		"missionStatus":["progress","progress","progress","reset",7],
		"broken":"not-an-array"
	}`)
	got := DecodeCompactCharacterMissionV2Statuses(raw)
	if len(got) != 1 || got[0].CharacterID != 4 || got[0].MissionStatus != "7" || got[0].MissionID != 0 {
		t.Fatalf("decoded valid rows = %#v", got)
	}
}

func TestResolveCharacterMissionV2StatusesSourceSelectionAndDeduplication(t *testing.T) {
	if ResolveCharacterMissionV2Statuses(nil) != nil {
		t.Fatal("nil raw data returned statuses")
	}
	standard := []RawUserCharacterMissionV2Status{{CharacterID: 1, ParameterGroupID: 2, Seq: 3}}
	if got := ResolveCharacterMissionV2Statuses(&RawUserData{UserCharacterMissionV2Statuses: standard}); !reflect.DeepEqual(got, standard) {
		t.Fatalf("standard-only result = %#v", got)
	}
	compact := []byte(`{"characterId":[9],"parameterGroupId":[8],"seq":[7]}`)
	if got := ResolveCharacterMissionV2Statuses(&RawUserData{CompactUserCharacterMissionV2Statuses: compact}); len(got) != 1 || got[0].CharacterID != 9 {
		t.Fatalf("compact-only result = %#v", got)
	}
	if got := mergeCharacterMissionV2Statuses(nil, nil); got != nil {
		t.Fatalf("empty merge = %#v", got)
	}
}
