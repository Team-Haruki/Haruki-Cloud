package card

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/utils/drawing"
)

func TestBuildCardListRequestResolvesIDsFromQuery(t *testing.T) {
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"},
			{ID: 1002, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cool", Prefix: "Card B", AssetBundleName: "card_b"},
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{Query: "mnr 4星", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(req.Cards))
	}
	if req.Cards[0].CardID != 1001 || req.Cards[1].CardID != 1002 {
		t.Fatalf("unexpected cards: %+v", req.Cards)
	}
}

func TestBuildCardListRequestResolvesAdvancedFiltersFromQuery(t *testing.T) {
	cardInfo := &masterdata.Card{ID: 1003, CharacterID: 21, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card C", AssetBundleName: "card_c", SupportUnit: "idol"}
	source := &lookupTestSource{
		cards:      []*masterdata.Card{cardInfo},
		characters: map[int]*masterdata.Character{21: {ID: 21, FirstName: "初音", GivenName: "未来", Unit: "piapro"}},
		filterFunc: func(info *CardQueryInfo) ([]*masterdata.Card, error) {
			if info == nil {
				t.Fatal("expected query info")
			}
			if info.EventID != 123 {
				t.Fatalf("unexpected event id: %+v", info)
			}
			if info.MainUnit != "piapro" || info.SupportUnit != "idol" {
				t.Fatalf("unexpected unit filter: %+v", info)
			}
			if info.SupplyType != SupplyFes {
				t.Fatalf("unexpected supply filter: %+v", info)
			}
			if info.Year != 2025 {
				t.Fatalf("unexpected year filter: %+v", info)
			}
			return []*masterdata.Card{cardInfo}, nil
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{Query: "event123 mmjv fes 25年", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 1 || req.Cards[0].CardID != 1003 {
		t.Fatalf("unexpected cards: %+v", req.Cards)
	}
}

func TestBuildCardListRequestIncludesLimitedIconPaths(t *testing.T) {
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"},
		},
		supplyByCard: map[int]string{
			1001: "term_limited",
		},
	}
	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{Query: "1001", Region: "jp"})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if req.TermLimitedIconPath == nil || !strings.Contains(*req.TermLimitedIconPath, "term_limited.png") {
		t.Fatalf("unexpected term limited icon path: %+v", req.TermLimitedIconPath)
	}
	if req.FesLimitedIconPath == nil || !strings.Contains(*req.FesLimitedIconPath, "fes_limited.png") {
		t.Fatalf("unexpected fes limited icon path: %+v", req.FesLimitedIconPath)
	}
}

func TestBuildCardBoxRequestMarksOwnedCardsFromDetailedProfile(t *testing.T) {
	source := &lookupTestSource{}
	builder := NewBuilder(source, nil, nil, nil)
	req, err := builder.BuildCardBoxRequest([]*masterdata.Card{
		{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"},
		{ID: 1002, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cool", Prefix: "Card B", AssetBundleName: "card_b"},
	}, "jp", &drawing.DetailedProfileCardRequest{UserCards: []any{map[string]any{"cardId": 1002}}}, false, false, true)
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if len(req.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(req.Cards))
	}
	if req.Cards[0].Card.CardID != 1001 || req.Cards[0].HasCard {
		t.Fatalf("unexpected first card state: %+v", req.Cards[0])
	}
	if req.Cards[1].Card.CardID != 1002 || !req.Cards[1].HasCard {
		t.Fatalf("unexpected second card state: %+v", req.Cards[1])
	}
}

func TestBuildCardBoxRequestAppliesDisplayFlagsAndBeforeSetting(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)
	useAfterTraining := false
	req, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp", ShowID: true, UseAfterTraining: &useAfterTraining}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if !req.ShowID || req.ShowBox {
		t.Fatalf("unexpected request flags: %+v", req)
	}
	if len(req.Cards) != 1 || req.Cards[0].Card.IsAfterTraining == nil || *req.Cards[0].Card.IsAfterTraining {
		t.Fatalf("unexpected after-training state: %+v", req.Cards)
	}
}

func TestBuildCardBoxRequestUsesOwnedCardDefaultImageEvenWhenBeforeIsSet(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)
	useAfterTraining := false
	req, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp", UseAfterTraining: &useAfterTraining, DetailedProfile: &drawing.DetailedProfileCardRequest{UserCards: []any{map[string]any{"cardId": 1001, "defaultImage": "special_training", "specialTrainingStatus": "done"}}}}})
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if len(req.Cards) != 1 || req.Cards[0].Card.IsAfterTraining == nil || !*req.Cards[0].Card.IsAfterTraining {
		t.Fatalf("expected owned card to keep special_training default image: %+v", req.Cards)
	}
}

