package education

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/userdata"
)

type educationContextKey string

type contextAwareSource struct {
	*testSource
	ctx       context.Context
	wantKey   educationContextKey
	wantValue string
}

func (s *contextAwareSource) WithContext(ctx context.Context) DataSource {
	clone := *s
	clone.ctx = ctx
	return &clone
}

func (s *contextAwareSource) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	if s.ctx == nil {
		return nil
	}
	value, _ := s.ctx.Value(s.wantKey).(string)
	if value != s.wantValue {
		return nil
	}
	return s.testSource.GetAreaItemLevels(areaItemID)
}

type testSource struct {
	region             renderregion.Value
	boxes              map[string]map[int]*ResourceBox
	areaItems          map[int]*AreaItem
	areaLevels         map[int]map[int]*AreaItemLevel
	ranks              map[int]map[int]*CharacterRank
	bonds              []*Bond
	bondLevels         []*BondLevel
	charStyles         map[int]*GameCharacterStyle
	leaderRequirements []LeaderMissionRequirement
	leaderMaxPlayLimit int
	gates              map[int]map[int]*MysekaiGateLevel
	shopItems          map[int]*ShopItem
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

func (s *testSource) GetAreaItems() []*AreaItem {
	if len(s.areaItems) == 0 {
		return nil
	}
	out := make([]*AreaItem, 0, len(s.areaItems))
	for _, item := range s.areaItems {
		out = append(out, item)
	}
	return out
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

func (s *testSource) GetBonds() []*Bond {
	return s.bonds
}

func (s *testSource) GetBondLevels() []*BondLevel {
	return s.bondLevels
}

func (s *testSource) GetGameCharacterStyle(gameID int) *GameCharacterStyle {
	return s.charStyles[gameID]
}

func (s *testSource) GetLeaderMissionRequirements() ([]LeaderMissionRequirement, int) {
	return append([]LeaderMissionRequirement(nil), s.leaderRequirements...), s.leaderMaxPlayLimit
}

func (s *testSource) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	return s.gates[gateID][level]
}

func (s *testSource) GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem {
	return s.shopItems[resourceBoxID]
}

func (s *testSource) GetShopItems() []*ShopItem {
	if len(s.shopItems) == 0 {
		return nil
	}
	out := make([]*ShopItem, 0, len(s.shopItems))
	for _, item := range s.shopItems {
		if item == nil {
			continue
		}
		clone := *item
		clone.Costs = append([]ShopItemCost(nil), item.Costs...)
		out = append(out, &clone)
	}
	return out
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

func TestBuildPowerBonusDetailRequestFromSnapshotCapsUnreleasedAreaItemLevel(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 100,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userAreas": []map[string]any{
			{"areaItems": []map[string]any{
				{"areaItemId": 101, "level": 3},
			}},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.CN)
	controller.RegisterSource(&testSource{
		region: renderregion.CN,
		boxes: map[string]map[int]*ResourceBox{
			"shop_item": {
				11: {ID: 11, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 101, ResourceLevel: 2}}},
			},
		},
		areaLevels: map[int]map[int]*AreaItemLevel{
			101: {
				1: {AreaItemID: 101, Level: 1, TargetGameCharacterID: 1, Power1BonusRate: 1.0},
				2: {AreaItemID: 101, Level: 2, TargetGameCharacterID: 1, Power1BonusRate: 2.0},
				3: {AreaItemID: 101, Level: 3, TargetGameCharacterID: 1, Power1BonusRate: 3.0},
			},
		},
		shopItems: map[int]*ShopItem{
			11: {ID: 10011, ResourceBoxID: 11, StartAt: 50},
		},
	})

	req, err := controller.BuildPowerBonusDetailRequestFromSnapshot(PowerBonusQuery{Region: renderregion.CN})
	if err != nil {
		t.Fatalf("BuildPowerBonusDetailRequestFromSnapshot() error = %v", err)
	}
	if got := req.CharaBonuses[0]; got.AreaItem != 2.0 || got.Total != 2.0 {
		t.Fatalf("unexpected capped char bonus: %+v", got)
	}
}

