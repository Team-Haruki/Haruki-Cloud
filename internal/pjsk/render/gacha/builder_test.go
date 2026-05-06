package gacha

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type testGachaSource struct {
	region     renderregion.Value
	gachas     []*masterdata.Gacha
	gachaByID  map[int]*masterdata.Gacha
	cardByID   map[int]*masterdata.Card
	ceilByID   map[int]string
	eventCards map[int][]int
}

func newTestGachaSource(region renderregion.Value) *testGachaSource {
	return &testGachaSource{
		region:     region,
		gachaByID:  make(map[int]*masterdata.Gacha),
		cardByID:   make(map[int]*masterdata.Card),
		ceilByID:   make(map[int]string),
		eventCards: make(map[int][]int),
	}
}

func (s *testGachaSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testGachaSource) GetGachaByID(id int) (*masterdata.Gacha, error) {
	if item, ok := s.gachaByID[id]; ok {
		return new(*item), nil
	}
	return nil, fmt.Errorf("gacha not found: %d", id)
}

func (s *testGachaSource) GetGachas() []*masterdata.Gacha {
	result := make([]*masterdata.Gacha, 0, len(s.gachas))
	for _, item := range s.gachas {
		result = append(result, new(*item))
	}
	return result
}

func (s *testGachaSource) GetGachaByEventID(eventID int) (*masterdata.Gacha, error) {
	cardIDs, ok := s.eventCards[eventID]
	if !ok || len(cardIDs) == 0 {
		return nil, fmt.Errorf("gacha not found for event: %d", eventID)
	}
	sorted := append([]int(nil), cardIDs...)
	sort.Ints(sorted)
	idx := len(sorted) - 1
	if idx > 2 {
		idx = 2
	}
	targetCardID := sorted[idx]
	for _, item := range s.gachas {
		if testGachaContainsPickup(item, targetCardID) {
			return new(*item), nil
		}
	}
	return nil, fmt.Errorf("gacha not found for event: %d", eventID)
}

func testGachaContainsPickup(gachaInfo *masterdata.Gacha, cardID int) bool {
	for _, pickup := range gachaInfo.GachaPickups {
		if pickup.CardID == cardID {
			return true
		}
	}
	return false
}

func (s *testGachaSource) GetCardByID(id int) (*masterdata.Card, error) {
	if item, ok := s.cardByID[id]; ok {
		return new(*item), nil
	}
	return nil, fmt.Errorf("card not found: %d", id)
}

func (s *testGachaSource) GetGachaCeilItemAssetbundleName(id int) (string, error) {
	if item, ok := s.ceilByID[id]; ok {
		return item, nil
	}
	return "", fmt.Errorf("gacha ceil item not found: %d", id)
}

func TestBuildGachaListRequestOnlyCurrentFiltersAndOrdersByNewest(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)

	current := &masterdata.Gacha{ID: 2, Name: "Current", GachaType: "ceil", AssetBundleName: "gacha_2", StartAt: now - 10_000, EndAt: now + 10_000}
	past := &masterdata.Gacha{ID: 1, Name: "Past", GachaType: "ceil", AssetBundleName: "gacha_1", StartAt: now - 30_000, EndAt: now - 20_000}
	future := &masterdata.Gacha{ID: 3, Name: "Future", GachaType: "ceil", AssetBundleName: "gacha_3", StartAt: now + 20_000, EndAt: now + 30_000}
	source.gachas = []*masterdata.Gacha{past, current, future}
	source.gachaByID[past.ID] = past
	source.gachaByID[current.ID] = current
	source.gachaByID[future.ID] = future

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildGachaListRequest(ListQuery{Region: renderregion.JP, OnlyCurrent: true})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	if len(req.Gachas) != 1 {
		t.Fatalf("expected 1 gacha, got %d", len(req.Gachas))
	}
	if req.Gachas[0].Name != "Current" {
		t.Fatalf("unexpected gacha name: %s", req.Gachas[0].Name)
	}
}

