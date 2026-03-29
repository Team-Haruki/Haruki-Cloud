package card

import (
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestBuildCardListRequestResolvesIDsFromQuery(t *testing.T) {
	source := &lookupTestSource{
		cards: []*masterdata.Card{
			{
				ID:              1001,
				CharacterID:     5,
				CardRarityType:  "rarity_4",
				Attr:            "cute",
				Prefix:          "Card A",
				AssetBundleName: "card_a",
			},
			{
				ID:              1002,
				CharacterID:     5,
				CardRarityType:  "rarity_4",
				Attr:            "cool",
				Prefix:          "Card B",
				AssetBundleName: "card_b",
			},
		},
	}

	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{
		Query:  "mnr 4星",
		Region: "jp",
	})
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
	cardInfo := &masterdata.Card{
		ID:              1003,
		CharacterID:     21,
		CardRarityType:  "rarity_4",
		Attr:            "cute",
		Prefix:          "Card C",
		AssetBundleName: "card_c",
		SupportUnit:     "idol",
	}
	source := &lookupTestSource{
		cards: []*masterdata.Card{cardInfo},
		characters: map[int]*masterdata.Character{
			21: {ID: 21, FirstName: "初音", GivenName: "未来", Unit: "piapro"},
		},
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
	req, err := controller.BuildCardListRequest(ListRequest{
		Query:  "event123 mmjv fes 25年",
		Region: "jp",
	})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 1 || req.Cards[0].CardID != 1003 {
		t.Fatalf("unexpected cards: %+v", req.Cards)
	}
}

func TestBuildCardListRequestPrefers25UnitAliasOverCardID(t *testing.T) {
	cardInfo := &masterdata.Card{
		ID:              1025,
		CharacterID:     26,
		CardRarityType:  "rarity_4",
		Attr:            "cool",
		Prefix:          "Card 25",
		AssetBundleName: "card_25",
	}
	source := &lookupTestSource{
		cards: []*masterdata.Card{cardInfo},
		filterFunc: func(info *CardQueryInfo) ([]*masterdata.Card, error) {
			if info == nil {
				t.Fatal("expected query info")
			}
			if info.Type != QueryTypeFilter {
				t.Fatalf("expected filter query, got %+v", info)
			}
			if info.Unit != "school_refusal" {
				t.Fatalf("unexpected unit filter: %+v", info)
			}
			return []*masterdata.Card{cardInfo}, nil
		},
	}

	controller := NewController(source, nil, nil, nil)
	req, err := controller.BuildCardListRequest(ListRequest{
		Query:  "25",
		Region: "jp",
	})
	if err != nil {
		t.Fatalf("BuildCardListRequest() error = %v", err)
	}
	if len(req.Cards) != 1 || req.Cards[0].CardID != 1025 {
		t.Fatalf("unexpected cards: %+v", req.Cards)
	}
}