func TestControllerWithContextClonesEducationSource(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userAreas": []map[string]any{
			{"areaItems": []map[string]any{
				{"areaItemId": 101, "level": 2},
			}},
		},
	})

	source := &contextAwareSource{
		testSource: &testSource{
			region: renderregion.JP,
			areaLevels: map[int]map[int]*AreaItemLevel{
				101: {2: {AreaItemID: 101, Level: 2, TargetGameCharacterID: 1, Power1BonusRate: 3.0}},
			},
		},
		wantKey:   educationContextKey("trace"),
		wantValue: "education-power",
	}

	controller := NewController(nil, nil, snapshot, renderregion.JP)
	controller.RegisterSource(source)
	ctx := context.WithValue(context.Background(), educationContextKey("trace"), "education-power")

	req, err := controller.WithContext(ctx).BuildPowerBonusDetailRequestFromSnapshot(PowerBonusQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildPowerBonusDetailRequestFromSnapshot() error = %v", err)
	}
	if len(req.CharaBonuses) != 26 {
		t.Fatalf("unexpected char bonus len: %d", len(req.CharaBonuses))
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
				20: {ID: 20, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 102, ResourceLevel: 2}}},
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
			20: {ID: 10020, ResourceBoxID: 20, Costs: []ShopItemCost{
				{ResourceType: "coin", Quantity: 150},
				{ResourceType: "material", ResourceID: 202, Quantity: 2},
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
		t.Fatalf("unexpected aligned current level payload: %+v", got)
	}
	if got := second.Levels[1]; got.Level != 3 || !got.CanUpgrade || len(got.Materials) != 2 {
		t.Fatalf("unexpected upgrade level payload: %+v", got)
	}
}

func TestBuildAreaItemUpgradeMaterialsRequestFromSnapshotAppliesFilters(t *testing.T) {
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
				{"areaItemId": 201, "level": 1},
			}},
		},
		"userMaterials": []map[string]any{
			{"materialId": 301, "quantity": 50},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.JP)
	controller.RegisterSource(&testSource{
		region: renderregion.JP,
		boxes: map[string]map[int]*ResourceBox{
			"shop_item": {
				31: {ID: 31, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 201, ResourceLevel: 2}}},
				32: {ID: 32, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 202, ResourceLevel: 1}}},
				33: {ID: 33, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 203, ResourceLevel: 1}}},
				34: {ID: 34, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 204, ResourceLevel: 1}}},
				35: {ID: 35, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 205, ResourceLevel: 1}}},
				36: {ID: 36, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 206, ResourceLevel: 1}}},
			},
		},
		areaItems: map[int]*AreaItem{
			201: {ID: 201, AreaID: 1, AssetbundleName: "item_201"},
			202: {ID: 202, AreaID: 8, AssetbundleName: "item_202"},
			203: {ID: 203, AreaID: 2, AssetbundleName: "item_203"},
			204: {ID: 204, AreaID: 11, AssetbundleName: "item_204"},
			205: {ID: 205, AreaID: 13, AssetbundleName: "item_205"},
			206: {ID: 206, AreaID: 3, AssetbundleName: "item_206"},
		},
		areaLevels: map[int]map[int]*AreaItemLevel{
			201: {
				1: {AreaItemID: 201, Level: 1, TargetGameCharacterID: 21, Power1BonusRate: 1.0},
				2: {AreaItemID: 201, Level: 2, TargetGameCharacterID: 21, Power1BonusRate: 2.0},
			},
			202: {
				1: {AreaItemID: 202, Level: 1, TargetUnit: "street", Power1BonusRate: 1.0},
			},
			203: {
				1: {AreaItemID: 203, Level: 1, TargetCardAttr: "cute", Power1BonusRate: 1.0},
			},
			204: {
				1: {AreaItemID: 204, Level: 1, Power1BonusRate: 1.0},
			},
			205: {
				1: {AreaItemID: 205, Level: 1, Power1BonusRate: 1.0},
			},
			206: {
				1: {AreaItemID: 206, Level: 1, TargetUnit: "piapro", Power1BonusRate: 1.0},
			},
		},
		shopItems: map[int]*ShopItem{
			31: {ID: 20031, ResourceBoxID: 31, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 301, Quantity: 1}}},
			32: {ID: 20032, ResourceBoxID: 32, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 301, Quantity: 1}}},
			33: {ID: 20033, ResourceBoxID: 33, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 301, Quantity: 1}}},
			34: {ID: 20034, ResourceBoxID: 34, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 301, Quantity: 1}}},
			35: {ID: 20035, ResourceBoxID: 35, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 301, Quantity: 1}}},
			36: {ID: 20036, ResourceBoxID: 36, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 301, Quantity: 1}}},
		},
	})

	tests := []struct {
		name        string
		query       AreaItemQuery
		wantItemIDs []int
	}{
		{name: "filter by unit", query: AreaItemQuery{Region: renderregion.JP, Unit: "street"}, wantItemIDs: []int{202}},
		{name: "filter by character", query: AreaItemQuery{Region: renderregion.JP, Cid: 21}, wantItemIDs: []int{201}},
		{name: "filter by attr", query: AreaItemQuery{Region: renderregion.JP, Attr: "cute"}, wantItemIDs: []int{203}},
		{name: "filter by tree", query: AreaItemQuery{Region: renderregion.JP, Tree: true}, wantItemIDs: []int{204}},
		{name: "filter by flower", query: AreaItemQuery{Region: renderregion.JP, Flower: true}, wantItemIDs: []int{205}},
		{name: "filter by piapro", query: AreaItemQuery{Region: renderregion.JP, Unit: "piapro"}, wantItemIDs: []int{201, 206}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(tt.query)
			if err != nil {
				t.Fatalf("BuildAreaItemUpgradeMaterialsRequestFromSnapshot() error = %v", err)
			}
			if len(req.AreaItems) != len(tt.wantItemIDs) {
				t.Fatalf("unexpected area item count: got=%d want=%d payload=%+v", len(req.AreaItems), len(tt.wantItemIDs), req.AreaItems)
			}
			for idx, wantID := range tt.wantItemIDs {
				if req.AreaItems[idx].ItemID != wantID {
					t.Fatalf("unexpected item order at %d: got=%d want=%d", idx, req.AreaItems[idx].ItemID, wantID)
				}
			}
		})
	}
}