func TestBuildGachaListRequestSlicesAndDefaultsToLatestPage(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)

	for i := 0; i < 25; i++ {
		item := &masterdata.Gacha{
			ID:              i + 1,
			Name:            fmt.Sprintf("Gacha-%02d", i+1),
			GachaType:       "ceil",
			AssetBundleName: fmt.Sprintf("gacha_%02d", i+1),
			StartAt:         now - int64((25-i)*1000),
			EndAt:           now + 60_000,
		}
		source.gachas = append(source.gachas, item)
		source.gachaByID[item.ID] = item
	}
	future := &masterdata.Gacha{
		ID:              999,
		Name:            "Future",
		GachaType:       "ceil",
		AssetBundleName: "future",
		StartAt:         now + 60_000,
		EndAt:           now + 120_000,
	}
	source.gachas = append(source.gachas, future)
	source.gachaByID[future.ID] = future

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildGachaListRequest(ListQuery{Region: renderregion.JP, IncludePast: true, PageSize: 10})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	if len(req.Gachas) != 5 {
		t.Fatalf("expected latest page to contain 5 gachas, got %d", len(req.Gachas))
	}
	if req.CurrentPage != 3 || req.TotalPage != 3 {
		t.Fatalf("expected current/total page to be 3/3, got %d/%d", req.CurrentPage, req.TotalPage)
	}
	if !req.PrePaginated {
		t.Fatalf("expected request to be marked pre_paginated")
	}
	if req.Filter.Page != 3 {
		t.Fatalf("expected filter page to match current page, got %d", req.Filter.Page)
	}
	if req.PageSize != 10 {
		t.Fatalf("expected page size 10, got %d", req.PageSize)
	}
	if req.Gachas[0].ID != 21 || req.Gachas[len(req.Gachas)-1].ID != 25 {
		t.Fatalf("expected ascending gacha order, got first=%d last=%d", req.Gachas[0].ID, req.Gachas[len(req.Gachas)-1].ID)
	}
}

func TestBuildGachaListRequestFiltersYearByStartAtAndExcludesFutureByDefault(t *testing.T) {
	source := newTestGachaSource(renderregion.JP)
	started2025 := &masterdata.Gacha{ID: 1, Name: "Started2025", GachaType: "ceil", AssetBundleName: "g1", StartAt: time.Date(2025, 12, 15, 12, 0, 0, 0, time.Local).UnixMilli(), EndAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local).UnixMilli()}
	ended2025 := &masterdata.Gacha{ID: 2, Name: "Ended2025", GachaType: "ceil", AssetBundleName: "g2", StartAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.Local).UnixMilli(), EndAt: time.Date(2025, 1, 10, 12, 0, 0, 0, time.Local).UnixMilli()}
	started2026 := &masterdata.Gacha{ID: 4, Name: "Started2026", GachaType: "ceil", AssetBundleName: "g4", StartAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local).UnixMilli(), EndAt: time.Date(2026, 1, 10, 12, 0, 0, 0, time.Local).UnixMilli()}
	source.gachas = []*masterdata.Gacha{started2025, ended2025, started2026}
	for _, item := range source.gachas {
		source.gachaByID[item.ID] = item
	}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildGachaListRequest(ListQuery{Region: renderregion.JP, IncludePast: true, Year: 2025})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	if len(req.Gachas) != 2 {
		t.Fatalf("expected only started 2025 gachas, got %d", len(req.Gachas))
	}
	if req.Gachas[0].ID != 2 || req.Gachas[1].ID != 1 {
		t.Fatalf("unexpected year-filtered gacha order: %+v", req.Gachas)
	}
}