func TestBuildCardBoxRequestRejectsShowBoxWithoutOwnedCardData(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)
	useAfterTraining := true
	_, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "jp", ShowBox: true, UseAfterTraining: &useAfterTraining}})
	if err == nil {
		t.Fatal("expected show_box without owned-card data to fail")
	}
	if !strings.Contains(err.Error(), "box") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildCardBoxRequestRejectsExplicitRegionWithoutSource(t *testing.T) {
	source := &lookupTestSource{cards: []*masterdata.Card{{ID: 1001, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a"}}}
	controller := NewController(source, nil, nil, nil)

	_, err := controller.BuildCardBoxRequest([]Query{{Query: "1001", Region: "cn"}})
	if err == nil {
		t.Fatal("expected explicit cn lookup without a cn source to fail")
	}
	if !strings.Contains(err.Error(), "region cn") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildCardBoxRequestUsesOwnedCardVisualStateFromDetailedProfile(t *testing.T) {
	source := &lookupTestSource{}
	builder := NewBuilder(source, nil, nil, nil)
	req, err := builder.BuildCardBoxRequest([]*masterdata.Card{
		{ID: 190, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card A", AssetBundleName: "card_a", InitialSpecialTrainingStatus: "not_done"},
		{ID: 191, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card B", AssetBundleName: "card_b", InitialSpecialTrainingStatus: "not_done"},
	}, "jp", &drawing.DetailedProfileCardRequest{UserCards: []any{
		map[string]any{"cardId": 190, "level": 60, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "normal"},
		map[string]any{"cardId": 191, "level": 50, "masterRank": 2, "specialTrainingStatus": "done", "defaultImage": "special_training"},
	}}, false, false, true)
	if err != nil {
		t.Fatalf("BuildCardBoxRequest() error = %v", err)
	}
	if len(req.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(req.Cards))
	}
	first := req.Cards[0].Card
	if len(first.ThumbnailInfo) != 1 {
		t.Fatalf("expected 1 thumbnail for first card, got %+v", first.ThumbnailInfo)
	}
	if first.IsAfterTraining == nil || *first.IsAfterTraining {
		t.Fatalf("expected first card display to stay normal, got %+v", first.IsAfterTraining)
	}
	if !first.ThumbnailInfo[0].IsPcard {
		t.Fatalf("expected first card thumbnail to be pcard")
	}
	if first.ThumbnailInfo[0].Level == nil || *first.ThumbnailInfo[0].Level != 60 {
		t.Fatalf("unexpected first card level: %+v", first.ThumbnailInfo[0].Level)
	}
	if first.ThumbnailInfo[0].TrainRank == nil || *first.ThumbnailInfo[0].TrainRank != 5 {
		t.Fatalf("unexpected first card train rank: %+v", first.ThumbnailInfo[0].TrainRank)
	}
	if first.ThumbnailInfo[0].RareImgPath != "static_images/card/rare_star_after_training.png" {
		t.Fatalf("expected first card to use after-training stars, got %q", first.ThumbnailInfo[0].RareImgPath)
	}
	second := req.Cards[1].Card
	if len(second.ThumbnailInfo) != 1 {
		t.Fatalf("expected 1 thumbnail for second card, got %+v", second.ThumbnailInfo)
	}
	if second.IsAfterTraining == nil || !*second.IsAfterTraining {
		t.Fatalf("expected second card display to use after training, got %+v", second.IsAfterTraining)
	}
	if second.ThumbnailInfo[0].RareImgPath != "static_images/card/rare_star_after_training.png" {
		t.Fatalf("expected second card to use after-training stars, got %q", second.ThumbnailInfo[0].RareImgPath)
	}
}

func TestRenderCardListAutoSwitchesToCardBoxWhenTooManyCards(t *testing.T) {
	source := &lookupTestSource{}
	for i := 1; i <= cardListAutoBoxThreshold; i++ {
		source.cards = append(source.cards, &masterdata.Card{ID: 2000 + i, CharacterID: 5, CardRarityType: "rarity_4", Attr: "cute", Prefix: "Card", AssetBundleName: "card_a"})
	}
	var calledPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calledPath = r.URL.Path; _, _ = w.Write([]byte("png")) }))
	defer server.Close()
	controller := NewController(source, nil, drawing.NewHarukiDrawingClient(server.URL), nil)
	cardIDs := make([]int, 0, len(source.cards))
	for _, card := range source.cards {
		cardIDs = append(cardIDs, card.ID)
	}
	_, err := controller.RenderCardList(ListRequest{CardIDs: cardIDs, Region: "jp"})
	if err != nil {
		t.Fatalf("RenderCardList() error = %v", err)
	}
	if calledPath != "/api/pjsk/card/box" {
		t.Fatalf("expected card box render path, got %q", calledPath)
	}
}
