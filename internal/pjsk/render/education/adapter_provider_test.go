package education

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"

	_ "github.com/mattn/go-sqlite3"
)

type educationAdapterContextKey struct{}

type populatedEducationMasterProvider struct {
	provider.MasterDataProvider
	education provider.EducationProvider
}

func (p populatedEducationMasterProvider) Education() provider.EducationProvider { return p.education }

type populatedEducationProvider struct{}

func (populatedEducationProvider) GetChallengeRewardsByCharacter(context.Context, int) []*provider.ChallengeReward {
	return []*provider.ChallengeReward{{ID: 1, CharacterID: 2, HighScore: 3, ResourceBoxID: 4}}
}
func (populatedEducationProvider) GetResourceBoxByPurpose(context.Context, string, int) *provider.ResourceBox {
	return &provider.ResourceBox{ID: 5}
}
func (populatedEducationProvider) GetResourceBoxesByPurpose(context.Context, string) []*provider.ResourceBox {
	return []*provider.ResourceBox{{ID: 6}}
}
func (populatedEducationProvider) GetAreaItems(context.Context) []*provider.AreaItem {
	return []*provider.AreaItem{{ID: 7}}
}
func (populatedEducationProvider) GetAreaItem(context.Context, int) *provider.AreaItem {
	return &provider.AreaItem{ID: 8}
}
func (populatedEducationProvider) GetAreaItemLevels(context.Context, int) []*provider.AreaItemLevel {
	return []*provider.AreaItemLevel{{AreaItemID: 9, Level: 1}}
}
func (populatedEducationProvider) GetAreaItemLevel(context.Context, int, int) *provider.AreaItemLevel {
	return &provider.AreaItemLevel{AreaItemID: 10, Level: 2}
}
func (populatedEducationProvider) GetCharacterLevels(context.Context) []*provider.CharacterLevel {
	return []*provider.CharacterLevel{nil, {Level: 11, TotalExp: 12}}
}
func (populatedEducationProvider) GetCharacterRank(context.Context, int, int) *provider.CharacterRank {
	return &provider.CharacterRank{CharacterID: 13, Rank: 14, Power1BonusRate: 0.15}
}
func (populatedEducationProvider) GetBonds(context.Context) []*provider.Bond {
	return []*provider.Bond{{GroupID: 16, CharacterID1: 17, CharacterID2: 18}}
}
func (populatedEducationProvider) GetBondLevels(context.Context) []*provider.BondLevel {
	return []*provider.BondLevel{{Level: 19, TotalExp: 20}}
}
func (populatedEducationProvider) GetGameCharacterStyle(context.Context, int) *provider.GameCharacterStyle {
	return &provider.GameCharacterStyle{GameID: 21, CharacterID: 22, ColorCode: "#ffffff"}
}
func (populatedEducationProvider) GetCharacterMissions(context.Context, int) []*provider.CharacterMission {
	return []*provider.CharacterMission{nil, {ID: 23, CharacterID: 24, CharacterMissionType: "live", ParameterGroupID: 25, IsAchievementMission: true}}
}
func (populatedEducationProvider) GetCharacterMissionParameterGroups(context.Context, int) []*provider.CharacterMissionParameterGroup {
	return []*provider.CharacterMissionParameterGroup{nil, {GameID: 26, Seq: 27, Requirement: 28, Exp: 29, Quantity: 30}}
}
func (populatedEducationProvider) GetLeaderMissionRequirements(context.Context) ([]provider.LeaderMissionRequirement, int) {
	return []provider.LeaderMissionRequirement{{Seq: 31, Requirement: 32}}, 33
}
func (populatedEducationProvider) GetMysekaiGateLevel(context.Context, int, int) *provider.MysekaiGateLevel {
	return &provider.MysekaiGateLevel{GateID: 34, Level: 35, PowerBonusRate: 0.36}
}
func (populatedEducationProvider) GetShopItemByResourceBoxID(context.Context, int) *provider.ShopItem {
	return &provider.ShopItem{ID: 37}
}
func (populatedEducationProvider) GetShopItems(context.Context) []*provider.ShopItem {
	return []*provider.ShopItem{{ID: 38}}
}