func TestBuildGachaListRequestRereleaseAndRecallUsePrefix(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)
	rerelease := &masterdata.Gacha{ID: 1, Name: "[复刻] Rerelease", GachaType: "ceil", AssetBundleName: "g1", StartAt: now - 10_000, EndAt: now + 10_000}
	rereleaseMiddle := &masterdata.Gacha{ID: 2, Name: "Not [复刻] Prefix", GachaType: "ceil", AssetBundleName: "g2", StartAt: now - 10_000, EndAt: now + 10_000}
	recall := &masterdata.Gacha{ID: 3, Name: "[回响] Recall", GachaType: "ceil", AssetBundleName: "g3", StartAt: now - 10_000, EndAt: now + 10_000}
	source.gachas = []*masterdata.Gacha{rerelease, rereleaseMiddle, recall}
	for _, item := range source.gachas {
		source.gachaByID[item.ID] = item
	}

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildGachaListRequest(ListQuery{Region: renderregion.JP, IncludePast: true, IsRerelease: true})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	if len(req.Gachas) != 1 || req.Gachas[0].ID != 1 {
		t.Fatalf("expected only prefix rerelease gacha, got %+v", req.Gachas)
	}

	req, err = builder.BuildGachaListRequest(ListQuery{Region: renderregion.JP, IncludePast: true, IsRecall: true})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	if len(req.Gachas) != 1 || req.Gachas[0].ID != 3 {
		t.Fatalf("expected only recall prefix gacha, got %+v", req.Gachas)
	}
}

func TestBuildGachaDetailRequestComputesRatesAndPickupCards(t *testing.T) {
	source := newTestGachaSource(renderregion.JP)

	gachaInfo := &masterdata.Gacha{
		ID:              100,
		Name:            "Test Gacha",
		GachaType:       "ceil",
		AssetBundleName: "banner_gacha100",
		StartAt:         1000,
		EndAt:           2000,
		GachaCeilItemID: new(88),
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_2", LotteryType: "normal", Rate: 87},
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 13},
		},
		GachaDetails: []masterdata.GachaDetail{
			{CardID: 2001, Weight: 40},
			{CardID: 2002, Weight: 60},
		},
		GachaPickups: []masterdata.GachaPickup{
			{CardID: 2001},
		},
		GachaBehaviors: []masterdata.GachaBehavior{
			{GachaBehaviorType: "over_rarity_4_once", SpinCount: 10, CostResourceType: "jewel", CostResourceQuantity: 3000},
			{GachaBehaviorType: "normal", SpinCount: 1, CostResourceType: "gacha_ticket", CostResourceQuantity: 1},
		},
		GachaInformation: masterdata.GachaInformation{
			Summary:     "summary",
			Description: "desc",
		},
	}
	source.gachas = []*masterdata.Gacha{gachaInfo}
	source.gachaByID[gachaInfo.ID] = gachaInfo
	source.cardByID[2001] = &masterdata.Card{ID: 2001, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_2001"}
	source.cardByID[2002] = &masterdata.Card{ID: 2002, CardRarityType: "rarity_2", Attr: "cute", AssetBundleName: "card_2002"}
	source.ceilByID[88] = "ceil_item_limited"

	builder := NewBuilder(source, assets.NewAssetHelper("", nil))
	req, err := builder.BuildGachaDetailRequest(DetailQuery{Region: renderregion.JP, GachaID: 100})
	if err != nil {
		t.Fatalf("BuildGachaDetailRequest failed: %v", err)
	}
	if req.Gacha.PickupCount != 1 {
		t.Fatalf("expected pickup count 1, got %d", req.Gacha.PickupCount)
	}
	if req.Gacha.Rarity4Count != 1 || req.Gacha.Rarity2Count != 1 {
		t.Fatalf("unexpected rarity counts: %+v", req.Gacha)
	}
	if req.WeightInfo.Rarity4Rate == nil || *req.WeightInfo.Rarity4Rate != 0.13 {
		t.Fatalf("unexpected rarity_4_rate: %#v", req.WeightInfo.Rarity4Rate)
	}
	if req.WeightInfo.GuaranteedRates["rarity_4"] != 1 {
		t.Fatalf("unexpected guaranteed rates: %+v", req.WeightInfo.GuaranteedRates)
	}
	if len(req.PickupCards) != 1 {
		t.Fatalf("expected 1 pickup card, got %d", len(req.PickupCards))
	}
	if req.PickupCards[0].ID != 2001 {
		t.Fatalf("unexpected pickup card id: %d", req.PickupCards[0].ID)
	}
	if len(req.Gacha.Behaviors) != 2 {
		t.Fatalf("expected 2 gacha behaviors, got %d", len(req.Gacha.Behaviors))
	}
	if req.Gacha.Behaviors[0].CostIconPath == nil || *req.Gacha.Behaviors[0].CostIconPath != "static_images/jewel.png" {
		t.Fatalf("unexpected jewel cost icon path: %+v", req.Gacha.Behaviors[0].CostIconPath)
	}
	if req.Gacha.Behaviors[1].CostIconPath == nil || *req.Gacha.Behaviors[1].CostIconPath != "asset/jp-assets/startapp/thumbnail/gacha_ticket/gacha_ticket.png" {
		t.Fatalf("unexpected ticket cost icon path: %+v", req.Gacha.Behaviors[1].CostIconPath)
	}
	if req.Gacha.CeilItemImgPath == nil || *req.Gacha.CeilItemImgPath != "asset/jp-assets/startapp/thumbnail/gacha_item/ceil_item_limited.png" {
		t.Fatalf("unexpected ceil item icon path: %+v", req.Gacha.CeilItemImgPath)
	}
}

