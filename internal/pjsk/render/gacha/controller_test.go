package gacha

import (
	"context"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type gachaContextKey string

type contextAwareGachaSource struct {
	*testGachaSource
	ctx       context.Context
	wantKey   gachaContextKey
	wantValue string
}

func (s *contextAwareGachaSource) WithContext(ctx context.Context) DataSource {
	clone := *s
	clone.ctx = ctx
	return &clone
}

func (s *contextAwareGachaSource) GetGachas() []*masterdata.Gacha {
	if s.ctx == nil {
		return nil
	}
	value, _ := s.ctx.Value(s.wantKey).(string)
	if value != s.wantValue {
		return nil
	}
	return s.testGachaSource.GetGachas()
}

func TestControllerBuildGachaListRequestUsesRequestedRegionSource(t *testing.T) {
	cn := newTestGachaSource(renderregion.CN)
	cnItem := &masterdata.Gacha{ID: 1, Name: "CN_NAME", GachaType: "ceil", AssetBundleName: "cn_gacha", StartAt: 100, EndAt: 200}
	cn.gachas = []*masterdata.Gacha{cnItem}
	cn.gachaByID[cnItem.ID] = cnItem

	jp := newTestGachaSource(renderregion.JP)
	jpItem := &masterdata.Gacha{ID: 1, Name: "JP_NAME", GachaType: "ceil", AssetBundleName: "jp_gacha", StartAt: 100, EndAt: 200}
	jp.gachas = []*masterdata.Gacha{jpItem}
	jp.gachaByID[jpItem.ID] = jpItem

	controller := NewController(cn, nil, assets.NewAssetHelper("", nil))
	controller.RegisterSource(jp)

	req, err := controller.BuildGachaListRequest(ListQuery{
		Region:      renderregion.JP,
		IncludePast: true,
	})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	if len(req.Gachas) != 1 {
		t.Fatalf("expected 1 gacha, got %d", len(req.Gachas))
	}
	if req.Gachas[0].Name != "JP_NAME" {
		t.Fatalf("expected JP source gacha name, got %q", req.Gachas[0].Name)
	}
}

func TestControllerBuildGachaDetailRequestUsesNegativeIndex(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)

	older := &masterdata.Gacha{ID: 10, Name: "Older", GachaType: "ceil", AssetBundleName: "older", StartAt: now - 20_000, EndAt: now - 10_000}
	latest := &masterdata.Gacha{
		ID:              20,
		Name:            "Latest",
		GachaType:       "ceil",
		AssetBundleName: "latest",
		StartAt:         now - 5_000,
		EndAt:           now + 5_000,
		GachaDetails: []masterdata.GachaDetail{
			{CardID: 1001, Weight: 100},
		},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 100},
		},
	}
	source.gachas = []*masterdata.Gacha{latest, older}
	source.gachaByID[older.ID] = older
	source.gachaByID[latest.ID] = latest
	source.cardByID[1001] = &masterdata.Card{ID: 1001, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_1001"}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildGachaDetailRequest(DetailQuery{
		Region:   renderregion.JP,
		NegIndex: -1,
	})
	if err != nil {
		t.Fatalf("BuildGachaDetailRequest failed: %v", err)
	}
	if req.Gacha.ID != 20 {
		t.Fatalf("expected latest gacha id 20, got %d", req.Gacha.ID)
	}
}

