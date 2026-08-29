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
		if _, err := client.Challengelivehighscorereward.Create().
			SetGameID(item.id).
			SetCharacterID(item.characterID).
			SetHighScore(item.highScore).
			SetResourceBoxID(item.resourceBoxID).
			SetServerRegion(item.region.String()).
			Save(ctx); err != nil {
			t.Fatalf("create challenge reward %d: %v", item.id, err)
		}
	}

	if _, err := client.Resourceboxe.Create().
		SetGameID(100).
		SetResourceBoxPurpose("challenge").
		SetResourceBoxType("expand").
		SetDescription("database box").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create challenge resource box: %v", err)
	}
	if _, err := client.Resourceboxe.Create().
		SetGameID(101).
		SetResourceBoxPurpose("shop").
		SetResourceBoxType("expand").
		SetDetails(json.RawMessage(`[{"resourceType":"coin","resourceId":1,"resourceQuantity":5}]`)).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create shop resource box: %v", err)
	}

	if _, err := client.Areaitem.Create().
		SetGameID(10).
		SetAreaID(1).
		SetName("Sekai tree").
		SetAssetbundleName("area_item_10").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create area item: %v", err)
	}
	if _, err := client.Areaitem.Create().
		SetGameID(11).
		SetName("other region").
		SetServerRegion(renderregion.TW.String()).
		Save(ctx); err != nil {
		t.Fatalf("create other-region area item: %v", err)
	}
	for _, level := range []int64{1, 2} {
		if _, err := client.Areaitemlevel.Create().
			SetAreaItemID(10).
			SetLevel(level).
			SetTargetUnit("idol").
			SetTargetCardAttr("cute").
			SetTargetGameCharacterID(5).
			SetPower1BonusRate(float64(level) * 0.5).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create area item level %d: %v", level, err)
		}
	}

	if _, err := client.Characterrank.Create().
		SetGameID(1).
		SetCharacterID(5).
		SetCharacterRank(10).
		SetPower1BonusRate(2.5).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create character rank: %v", err)
	}
	for _, item := range []struct {
		id, level, totalExp int64
		levelType           string
	}{
		{id: 1, levelType: "character", level: 0, totalExp: 0},
		{id: 2, levelType: "character", level: 10, totalExp: 1234},
		{id: 3, levelType: "bonds", level: 1, totalExp: 100},
	} {
		if _, err := client.Level.Create().
			SetGameID(item.id).
			SetLevelType(item.levelType).
			SetLevel(item.level).
			SetTotalExp(item.totalExp).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create %s level %d: %v", item.levelType, item.id, err)
		}
	}

	if _, err := client.Bond.Create().
		SetGameID(1).
		SetGroupID(50).
		SetCharacterId1(5).
		SetCharacterId2(6).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create bond: %v", err)
	}
	if _, err := client.Gamecharacterunit.Create().
		SetGameID(500).
		SetGameCharacterID(5).
		SetUnit("idol").
		SetColorCode(" #abcdef ").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create game character style: %v", err)
	}

	for _, item := range []struct {
		gameID, seq, requirement, exp, quantity int64
	}{
		{gameID: 1, seq: 1, requirement: 5, exp: 10, quantity: 1},
		{gameID: 1, seq: 2, requirement: 12, exp: 20, quantity: 1},
		{gameID: 101, seq: 1, requirement: 20, exp: 30, quantity: 2},
		{gameID: 999, seq: 1, requirement: 1, exp: 1, quantity: 1},
	} {
		if _, err := client.Charactermissionv2Parametergroup.Create().
			SetGameID(item.gameID).
			SetSeq(item.seq).
			SetRequirement(item.requirement).
			SetExp(item.exp).
			SetQuantity(item.quantity).
			SetServerRegion(renderregion.JP.String()).
			Save(ctx); err != nil {
			t.Fatalf("create mission parameter group %d/%d: %v", item.gameID, item.seq, err)
		}
	}

	if _, err := client.Mysekaigatelevel.Create().
		SetGameID(1).
		SetMysekaiGateID(7).
		SetLevel(3).
		SetPowerBonusRate(1.25).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create gate level: %v", err)
	}
	if _, err := client.Shopitem.Create().
		SetGameID(300).
		SetShopID(30).
		SetSeq(2).
		SetResourceBoxID(101).
		SetReleaseConditionID(9).
		SetStartAt(123456).
		SetCosts(json.RawMessage(`[{"cost":{"resourceType":"jewel","resourceId":1,"quantity":300}}]`)).
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create shop item: %v", err)
	}

	education := provider.education
	if got := education.GetChallengeRewardsByCharacter(ctx, 0); got != nil {
		t.Fatalf("invalid character rewards = %+v, want nil", got)
	}
	rewards := education.GetChallengeRewardsByCharacter(ctx, 5)
	if len(rewards) != 2 || rewards[0].CharacterID != 5 {
		t.Fatalf("challenge rewards = %+v, want two JP rewards", rewards)
	}
	rewards[0].HighScore = -1
	if cached := education.GetChallengeRewardsByCharacter(ctx, 5); cached[0].HighScore == -1 {
		t.Fatal("challenge reward result aliases the provider cache")
	}
	if got := education.GetChallengeRewardsByCharacter(ctx, 99); got != nil {
		t.Fatalf("missing character rewards = %+v, want nil", got)
	}

	if got := education.GetResourceBoxByPurpose(ctx, "", 0); got != nil {
		t.Fatalf("invalid resource box = %+v, want nil", got)
	}
	databaseBox := education.GetResourceBoxByPurpose(ctx, "challenge", 100)
	if databaseBox == nil || databaseBox.Description != "database box" || len(databaseBox.Details) != 1 || databaseBox.Details[0].ResourceType != "jewel" {
		t.Fatalf("database resource box = %+v", databaseBox)
	}
	localBox := education.GetResourceBoxByPurpose(ctx, "", 200)
	if localBox == nil || localBox.Description != "local only" || len(localBox.Details) != 1 {
		t.Fatalf("merged local resource box = %+v", localBox)
	}
	shopBox := education.GetResourceBoxByPurpose(ctx, "shop", 101)
	if shopBox == nil || len(shopBox.Details) != 1 || shopBox.Details[0].ResourceType != "coin" {
		t.Fatalf("shop resource box = %+v", shopBox)
	}
	shopBox.Details[0].ResourceQuantity = -1
	if cached := education.GetResourceBoxByPurpose(ctx, "shop", 101); cached.Details[0].ResourceQuantity != 5 {
		t.Fatal("resource box details alias the provider cache")
	}
	if got := education.GetResourceBoxByPurpose(ctx, "missing", 100); got != nil {
		t.Fatalf("wrong-purpose resource box = %+v, want nil", got)
	}
	if got := education.GetResourceBoxesByPurpose(ctx, "missing"); got != nil {
		t.Fatalf("missing-purpose resource boxes = %+v, want nil", got)
	}
	if got := education.GetResourceBoxesByPurpose(ctx, ""); len(got) != 3 {
		t.Fatalf("all resource boxes = %+v, want three", got)
	}
	if got := education.GetResourceBoxesByPurpose(ctx, "challenge"); len(got) != 1 || got[0].ID != 100 {
		t.Fatalf("challenge resource boxes = %+v", got)
	}

	if got := education.GetAreaItem(ctx, 0); got != nil {
		t.Fatalf("invalid area item = %+v, want nil", got)
	}
	area := education.GetAreaItem(ctx, 10)
	if area == nil || area.Name != "Sekai tree" {
		t.Fatalf("area item = %+v", area)
	}
	area.Name = "mutated"
	if cached := education.GetAreaItem(ctx, 10); cached.Name != "Sekai tree" {
		t.Fatal("area item result aliases the provider cache")
	}
	if got := education.GetAreaItem(ctx, 99); got != nil {
		t.Fatalf("missing area item = %+v, want nil", got)
	}
	if got := education.GetAreaItems(ctx); len(got) != 1 || got[0].ID != 10 {
		t.Fatalf("area items = %+v", got)
	}
	levels := education.GetAreaItemLevels(ctx, 10)
	if len(levels) != 2 || levels[1].Level != 2 {
		t.Fatalf("area item levels = %+v", levels)
	}
	levels[0].Power1BonusRate = -1
	if cached := education.GetAreaItemLevel(ctx, 10, 1); cached == nil || cached.Power1BonusRate != 0.5 {
		t.Fatalf("cached area item level = %+v", cached)
	}
	if education.GetAreaItemLevels(ctx, 0) != nil || education.GetAreaItemLevel(ctx, 0, 1) != nil || education.GetAreaItemLevel(ctx, 10, 0) != nil || education.GetAreaItemLevel(ctx, 99, 1) != nil {
		t.Fatal("invalid or missing area-item levels should return nil")
	}

	rank := education.GetCharacterRank(ctx, 5, 10)
	if rank == nil || rank.Power1BonusRate != 2.5 {
		t.Fatalf("character rank = %+v", rank)
	}
	rank.Power1BonusRate = -1
	if cached := education.GetCharacterRank(ctx, 5, 10); cached.Power1BonusRate != 2.5 {
		t.Fatal("character rank result aliases the provider cache")
	}
	if education.GetCharacterRank(ctx, 0, 10) != nil || education.GetCharacterRank(ctx, 5, 0) != nil || education.GetCharacterRank(ctx, 99, 1) != nil {
		t.Fatal("invalid or missing character ranks should return nil")
	}
	characterLevels := education.GetCharacterLevels(ctx)
	if len(characterLevels) != 1 || characterLevels[0].Level != 10 {
		t.Fatalf("character levels = %+v", characterLevels)
	}
	characterLevels[0].TotalExp = -1
	if cached := education.GetCharacterLevels(ctx); cached[0].TotalExp != 1234 {
		t.Fatal("character level result aliases the provider cache")
	}

	bonds := education.GetBonds(ctx)
	if len(bonds) != 1 || bonds[0].GroupID != 50 {
		t.Fatalf("bonds = %+v", bonds)
	}
	bonds[0].GroupID = -1
	if cached := education.GetBonds(ctx); cached[0].GroupID != 50 {
		t.Fatal("bond result aliases the provider cache")
	}
	bondLevels := education.GetBondLevels(ctx)
	if len(bondLevels) != 1 || bondLevels[0].TotalExp != 100 {
		t.Fatalf("bond levels = %+v", bondLevels)
	}
	style := education.GetGameCharacterStyle(ctx, 500)
	if style == nil || style.CharacterID != 5 || style.ColorCode != "#abcdef" {
		t.Fatalf("game character style = %+v", style)
	}
	style.ColorCode = "mutated"
	if cached := education.GetGameCharacterStyle(ctx, 500); cached.ColorCode != "#abcdef" {
		t.Fatal("game character style result aliases the provider cache")
	}
	if education.GetGameCharacterStyle(ctx, 0) != nil || education.GetGameCharacterStyle(ctx, 999) != nil {
		t.Fatal("invalid or missing game character styles should return nil")
	}

	missions := education.GetCharacterMissions(ctx, 5)
	if len(missions) != 1 || missions[0].ID != 501 || !missions[0].IsAchievementMission {
		t.Fatalf("character missions = %+v", missions)
	}
	missions[0].ID = -1
	if cached := education.GetCharacterMissions(ctx, 5); cached[0].ID != 501 {
		t.Fatal("character mission result aliases the provider cache")
	}
	groups := education.GetCharacterMissionParameterGroups(ctx, 101)
	if len(groups) != 1 || groups[0].Requirement != 20 {
		t.Fatalf("mission parameter groups = %+v", groups)
	}
	requirements, maxPlayLimit := education.GetLeaderMissionRequirements(ctx)
	if len(requirements) != 1 || requirements[0].Requirement != 20 || maxPlayLimit != 12 {
		t.Fatalf("leader requirements = %+v, max=%d", requirements, maxPlayLimit)
	}
	requirements[0].Requirement = -1
	if cached, _ := education.GetLeaderMissionRequirements(ctx); cached[0].Requirement != 20 {
		t.Fatal("leader requirement result aliases the provider cache")
	}
	if education.GetCharacterMissions(ctx, 0) != nil || education.GetCharacterMissions(ctx, 99) != nil ||
		education.GetCharacterMissionParameterGroups(ctx, 0) != nil || education.GetCharacterMissionParameterGroups(ctx, 404) != nil {
		t.Fatal("invalid or missing mission lookups should return nil")
	}

	gate := education.GetMysekaiGateLevel(ctx, 7, 3)
	if gate == nil || gate.PowerBonusRate != 1.25 {
		t.Fatalf("gate level = %+v", gate)
	}
	gate.PowerBonusRate = -1
	if cached := education.GetMysekaiGateLevel(ctx, 7, 3); cached.PowerBonusRate != 1.25 {
		t.Fatal("gate-level result aliases the provider cache")
	}
	if education.GetMysekaiGateLevel(ctx, 0, 3) != nil || education.GetMysekaiGateLevel(ctx, 7, 0) != nil || education.GetMysekaiGateLevel(ctx, 99, 1) != nil {
		t.Fatal("invalid or missing gate levels should return nil")
	}

	shop := education.GetShopItemByResourceBoxID(ctx, 101)
	if shop == nil || shop.ID != 300 || len(shop.Costs) != 1 || shop.Costs[0].Quantity != 300 {
		t.Fatalf("shop item = %+v", shop)
	}
	shop.Costs[0].Quantity = -1
	if cached := education.GetShopItemByResourceBoxID(ctx, 101); cached.Costs[0].Quantity != 300 {
		t.Fatal("shop item costs alias the provider cache")
	}
	if got := education.GetShopItems(ctx); len(got) != 1 || got[0].ID != 300 {
		t.Fatalf("shop items = %+v", got)
	}
	if education.GetShopItemByResourceBoxID(ctx, 0) != nil || education.GetShopItemByResourceBoxID(ctx, 999) != nil {
		t.Fatal("invalid or missing shop items should return nil")
	}
}

func TestDBEducationProviderReturnsNilWhenDatabaseUnavailable(t *testing.T) {
	ctx := context.Background()
	provider := openProviderBehaviorDB(t, "education_errors")
	if err := provider.client.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
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
			if got != nil && !reflect.ValueOf(got).IsNil() {
				t.Fatalf("database-error result = %#v, want nil", got)
			}
		})
	}
}