func TestBuildGachaListRequestUsesOnDemandLogoFallback(t *testing.T) {
	dir := t.TempDir()
	logoPath := filepath.Join(dir, "asset", "jp-assets", "ondemand", "gacha", "ab_gacha_392", "logo", "logo.png")
	if err := os.MkdirAll(filepath.Dir(logoPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(logoPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)
	item := &masterdata.Gacha{
		ID:              392,
		Name:            "OnDemand Gacha",
		GachaType:       "ceil",
		AssetBundleName: "",
		StartAt:         now - 60_000,
		EndAt:           now + 60_000,
	}
	source.gachas = []*masterdata.Gacha{item}
	source.gachaByID[item.ID] = item

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildGachaListRequest(ListQuery{Region: renderregion.JP, PageSize: 1})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	got := req.GachaLogos[item.ID]
	if want := filepath.ToSlash(filepath.Join("asset", "jp-assets", "ondemand", "gacha", "ab_gacha_392", "logo", "logo.png")); got != want {
		t.Fatalf("expected logo path %q, got %q", want, got)
	}
}

func TestBuildGachaListRequestIncludesBannerFallbackPath(t *testing.T) {
	dir := t.TempDir()
	bannerPath := filepath.Join(
		dir,
		"asset",
		"jp-assets",
		"ondemand",
		"gacha",
		"ab_gacha_401",
		"screen",
		"texture",
		"bg_gacha401.png",
	)
	if err := os.MkdirAll(filepath.Dir(bannerPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(bannerPath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)
	item := &masterdata.Gacha{
		ID:              401,
		Name:            "Banner Fallback Gacha",
		GachaType:       "ceil",
		AssetBundleName: "",
		StartAt:         now - 60_000,
		EndAt:           now + 60_000,
	}
	source.gachas = []*masterdata.Gacha{item}
	source.gachaByID[item.ID] = item

	builder := NewBuilder(source, assets.NewAssetHelper(dir, nil))
	req, err := builder.BuildGachaListRequest(ListQuery{Region: renderregion.JP, PageSize: 1})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	got := req.GachaBanners[item.ID]
	if want := filepath.ToSlash(
		filepath.Join("asset", "jp-assets", "ondemand", "gacha", "ab_gacha_401", "screen", "texture", "bg_gacha401.png"),
	); got != want {
		t.Fatalf("expected banner path %q, got %q", want, got)
	}
}
