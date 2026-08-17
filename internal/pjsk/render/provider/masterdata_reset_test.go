package provider

import (
	"testing"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestDatabaseProviderResetMasterdataCacheClearsPermanentCaches(t *testing.T) {
	cards := &dbCardProvider{}
	cards.init()
	cards.supplyByID[1] = "supply"
	characters := &dbCharacterProvider{}
	characters.init()
	characters.charCache[1] = &masterdata.Character{ID: 1}
	gachas := &dbGachaProvider{}
	gachas.init()
	gachas.gachas = []*masterdata.Gacha{{ID: 1}}
	stamps := &dbStampProvider{loaded: true, stamps: []masterdata.Stamp{{ID: 1}}}
	frames := &dbPlayerFrameProvider{}
	frames.init()
	frames.frameCache[1] = &masterdata.PlayerFrame{ID: 1}
	mysekai := &dbMySekaiProvider{
		lists:       map[string][]map[string]any{"mysekaiFixtures.json": {{"id": 1}}},
		mapsByID:    map[string]map[int]map[string]any{"mysekaiFixtures.json": {1: {"id": 1}}},
		unavailable: map[string]struct{}{"mysekaiBlueprints.json": {}},
	}

	provider := &DatabaseProvider{
		cards:        cards,
		characters:   characters,
		gachas:       gachas,
		stamps:       stamps,
		playerFrames: frames,
		mysekai:      mysekai,
	}
	provider.ResetMasterdataCache()

	if len(cards.supplyByID) != 0 {
		t.Fatalf("card supply cache was not reset: %+v", cards.supplyByID)
	}
	if len(characters.charCache) != 0 {
		t.Fatalf("character cache was not reset: %+v", characters.charCache)
	}
	if len(gachas.gachas) != 0 {
		t.Fatalf("gacha list cache was not reset: %+v", gachas.gachas)
	}
	if stamps.loaded || len(stamps.stamps) != 0 {
		t.Fatalf("stamp cache was not reset: loaded=%v stamps=%+v", stamps.loaded, stamps.stamps)
	}
	if len(frames.frameCache) != 0 {
		t.Fatalf("player frame cache was not reset: %+v", frames.frameCache)
	}
	if len(mysekai.lists) != 0 || len(mysekai.mapsByID) != 0 || len(mysekai.unavailable) != 0 {
		t.Fatalf("mysekai provider cache was not reset: lists=%+v maps=%+v unavailable=%+v", mysekai.lists, mysekai.mapsByID, mysekai.unavailable)
	}
}