func TestBuildBondsRequestFromSnapshot(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 1, "member3": 1, "member4": 1, "member5": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userBonds": []map[string]any{
			{"bondsGroupId": 2127, "rank": 2, "exp": 7},
		},
		"userCharacters": []map[string]any{
			{"characterId": 21, "characterRank": 55},
			{"characterId": 27, "characterRank": 33},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.JP)
	controller.RegisterSource(&testSource{
		region: renderregion.JP,
		bonds: []*Bond{
			{GroupID: 2127, CharacterID1: 21, CharacterID2: 27},
		},
		bondLevels: []*BondLevel{
			{Level: 1, TotalExp: 0},
			{Level: 2, TotalExp: 10},
			{Level: 3, TotalExp: 30},
		},
		charStyles: map[int]*GameCharacterStyle{
			21: {GameID: 21, CharacterID: 21, ColorCode: "#112233"},
			27: {GameID: 27, CharacterID: 27, ColorCode: "#445566"},
		},
	})

	req, err := controller.BuildBondsRequestFromSnapshot(BondsQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildBondsRequestFromSnapshot() error = %v", err)
	}
	if req.Profile.ID != "1001" {
		t.Fatalf("unexpected profile id: %s", req.Profile.ID)
	}
	if req.MaxLevel != 3 {
		t.Fatalf("unexpected max level: %d", req.MaxLevel)
	}
	if len(req.Bonds) != 1 {
		t.Fatalf("unexpected bonds payload: %+v", req.Bonds)
	}

	bond := req.Bonds[0]
	if bond.CharaID1 != 21 || bond.CharaID2 != 27 {
		t.Fatalf("unexpected bond pair: %+v", bond)
	}
	if !strings.Contains(bond.CharaIconPath1, "miku.png") {
		t.Fatalf("unexpected icon path1: %q", bond.CharaIconPath1)
	}
	if !strings.Contains(bond.CharaIconPath2, "chr_icon_27.png") {
		t.Fatalf("unexpected icon path2: %q", bond.CharaIconPath2)
	}
	if bond.CharaRank1 != 55 || bond.CharaRank2 != 33 {
		t.Fatalf("unexpected character ranks: %+v", bond)
	}
	if bond.BondLevel != 2 || !bond.HasBond {
		t.Fatalf("unexpected bond state: %+v", bond)
	}
	if bond.NeedExp == nil || *bond.NeedExp != 13 {
		t.Fatalf("unexpected need exp: %+v", bond.NeedExp)
	}
	if len(bond.Color1) != 3 || bond.Color1[0] != 17 || bond.Color1[1] != 34 || bond.Color1[2] != 51 {
		t.Fatalf("unexpected color1: %+v", bond.Color1)
	}
	if len(bond.Color2) != 3 || bond.Color2[0] != 68 || bond.Color2[1] != 85 || bond.Color2[2] != 102 {
		t.Fatalf("unexpected color2: %+v", bond.Color2)
	}
}