func TestControllerBuildGachaDetailRequestNegativeIndexIgnoresFutureGacha(t *testing.T) {
	now := time.Now().UnixMilli()
	source := newTestGachaSource(renderregion.JP)

	past := &masterdata.Gacha{
		ID:              10,
		Name:            "Past",
		GachaType:       "ceil",
		AssetBundleName: "past",
		StartAt:         now - 20_000,
		EndAt:           now - 10_000,
		GachaDetails: []masterdata.GachaDetail{
			{CardID: 1001, Weight: 100},
		},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 100},
		},
	}
	current := &masterdata.Gacha{
		ID:              20,
		Name:            "Current",
		GachaType:       "ceil",
		AssetBundleName: "current",
		StartAt:         now - 5_000,
		EndAt:           now + 5_000,
		GachaDetails: []masterdata.GachaDetail{
			{CardID: 1002, Weight: 100},
		},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 100},
		},
	}
	future := &masterdata.Gacha{
		ID:              30,
		Name:            "Future",
		GachaType:       "ceil",
		AssetBundleName: "future",
		StartAt:         now + 20_000,
		EndAt:           now + 30_000,
		GachaDetails: []masterdata.GachaDetail{
			{CardID: 1003, Weight: 100},
		},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 100},
		},
	}
	source.gachas = []*masterdata.Gacha{past, future, current}
	source.gachaByID[past.ID] = past
	source.gachaByID[current.ID] = current
	source.gachaByID[future.ID] = future
	source.cardByID[1001] = &masterdata.Card{ID: 1001, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_1001"}
	source.cardByID[1002] = &masterdata.Card{ID: 1002, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_1002"}
	source.cardByID[1003] = &masterdata.Card{ID: 1003, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_1003"}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildGachaDetailRequest(DetailQuery{
		Region:   renderregion.JP,
		NegIndex: -1,
	})
	if err != nil {
		t.Fatalf("BuildGachaDetailRequest failed: %v", err)
	}
	if req.Gacha.ID != 20 {
		t.Fatalf("expected latest started gacha id 20, got %d", req.Gacha.ID)
	}
}

func TestControllerBuildGachaDetailRequestUsesEventID(t *testing.T) {
	source := newTestGachaSource(renderregion.JP)

	eventID := 123
	source.eventCards[eventID] = []int{1001, 1002, 1003}

	gachaInfo := &masterdata.Gacha{
		ID:              30,
		Name:            "Event Gacha",
		GachaType:       "ceil",
		AssetBundleName: "event_gacha",
		StartAt:         100,
		EndAt:           200,
		GachaDetails: []masterdata.GachaDetail{
			{CardID: 1003, Weight: 100},
		},
		GachaPickups: []masterdata.GachaPickup{
			{CardID: 1003},
		},
		GachaCardRarityRates: []masterdata.GachaCardRarityRate{
			{CardRarityType: "rarity_4", LotteryType: "normal", Rate: 100},
		},
	}
	source.gachas = []*masterdata.Gacha{gachaInfo}
	source.gachaByID[gachaInfo.ID] = gachaInfo
	source.cardByID[1003] = &masterdata.Card{ID: 1003, CardRarityType: "rarity_4", Attr: "cool", AssetBundleName: "card_1003"}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	req, err := controller.BuildGachaDetailRequest(DetailQuery{
		Region:  renderregion.JP,
		EventID: eventID,
	})
	if err != nil {
		t.Fatalf("BuildGachaDetailRequest failed: %v", err)
	}
	if req.Gacha.ID != 30 {
		t.Fatalf("expected event gacha id 30, got %d", req.Gacha.ID)
	}
}

func TestControllerWithContextClonesGachaSource(t *testing.T) {
	source := &contextAwareGachaSource{
		testGachaSource: newTestGachaSource(renderregion.JP),
		wantKey:         gachaContextKey("trace"),
		wantValue:       "gacha-list",
	}
	gachaInfo := &masterdata.Gacha{ID: 1, Name: "Ctx Gacha", GachaType: "ceil", AssetBundleName: "ctx_gacha", StartAt: 100, EndAt: 200}
	source.gachas = []*masterdata.Gacha{gachaInfo}
	source.gachaByID[gachaInfo.ID] = gachaInfo

	controller := NewController(source, nil, assets.NewAssetHelper("", nil))
	ctx := context.WithValue(context.Background(), gachaContextKey("trace"), "gacha-list")

	req, err := controller.WithContext(ctx).BuildGachaListRequest(ListQuery{
		Region:      renderregion.JP,
		IncludePast: true,
	})
	if err != nil {
		t.Fatalf("BuildGachaListRequest failed: %v", err)
	}
	if len(req.Gachas) != 1 || req.Gachas[0].Name != "Ctx Gacha" {
		t.Fatalf("unexpected gacha list payload: %+v", req.Gachas)
	}
}
