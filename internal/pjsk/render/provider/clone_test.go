package provider

import "testing"

// ── cloneEdChallengeRewards ─────────────────────────────────────────────

func TestCloneEdChallengeRewardsNil(t *testing.T) {
	if got := cloneEdChallengeRewards(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestCloneEdChallengeRewardsEmpty(t *testing.T) {
	if got := cloneEdChallengeRewards([]*ChallengeReward{}); got != nil {
		t.Fatal("expected nil for empty slice")
	}
}

func TestCloneEdChallengeRewardsDeepCopy(t *testing.T) {
	orig := []*ChallengeReward{
		{ID: 1, CharacterID: 21, HighScore: 1000, ResourceBoxID: 5},
		{ID: 2, CharacterID: 22, HighScore: 2000, ResourceBoxID: 6},
	}
	clone := cloneEdChallengeRewards(orig)
	if len(clone) != 2 {
		t.Fatalf("expected 2, got %d", len(clone))
	}
	clone[0].HighScore = 9999
	if orig[0].HighScore != 1000 {
		t.Fatal("mutation leaked to original")
	}
	if clone[0] == orig[0] {
		t.Fatal("pointers must differ")
	}
}

func TestCloneEdChallengeRewardsSkipsNils(t *testing.T) {
	orig := []*ChallengeReward{nil, {ID: 3}}
	clone := cloneEdChallengeRewards(orig)
	if len(clone) != 1 || clone[0].ID != 3 {
		t.Fatal("nil entries should be skipped")
	}
}

// ── cloneEdResourceBox ──────────────────────────────────────────────────

func TestCloneEdResourceBoxNil(t *testing.T) {
	if got := cloneEdResourceBox(nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestCloneEdResourceBoxDeepCopy(t *testing.T) {
	orig := &ResourceBox{
		ID:                 10,
		ResourceBoxPurpose: "challenge_live_high_score",
		Details: []ResourceBoxDetail{
			{ResourceType: "jewel", ResourceQuantity: 100},
			{ResourceType: "coin", ResourceQuantity: 500},
		},
	}
	clone := cloneEdResourceBox(orig)
	if clone == orig {
		t.Fatal("must be different pointer")
	}
	clone.Details[0].ResourceQuantity = 9999
	if orig.Details[0].ResourceQuantity != 100 {
		t.Fatal("Details slice is not independent")
	}
	clone.Details = append(clone.Details, ResourceBoxDetail{ResourceType: "extra"})
	if len(orig.Details) != 2 {
		t.Fatal("append leaked to original")
	}
}

func TestCloneEdResourceBoxEmptyDetails(t *testing.T) {
	orig := &ResourceBox{ID: 5}
	clone := cloneEdResourceBox(orig)
	if len(clone.Details) != 0 {
		t.Fatal("expected empty Details")
	}
}

// ── cloneEdAreaItem ─────────────────────────────────────────────────────

func TestCloneEdAreaItemNil(t *testing.T) {
	if got := cloneEdAreaItem(nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestCloneEdAreaItemDeepCopy(t *testing.T) {
	orig := &AreaItem{ID: 1, Name: "item1", AreaID: 10}
	clone := cloneEdAreaItem(orig)
	if clone == orig {
		t.Fatal("must be different pointer")
	}
	clone.Name = "changed"
	if orig.Name != "item1" {
		t.Fatal("mutation leaked")
	}
}

// ── cloneEdAreaItemLevels ───────────────────────────────────────────────

func TestCloneEdAreaItemLevelsNil(t *testing.T) {
	if got := cloneEdAreaItemLevels(nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestCloneEdAreaItemLevelsEmpty(t *testing.T) {
	if got := cloneEdAreaItemLevels([]*AreaItemLevel{}); got != nil {
		t.Fatal("expected nil for empty slice")
	}
}

func TestCloneEdAreaItemLevelsDeepCopy(t *testing.T) {
	orig := []*AreaItemLevel{
		{AreaItemID: 1, Level: 1, Power1BonusRate: 1.5},
		{AreaItemID: 1, Level: 2, Power1BonusRate: 2.0},
	}
	clone := cloneEdAreaItemLevels(orig)
	if len(clone) != 2 {
		t.Fatalf("expected 2, got %d", len(clone))
	}
	clone[0].Power1BonusRate = 99.9
	if orig[0].Power1BonusRate != 1.5 {
		t.Fatal("mutation leaked")
	}
}

func TestCloneEdAreaItemLevelsSkipsNils(t *testing.T) {
	orig := []*AreaItemLevel{nil, {AreaItemID: 2, Level: 1}}
	clone := cloneEdAreaItemLevels(orig)
	if len(clone) != 1 || clone[0].AreaItemID != 2 {
		t.Fatal("nil entries should be skipped")
	}
}

// ── cloneEdAreaItemLevel ────────────────────────────────────────────────

func TestCloneEdAreaItemLevelNil(t *testing.T) {
	if got := cloneEdAreaItemLevel(nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestCloneEdAreaItemLevelDeepCopy(t *testing.T) {
	orig := &AreaItemLevel{AreaItemID: 5, Level: 3, TargetUnit: "idol"}
	clone := cloneEdAreaItemLevel(orig)
	if clone == orig {
		t.Fatal("must be different pointer")
	}
	clone.TargetUnit = "street"
	if orig.TargetUnit != "idol" {
		t.Fatal("mutation leaked")
	}
}

// ── cloneEdCharacterRank ────────────────────────────────────────────────

func TestCloneEdCharacterRankNil(t *testing.T) {
	if got := cloneEdCharacterRank(nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestCloneEdCharacterRankDeepCopy(t *testing.T) {
	orig := &CharacterRank{CharacterID: 1, Rank: 50, Power1BonusRate: 3.0}
	clone := cloneEdCharacterRank(orig)
	if clone == orig {
		t.Fatal("must be different pointer")
	}
	clone.Power1BonusRate = 99.0
	if orig.Power1BonusRate != 3.0 {
		t.Fatal("mutation leaked")
	}
}

// ── cloneEdMysekaiGateLevel ─────────────────────────────────────────────

func TestCloneEdMysekaiGateLevelNil(t *testing.T) {
	if got := cloneEdMysekaiGateLevel(nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestCloneEdMysekaiGateLevelDeepCopy(t *testing.T) {
	orig := &MysekaiGateLevel{GateID: 3, Level: 10, PowerBonusRate: 5.5}
	clone := cloneEdMysekaiGateLevel(orig)
	if clone == orig {
		t.Fatal("must be different pointer")
	}
	clone.PowerBonusRate = 0.0
	if orig.PowerBonusRate != 5.5 {
		t.Fatal("mutation leaked")
	}
}

// ── cloneEdShopItem ─────────────────────────────────────────────────────

func TestCloneEdShopItemNil(t *testing.T) {
	if got := cloneEdShopItem(nil); got != nil {
		t.Fatal("expected nil")
	}
}

func TestCloneEdShopItemDeepCopy(t *testing.T) {
	orig := &ShopItem{
		ID:            1,
		ResourceBoxID: 10,
		Costs: []ShopItemCost{
			{ResourceType: "jewel", ResourceID: 1, Quantity: 300},
			{ResourceType: "coin", ResourceID: 2, Quantity: 1000},
		},
	}
	clone := cloneEdShopItem(orig)
	if clone == orig {
		t.Fatal("must be different pointer")
	}
	clone.Costs[0].Quantity = 9999
	if orig.Costs[0].Quantity != 300 {
		t.Fatal("Costs slice is not independent")
	}
	clone.Costs = append(clone.Costs, ShopItemCost{})
	if len(orig.Costs) != 2 {
		t.Fatal("append leaked to original")
	}
}

func TestCloneEdShopItemEmptyCosts(t *testing.T) {
	orig := &ShopItem{ID: 7}
	clone := cloneEdShopItem(orig)
	if len(clone.Costs) != 0 {
		t.Fatal("expected empty Costs")
	}
}
