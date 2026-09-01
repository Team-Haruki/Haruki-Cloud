package gacha

import (
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestGachaListRefactorHelpers(t *testing.T) {
	page, pageSize := normalizeGachaListPage(-1, 0)
	if page != 0 || pageSize != defaultGachaListPageSize {
		t.Fatalf("normalized page = %d/%d", page, pageSize)
	}
	briefs := []drawing.GachaBrief{{ID: 1}, {ID: 2}, {ID: 3}}
	paged, current, total := paginateGachaList(briefs, 2, 99)
	if current != 2 || total != 2 || len(paged) != 1 || paged[0].ID != 3 {
		t.Fatalf("paginated list = %#v, %d/%d", paged, current, total)
	}
	logos, banners := selectGachaListAssets(paged, map[int]string{3: "logo"}, map[int]string{3: "banner"})
	if logos[3] != "logo" || banners[3] != "banner" {
		t.Fatalf("selected assets = %#v / %#v", logos, banners)
	}
}

func TestGachaMatchesListQueryBranches(t *testing.T) {
	now := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	current := &masterdata.Gacha{
		ID: 1, Name: "[复刻] Current", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(),
		GachaPickups: []masterdata.GachaPickup{{CardID: 10}},
	}
	valid := ListQuery{Year: 2026, IncludeFuture: true, IncludePast: true, CardID: 10, IsRerelease: true, OnlyCurrent: true}
	if !gachaMatchesListQuery(current, valid, "current", now) {
		t.Fatal("matching gacha was rejected")
	}
	if gachaMatchesListQuery(current, valid, "missing", now) {
		t.Fatal("keyword mismatch was accepted")
	}
	if gachaMatchesListQuery(current, ListQuery{IncludeFuture: true, IncludePast: true, CardID: 99}, "", now) {
		t.Fatal("card mismatch was accepted")
	}
	if gachaMatchesListQuery(current, ListQuery{IncludeFuture: true, IncludePast: true, IsRecall: true}, "", now) {
		t.Fatal("recall mismatch was accepted")
	}
}

func TestGachaMatchesTimeWindowBranches(t *testing.T) {
	now := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)
	if gachaMatchesTimeWindow(ListQuery{Year: 2025, IncludePast: true, IncludeFuture: true}, past, future, now) {
		t.Fatal("year mismatch was accepted")
	}
	if gachaMatchesTimeWindow(ListQuery{IncludePast: true}, future, future.Add(time.Hour), now) {
		t.Fatal("future gacha was accepted")
	}
	if gachaMatchesTimeWindow(ListQuery{IncludeFuture: true}, past.Add(-time.Hour), past, now) {
		t.Fatal("past gacha was accepted")
	}
	if gachaMatchesTimeWindow(ListQuery{IncludePast: true, IncludeFuture: true, OnlyCurrent: true}, past.Add(-time.Hour), past, now) {
		t.Fatal("non-current gacha was accepted")
	}
}

func TestGachaDetailRateHelpers(t *testing.T) {
	gachaInfo := &masterdata.Gacha{
		GachaPickups: []masterdata.GachaPickup{{CardID: 1}, {CardID: 1}, {CardID: 2}},
		GachaBehaviors: []masterdata.GachaBehavior{
			{GachaBehaviorType: "over_rarity_3_once"},
			{GachaBehaviorType: "over_rarity_4_once"},
			{GachaBehaviorType: "over_rarity_3_once"},
		},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_2", LotteryType: "normal", Rate: 80},
			{CardRarityType: "rarity_3", LotteryType: "normal", Rate: 15},
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 5},
			{CardRarityType: "rarity_1", LotteryType: "guaranteed", Rate: 100},
		},
	}
	if ids := uniqueGachaPickupIDs(gachaInfo); len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("pickup ids = %#v", ids)
	}
	if got := gachaGuaranteedType(gachaInfo); got != "rarity_4" {
		t.Fatalf("guaranteed type = %q", got)
	}
	state := newGachaDetailCardState()
	weight := buildGachaWeight(gachaInfo, state)
	if weight.Rarity4Rate == nil || *weight.Rarity4Rate != 0.05 || weight.GuaranteedRates["rarity_4"] != 1 {
		t.Fatalf("weight = %+v", weight)
	}
	if got := guaranteedGachaRates(state.rarityRates, ""); len(got) != 0 {
		t.Fatalf("unguaranteed rates = %#v", got)
	}
}

func TestGachaDetailCardStateBranches(t *testing.T) {
	state := newGachaDetailCardState()
	if state.cardRate(1) != 0 {
		t.Fatal("missing card rate was non-zero")
	}
	state.cardRarity[1] = "rarity_4"
	state.rarityWeights["rarity_4"] = 100
	state.rarityRates["rarity_4"] = 0.03
	state.cardWeight[1] = 25
	if got := state.cardRate(1); got != 0.0075 {
		t.Fatalf("card rate = %v", got)
	}
	if nonEmptyGachaPath("") != nil || nonEmptyGachaPath("path") == nil {
		t.Fatal("path pointer helper failed")
	}
	source := newTestGachaSource(renderregion.JP)
	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	if builder.gachaCeilItemPath(&masterdata.Gacha{}, renderregion.JP) != nil {
		t.Fatal("empty ceil item produced a path")
	}
}
