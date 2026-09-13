package deck

import (
	"haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	"strings"
	"testing"
)

func TestRemoteSupportCardsPreserveResultConfiguration(t *testing.T) {
	var remote remoteRecommendResult
	err := jsonutil.Unmarshal([]byte(`{"decks":[{"cards":[],"support_deck_bonus_rate":12.75,"support_deck_cards":[{"card_id":123,"bonus":12.75,"level":60,"skill_level":4,"master_rank":5,"after_training":true,"default_image":"special_training"}]},{"cards":[]}]}`), &remote)
	if err != nil {
		t.Fatal(err)
	}
	decks := convertRemoteDecks(remote.Decks)
	if len(decks[0].SupportCards) != 1 || len(decks[1].SupportCards) != 0 {
		t.Fatalf("support configuration lost: %+v", decks)
	}
	card := decks[0].SupportCards[0]
	if card.CardID != 123 || card.EventBonusRate != 12.75 || card.SkillLevel != 4 || card.MasterRank != 5 || card.Level != 60 || !card.IsAfterTraining || card.DefaultImage != "special_training" {
		t.Fatalf("support card changed: %+v", card)
	}
	if decks[0].SupportDeckBonusRate != card.EventBonusRate {
		t.Fatal("support total changed")
	}
}

func TestSupportCardsReachDrawingWithIndependentLineups(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	var remote remoteRecommendResult
	err := jsonutil.Unmarshal([]byte(`{"decks":[{"cards":[{"card_id":1001,"level":50,"skill_level":4,"default_image":"normal"}],"support_deck_bonus_rate":12.75,"support_deck_cards":[{"card_id":1002,"bonus":12.75,"level":60,"skill_level":3,"master_rank":2,"after_training":true,"default_image":"special_training"}]},{"cards":[{"card_id":1002,"level":60,"default_image":"special_training"}],"support_deck_bonus_rate":5,"support_deck_cards":[{"card_id":1001,"bonus":5,"level":50,"skill_level":2,"master_rank":1,"after_training":false,"default_image":"normal"}]}]}`), &remote)
	if err != nil {
		t.Fatal(err)
	}
	request, err := controller.buildDrawingRequestFromRecommendResult(renderregion.JP, "wl", AutoQuery{Region: "jp"}, map[string]any{}, nil, &RecommendResult{Decks: convertRemoteDecks(remote.Decks)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, second := request.DeckData[0].SupportCardData, request.DeckData[1].SupportCardData
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("missing support rows: %+v", request.DeckData)
	}
	if first[0].CardThumbnail.CardID != 1002 || second[0].CardThumbnail.CardID != 1001 || first[0].EventBonusRate != 12.75 || second[0].EventBonusRate != 5 {
		t.Fatalf("support rows mixed: %+v", request.DeckData)
	}
	if first[0].SkillLevel != "3" || first[0].CardThumbnail.TrainRank == nil || *first[0].CardThumbnail.TrainRank != 2 || !first[0].IsAfterTraining {
		t.Fatalf("support configuration changed: %+v", first[0])
	}
	payload, err := jsonutil.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"support_card_data"`) {
		t.Fatal("support data missing from drawing JSON")
	}
}
