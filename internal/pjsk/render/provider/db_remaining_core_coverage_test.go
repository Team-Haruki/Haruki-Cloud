package provider

import (
	"context"
	"encoding/json"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

func TestDBCharacterPlayerFrameAndStampProviders(t *testing.T) {
	ctx := context.Background()
	p := openProviderBehaviorDB(t, "remaining_core")
	client := p.client
	{

		_, err := client.Gamecharacter.Create().
			SetGameID(1).
			SetFirstName("Hoshino").
			SetGivenName("Ichika").
			SetUnit("light_sound").
			SetGender("female").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create character: %v", err)
	}

	for _, unit := range []struct {
		id, characterID int64
		name, color     string
	}{
		{id: 10, characterID: 1, name: "light_sound", color: " #33AAFF "},
		{id: 11, characterID: 2, name: "idol", color: " "},
	} {
		{
			_, err := client.Gamecharacterunit.Create().
				SetGameID(unit.id).
				SetGameCharacterID(unit.characterID).
				SetUnit(unit.name).
				SetColorCode(unit.color).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create character unit %d: %v", unit.id, err)
		}

	}

	characters := p.characters
	{
		_, err := characters.GetByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetByID(0) should fail")
	}

	character, err := characters.GetByID(ctx, 1)
	{
		testutil.Require(t, !(err != nil), "GetByID(1) = %+v, %v", character, err)
		testutil.Require(t, !(character.FirstName != "Hoshino"), "GetByID(1) = %+v, %v", character, err)
		testutil.Require(t, !(character.GivenName != "Ichika"), "GetByID(1) = %+v, %v", character, err)
	}

	character.FirstName = "mutated"
	{
		cached, err := characters.GetByID(ctx, 1)
		{
			testutil.Require(t, !(err != nil), "cached character = %+v, %v", cached, err)
			testutil.Require(t, !(cached.FirstName != "Hoshino"), "cached character = %+v, %v", cached, err)
		}
	}
	{

		_, err := characters.GetByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing character should fail")
	}
	{

		color, ok := characters.GetColorCode(ctx, 0)
		{
			testutil.Require(t, !(ok), "zero character color = %q, %v", color, ok)
			testutil.Require(t, !(color != ""), "zero character color = %q, %v", color, ok)
		}
	}
	{

		color, ok := characters.GetColorCode(ctx, 1)
		{
			testutil.Require(t, ok, "character color = %q, %v", color, ok)
			testutil.Require(t, !(color != "#33AAFF"), "character color = %q, %v", color, ok)
		}
	}
	{

		color, ok := characters.GetColorCode(ctx, 1)
		{
			testutil.Require(t, ok, "cached character color = %q, %v", color, ok)
			testutil.Require(t, !(color != "#33AAFF"), "cached character color = %q, %v", color, ok)
		}
	}
	{

		color, ok := characters.GetColorCode(ctx, 2)
		{
			testutil.Require(t, !(ok), "blank character color = %q, %v", color, ok)
			testutil.Require(t, !(color != ""), "blank character color = %q, %v", color, ok)
		}
	}
	{

		color, ok := characters.GetColorCode(ctx, 2)
		{
			testutil.Require(t, !(ok), "cached blank character color = %q, %v", color, ok)
			testutil.Require(t, !(color != ""), "cached blank character color = %q, %v", color, ok)
		}
	}
	{

		_, ok := characters.GetColorCode(ctx, 404)
		testutil.RequireArgs(t, !(ok), "missing character color should fail")
	}
	{

		_, err := characters.GetGameCharacterUnit(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetGameCharacterUnit(0) should fail")
	}

	unit, err := characters.GetGameCharacterUnit(ctx, 10)
	{
		testutil.Require(t, !(err != nil), "GetGameCharacterUnit(10) = %+v, %v", unit, err)
		testutil.Require(t, !(unit.GameCharacterID != 1), "GetGameCharacterUnit(10) = %+v, %v", unit, err)
		testutil.Require(t, !(unit.Unit != "light_sound"), "GetGameCharacterUnit(10) = %+v, %v", unit, err)
	}

	unit.Unit = "mutated"
	{
		cached, err := characters.GetGameCharacterUnit(ctx, 10)
		{
			testutil.Require(t, !(err != nil), "cached character unit = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Unit != "light_sound"), "cached character unit = %+v, %v", cached, err)
		}
	}
	{

		_, err := characters.GetGameCharacterUnit(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing character unit should fail")
	}
	{

		_, err := client.Playerframe.Create().
			SetGameID(20).
			SetSeq(2).
			SetPlayerFrameGroupID(30).
			SetDescription("frame description").
			SetGameCharacterID(1).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create player frame: %v", err)
	}
	{

		_, err := client.Playerframegroup.Create().
			SetGameID(30).
			SetSeq(3).
			SetName("frame group").
			SetAssetbundleName("frame_asset").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create player frame group: %v", err)
	}

	frames := p.playerFrames
	{
		_, err := frames.GetByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetByID(0) should fail for player frames")
	}

	frame, err := frames.GetByID(ctx, 20)
	{
		testutil.Require(t, !(err != nil), "GetByID(20) = %+v, %v", frame, err)
		testutil.Require(t, !(frame.PlayerFrameGroupID != 30), "GetByID(20) = %+v, %v", frame, err)
		testutil.Require(t, !(frame.Description != "frame description"), "GetByID(20) = %+v, %v", frame, err)
	}

	frame.Description = "mutated"
	{
		cached, err := frames.GetByID(ctx, 20)
		{
			testutil.Require(t, !(err != nil), "cached frame = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Description != "frame description"), "cached frame = %+v, %v", cached, err)
		}
	}
	{

		_, err := frames.GetByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing player frame should fail")
	}
	{

		_, err := frames.GetGroupByID(ctx, 0)
		testutil.RequireArgs(t, !(err == nil), "GetGroupByID(0) should fail")
	}

	group, err := frames.GetGroupByID(ctx, 30)
	{
		testutil.Require(t, !(err != nil), "GetGroupByID(30) = %+v, %v", group, err)
		testutil.Require(t, !(group.Name != "frame group"), "GetGroupByID(30) = %+v, %v", group, err)
		testutil.Require(t, !(group.AssetBundleName != "frame_asset"), "GetGroupByID(30) = %+v, %v", group, err)
	}

	group.Name = "mutated"
	{
		cached, err := frames.GetGroupByID(ctx, 30)
		{
			testutil.Require(t, !(err != nil), "cached frame group = %+v, %v", cached, err)
			testutil.Require(t, !(cached.Name != "frame group"), "cached frame group = %+v, %v", cached, err)
		}
	}
	{

		_, err := frames.GetGroupByID(ctx, 404)
		testutil.RequireArgs(t, !(err == nil), "missing player frame group should fail")
	}

	for _, stamp := range []struct {
		id, characterID, characterID2 int64
		asset                         string
		region                        renderregion.Value
	}{
		{id: 40, characterID: 1, characterID2: 2, asset: "stamp_asset", region: renderregion.JP},
		{id: 41, characterID: 3, characterID2: 4, asset: "other_region", region: renderregion.TW},
	} {
		{
			_, err := client.Stamp.Create().
				SetGameID(stamp.id).
				SetAssetbundleName(stamp.asset).
				SetCharacterId1(stamp.characterID).
				SetCharacterId2(stamp.characterID2).
				SetServerRegion(stamp.region.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create stamp %d: %v", stamp.id, err)
		}

	}
	stamps, err := p.stamps.GetAll(ctx)
	{
		testutil.Require(t, !(err != nil), "GetAll stamps = %+v, %v", stamps, err)
		testutil.Require(t, !(len(stamps) != 1), "GetAll stamps = %+v, %v", stamps, err)
		testutil.Require(t, !(stamps[0].ID != 40), "GetAll stamps = %+v, %v", stamps, err)
		testutil.Require(t, !(stamps[0].CharacterID2 != 2), "GetAll stamps = %+v, %v", stamps, err)
	}

	stamps[0].AssetBundleName = "mutated"
	{
		cached, err := p.stamps.GetAll(ctx)
		{
			testutil.Require(t, !(err != nil), "cached stamps = %+v, %v", cached, err)
			testutil.Require(t, !(cached[0].AssetBundleName != "stamp_asset"), "cached stamps = %+v, %v", cached, err)
		}
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	{
		_, err := (&dbStampProvider{client: client, region: renderregion.JP}).GetAll(canceled)
		testutil.RequireArgs(t, !(err == nil), "canceled stamp query should fail")
	}

}

func TestDBVLiveProviderParsesFiltersAndSorts(t *testing.T) {
	ctx := context.Background()
	p := openProviderBehaviorDB(t, "remaining_vlive")
	client := p.client
	{

		_, err := (&dbVLiveProvider{}).GetLives(ctx, renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "nil-client VLive provider should fail")
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
		{id: 3, startAt: 50, endAt: 60, name: "earlier", asset: "earlier", region: renderregion.JP},
		{id: 4, startAt: 40, endAt: 50, name: "other", asset: "other", region: renderregion.TW},
	} {
		{
			_, err := client.Virtuallive.Create().
				SetGameID(live.id).
				SetName(live.name).
				SetAssetbundleName(live.asset).
				SetStartAt(live.startAt).
				SetEndAt(live.endAt).
				SetVirtualLiveSchedules(live.schedules).
				SetVirtualLiveRewards(live.rewards).
				SetVirtualLiveCharacters(live.characters).
				SetServerRegion(live.region.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create virtual live %d: %v", live.id, err)
		}

	}

	lives, err := p.vlives.GetLives(ctx, "")
	{
		testutil.Require(t, !(err != nil), "GetLives() = %+v, %v", lives, err)
		testutil.Require(t, !(len(lives) != 3), "GetLives() = %+v, %v", lives, err)
		testutil.Require(t, !(lives[0].ID != 3), "GetLives() = %+v, %v", lives, err)
		testutil.Require(t, !(lives[1].ID != 1), "GetLives() = %+v, %v", lives, err)
		testutil.Require(t, !(lives[2].ID != 2), "GetLives() = %+v, %v", lives, err)
	}
	{
		testutil.Require(t, !(len(lives[2].Schedules) != 1), "parsed virtual live = %+v", lives[2])
		testutil.Require(t, !(lives[2].Schedules[0].EndAt != 200), "parsed virtual live = %+v", lives[2])
		testutil.Require(t, !(len(lives[2].Rewards) != 1), "parsed virtual live = %+v", lives[2])
		testutil.Require(t, !(lives[2].Rewards[0].ResourceBoxID != 9), "parsed virtual live = %+v", lives[2])
		testutil.Require(t, !(len(lives[2].Characters) != 1), "parsed virtual live = %+v", lives[2])
		testutil.Require(t, !(lives[2].Characters[0].GameCharacterUnitID != 10), "parsed virtual live = %+v", lives[2])
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	{
		_, err := p.vlives.GetLives(canceled, renderregion.JP)
		testutil.RequireArgs(t, !(err == nil), "canceled virtual live query should fail")
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
		{
			got := vliveInt64Number(test.value)
			testutil.Require(t, !(got != test.want), "vliveInt64Number(%T) = %d, want %d", test.value, got, test.want)
		}

	}
	{
		testutil.RequireArgs(t, !(vliveIntNumber(float64(6)) != 6), "virtual live primitive conversion mismatch")
		testutil.RequireArgs(t, !(vliveString("text") != "text"), "virtual live primitive conversion mismatch")
		testutil.RequireArgs(t, !(vliveString(7) != ""), "virtual live primitive conversion mismatch")
	}

}
