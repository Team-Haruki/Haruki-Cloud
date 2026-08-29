package provider

import (
	"context"
	"encoding/json"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestDBCharacterPlayerFrameAndStampProviders(t *testing.T) {
	ctx := context.Background()
	p := openProviderBehaviorDB(t, "remaining_core")
	client := p.client

	if _, err := client.Gamecharacter.Create().
		SetGameID(1).
		SetFirstName("Hoshino").
		SetGivenName("Ichika").
		SetUnit("light_sound").
		SetGender("female").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create character: %v", err)
	}
	for _, unit := range []struct {
		id, characterID int64
		name, color     string
	}{
		{id: 10, characterID: 1, name: "light_sound", color: " #33AAFF "},
		{id: 11, characterID: 2, name: "idol", color: " "},
	} {
		if _, err := client.Gamecharacterunit.Create().
			SetGameID(unit.id).
			SetGameCharacterID(unit.characterID).
			SetUnit(unit.name).
			SetColorCode(unit.color).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create character unit %d: %v", unit.id, err)
		}
	}

	characters := p.characters
	if _, err := characters.GetByID(ctx, 0); err == nil {
		t.Fatal("GetByID(0) should fail")
	}
	character, err := characters.GetByID(ctx, 1)
	if err != nil || character.FirstName != "Hoshino" || character.GivenName != "Ichika" {
		t.Fatalf("GetByID(1) = %+v, %v", character, err)
	}
	character.FirstName = "mutated"
	if cached, err := characters.GetByID(ctx, 1); err != nil || cached.FirstName != "Hoshino" {
		t.Fatalf("cached character = %+v, %v", cached, err)
	}
	if _, err := characters.GetByID(ctx, 404); err == nil {
		t.Fatal("missing character should fail")
	}
	if color, ok := characters.GetColorCode(ctx, 0); ok || color != "" {
		t.Fatalf("zero character color = %q, %v", color, ok)
	}
	if color, ok := characters.GetColorCode(ctx, 1); !ok || color != "#33AAFF" {
		t.Fatalf("character color = %q, %v", color, ok)
	}
	if color, ok := characters.GetColorCode(ctx, 1); !ok || color != "#33AAFF" {
		t.Fatalf("cached character color = %q, %v", color, ok)
	}
	if color, ok := characters.GetColorCode(ctx, 2); ok || color != "" {
		t.Fatalf("blank character color = %q, %v", color, ok)
	}
	if color, ok := characters.GetColorCode(ctx, 2); ok || color != "" {
		t.Fatalf("cached blank character color = %q, %v", color, ok)
	}
	if _, ok := characters.GetColorCode(ctx, 404); ok {
		t.Fatal("missing character color should fail")
	}
	if _, err := characters.GetGameCharacterUnit(ctx, 0); err == nil {
		t.Fatal("GetGameCharacterUnit(0) should fail")
	}
	unit, err := characters.GetGameCharacterUnit(ctx, 10)
	if err != nil || unit.GameCharacterID != 1 || unit.Unit != "light_sound" {
		t.Fatalf("GetGameCharacterUnit(10) = %+v, %v", unit, err)
	}
	unit.Unit = "mutated"
	if cached, err := characters.GetGameCharacterUnit(ctx, 10); err != nil || cached.Unit != "light_sound" {
		t.Fatalf("cached character unit = %+v, %v", cached, err)
	}
	if _, err := characters.GetGameCharacterUnit(ctx, 404); err == nil {
		t.Fatal("missing character unit should fail")
	}

	if _, err := client.Playerframe.Create().
		SetGameID(20).
		SetSeq(2).
		SetPlayerFrameGroupID(30).
		SetDescription("frame description").
		SetGameCharacterID(1).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create player frame: %v", err)
	}
	if _, err := client.Playerframegroup.Create().
		SetGameID(30).
		SetSeq(3).
		SetName("frame group").
		SetAssetbundleName("frame_asset").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create player frame group: %v", err)
	}
	frames := p.playerFrames
	if _, err := frames.GetByID(ctx, 0); err == nil {
		t.Fatal("GetByID(0) should fail for player frames")
	}
	frame, err := frames.GetByID(ctx, 20)
	if err != nil || frame.PlayerFrameGroupID != 30 || frame.Description != "frame description" {
		t.Fatalf("GetByID(20) = %+v, %v", frame, err)
	}
	frame.Description = "mutated"
	if cached, err := frames.GetByID(ctx, 20); err != nil || cached.Description != "frame description" {
		t.Fatalf("cached frame = %+v, %v", cached, err)
	}
	if _, err := frames.GetByID(ctx, 404); err == nil {
		t.Fatal("missing player frame should fail")
	}
	if _, err := frames.GetGroupByID(ctx, 0); err == nil {
		t.Fatal("GetGroupByID(0) should fail")
	}
	group, err := frames.GetGroupByID(ctx, 30)
	if err != nil || group.Name != "frame group" || group.AssetBundleName != "frame_asset" {
		t.Fatalf("GetGroupByID(30) = %+v, %v", group, err)
	}
	group.Name = "mutated"
	if cached, err := frames.GetGroupByID(ctx, 30); err != nil || cached.Name != "frame group" {
		t.Fatalf("cached frame group = %+v, %v", cached, err)
	}
	if _, err := frames.GetGroupByID(ctx, 404); err == nil {
		t.Fatal("missing player frame group should fail")
	}

	for _, stamp := range []struct {
		id, characterID, characterID2 int64
		asset                         string
		region                        renderregion.Value
	}{
		{id: 40, characterID: 1, characterID2: 2, asset: "stamp_asset", region: renderregion.JP},
		{id: 41, characterID: 3, characterID2: 4, asset: "other_region", region: renderregion.TW},
	} {
		if _, err := client.Stamp.Create().
			SetGameID(stamp.id).
			SetAssetbundleName(stamp.asset).
			SetCharacterId1(stamp.characterID).
			SetCharacterId2(stamp.characterID2).
			SetServerRegion(stamp.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create stamp %d: %v", stamp.id, err)
		}
	}
	stamps, err := p.stamps.GetAll(ctx)
	if err != nil || len(stamps) != 1 || stamps[0].ID != 40 || stamps[0].CharacterID2 != 2 {
		t.Fatalf("GetAll stamps = %+v, %v", stamps, err)
	}
	stamps[0].AssetBundleName = "mutated"
	if cached, err := p.stamps.GetAll(ctx); err != nil || cached[0].AssetBundleName != "stamp_asset" {
		t.Fatalf("cached stamps = %+v, %v", cached, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := (&dbStampProvider{client: client, region: renderregion.JP}).GetAll(canceled); err == nil {
		t.Fatal("canceled stamp query should fail")
	}
}

func TestDBVLiveProviderParsesFiltersAndSorts(t *testing.T) {
	ctx := context.Background()
	p := openProviderBehaviorDB(t, "remaining_vlive")
	client := p.client

	if _, err := (&dbVLiveProvider{}).GetLives(ctx, renderregion.JP); err == nil {
		t.Fatal("nil-client VLive provider should fail")
	}
	for _, live := range []struct {
		id, startAt, endAt int64
		name, asset        string
		schedules          json.RawMessage
		rewards            json.RawMessage
		characters         json.RawMessage
		region             renderregion.Value
	}{
		{
			id: 2, startAt: 100, endAt: 300, name: "second", asset: "live_two",
			schedules:  json.RawMessage(`[{"startAt":100,"endAt":200},{"startAt":0,"endAt":1},{"startAt":"bad","endAt":2}]`),
			rewards:    json.RawMessage(`[{"virtualLiveType":"normal","resourceBoxId":9},{"resourceBoxId":0}]`),
			characters: json.RawMessage(`[{"gameCharacterUnitId":10,"virtualLivePerformanceType":"main"},{"gameCharacterUnitId":0}]`),
			region:     renderregion.JP,
		},
		{id: 1, startAt: 100, endAt: 250, name: "first", asset: "live_one", schedules: json.RawMessage(`{}`), rewards: json.RawMessage(`{}`), characters: json.RawMessage(`{}`), region: renderregion.JP},
		{id: 3, startAt: 50, endAt: 60, name: "other", asset: "other", region: renderregion.TW},
	} {
		if _, err := client.Virtuallive.Create().
			SetGameID(live.id).
			SetName(live.name).
			SetAssetbundleName(live.asset).
			SetStartAt(live.startAt).
			SetEndAt(live.endAt).
			SetVirtualLiveSchedules(live.schedules).
			SetVirtualLiveRewards(live.rewards).
			SetVirtualLiveCharacters(live.characters).
			SetServerRegion(live.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create virtual live %d: %v", live.id, err)
		}
	}

	lives, err := p.vlives.GetLives(ctx, "")
	if err != nil || len(lives) != 2 || lives[0].ID != 1 || lives[1].ID != 2 {
		t.Fatalf("GetLives() = %+v, %v", lives, err)
	}
	if len(lives[1].Schedules) != 1 || lives[1].Schedules[0].EndAt != 200 || len(lives[1].Rewards) != 1 || lives[1].Rewards[0].ResourceBoxID != 9 || len(lives[1].Characters) != 1 || lives[1].Characters[0].GameCharacterUnitID != 10 {
		t.Fatalf("parsed virtual live = %+v", lives[1])
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.vlives.GetLives(canceled, renderregion.JP); err == nil {
		t.Fatal("canceled virtual live query should fail")
	}

	for _, test := range []struct {
		value any
		want  int64
	}{
		{value: int64(1), want: 1},
		{value: int(2), want: 2},
		{value: float64(3), want: 3},
		{value: float32(4), want: 4},
		{value: "5", want: 0},
	} {
		if got := vliveInt64Number(test.value); got != test.want {
			t.Fatalf("vliveInt64Number(%T) = %d, want %d", test.value, got, test.want)
		}
	}
	if vliveIntNumber(float64(6)) != 6 || vliveString("text") != "text" || vliveString(7) != "" {
		t.Fatal("virtual live primitive conversion mismatch")
	}
}