func TestProviderAdapterEmptyDatabaseAndContext(t *testing.T) {
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:education_adapter_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	source := provider.NewDatabaseProvider(client, renderregion.JP)
	adapter := NewProviderAdapter(source)
	ctx := context.WithValue(context.Background(), educationAdapterContextKey{}, "request")
	withContext := adapter.WithContext(ctx)
	if withContext == nil {
		t.Fatal("WithContext returned nil")
	}
	if (*ProviderAdapter)(nil).WithContext(ctx) != nil {
		t.Fatal("nil adapter WithContext returned a source")
	}
	a := withContext.(*ProviderAdapter)
	if a.Context() != ctx {
		t.Fatal("adapter did not retain the request context")
	}
	assertEmptyEducationCore(t, a)
	assertEmptyEducationProgression(t, a)
	assertEmptyEducationMissions(t, a)
}

func assertEmptyEducationCore(t *testing.T, a *ProviderAdapter) {
	t.Helper()
	if got := a.GetChallengeRewardsByCharacter(1); len(got) != 0 {
		t.Fatalf("challenge rewards = %#v", got)
	}
	if a.GetResourceBoxByPurpose("test", 1) != nil || len(a.GetResourceBoxesByPurpose("test")) != 0 {
		t.Fatal("empty resource boxes returned data")
	}
	if len(a.GetAreaItems()) != 0 || a.GetAreaItem(1) != nil || len(a.GetAreaItemLevels(1)) != 0 || a.GetAreaItemLevel(1, 1) != nil {
		t.Fatal("empty area masterdata returned data")
	}
}

func assertEmptyEducationProgression(t *testing.T, a *ProviderAdapter) {
	t.Helper()
	if len(a.GetCharacterLevels()) != 0 || a.GetCharacterRank(1, 1) != nil {
		t.Fatal("empty character progression returned data")
	}
	if len(a.GetBonds()) != 0 || len(a.GetBondLevels()) != 0 || a.GetGameCharacterStyle(1) != nil {
		t.Fatal("empty bonds masterdata returned data")
	}
}

func assertEmptyEducationMissions(t *testing.T, a *ProviderAdapter) {
	t.Helper()
	if len(a.GetCharacterMissions(1)) != 0 || len(a.GetCharacterMissionParameterGroups(1)) != 0 {
		t.Fatal("empty character missions returned data")
	}
	if requirements, maxPlayLimit := a.GetLeaderMissionRequirements(); len(requirements) != 0 || maxPlayLimit != 0 {
		t.Fatalf("leader requirements = %#v, %d", requirements, maxPlayLimit)
	}
	if a.GetMysekaiGateLevel(1, 1) != nil || a.GetShopItemByResourceBoxID(1) != nil || len(a.GetShopItems()) != 0 {
		t.Fatal("empty MySekai shop masterdata returned data")
	}
}

