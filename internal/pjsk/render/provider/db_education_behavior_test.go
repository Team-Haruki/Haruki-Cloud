package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

func openProviderBehaviorDB(t *testing.T, name string) *DatabaseProvider {
	t.Helper()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano()))
	return NewDatabaseProvider(client, renderregion.JP)
}

func TestDBEducationProviderLoadsAndClonesAllMasterdata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "characterMissionV2s.json", `[
		{"id":501,"characterId":5,"characterMissionType":"leader","parameterGroupId":101,"isAchievementMission":true}
	]`)
	writeTestFile(t, root, "resourceBoxes.json", `[
		{"ID":100,"ResourceBoxPurpose":"challenge","ResourceBoxType":"expand","Description":"duplicate local"},
		{"ID":200,"ResourceBoxPurpose":"local","ResourceBoxType":"expand","Description":"local only"}
	]`)
	writeTestFile(t, root, "resourceBoxDetails.json", `[
		{"resourceBoxId":100,"resourceBoxPurpose":"challenge","resourceType":"jewel","resourceQuantity":100},
		{"resourceBoxId":200,"resourceBoxPurpose":"local","resourceType":"material","resourceId":15,"resourceQuantity":2}
	]`)

	provider := openProviderBehaviorDB(t, "education_success")
	provider.education.store = newLocalStore(root)
	client := provider.client

	for _, item := range []struct {
		id, characterID, highScore, resourceBoxID int64
		region                                    renderregion.Value
	}{
		{id: 1, characterID: 5, highScore: 1000, resourceBoxID: 100, region: renderregion.JP},
		{id: 2, characterID: 5, highScore: 2000, resourceBoxID: 101, region: renderregion.JP},
		{id: 3, characterID: 5, highScore: 3000, resourceBoxID: 102, region: renderregion.TW},
	} {
		{
			_, err := client.Challengelivehighscorereward.Create().
				SetGameID(item.id).
				SetCharacterID(item.characterID).
				SetHighScore(item.highScore).
				SetResourceBoxID(item.resourceBoxID).
				SetServerRegion(item.region.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create challenge reward %d: %v", item.id, err)
		}

	}
	{

		_, err := client.Resourceboxe.Create().
			SetGameID(100).
			SetResourceBoxPurpose("challenge").
			SetResourceBoxType("expand").
			SetDescription("database box").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create challenge resource box: %v", err)
	}
	{

		_, err := client.Resourceboxe.Create().
			SetGameID(101).
			SetResourceBoxPurpose("shop").
			SetResourceBoxType("expand").
			SetDetails(json.RawMessage(`[{"resourceType":"coin","resourceId":1,"resourceQuantity":5}]`)).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create shop resource box: %v", err)
	}
	{

		_, err := client.Areaitem.Create().
			SetGameID(10).
			SetAreaID(1).
			SetName("Sekai tree").
			SetAssetbundleName("area_item_10").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create area item: %v", err)
	}
	{

		_, err := client.Areaitem.Create().
			SetGameID(11).
			SetName("other region").
			SetServerRegion(renderregion.TW.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create other-region area item: %v", err)
	}

	for _, level := range []int64{1, 2} {
		{
			_, err := client.Areaitemlevel.Create().
				SetAreaItemID(10).
				SetLevel(level).
				SetTargetUnit("idol").
				SetTargetCardAttr("cute").
				SetTargetGameCharacterID(5).
				SetPower1BonusRate(float64(level) * 0.5).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create area item level %d: %v", level, err)
		}

	}
	{

		_, err := client.Characterrank.Create().
			SetGameID(1).
			SetCharacterID(5).
			SetCharacterRank(10).
			SetPower1BonusRate(2.5).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create character rank: %v", err)
	}

	for _, item := range []struct {
		id, level, totalExp int64
		levelType           string
	}{
		{id: 1, levelType: "character", level: 0, totalExp: 0},
		{id: 2, levelType: "character", level: 10, totalExp: 1234},
		{id: 3, levelType: "bonds", level: 1, totalExp: 100},
	} {
		{
			_, err := client.Level.Create().
				SetGameID(item.id).
				SetLevelType(item.levelType).
				SetLevel(item.level).
				SetTotalExp(item.totalExp).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create %s level %d: %v", item.levelType, item.id, err)
		}

	}
	{

		_, err := client.Bond.Create().
			SetGameID(1).
			SetGroupID(50).
			SetCharacterId1(5).
			SetCharacterId2(6).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create bond: %v", err)
	}
	{

		_, err := client.Gamecharacterunit.Create().
			SetGameID(500).
			SetGameCharacterID(5).
			SetUnit("idol").
			SetColorCode(" #abcdef ").
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create game character style: %v", err)
	}

	for _, item := range []struct {
		gameID, seq, requirement, exp, quantity int64
	}{
		{gameID: 1, seq: 1, requirement: 5, exp: 10, quantity: 1},
		{gameID: 1, seq: 2, requirement: 12, exp: 20, quantity: 1},
		{gameID: 101, seq: 1, requirement: 20, exp: 30, quantity: 2},
		{gameID: 999, seq: 1, requirement: 1, exp: 1, quantity: 1},
	} {
		{
			_, err := client.Charactermissionv2Parametergroup.Create().
				SetGameID(item.gameID).
				SetSeq(item.seq).
				SetRequirement(item.requirement).
				SetExp(item.exp).
				SetQuantity(item.quantity).
				SetServerRegion(renderregion.JP.String()).
				Save(ctx)
			testutil.Require(t, !(err != nil), "create mission parameter group %d/%d: %v", item.gameID, item.seq, err)
		}

	}
	{

		_, err := client.Mysekaigatelevel.Create().
			SetGameID(1).
			SetMysekaiGateID(7).
			SetLevel(3).
			SetPowerBonusRate(1.25).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create gate level: %v", err)
	}
	{

		_, err := client.Shopitem.Create().
			SetGameID(300).
			SetShopID(30).
			SetSeq(2).
			SetResourceBoxID(101).
			SetReleaseConditionID(9).
			SetStartAt(123456).
			SetCosts(json.RawMessage(`[{"cost":{"resourceType":"jewel","resourceId":1,"quantity":300}}]`)).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx)
		testutil.Require(t, !(err != nil), "create shop item: %v", err)
	}

	education := provider.education
	{
		got := education.GetChallengeRewardsByCharacter(ctx, 0)
		testutil.Require(t, !(got != nil), "invalid character rewards = %+v, want nil", got)
	}

	rewards := education.GetChallengeRewardsByCharacter(ctx, 5)
	{
		testutil.Require(t, !(len(rewards) != 2), "challenge rewards = %+v, want two JP rewards", rewards)
		testutil.Require(t, !(rewards[0].CharacterID != 5), "challenge rewards = %+v, want two JP rewards", rewards)
	}

	rewards[0].HighScore = -1
	{
		cached := education.GetChallengeRewardsByCharacter(ctx, 5)
		testutil.RequireArgs(t, !(cached[0].HighScore == -1), "challenge reward result aliases the provider cache")
	}
	{

		got := education.GetChallengeRewardsByCharacter(ctx, 99)
		testutil.Require(t, !(got != nil), "missing character rewards = %+v, want nil", got)
	}
	{

		got := education.GetResourceBoxByPurpose(ctx, "", 0)
		testutil.Require(t, !(got != nil), "invalid resource box = %+v, want nil", got)
	}

	databaseBox := education.GetResourceBoxByPurpose(ctx, "challenge", 100)
	{
		testutil.Require(t, !(databaseBox == nil), "database resource box = %+v", databaseBox)
		testutil.Require(t, !(databaseBox.Description != "database box"), "database resource box = %+v", databaseBox)
		testutil.Require(t, !(len(databaseBox.Details) != 1), "database resource box = %+v", databaseBox)
		testutil.Require(t, !(databaseBox.Details[0].ResourceType != "jewel"), "database resource box = %+v", databaseBox)
	}

	localBox := education.GetResourceBoxByPurpose(ctx, "", 200)
	{
		testutil.Require(t, !(localBox == nil), "merged local resource box = %+v", localBox)
		testutil.Require(t, !(localBox.Description != "local only"), "merged local resource box = %+v", localBox)
		testutil.Require(t, !(len(localBox.Details) != 1), "merged local resource box = %+v", localBox)
	}

	shopBox := education.GetResourceBoxByPurpose(ctx, "shop", 101)
	{
		testutil.Require(t, !(shopBox == nil), "shop resource box = %+v", shopBox)
		testutil.Require(t, !(len(shopBox.Details) != 1), "shop resource box = %+v", shopBox)
		testutil.Require(t, !(shopBox.Details[0].ResourceType != "coin"), "shop resource box = %+v", shopBox)
	}

	shopBox.Details[0].ResourceQuantity = -1
	{
		cached := education.GetResourceBoxByPurpose(ctx, "shop", 101)
		testutil.RequireArgs(t, !(cached.Details[0].ResourceQuantity != 5), "resource box details alias the provider cache")
	}
	{

		got := education.GetResourceBoxByPurpose(ctx, "missing", 100)
		testutil.Require(t, !(got != nil), "wrong-purpose resource box = %+v, want nil", got)
	}
	{

		got := education.GetResourceBoxesByPurpose(ctx, "missing")
		testutil.Require(t, !(got != nil), "missing-purpose resource boxes = %+v, want nil", got)
	}
	{

		got := education.GetResourceBoxesByPurpose(ctx, "")
		testutil.Require(t, !(len(got) != 3), "all resource boxes = %+v, want three", got)
	}
	{

		got := education.GetResourceBoxesByPurpose(ctx, "challenge")
		{
			testutil.Require(t, !(len(got) != 1), "challenge resource boxes = %+v", got)
			testutil.Require(t, !(got[0].ID != 100), "challenge resource boxes = %+v", got)
		}
	}
	{

		got := education.GetAreaItem(ctx, 0)
		testutil.Require(t, !(got != nil), "invalid area item = %+v, want nil", got)
	}

	area := education.GetAreaItem(ctx, 10)
	{
		testutil.Require(t, !(area == nil), "area item = %+v", area)
		testutil.Require(t, !(area.Name != "Sekai tree"), "area item = %+v", area)
	}

	area.Name = "mutated"
	{
		cached := education.GetAreaItem(ctx, 10)
		testutil.RequireArgs(t, !(cached.Name != "Sekai tree"), "area item result aliases the provider cache")
	}
	{

		got := education.GetAreaItem(ctx, 99)
		testutil.Require(t, !(got != nil), "missing area item = %+v, want nil", got)
	}
	{

		got := education.GetAreaItems(ctx)
		{
			testutil.Require(t, !(len(got) != 1), "area items = %+v", got)
			testutil.Require(t, !(got[0].ID != 10), "area items = %+v", got)
		}
	}

	levels := education.GetAreaItemLevels(ctx, 10)
	{
		testutil.Require(t, !(len(levels) != 2), "area item levels = %+v", levels)
		testutil.Require(t, !(levels[1].Level != 2), "area item levels = %+v", levels)
	}

	levels[0].Power1BonusRate = -1
	{
		cached := education.GetAreaItemLevel(ctx, 10, 1)
		{
			testutil.Require(t, !(cached == nil), "cached area item level = %+v", cached)
			testutil.Require(t, !(cached.Power1BonusRate != 0.5), "cached area item level = %+v", cached)
		}
	}
	{
		testutil.RequireArgs(t, !(education.GetAreaItemLevels(ctx, 0) != nil), "invalid or missing area-item levels should return nil")
		testutil.RequireArgs(t, !(education.GetAreaItemLevel(ctx, 0, 1) != nil), "invalid or missing area-item levels should return nil")
		testutil.RequireArgs(t, !(education.GetAreaItemLevel(ctx, 10, 0) != nil), "invalid or missing area-item levels should return nil")
		testutil.RequireArgs(t, !(education.GetAreaItemLevel(ctx, 99, 1) != nil), "invalid or missing area-item levels should return nil")
	}

	rank := education.GetCharacterRank(ctx, 5, 10)
	{
		testutil.Require(t, !(rank == nil), "character rank = %+v", rank)
		testutil.Require(t, !(rank.Power1BonusRate != 2.5), "character rank = %+v", rank)
	}

	rank.Power1BonusRate = -1
	{
		cached := education.GetCharacterRank(ctx, 5, 10)
		testutil.RequireArgs(t, !(cached.Power1BonusRate != 2.5), "character rank result aliases the provider cache")
	}
	{
		testutil.RequireArgs(t, !(education.GetCharacterRank(ctx, 0, 10) != nil), "invalid or missing character ranks should return nil")
		testutil.RequireArgs(t, !(education.GetCharacterRank(ctx, 5, 0) != nil), "invalid or missing character ranks should return nil")
		testutil.RequireArgs(t, !(education.GetCharacterRank(ctx, 99, 1) != nil), "invalid or missing character ranks should return nil")
	}

	characterLevels := education.GetCharacterLevels(ctx)
	{
		testutil.Require(t, !(len(characterLevels) != 1), "character levels = %+v", characterLevels)
		testutil.Require(t, !(characterLevels[0].Level != 10), "character levels = %+v", characterLevels)
	}

	characterLevels[0].TotalExp = -1
	{
		cached := education.GetCharacterLevels(ctx)
		testutil.RequireArgs(t, !(cached[0].TotalExp != 1234), "character level result aliases the provider cache")
	}

	bonds := education.GetBonds(ctx)
	{
		testutil.Require(t, !(len(bonds) != 1), "bonds = %+v", bonds)
		testutil.Require(t, !(bonds[0].GroupID != 50), "bonds = %+v", bonds)
	}

	bonds[0].GroupID = -1
	{
		cached := education.GetBonds(ctx)
		testutil.RequireArgs(t, !(cached[0].GroupID != 50), "bond result aliases the provider cache")
	}

	bondLevels := education.GetBondLevels(ctx)
	{
		testutil.Require(t, !(len(bondLevels) != 1), "bond levels = %+v", bondLevels)
		testutil.Require(t, !(bondLevels[0].TotalExp != 100), "bond levels = %+v", bondLevels)
	}

	style := education.GetGameCharacterStyle(ctx, 500)
	{
		testutil.Require(t, !(style == nil), "game character style = %+v", style)
		testutil.Require(t, !(style.CharacterID != 5), "game character style = %+v", style)
		testutil.Require(t, !(style.ColorCode != "#abcdef"), "game character style = %+v", style)
	}

	style.ColorCode = "mutated"
	{
		cached := education.GetGameCharacterStyle(ctx, 500)
		testutil.RequireArgs(t, !(cached.ColorCode != "#abcdef"), "game character style result aliases the provider cache")
	}
	{
		testutil.RequireArgs(t, !(education.GetGameCharacterStyle(ctx, 0) != nil), "invalid or missing game character styles should return nil")
		testutil.RequireArgs(t, !(education.GetGameCharacterStyle(ctx, 999) != nil), "invalid or missing game character styles should return nil")
	}

	missions := education.GetCharacterMissions(ctx, 5)
	{
		testutil.Require(t, !(len(missions) != 1), "character missions = %+v", missions)
		testutil.Require(t, !(missions[0].ID != 501), "character missions = %+v", missions)
		testutil.Require(t, missions[0].IsAchievementMission, "character missions = %+v", missions)
	}

	missions[0].ID = -1
	{
		cached := education.GetCharacterMissions(ctx, 5)
		testutil.RequireArgs(t, !(cached[0].ID != 501), "character mission result aliases the provider cache")
	}

	groups := education.GetCharacterMissionParameterGroups(ctx, 101)
	{
		testutil.Require(t, !(len(groups) != 1), "mission parameter groups = %+v", groups)
		testutil.Require(t, !(groups[0].Requirement != 20), "mission parameter groups = %+v", groups)
	}

	requirements, maxPlayLimit := education.GetLeaderMissionRequirements(ctx)
	{
		testutil.Require(t, !(len(requirements) != 1), "leader requirements = %+v, max=%d", requirements, maxPlayLimit)
		testutil.Require(t, !(requirements[0].Requirement != 20), "leader requirements = %+v, max=%d", requirements, maxPlayLimit)
		testutil.Require(t, !(maxPlayLimit != 12), "leader requirements = %+v, max=%d", requirements, maxPlayLimit)
	}

	requirements[0].Requirement = -1
	{
		cached, _ := education.GetLeaderMissionRequirements(ctx)
		testutil.RequireArgs(t, !(cached[0].Requirement != 20), "leader requirement result aliases the provider cache")
	}
	{
		testutil.RequireArgs(t, !(education.GetCharacterMissions(ctx, 0) != nil), "invalid or missing mission lookups should return nil")
		testutil.RequireArgs(t, !(education.GetCharacterMissions(ctx, 99) != nil), "invalid or missing mission lookups should return nil")
		testutil.RequireArgs(t, !(education.GetCharacterMissionParameterGroups(ctx, 0) != nil), "invalid or missing mission lookups should return nil")
		testutil.RequireArgs(t, !(education.GetCharacterMissionParameterGroups(ctx, 404) != nil), "invalid or missing mission lookups should return nil")
	}

	gate := education.GetMysekaiGateLevel(ctx, 7, 3)
	{
		testutil.Require(t, !(gate == nil), "gate level = %+v", gate)
		testutil.Require(t, !(gate.PowerBonusRate != 1.25), "gate level = %+v", gate)
	}

	gate.PowerBonusRate = -1
	{
		cached := education.GetMysekaiGateLevel(ctx, 7, 3)
		testutil.RequireArgs(t, !(cached.PowerBonusRate != 1.25), "gate-level result aliases the provider cache")
	}
	{
		testutil.RequireArgs(t, !(education.GetMysekaiGateLevel(ctx, 0, 3) != nil), "invalid or missing gate levels should return nil")
		testutil.RequireArgs(t, !(education.GetMysekaiGateLevel(ctx, 7, 0) != nil), "invalid or missing gate levels should return nil")
		testutil.RequireArgs(t, !(education.GetMysekaiGateLevel(ctx, 99, 1) != nil), "invalid or missing gate levels should return nil")
	}

	shop := education.GetShopItemByResourceBoxID(ctx, 101)
	{
		testutil.Require(t, !(shop == nil), "shop item = %+v", shop)
		testutil.Require(t, !(shop.ID != 300), "shop item = %+v", shop)
		testutil.Require(t, !(len(shop.Costs) != 1), "shop item = %+v", shop)
		testutil.Require(t, !(shop.Costs[0].Quantity != 300), "shop item = %+v", shop)
	}

	shop.Costs[0].Quantity = -1
	{
		cached := education.GetShopItemByResourceBoxID(ctx, 101)
		testutil.RequireArgs(t, !(cached.Costs[0].Quantity != 300), "shop item costs alias the provider cache")
	}
	{

		got := education.GetShopItems(ctx)
		{
			testutil.Require(t, !(len(got) != 1), "shop items = %+v", got)
			testutil.Require(t, !(got[0].ID != 300), "shop items = %+v", got)
		}
	}
	{
		testutil.RequireArgs(t, !(education.GetShopItemByResourceBoxID(ctx, 0) != nil), "invalid or missing shop items should return nil")
		testutil.RequireArgs(t, !(education.GetShopItemByResourceBoxID(ctx, 999) != nil), "invalid or missing shop items should return nil")
	}

}