func TestBuildBondsRequestFromSnapshotUsesBaseCharacterForStyledPairs(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 1, "member3": 1, "member4": 1, "member5": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userBonds": []map[string]any{
			{"bondsGroupId": 9127, "rank": 7, "exp": 4},
		},
		"userCharacters": []map[string]any{
			{"characterId": 21, "characterRank": 55},
			{"characterId": 27, "characterRank": 33},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.JP)
	controller.RegisterSource(&testSource{
		region: renderregion.JP,
		bonds: []*Bond{
			{GroupID: 9127, CharacterID1: 121, CharacterID2: 27},
		},
		bondLevels: []*BondLevel{
			{Level: 1, TotalExp: 0},
			{Level: 7, TotalExp: 70},
			{Level: 8, TotalExp: 90},
		},
		charStyles: map[int]*GameCharacterStyle{
			121: {GameID: 121, CharacterID: 21, ColorCode: "#112233"},
			27:  {GameID: 27, CharacterID: 27, ColorCode: "#445566"},
		},
	})

	req, err := controller.BuildBondsRequestFromSnapshot(BondsQuery{Region: renderregion.JP, Cid: 21})
	if err != nil {
		t.Fatalf("BuildBondsRequestFromSnapshot() error = %v", err)
	}
	if len(req.Bonds) != 1 {
		t.Fatalf("unexpected bonds payload: %+v", req.Bonds)
	}

	bond := req.Bonds[0]
	if bond.CharaID1 != 121 || bond.CharaID2 != 27 {
		t.Fatalf("unexpected bond pair: %+v", bond)
	}
	if bond.CharaRank1 != 55 || bond.CharaRank2 != 33 {
		t.Fatalf("unexpected character ranks: %+v", bond)
	}
	if !strings.Contains(bond.CharaIconPath1, "miku.png") {
		t.Fatalf("unexpected icon path1: %q", bond.CharaIconPath1)
	}
}

func TestBuildBondsRequestFromSnapshotLimitsToTopTen(t *testing.T) {
	userBonds := make([]map[string]any, 0, 12)
	bondsMaster := make([]*Bond, 0, 12)
	bondLevels := make([]*BondLevel, 0, 12)
	for i := 1; i <= 12; i++ {
		groupID := 3000 + i
		userBonds = append(userBonds, map[string]any{
			"bondsGroupId": groupID,
			"rank":         i,
			"exp":          0,
		})
		bondsMaster = append(bondsMaster, &Bond{
			GroupID:      groupID,
			CharacterID1: i,
			CharacterID2: 100 + i,
		})
		bondLevels = append(bondLevels, &BondLevel{
			Level:    i,
			TotalExp: i * 10,
		})
	}

	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 1, "member3": 1, "member4": 1, "member5": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userBonds": userBonds,
	})

	controller := NewController(nil, nil, snapshot, renderregion.JP)
	controller.RegisterSource(&testSource{
		region:     renderregion.JP,
		bonds:      bondsMaster,
		bondLevels: bondLevels,
	})

	req, err := controller.BuildBondsRequestFromSnapshot(BondsQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildBondsRequestFromSnapshot() error = %v", err)
	}
	if len(req.Bonds) != 10 {
		t.Fatalf("expected top 10 bonds, got %d", len(req.Bonds))
	}
	if req.Bonds[0].BondLevel != 12 {
		t.Fatalf("expected highest bond level first, got %+v", req.Bonds[0])
	}
	if req.Bonds[len(req.Bonds)-1].BondLevel != 3 {
		t.Fatalf("expected tenth bond level to be 3, got %+v", req.Bonds[len(req.Bonds)-1])
	}
}