func TestProviderAdapterConversions(t *testing.T) {
	shop := convertShopItem(&provider.ShopItem{
		ID: 1, ShopID: 2, Seq: 3, ResourceBoxID: 4, ReleaseConditionID: 5, StartAt: 6,
		Costs: []provider.ShopItemCost{{ResourceType: "material", ResourceID: 7, Quantity: 8}},
	})
	if shop == nil || shop.ID != 1 || len(shop.Costs) != 1 || shop.Costs[0].Quantity != 8 {
		t.Fatalf("converted shop = %#v", shop)
	}
	box := convertResourceBox(&provider.ResourceBox{
		ID: 10, ResourceBoxPurpose: "challenge", ResourceBoxType: "expand", Description: "box",
		Details: []provider.ResourceBoxDetail{{ResourceType: "coin", ResourceID: 11, ResourceLevel: 12, ResourceQuantity: 13}},
	})
	if box == nil || box.ID != 10 || len(box.Details) != 1 || box.Details[0].ResourceQuantity != 13 {
		t.Fatalf("converted resource box = %#v", box)
	}
	area := convertAreaItem(&provider.AreaItem{ID: 20, AreaID: 21, Name: "area", AssetbundleName: "asset"})
	if !reflect.DeepEqual(area, &AreaItem{ID: 20, AreaID: 21, Name: "area", AssetbundleName: "asset"}) {
		t.Fatalf("converted area item = %#v", area)
	}
	level := convertAreaItemLevel(&provider.AreaItemLevel{
		AreaItemID: 30, Level: 2, TargetUnit: "unit", TargetCardAttr: "cool", TargetGameCharacterID: 31, Power1BonusRate: 0.2,
	})
	if level == nil || level.AreaItemID != 30 || level.Power1BonusRate != 0.2 {
		t.Fatalf("converted area level = %#v", level)
	}

	if convertShopItem(nil) != nil || convertResourceBox(nil) != nil || convertAreaItem(nil) != nil || convertAreaItemLevel(nil) != nil {
		t.Fatal("a conversion returned non-nil for nil input")
	}
}

func TestProviderAdapterPopulatedConversions(t *testing.T) {
	adapter := NewProviderAdapter(populatedEducationMasterProvider{education: populatedEducationProvider{}})
	assertPopulatedEducationCore(t, adapter)
	assertPopulatedEducationProgression(t, adapter)
	assertPopulatedEducationMissions(t, adapter)
}

func assertPopulatedEducationCore(t *testing.T, adapter *ProviderAdapter) {
	t.Helper()
	if got := adapter.GetChallengeRewardsByCharacter(1); len(got) != 1 || got[0].ResourceBoxID != 4 {
		t.Fatalf("challenge rewards = %#v", got)
	}
	if adapter.GetResourceBoxByPurpose("purpose", 1).ID != 5 || adapter.GetResourceBoxesByPurpose("purpose")[0].ID != 6 {
		t.Fatal("resource box conversion failed")
	}
	if adapter.GetAreaItems()[0].ID != 7 || adapter.GetAreaItem(8).ID != 8 ||
		adapter.GetAreaItemLevels(9)[0].AreaItemID != 9 || adapter.GetAreaItemLevel(10, 2).AreaItemID != 10 {
		t.Fatal("area conversion failed")
	}
}

func assertPopulatedEducationProgression(t *testing.T, adapter *ProviderAdapter) {
	t.Helper()
	levels := adapter.GetCharacterLevels()
	if len(levels) != 2 || levels[0] != nil || levels[1].TotalExp != 12 || adapter.GetCharacterRank(13, 14).Rank != 14 {
		t.Fatalf("character progression = %#v", levels)
	}
	if adapter.GetBonds()[0].CharacterID2 != 18 || adapter.GetBondLevels()[0].TotalExp != 20 ||
		adapter.GetGameCharacterStyle(21).ColorCode != "#ffffff" {
		t.Fatal("bond conversion failed")
	}
}

func assertPopulatedEducationMissions(t *testing.T, adapter *ProviderAdapter) {
	t.Helper()
	missions := adapter.GetCharacterMissions(24)
	groups := adapter.GetCharacterMissionParameterGroups(25)
	if len(missions) != 2 || missions[0] != nil || missions[1].ParameterGroupID != 25 ||
		len(groups) != 2 || groups[0] != nil || groups[1].Quantity != 30 {
		t.Fatalf("mission conversion = %#v, %#v", missions, groups)
	}
	requirements, maxPlayLimit := adapter.GetLeaderMissionRequirements()
	if len(requirements) != 1 || requirements[0].Requirement != 32 || maxPlayLimit != 33 {
		t.Fatalf("leader requirements = %#v, %d", requirements, maxPlayLimit)
	}
	if adapter.GetMysekaiGateLevel(34, 35).PowerBonusRate != 0.36 ||
		adapter.GetShopItemByResourceBoxID(37).ID != 37 || adapter.GetShopItems()[0].ID != 38 {
		t.Fatal("MySekai conversion failed")
	}
}
