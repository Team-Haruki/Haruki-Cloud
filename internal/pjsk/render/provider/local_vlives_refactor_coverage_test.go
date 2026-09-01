package provider

import (
	"testing"

	json "haruki-cloud/internal/jsonutil"
)

func TestDecodeLocalVLiveCollections(t *testing.T) {
	schedules := decodeLocalVLiveSchedules(json.RawMessage(`[
		{"startAt":100,"endAt":200},
		{"startAt":0,"endAt":200},
		{"startAt":100,"endAt":0}
	]`))
	if len(schedules) != 1 || schedules[0].StartAt != 100 || schedules[0].EndAt != 200 {
		t.Fatalf("schedules = %#v", schedules)
	}
	rewards := decodeLocalVLiveRewards(json.RawMessage(`[
		{"virtualLiveType":"normal","resourceBoxId":7},
		{"resourceBoxId":0}
	]`))
	if len(rewards) != 1 || rewards[0].ResourceBoxID != 7 {
		t.Fatalf("rewards = %#v", rewards)
	}
	characters := decodeLocalVLiveCharacters(json.RawMessage(`[
		{"gameCharacterUnitId":9,"virtualLivePerformanceType":"main"},
		{"gameCharacterUnitId":0}
	]`))
	if len(characters) != 1 || characters[0].GameCharacterUnitID != 9 {
		t.Fatalf("characters = %#v", characters)
	}
}

func TestBuildLocalVLiveAndMalformedCollections(t *testing.T) {
	item := localVirtualLiveJSON{
		ID: 1, Name: "Live", AssetBundleName: "live", StartAt: 10, EndAt: 20,
		VirtualLiveSchedules:  json.RawMessage(`{`),
		VirtualLiveRewards:    json.RawMessage(`{`),
		VirtualLiveCharacters: json.RawMessage(`{`),
	}
	live := buildLocalVLive(item)
	if live.ID != 1 || live.Name != "Live" || len(live.Schedules) != 0 || len(live.Rewards) != 0 || len(live.Characters) != 0 {
		t.Fatalf("local live = %#v", live)
	}
}