func TestBuildLeaderCountRequestFromSnapshot(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 1, "member3": 1, "member4": 1, "member5": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userCharacterMissionV2s": []map[string]any{
			{"characterMissionType": "play_live_ex", "characterId": 2, "progress": 11},
		},
		"userCharacterLiveUsageCounts": []map[string]any{
			{"characterId": 2, "characterLiveUsageType": "leader", "usageCount": 66},
		},
		"userCharacterMissionV2Statuses": []map[string]any{
			{"parameterGroupId": 101, "seq": 1, "characterId": 2},
			{"parameterGroupId": 101, "seq": 2, "characterId": 2},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.JP)
	controller.RegisterSource(&testSource{
		region: renderregion.JP,
		leaderRequirements: []LeaderMissionRequirement{
			{Seq: 1, Requirement: 3},
			{Seq: 2, Requirement: 7},
		},
		leaderMaxPlayLimit: 120,
	})

	req, err := controller.BuildLeaderCountRequestFromSnapshot(LeaderCountQuery{Region: renderregion.JP})
	if err != nil {
		t.Fatalf("BuildLeaderCountRequestFromSnapshot() error = %v", err)
	}
	if req.Profile.ID != "1001" {
		t.Fatalf("unexpected profile id: %s", req.Profile.ID)
	}
	if req.MaxPlayCount != 120 {
		t.Fatalf("unexpected max play count: %d", req.MaxPlayCount)
	}
	if len(req.LeaderCounts) != 26 {
		t.Fatalf("unexpected leader count size: %d", len(req.LeaderCounts))
	}

	first := req.LeaderCounts[0]
	if first.CharaID != 2 {
		t.Fatalf("unexpected first character: %+v", first)
	}
	if first.PlayCount != 66 {
		t.Fatalf("unexpected play count: %+v", first)
	}
	if first.ExLevel != 3 || first.ExCount != 21 {
		t.Fatalf("unexpected ex progress: %+v", first)
	}
	if strings.TrimSpace(first.CharaIconPath) == "" {
		t.Fatalf("expected chara icon path")
	}
}

func TestBuildLeaderCountRequestFromSnapshotFallsBackToLegacyMissionStatuses(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 1, "member3": 1, "member4": 1, "member5": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userCharacterMissionV2s": []map[string]any{
			{"characterMissionType": "play_live_ex", "characterId": 2, "progress": 11},
		},
		"userCharacterLiveUsageCounts": []map[string]any{
			{"characterId": 2, "characterLiveUsageType": "leader", "usageCount": 66},
		},
		"userCharacterMissionStatuses": []map[string]any{
			{"parameterGroupId": 101, "seq": 1, "characterId": 2},
			{"parameterGroupId": 101, "seq": 2, "characterId": 2},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.CN)
	controller.RegisterSource(&testSource{
		region: renderregion.CN,
		leaderRequirements: []LeaderMissionRequirement{
			{Seq: 1, Requirement: 3},
			{Seq: 2, Requirement: 7},
		},
		leaderMaxPlayLimit: 120,
	})

	req, err := controller.BuildLeaderCountRequestFromSnapshot(LeaderCountQuery{Region: renderregion.CN})
	if err != nil {
		t.Fatalf("BuildLeaderCountRequestFromSnapshot() error = %v", err)
	}
	if len(req.LeaderCounts) == 0 {
		t.Fatalf("expected leader counts")
	}
	first := req.LeaderCounts[0]
	if first.CharaID != 2 {
		t.Fatalf("unexpected first character: %+v", first)
	}
	if first.ExLevel != 3 || first.ExCount != 21 {
		t.Fatalf("unexpected ex progress from legacy statuses: %+v", first)
	}
}

