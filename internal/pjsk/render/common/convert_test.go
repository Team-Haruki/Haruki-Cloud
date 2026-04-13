package common

import (
	"encoding/json"
	"testing"

	sekaiDB "haruki-cloud/database/sekai"
)

func TestConvertCardEntityAcceptsObjectCardParameters(t *testing.T) {
	entity := &sekaiDB.Card{
		GameID:          1001,
		CharacterID:     21,
		CardRarityType:  "rarity_4",
		Attr:            "cute",
		Prefix:          "Test Card",
		AssetbundleName: "card_test",
		CardParameters: json.RawMessage(`{
			"param2": {"power": 220},
			"param1": {"power": 110},
			"param3": 330
		}`),
	}

	card, err := ConvertCardEntity(entity)
	if err != nil {
		t.Fatalf("ConvertCardEntity() error = %v", err)
	}
	if card == nil {
		t.Fatal("expected card model")
	}
	if len(card.CardParameters) != 3 {
		t.Fatalf("expected 3 card parameters, got %d", len(card.CardParameters))
	}
	if got := card.CardParameters[0]; got.CardParameterType != "param1" || got.Power != 110 || got.CardID != 1001 {
		t.Fatalf("unexpected param1: %+v", got)
	}
	if got := card.CardParameters[1]; got.CardParameterType != "param2" || got.Power != 220 || got.CardID != 1001 {
		t.Fatalf("unexpected param2: %+v", got)
	}
	if got := card.CardParameters[2]; got.CardParameterType != "param3" || got.Power != 330 || got.CardID != 1001 {
		t.Fatalf("unexpected param3: %+v", got)
	}
}
