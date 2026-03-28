package deck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
)

type testCardSource struct {
	region renderregion.Value
	cards  map[int]*masterdata.Card
}

func (s *testCardSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	return s.cards[id], nil
}

type testEventSource struct {
	events map[int]*masterdata.Event
}

func (s *testEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	return s.events[id], nil
}

func (s *testEventSource) GetEvents() []*masterdata.Event {
	result := make([]*masterdata.Event, 0, len(s.events))
	for _, eventInfo := range s.events {
		result = append(result, eventInfo)
	}
	return result
}

func TestBuildAutoRecommendRequestFallback(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	eventID := 7
	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		Limit:         2,
		EventID:       &eventID,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if request.RecommendType != "event" {
		t.Fatalf("unexpected recommend type: %s", request.RecommendType)
	}
	if request.EventID == nil || *request.EventID != eventID {
		t.Fatalf("unexpected event id: %+v", request.EventID)
	}
	if request.EventName == nil || *request.EventName != "Deck Event" {
		t.Fatalf("unexpected event name: %+v", request.EventName)
	}
	if request.LiveType == nil || *request.LiveType != "multi" {
		t.Fatalf("unexpected live type: %+v", request.LiveType)
	}
	if request.Target == nil || *request.Target != "score" {
		t.Fatalf("unexpected target: %+v", request.Target)
	}
	if len(request.DeckData) != 1 {
		t.Fatalf("unexpected deck data count: %d", len(request.DeckData))
	}
	if request.DeckData[0].TotalPower == nil || *request.DeckData[0].TotalPower <= 0 {
		t.Fatalf("unexpected total power: %+v", request.DeckData[0].TotalPower)
	}
	if len(request.DeckData[0].CardData) != 2 {
		t.Fatalf("unexpected card count: %d", len(request.DeckData[0].CardData))
	}
	if request.DeckData[0].CardData[0].CardThumbnail.CardID != 1002 {
		t.Fatalf("unexpected card order: %+v", request.DeckData[0].CardData)
	}
}

func TestBuildAutoRecommendRequestLocalEngineStubError(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{
		Enabled:        true,
		UseLocalEngine: true,
		MasterdataDir:  t.TempDir(),
	})

	_, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
	})
	if err == nil {
		t.Fatalf("expected local engine error")
	}
	if !strings.Contains(err.Error(), "pjsk_deck_cgo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildRecommendOptionBonusTargets(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "bonus", AutoQuery{
		Region:        "jp",
		RecommendType: "bonus",
		Limit:         3,
		Args:          "150 160",
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["target"] != "bonus" {
		t.Fatalf("unexpected target: %+v", option["target"])
	}
	if option["algorithm"] != "dfs" {
		t.Fatalf("unexpected algorithm: %+v", option["algorithm"])
	}
	targets, ok := option["target_bonus_list"].([]int)
	if !ok {
		t.Fatalf("unexpected target_bonus_list type: %T", option["target_bonus_list"])
	}
	if len(targets) != 2 || targets[0] != 150 || targets[1] != 160 {
		t.Fatalf("unexpected target bonus list: %+v", targets)
	}
}

func newTestDeckController(t *testing.T, cfg RecommendConfig) *Controller {
	t.Helper()

	tempDir := t.TempDir()
	userJSONPath := filepath.Join(tempDir, "user.json")
	userJSON := `{
		"now": 1700000000000,
		"userGamedata": {"userId": 10001, "name": "Deck User", "deck": 1, "rank": 123},
		"userProfile": {"profileImageType": "default"},
		"userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 1002, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
		"userCards": [
			{"cardId": 1001, "level": 50, "masterRank": 1, "specialTrainingStatus": "not_done", "defaultImage": "normal", "episodes": []},
			{"cardId": 1002, "level": 60, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []}
		]
	}`
	if err := os.WriteFile(userJSONPath, []byte(userJSON), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}

	assetHelper := assets.NewAssetHelper(tempDir, nil)
	snapshot := userdata.NewLocalFileService(nil, assetHelper, userdata.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      userJSONPath,
	})

	cardSource := &testCardSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				CharacterID:     1,
				CardRarityType:  "rarity_4",
				Attr:            "cute",
				AssetBundleName: "card_1001",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 100},
					{CardParameterType: "param2", Power: 100},
					{CardParameterType: "param3", Power: 100},
				},
			},
			1002: {
				ID:                              1002,
				CharacterID:                     2,
				CardRarityType:                  "rarity_4",
				Attr:                            "cool",
				AssetBundleName:                 "card_1002",
				SpecialTrainingPower1BonusFixed: 50,
				SpecialTrainingPower2BonusFixed: 50,
				SpecialTrainingPower3BonusFixed: 50,
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 200},
					{CardParameterType: "param2", Power: 200},
					{CardParameterType: "param3", Power: 200},
				},
			},
		},
	}
	eventSource := &testEventSource{
		events: map[int]*masterdata.Event{
			7: {
				ID:              7,
				Name:            "Deck Event",
				AssetBundleName: "deck_event_banner",
			},
		},
	}

	return NewControllerWithConfig(cardSource, eventSource, nil, assetHelper, snapshot, renderregion.JP, cfg, nil)
}