func TestBuildLeaderCountRequestFromSnapshotUsesCompactMissionStatuses(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 1, "member3": 1, "member4": 1, "member5": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userCharacterMissionV2s": []map[string]any{
			{"characterMissionType": "play_live_ex", "characterId": 2, "progress": 11},
		},
		"userCharacterLiveUsageCounts": []map[string]any{
			{"characterId": 2, "characterLiveUsageType": "leader", "usageCount": 66},
		},
		"compactUserCharacterMissionV2Statuses": map[string]any{
			"__ENUM__": map[string]any{
				"missionStatus": []string{"achieved", "received", "reset"},
			},
			"parameterGroupId": []int{101, 101, 101, 102},
			"seq":              []int{1, 2, 3, 1},
			"characterId":      []int{2, 2, 2, 3},
			"missionId":        []int{21101, 21101, 21101, 39999},
			"missionStatus":    []int{1, 1, 3, 1},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.CN)
	controller.RegisterSource(&testSource{
		region: renderregion.CN,
		leaderRequirements: []LeaderMissionRequirement{
			{Seq: 1, Requirement: 3},
			{Seq: 2, Requirement: 7},
			{Seq: 3, Requirement: 11},
		},
		leaderMaxPlayLimit: 120,
	})

	req, err := controller.BuildLeaderCountRequestFromSnapshot(LeaderCountQuery{Region: renderregion.CN})
	if err != nil {
		t.Fatalf("BuildLeaderCountRequestFromSnapshot() error = %v", err)
	}
	if len(req.LeaderCounts) == 0 {
		t.Fatalf("expected leader counts")
	}
	first := req.LeaderCounts[0]
	if first.CharaID != 2 {
		t.Fatalf("unexpected first character: %+v", first)
	}
	if first.ExLevel != 3 || first.ExCount != 21 {
		t.Fatalf("unexpected ex progress from compact statuses: %+v", first)
	}
}

func TestBuildLeaderCountRequestFromSnapshotMergesCompactAndStandardMissionStatuses(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 12345,
		"userGamedata": map[string]any{
			"userId": 1001,
			"name":   "tester",
			"deck":   1,
		},
		"userProfile": map[string]any{
			"profileImageType": "normal",
		},
		"userDecks": []map[string]any{
			{"deckId": 1, "leader": 1, "member1": 1, "member2": 1, "member3": 1, "member4": 1, "member5": 1},
		},
		"userCards": []map[string]any{
			{"cardId": 1, "level": 1},
		},
		"userCharacterMissionV2s": []map[string]any{
			{"characterMissionType": "play_live_ex", "characterId": 2, "progress": 162},
		},
		"userCharacterLiveUsageCounts": []map[string]any{
			{"characterId": 2, "characterLiveUsageType": "leader", "usageCount": 66},
		},
		"userCharacterMissionV2Statuses": []map[string]any{
			{"parameterGroupId": 101, "seq": 1, "characterId": 2},
			{"parameterGroupId": 101, "seq": 2, "characterId": 2},
			{"parameterGroupId": 101, "seq": 3, "characterId": 2},
			{"parameterGroupId": 101, "seq": 4, "characterId": 2},
			{"parameterGroupId": 101, "seq": 5, "characterId": 2},
			{"parameterGroupId": 101, "seq": 6, "characterId": 2},
			{"parameterGroupId": 101, "seq": 7, "characterId": 2},
			{"parameterGroupId": 101, "seq": 8, "characterId": 2},
			{"parameterGroupId": 101, "seq": 9, "characterId": 2},
			{"parameterGroupId": 101, "seq": 10, "characterId": 2},
			{"parameterGroupId": 101, "seq": 11, "characterId": 2},
			{"parameterGroupId": 101, "seq": 12, "characterId": 2},
			{"parameterGroupId": 101, "seq": 13, "characterId": 2},
			{"parameterGroupId": 101, "seq": 14, "characterId": 2},
			{"parameterGroupId": 101, "seq": 15, "characterId": 2},
			{"parameterGroupId": 101, "seq": 16, "characterId": 2},
			{"parameterGroupId": 101, "seq": 17, "characterId": 2},
		},
		"compactUserCharacterMissionV2Statuses": map[string]any{
			"__ENUM__": map[string]any{
				"missionStatus": []string{"achieved", "received", "reset"},
			},
			"parameterGroupId": []int{101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101, 101},
			"seq":              []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25},
			"characterId":      []int{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
			"missionId":        []int{21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101, 21101},
			"missionStatus":    []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.CN)
	controller.RegisterSource(&testSource{
		region: renderregion.CN,
		leaderRequirements: []LeaderMissionRequirement{
			{Seq: 1, Requirement: 500},
			{Seq: 4, Requirement: 600},
			{Seq: 7, Requirement: 700},
			{Seq: 10, Requirement: 800},
			{Seq: 13, Requirement: 900},
			{Seq: 16, Requirement: 1000},
			{Seq: 19, Requirement: 1100},
			{Seq: 22, Requirement: 1200},
			{Seq: 25, Requirement: 1300},
		},
		leaderMaxPlayLimit: 40000,
	})

	req, err := controller.BuildLeaderCountRequestFromSnapshot(LeaderCountQuery{Region: renderregion.CN})
	if err != nil {
		t.Fatalf("BuildLeaderCountRequestFromSnapshot() error = %v", err)
	}
	if len(req.LeaderCounts) == 0 {
		t.Fatalf("expected leader counts")
	}
	first := req.LeaderCounts[0]
	if first.CharaID != 2 {
		t.Fatalf("unexpected first character: %+v", first)
	}
	if first.ExLevel != 26 || first.ExCount != 21862 {
		t.Fatalf("unexpected merged ex progress: %+v", first)
	}
}

