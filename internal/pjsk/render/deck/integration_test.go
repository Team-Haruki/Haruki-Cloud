package deck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

type integrationTestInput struct {
	serviceURL         string
	userJSON           string
	masterdataRoot     string
	musicMetaJSON      string
	recommendType      string
	algorithm          string
	target             string
	liveType           string
	useFixedCharacters bool
}

func TestBuildAutoRecommendRequestWithDeckServiceIntegration(t *testing.T) {
	input := loadIntegrationTestInput()
	if input.serviceURL == "" || input.userJSON == "" || input.masterdataRoot == "" || input.musicMetaJSON == "" {
		t.Skip("set HARUKI_DECK_SERVICE_URL, HARUKI_TEST_USER_JSON, HARUKI_DECK_MASTERDATA_DIR, HARUKI_TEST_MUSIC_META_JSON to run integration test")
	}
	request, err := buildIntegrationRequest(input, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: input.serviceURL,
		MasterdataDir:  input.masterdataRoot,
		Timeout:        20 * time.Second,
		DefaultAlgs:    []string{"ga"},
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if request == nil || len(request.DeckData) == 0 {
		t.Fatalf("unexpected empty request: %+v", request)
	}
	if len(request.DeckData[0].CardData) == 0 {
		t.Fatalf("unexpected empty card data: %+v", request.DeckData[0])
	}
}

func loadIntegrationTestInput() integrationTestInput {
	input := integrationTestInput{
		serviceURL:         strings.TrimSpace(os.Getenv("HARUKI_DECK_SERVICE_URL")),
		userJSON:           strings.TrimSpace(os.Getenv("HARUKI_TEST_USER_JSON")),
		masterdataRoot:     strings.TrimSpace(os.Getenv("HARUKI_DECK_MASTERDATA_DIR")),
		musicMetaJSON:      strings.TrimSpace(os.Getenv("HARUKI_TEST_MUSIC_META_JSON")),
		recommendType:      strings.TrimSpace(os.Getenv("HARUKI_TEST_RECOMMEND_TYPE")),
		algorithm:          strings.TrimSpace(os.Getenv("HARUKI_TEST_ALGORITHM")),
		target:             strings.TrimSpace(os.Getenv("HARUKI_TEST_TARGET")),
		liveType:           strings.TrimSpace(os.Getenv("HARUKI_TEST_LIVE_TYPE")),
		useFixedCharacters: parseBoolEnv("HARUKI_TEST_USE_FIXED_CHARACTERS"),
	}
	if input.recommendType == "" {
		input.recommendType = "no_event"
	}
	if input.algorithm == "" {
		input.algorithm = "dfs"
	}
	if input.target == "" {
		input.target = "power"
	}
	if input.liveType == "" {
		input.liveType = "solo"
	}
	return input
}

func buildIntegrationRequest(input integrationTestInput, cfg RecommendConfig) (*drawing.DeckRequest, error) {
	cardSource, eventSource, err := loadIntegrationSources(input.masterdataRoot, "jp")
	if err != nil {
		return nil, err
	}
	integrationCards, _ := cardSource.(*integrationCardSource)

	snapshot := snapshot.NewLocalFileService(nil, assets.NewAssetHelper("", nil), snapshot.LocalFileConfig{
		DefaultRegion: renderregion.JP,
		UserJSON:      input.userJSON,
		MusicMetaJSON: input.musicMetaJSON,
	})
	if err := snapshot.Require(); err != nil {
		return nil, err
	}

	var fixedCharacters []int
	if input.useFixedCharacters {
		fixedCharacters = selectEligibleFixedCharacters(snapshot.RawData(), integrationCards)
	}

	controller := NewControllerWithConfig(
		cardSource,
		eventSource,
		nil,
		assets.NewAssetHelper("", nil),
		snapshot,
		renderregion.JP,
		cfg,
		nil,
	)

	return controller.BuildAutoRecommendRequest(AutoQuery{
		Region:          "jp",
		RecommendType:   input.recommendType,
		Algorithm:       input.algorithm,
		Target:          input.target,
		LiveType:        input.liveType,
		FixedCharacters: fixedCharacters,
		Limit:           1,
	})
}

func parseBoolEnv(name string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type integrationCardSource struct {
	region renderregion.Value
	cards  map[int]*masterdata.Card
}

func (s *integrationCardSource) DefaultRegion() renderregion.Value { return s.region }

func (s *integrationCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	return s.cards[id], nil
}

type integrationEventSource struct {
	region     renderregion.Value
	events     map[int]*masterdata.Event
	eventSlice []*masterdata.Event
}

func (s *integrationEventSource) DefaultRegion() renderregion.Value { return s.region }

func (s *integrationEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	return s.events[id], nil
}

func (s *integrationEventSource) GetEvents() []*masterdata.Event {
	return append([]*masterdata.Event(nil), s.eventSlice...)
}

func loadIntegrationSources(root, region string) (CardSource, EventSource, error) {
	baseDir, err := resolveDeckMasterdataDir(root, region)
	if err != nil {
		return nil, nil, err
	}
	if baseDir == "" {
		return nil, nil, os.ErrNotExist
	}

	read := func(name string, target any) error {
		data, err := os.ReadFile(filepath.Join(baseDir, name))
		if err != nil {
			return err
		}
		return json.Unmarshal(data, target)
	}

	var cards []masterdata.Card
	if err := read("cards.json", &cards); err != nil {
		return nil, nil, err
	}
	var events []masterdata.Event
	if err := read("events.json", &events); err != nil {
		return nil, nil, err
	}

	cardMap := make(map[int]*masterdata.Card, len(cards))
	for i := range cards {
		card := cards[i]
		cardMap[card.ID] = &card
	}

	eventMap := make(map[int]*masterdata.Event, len(events))
	eventSlice := make([]*masterdata.Event, 0, len(events))
	for i := range events {
		eventInfo := events[i]
		eventMap[eventInfo.ID] = &eventInfo
		eventSlice = append(eventSlice, &eventInfo)
	}

	return &integrationCardSource{
			region: renderregion.Normalize(region),
			cards:  cardMap,
		}, &integrationEventSource{
			region:     renderregion.Normalize(region),
			events:     eventMap,
			eventSlice: eventSlice,
		}, nil
}

func selectEligibleFixedCharacters(raw *snapshot.RawUserData, cards *integrationCardSource) []int {
	if raw == nil || cards == nil {
		return nil
	}
	result := make([]int, 0, 5)
	seenCharacters := make(map[int]struct{}, 5)
	for _, userCard := range raw.UserCards {
		card, ok := cards.cards[userCard.CardID]
		if !ok || card == nil {
			continue
		}
		switch card.CardRarityType {
		case "rarity_3", "rarity_4", "rarity_birthday":
			if _, exists := seenCharacters[card.CharacterID]; exists {
				continue
			}
			seenCharacters[card.CharacterID] = struct{}{}
			result = append(result, card.CharacterID)
			if len(result) == 5 {
				return result
			}
		}
	}
	return result
}
