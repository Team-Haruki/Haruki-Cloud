package gacha

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type testGachaSource struct {
	region    renderregion.Value
	gachas    []*masterdata.Gacha
	gachaByID map[int]*masterdata.Gacha
	cardByID  map[int]*masterdata.Card
	eventCards map[int][]int
}

func newTestGachaSource(region renderregion.Value) *testGachaSource {
	return &testGachaSource{
		region:    region,
		gachaByID: make(map[int]*masterdata.Gacha),
		cardByID:  make(map[int]*masterdata.Card),
		eventCards: make(map[int][]int),
	}
}

func (s *testGachaSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testGachaSource) GetGachaByID(id int) (*masterdata.Gacha, error) {
	if item, ok := s.gachaByID[id]; ok {
		copy := *item
		return &copy, nil
	}
	return nil, fmt.Errorf("gacha not found: %d", id)
}

func (s *testGachaSource) GetGachas() []*masterdata.Gacha {
	result := make([]*masterdata.Gacha, 0, len(s.gachas))
	for _, item := range s.gachas {
		copy := *item
		result = append(result, &copy)
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
			copy := *item
			return &copy, nil
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
		copy := *item
		return &copy, nil
	}
	return nil, fmt.Errorf("card not found: %d", id)
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

func TestBuildGachaDetailRequestComputesRatesAndPickupCards(t *testing.T) {
	source := newTestGachaSource(renderregion.JP)

	gachaInfo := &masterdata.Gacha{
		ID:              100,
		Name:            "Test Gacha",
		GachaType:       "ceil",
		AssetBundleName: "banner_gacha100",
		StartAt:         1000,
		EndAt:           2000,
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
	if want := filepath.ToSlash(logoPath); got != want {
		t.Fatalf("expected logo path %q, got %q", want, got)
	}
}
