package provider

import (
	"context"
	"testing"
)

func TestLocalEventProviderGetBanEventsOnlyReturnsBoxEvents(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "events.json", `[
		{"id":101,"eventType":"marathon","name":"box-1","assetbundleName":"e101","startAt":100,"aggregateAt":200},
		{"id":102,"eventType":"marathon","name":"mixed","assetbundleName":"e102","startAt":300,"aggregateAt":400},
		{"id":103,"eventType":"cheerful_carnival","name":"box-2","assetbundleName":"e103","startAt":500,"aggregateAt":600}
	]`)
	writeTestFile(t, root, "eventCards.json", `[
		{"eventId":101,"cardId":1001},
		{"eventId":102,"cardId":1002},
		{"eventId":103,"cardId":1003}
	]`)
	writeTestFile(t, root, "cards.json", `[
		{"id":1001,"characterId":10,"cardRarityType":"rarity_4","supportUnit":"none","cardSupplyId":1},
		{"id":1002,"characterId":10,"cardRarityType":"rarity_4","supportUnit":"none","cardSupplyId":1},
		{"id":1003,"characterId":10,"cardRarityType":"rarity_4","supportUnit":"none","cardSupplyId":1}
	]`)
	writeTestFile(t, root, "gameCharacters.json", `[
		{"id":10,"givenName":"杏","unit":"street"},
		{"id":5,"givenName":"愛莉","unit":"idol"},
		{"id":21,"givenName":"ミク","unit":"street"}
	]`)
	writeTestFile(t, root, "gameCharacterUnits.json", `[
		{"id":10,"gameCharacterId":10,"unit":"street","colorCode":"#fff"},
		{"id":21,"gameCharacterId":21,"unit":"street","colorCode":"#fff"},
		{"id":105,"gameCharacterId":5,"unit":"idol","colorCode":"#fff"}
	]`)
	writeTestFile(t, root, "eventDeckBonuses.json", `[
		{"id":1,"eventId":101,"gameCharacterUnitId":10,"cardAttr":"cool","bonusRate":50},
		{"id":2,"eventId":101,"gameCharacterUnitId":21,"cardAttr":"","bonusRate":25},
		{"id":3,"eventId":102,"gameCharacterUnitId":10,"cardAttr":"cool","bonusRate":50},
		{"id":4,"eventId":102,"gameCharacterUnitId":105,"cardAttr":"","bonusRate":25},
		{"id":5,"eventId":103,"gameCharacterUnitId":10,"cardAttr":"pure","bonusRate":50},
		{"id":6,"eventId":103,"gameCharacterUnitId":21,"cardAttr":"","bonusRate":25}
	]`)

	store := newLocalStore(root)
	characters := &localCharacterProvider{store: store}
	cards := &localCardProvider{store: store, characters: characters}
	events := &localEventProvider{store: store, cards: cards}

	result := events.GetBanEvents(context.Background(), 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 box events, got %+v", result)
	}
	if result[0].ID != 101 || result[1].ID != 103 {
		t.Fatalf("unexpected box events: %+v", result)
	}
}