func TestBuildAreaItemUpgradeMaterialsRequestFromSnapshotHidesUnreleasedFutureLevels(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 100,
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
				{"areaItemId": 301, "level": 1},
			}},
		},
		"userMaterials": []map[string]any{
			{"materialId": 401, "quantity": 50},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.CN)
	controller.RegisterSource(&testSource{
		region: renderregion.CN,
		boxes: map[string]map[int]*ResourceBox{
			"shop_item": {
				41: {ID: 41, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 301, ResourceLevel: 2}}},
				42: {ID: 42, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 301, ResourceLevel: 3}}},
				43: {ID: 43, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 301, ResourceLevel: 4}}},
			},
		},
		areaItems: map[int]*AreaItem{
			301: {ID: 301, AssetbundleName: "item_301"},
		},
		areaLevels: map[int]map[int]*AreaItemLevel{
			301: {
				1: {AreaItemID: 301, Level: 1, TargetUnit: "street", Power1BonusRate: 1.0},
				2: {AreaItemID: 301, Level: 2, TargetUnit: "street", Power1BonusRate: 2.0},
				3: {AreaItemID: 301, Level: 3, TargetUnit: "street", Power1BonusRate: 3.0},
				4: {AreaItemID: 301, Level: 4, TargetUnit: "street", Power1BonusRate: 4.0},
			},
		},
		shopItems: map[int]*ShopItem{
			41: {ID: 30041, ResourceBoxID: 41, StartAt: 50, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 401, Quantity: 1}}},
			42: {ID: 30042, ResourceBoxID: 42, StartAt: 200, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 401, Quantity: 1}}},
			43: {ID: 30043, ResourceBoxID: 43, StartAt: 300, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 401, Quantity: 1}}},
		},
	})

	req, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.CN})
	if err != nil {
		t.Fatalf("BuildAreaItemUpgradeMaterialsRequestFromSnapshot() error = %v", err)
	}
	if len(req.AreaItems) != 1 {
		t.Fatalf("expected 1 area item, got %d", len(req.AreaItems))
	}
	if got := req.AreaItems[0]; got.ItemID != 301 || len(got.Levels) != 1 {
		t.Fatalf("unexpected area item payload: %+v", got)
	}
	if got := req.AreaItems[0].Levels[0]; got.Level != 2 || !got.CanUpgrade {
		t.Fatalf("unexpected released level payload: %+v", got)
	}
}