func TestDBEducationProviderReturnsNilWhenDatabaseUnavailable(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "education_errors")
	{
		err := provider.client.Close()
		testutil.Require(t, !(err != nil), "close fixture database: %v", err)
	}

	education := provider.education

	tests := []struct {
		name string
		load func() any
	}{
		{name: "challenge rewards", load: func() any { return education.GetChallengeRewardsByCharacter(ctx, 5) }},
		{name: "resource boxes", load: func() any { return education.GetResourceBoxesByPurpose(ctx, "") }},
		{name: "area items", load: func() any { return education.GetAreaItems(ctx) }},
		{name: "character levels", load: func() any { return education.GetCharacterLevels(ctx) }},
		{name: "character ranks", load: func() any { return education.GetCharacterRank(ctx, 5, 1) }},
		{name: "bonds", load: func() any { return education.GetBonds(ctx) }},
		{name: "styles", load: func() any { return education.GetGameCharacterStyle(ctx, 5) }},
		{name: "missions", load: func() any { return education.GetCharacterMissions(ctx, 5) }},
		{name: "gate levels", load: func() any { return education.GetMysekaiGateLevel(ctx, 1, 1) }},
		{name: "shop items", load: func() any { return education.GetShopItems(ctx) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.load()
			testutil.Require(t, !(got != nil && !reflect.ValueOf(got).IsNil()), "database-error result = %#v, want nil", got)

		})
	}
}
