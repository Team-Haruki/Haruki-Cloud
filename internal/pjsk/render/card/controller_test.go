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
