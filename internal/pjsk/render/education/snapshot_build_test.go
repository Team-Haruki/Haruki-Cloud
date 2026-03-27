package education

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
)

type testSource struct {
	region     renderregion.Value
	boxes      map[string]map[int]*ResourceBox
	areaItems  map[int]*AreaItem
	areaLevels map[int]map[int]*AreaItemLevel
	ranks      map[int]map[int]*CharacterRank
	gates      map[int]map[int]*MysekaiGateLevel
	shopItems  map[int]*ShopItem
}

func (s *testSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testSource) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward { return nil }

func (s *testSource) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	if boxes, ok := s.boxes[purpose]; ok {
		return boxes[id]
	}
	return nil
}

func (s *testSource) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	boxes := s.boxes[purpose]
	if len(boxes) == 0 {
		return nil
	}
	out := make([]*ResourceBox, 0, len(boxes))
	for _, box := range boxes {
		out = append(out, box)
	}
	return out
}

func (s *testSource) GetAreaItem(id int) *AreaItem {
	return s.areaItems[id]
}

func (s *testSource) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	levels := s.areaLevels[areaItemID]
	if len(levels) == 0 {
		return nil
	}
	out := make([]*AreaItemLevel, 0, len(levels))
	for _, level := range levels {
		out = append(out, level)
	}
	return out
}

func (s *testSource) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
	return s.areaLevels[areaItemID][level]
}

func (s *testSource) GetCharacterRank(characterID, rank int) *CharacterRank {
	return s.ranks[characterID][rank]
}

func (s *testSource) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	return s.gates[gateID][level]
}

func (s *testSource) GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem {
	return s.shopItems[resourceBoxID]
}

func TestBuildPowerBonusDetailRequestFromSnapshot(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
			"coin":   1000,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 2, "member3": 3, "member4": 4, "member5": 5},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userAreas": []map[string]any{
			{"areaItems": []map[string]any{
				{"areaItemId": 101, "level": 2},
				{"areaItemId": 102, "level": 1},
				{"areaItemId": 103, "level": 1},
			}},
		},
		"userCharacters": []map[string]any{
			{"characterId": 1, "characterRank": 5},
		},
		"userMysekaiGates": []map[string]any{
			{"mysekaiGateId": 1, "mysekaiGateLevel": 2},
		},
		"userMysekaiFixtureGameCharacterPerformanceBonuses": []map[string]any{
			{"gameCharacterId": 1, "totalBonusRate": 12},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.JP)
	controller.RegisterSource(&testSource{
		region: renderregion.JP,
		areaLevels: map[int]map[int]*AreaItemLevel{
			101: {2: {AreaItemID: 101, Level: 2, TargetGameCharacterID: 1, Power1BonusRate: 3.0}},
			102: {1: {AreaItemID: 102, Level: 1, TargetUnit: "light_sound", Power1BonusRate: 2.5}},
			103: {1: {AreaItemID: 103, Level: 1, TargetCardAttr: "cute", Power1BonusRate: 1.0}},
		},
		ranks: map[int]map[int]*CharacterRank{
			1: {5: {CharacterID: 1, Rank: 5, Power1BonusRate: 4.0}},
		},
		gates: map[int]map[int]*MysekaiGateLevel{
			1: {2: {GateID: 1, Level: 2, PowerBonusRate: 2.0}},
		},
	})

	req, err := controller.BuildPowerBonusDetailRequestFromSnapshot(PowerBonusQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildPowerBonusDetailRequestFromSnapshot() error = %v", err)
	}
	if req.Profile.ID != "1001" {
		t.Fatalf("unexpected profile id: %s", req.Profile.ID)
	}
	if len(req.CharaBonuses) != 26 {
		t.Fatalf("expected 26 character bonuses, got %d", len(req.CharaBonuses))
	}
	if got := req.CharaBonuses[0]; got.AreaItem != 3.0 || got.Rank != 4.0 || !approxEqual(got.Fixture, 1.2) || !approxEqual(got.Total, 8.2) {
		t.Fatalf("unexpected char bonus: %+v", got)
	}
	if got := req.UnitBonuses[0]; got.Unit != "light_sound" || got.AreaItem != 2.5 || got.Gate != 2.0 || got.Total != 4.5 {
		t.Fatalf("unexpected light_sound bonus: %+v", got)
	}
	if got := req.UnitBonuses[len(req.UnitBonuses)-1]; got.Unit != "piapro" || got.Gate != 2.0 || got.Total != 2.0 {
		t.Fatalf("unexpected piapro bonus: %+v", got)
	}
	if got := req.AttrBonuses[0]; got.Attr != "cute" || got.AreaItem != 1.0 || got.Total != 1.0 {
		t.Fatalf("unexpected cute bonus: %+v", got)
	}
}