func TestBuildAreaItemUpgradeMaterialsRequestFromSnapshotCapsCurrentLevelToReleasedLevels(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 100,
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
				{"areaItemId": 401, "level": 4},
				{"areaItemId": 402, "level": 1},
			}},
		},
		"userMaterials": []map[string]any{
			{"materialId": 501, "quantity": 50},
		},
	})

	controller := NewController(nil, nil, snapshot, renderregion.CN)
	controller.RegisterSource(&testSource{
		region: renderregion.CN,
		boxes: map[string]map[int]*ResourceBox{
			"shop_item": {
				51: {ID: 51, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 401, ResourceLevel: 2}}},
				52: {ID: 52, Details: []ResourceBoxDetail{{ResourceType: "area_item", ResourceID: 402, ResourceLevel: 2}}},
			},
		},
		areaItems: map[int]*AreaItem{
			401: {ID: 401, AssetbundleName: "item_401"},
			402: {ID: 402, AssetbundleName: "item_402"},
		},
		areaLevels: map[int]map[int]*AreaItemLevel{
			401: {
				1: {AreaItemID: 401, Level: 1, TargetUnit: "street", Power1BonusRate: 1.0},
				2: {AreaItemID: 401, Level: 2, TargetUnit: "street", Power1BonusRate: 2.0},
				3: {AreaItemID: 401, Level: 3, TargetUnit: "street", Power1BonusRate: 3.0},
				4: {AreaItemID: 401, Level: 4, TargetUnit: "street", Power1BonusRate: 4.0},
			},
			402: {
				1: {AreaItemID: 402, Level: 1, TargetUnit: "idol", Power1BonusRate: 1.0},
				2: {AreaItemID: 402, Level: 2, TargetUnit: "idol", Power1BonusRate: 2.0},
			},
		},
		shopItems: map[int]*ShopItem{
			51: {ID: 40051, ResourceBoxID: 51, StartAt: 50, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 501, Quantity: 1}}},
			52: {ID: 40052, ResourceBoxID: 52, StartAt: 50, Costs: []ShopItemCost{{ResourceType: "material", ResourceID: 501, Quantity: 1}}},
		},
	})

	req, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.CN})
	if err != nil {
		t.Fatalf("BuildAreaItemUpgradeMaterialsRequestFromSnapshot() error = %v", err)
	}
	if len(req.AreaItems) != 2 {
		t.Fatalf("expected 2 area items, got %d", len(req.AreaItems))
	}
	if got := req.AreaItems[0]; got.ItemID != 401 || got.CurrentLevel != 2 || len(got.Levels) != 1 {
		t.Fatalf("unexpected capped area item payload: %+v", got)
	}
	if got := req.AreaItems[0].Levels[0]; got.Level != 2 || len(got.Materials) != 0 {
		t.Fatalf("unexpected capped current level payload: %+v", got)
	}
}

func TestBuildAreaItemUpgradeMaterialsRequestFromSnapshotFallsBackToShopSequenceWhenResourceBoxDetailsMissing(t *testing.T) {
	snapshot := mustSnapshot(t, map[string]any{
		"now": 100,
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
				{"areaItemId": 501, "level": 1},
			}},
		},
		"userMaterials": []map[string]any{
			{"materialId": 601, "quantity": 999},
		},
	})

	levels := make(map[int]*AreaItemLevel, 20)
	shopItems := make(map[int]*ShopItem, 15)
	for level := 1; level <= 20; level++ {
		levels[level] = &AreaItemLevel{
			AreaItemID:      501,
			Level:           level,
			TargetUnit:      "street",
			Power1BonusRate: float64(level),
		}
		if level > 15 {
			continue
		}
		resourceBoxID := 6000 + level
		shopItems[resourceBoxID] = &ShopItem{
			ID:            7000 + level,
			ShopID:        5,
			Seq:           level,
			ResourceBoxID: resourceBoxID,
			StartAt:       50,
			Costs: []ShopItemCost{{
				ResourceType: "material",
				ResourceID:   601,
				Quantity:     1,
			}},
		}
	}

	controller := NewController(nil, nil, snapshot, renderregion.CN)
	controller.RegisterSource(&testSource{
		region: renderregion.CN,
		boxes: map[string]map[int]*ResourceBox{
			"shop_item": {},
		},
		areaItems: map[int]*AreaItem{
			501: {ID: 501, AreaID: 5, AssetbundleName: "item_501"},
		},
		areaLevels: map[int]map[int]*AreaItemLevel{
			501: levels,
		},
		shopItems: shopItems,
	})

	req, err := controller.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(AreaItemQuery{Region: renderregion.CN})
	if err != nil {
		t.Fatalf("BuildAreaItemUpgradeMaterialsRequestFromSnapshot() error = %v", err)
	}
	if len(req.AreaItems) != 1 {
		t.Fatalf("expected 1 area item, got %d", len(req.AreaItems))
	}
	if got := req.AreaItems[0]; got.ItemID != 501 || got.CurrentLevel != 1 || len(got.Levels) != 14 {
		t.Fatalf("unexpected area item payload: %+v", got)
	}
	if got := req.AreaItems[0].Levels[0]; got.Level != 2 || !got.CanUpgrade {
		t.Fatalf("unexpected first fallback level payload: %+v", got)
	}
	if got := req.AreaItems[0].Levels[len(req.AreaItems[0].Levels)-1]; got.Level != 15 || !got.CanUpgrade {
		t.Fatalf("unexpected last fallback level payload: %+v", got)
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