func TestBuildAreaItemUpgradeMaterialsRequestFromSnapshot(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
			"coin":   1000,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 2, "member3": 3, "member4": 4, "member5": 5},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userAreas": []map[string]any{
			{"areaItems": []map[string]any{
				{"areaItemId": 101, "level": 1},
				{"areaItemId": 102, "level": 2},
			}},
		},
		"userMaterials": []map[string]any{
			{"materialId": 201, "quantity": 7},
			{"materialId": 202, "quantity": 10},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.JP)
	controller.RegisterSource(&testSource{
		region: renderregion.JP,
		boxes: map[string]map[int]*ResourceBox{
			"shop_item": {
				11: {ID: 11, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 101, ResourceLevel: 2}}},
				12: {ID: 12, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 101, ResourceLevel: 3}}},
				21: {ID: 21, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 102, ResourceLevel: 3}}},
			},
		},
		areaItems: map[int]*AreaItem{
			101: {ID: 101, AssetbundleName: "item_101"},
			102: {ID: 102, AssetbundleName: "item_102"},
		},
		areaLevels: map[int]map[int]*AreaItemLevel{
			101: {
				1: {AreaItemID: 101, Level: 1, TargetGameCharacterID: 1, Power1BonusRate: 1.0},
				2: {AreaItemID: 101, Level: 2, TargetGameCharacterID: 1, Power1BonusRate: 2.0},
				3: {AreaItemID: 101, Level: 3, TargetGameCharacterID: 1, Power1BonusRate: 3.0},
			},
			102: {
				1: {AreaItemID: 102, Level: 1, TargetUnit: "street", Power1BonusRate: 0.5},
				2: {AreaItemID: 102, Level: 2, TargetUnit: "street", Power1BonusRate: 1.0},
				3: {AreaItemID: 102, Level: 3, TargetUnit: "street", Power1BonusRate: 1.5},
			},
		},
		shopItems: map[int]*ShopItem{
			11: {ID: 10011, ResourceBoxID: 11, Costs: []ShopItemCost{
				{ResourceType: "coin", Quantity: 100},
				{ResourceType: "material", ResourceID: 201, Quantity: 5},
			}},
			12: {ID: 10012, ResourceBoxID: 12, Costs: []ShopItemCost{
				{ResourceType: "coin", Quantity: 200},
				{ResourceType: "material", ResourceID: 201, Quantity: 10},
			}},
			21: {ID: 10021, ResourceBoxID: 21, Costs: []ShopItemCost{
				{ResourceType: "coin", Quantity: 300},
				{ResourceType: "material", ResourceID: 202, Quantity: 4},
			}},
		},
	})

	req, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildAreaItemUpgradeMaterialsRequestFromSnapshot() error = %v", err)
	}
	if !req.HasProfile || req.Profile == nil || req.Profile.ID != "1001" {
		t.Fatalf("unexpected profile payload: %+v", req.Profile)
	}
	if len(req.AreaItems) != 2 {
		t.Fatalf("expected 2 area items, got %d", len(req.AreaItems))
	}

	first := req.AreaItems[0]
	if first.ItemID != 101 || first.CurrentLevel != 1 || len(first.Levels) != 2 {
		t.Fatalf("unexpected first area item: %+v", first)
	}
	if first.TargetIconPath == nil || !strings.Contains(*first.TargetIconPath, "chara_icon") {
		t.Fatalf("unexpected target icon path: %v", first.TargetIconPath)
	}
	if got := first.Levels[0]; got.Level != 2 || !got.CanUpgrade || len(got.Materials) != 2 {
		t.Fatalf("unexpected level 2 payload: %+v", got)
	}
	if got := first.Levels[0].Materials[0]; got.MaterialID != areaCoinMaterialID || got.Quantity != 100 || got.HaveQuantity != 1000 || got.SumQuantity != 100 || !got.IsEnough {
		t.Fatalf("unexpected coin material payload: %+v", got)
	}
	if got := first.Levels[1]; got.Level != 3 || got.CanUpgrade {
		t.Fatalf("unexpected level 3 payload: %+v", got)
	}
	if got := first.Levels[1].Materials[1]; got.MaterialID != 201 || got.SumQuantity != 15 || got.IsEnough {
		t.Fatalf("unexpected cumulative material payload: %+v", got)
	}

	second := req.AreaItems[1]
	if second.ItemID != 102 || second.CurrentLevel != 2 || len(second.Levels) != 2 {
		t.Fatalf("unexpected second area item: %+v", second)
	}
	if got := second.Levels[0]; got.Level != 2 || len(got.Materials) != 0 {
		t.Fatalf("expected historical level to have no materials: %+v", got)
	}
	if got := second.Levels[1]; got.Level != 3 || !got.CanUpgrade || len(got.Materials) != 2 {
		t.Fatalf("unexpected upgrade level payload: %+v", got)
	}
}

func mustSnapshot(t *testing.T, payload map[string]any) *userdata.Service {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	snapshot, err := userdata.NewFromBytes(nil, nil, renderregion.JP, data, nil, nil)
	if err != nil {
		t.Fatalf("NewFromBytes() error = %v", err)
	}
	return snapshot
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
