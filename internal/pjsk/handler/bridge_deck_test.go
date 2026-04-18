package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/utils/imagecache"
)

func TestResolveDeckRenderProfileAndSnapshotUsesSelectedBinding(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"cn": {
				"11111111111111": "CN User 1",
			},
			"jp": {
				"33333333333333": "JP User 1",
			},
		},
	})

	if _, err := service.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind first account: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "33333333333333"); err != nil {
		t.Fatalf("bind second account: %v", err)
	}

	_, expectedBinding, err := service.ResolveUserBindingBySelector(ctx, "qq", "42", "", "u1")
	if err != nil {
		t.Fatalf("resolve selector binding: %v", err)
	}

	provider := &runtimeSnapshotProviderStub{
		snapshot: &runtimeSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{Nickname: "selector-snapshot"},
		},
	}
	rc := NewRequestContext(ctx, &parser.ResolvedCommand{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings:  service,
		Snapshots: provider,
	})

	detail, snapshot, region, err := resolveDeckRenderProfileAndSnapshot(rc, "u1")
	if err != nil {
		t.Fatalf("resolveDeckRenderProfileAndSnapshot() error = %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if detail == nil || detail.Nickname != "selector-snapshot" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if region != "cn" {
		t.Fatalf("expected resolved region cn, got %q", region)
	}
	if len(provider.selectors) != 1 {
		t.Fatalf("expected one snapshot selector, got %d", len(provider.selectors))
	}

	selector := provider.selectors[0]
	if selector.IMPlatform != "qq" || selector.IMUserID != "42" {
		t.Fatalf("unexpected im selector: %+v", selector)
	}
	if selector.Region != renderregion.CN {
		t.Fatalf("unexpected selector region: %+v", selector.Region)
	}
	if selector.PJSKUserID != expectedBinding.PJSKUserID {
		t.Fatalf("expected selected binding uid %q, got %q", expectedBinding.PJSKUserID, selector.PJSKUserID)
	}
}

func TestExecuteDeckMySekaiBlocksCNRegion(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	params, err := json.Marshal(struct {
		Deck  map[string]any `json:"deck"`
		Query map[string]any `json:"query"`
	}{
		Deck:  map[string]any{},
		Query: map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeDeck(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module:            parser.ModuleDeck,
		Mode:              "deck-mysekai",
		Region:            "cn",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{}))
	if err != nil {
		t.Fatalf("executeDeck() error = %v", err)
	}
	assertSingleMySekaiUnavailableMessage(t, message)
}

func TestExecuteDeckMySekaiMaxProfileDoesNotRequireBinding(t *testing.T) {
	deckController := newHandlerTestDeckController(t)

	params, err := json.Marshal(struct {
		Deck  map[string]any `json:"deck"`
		Query map[string]any `json:"query"`
	}{
		Deck: map[string]any{
			"max_profile": true,
			"event_unit":  "idol",
			"event_attr":  "cute",
		},
		Query: map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeDeck(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module:            parser.ModuleDeck,
		Mode:              "deck-mysekai",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Decks:      deckController,
		Music:      newHandlerTestMusicController(t),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}))
	if err != nil {
		t.Fatalf("executeDeck() error = %v", err)
	}
	if len(message) != 2 {
		t.Fatalf("unexpected message: %+v", message)
	}
	if message[0].Type != onebot11.TypeText {
		t.Fatalf("unexpected first segment: %+v", message[0])
	}
	if message[1].Type != onebot11.TypeImage {
		t.Fatalf("unexpected second segment: %+v", message[1])
	}
}

func TestExecuteDeckEventMaxProfileDoesNotRequireBinding(t *testing.T) {
	deckController := newHandlerTestDeckController(t)

	params, err := json.Marshal(map[string]any{
		"max_profile": true,
		"event_unit":  "idol",
		"event_attr":  "cute",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeDeck(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module:            parser.ModuleDeck,
		Mode:              "deck-event",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Decks:      deckController,
		Music:      newHandlerTestMusicController(t),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}))
	if err != nil {
		t.Fatalf("executeDeck() error = %v", err)
	}
	if len(message) != 2 {
		t.Fatalf("unexpected message: %+v", message)
	}
	if message[0].Type != onebot11.TypeText {
		t.Fatalf("unexpected first segment: %+v", message[0])
	}
	if message[1].Type != onebot11.TypeImage {
		t.Fatalf("unexpected second segment: %+v", message[1])
	}
}

func assertSingleMySekaiUnavailableMessage(t *testing.T, message onebot11.Message) {
	t.Helper()
	if len(message) != 1 || message[0].Type != onebot11.TypeText {
		t.Fatalf("unexpected message: %+v", message)
	}
	data, ok := message[0].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected message data: %+v", message[0].Data)
	}
	if data.Text != "MySekai 功能在此区服暂未开放" {
		t.Fatalf("unexpected text: %q", data.Text)
	}
}

type handlerDeckMusicMetaSource struct {
	data []byte
}

func (s handlerDeckMusicMetaSource) Get(string) []byte {
	return append([]byte(nil), s.data...)
}

type handlerDeckCardSource struct {
	region renderregion.Value
	cards  map[int]*masterdata.Card
}

func (s *handlerDeckCardSource) DefaultRegion() renderregion.Value { return s.region }

func (s *handlerDeckCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	return s.cards[id], nil
}

func (s *handlerDeckCardSource) GetAllCards() ([]*masterdata.Card, error) {
	result := make([]*masterdata.Card, 0, len(s.cards))
	for _, cardInfo := range s.cards {
		result = append(result, cardInfo)
	}
	return result, nil
}

type handlerDeckEventSource struct {
	region renderregion.Value
	events map[int]*masterdata.Event
}

func (s *handlerDeckEventSource) DefaultRegion() renderregion.Value { return s.region }

func (s *handlerDeckEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	return s.events[id], nil
}

func (s *handlerDeckEventSource) GetEvents() []*masterdata.Event {
	result := make([]*masterdata.Event, 0, len(s.events))
	for _, eventInfo := range s.events {
		result = append(result, eventInfo)
	}
	return result
}

type handlerDeckMusicSource struct {
	region renderregion.Value
	musics map[int]*masterdata.Music
}

func (s *handlerDeckMusicSource) DefaultRegion() renderregion.Value { return s.region }

func (s *handlerDeckMusicSource) SearchMusic(string) (*masterdata.Music, error) {
	for _, musicInfo := range s.musics {
		return musicInfo, nil
	}
	return nil, nil
}

func (s *handlerDeckMusicSource) GetMusicByID(id int) (*masterdata.Music, error) {
	return s.musics[id], nil
}

func (s *handlerDeckMusicSource) GetMusicByEventID(int) (*masterdata.Music, error) {
	for _, musicInfo := range s.musics {
		return musicInfo, nil
	}
	return nil, nil
}

func (s *handlerDeckMusicSource) GetMusics() []*masterdata.Music {
	result := make([]*masterdata.Music, 0, len(s.musics))
	for _, musicInfo := range s.musics {
		result = append(result, musicInfo)
	}
	return result
}

func (s *handlerDeckMusicSource) GetBanEvents(int) []*masterdata.Event { return nil }

func (s *handlerDeckMusicSource) GetMusicLocalizedTitles(int) ([]string, error) { return nil, nil }

func (s *handlerDeckMusicSource) GetMusicDifficulties(int) ([]*masterdata.MusicDifficulty, error) {
	return nil, nil
}

func (s *handlerDeckMusicSource) GetMusicVocals(int) ([]*masterdata.MusicVocal, error) {
	return nil, nil
}

func (s *handlerDeckMusicSource) GetMusicTags(int) ([]string, error) { return nil, nil }

func (s *handlerDeckMusicSource) GetCharacterByID(int) (*masterdata.Character, error) {
	return nil, nil
}

func (s *handlerDeckMusicSource) GetOutsideCharacterByID(int) (string, error) { return "", nil }

func (s *handlerDeckMusicSource) GetPrimaryEventByMusicID(int) (*masterdata.Event, error) {
	return nil, nil
}

func (s *handlerDeckMusicSource) GetLimitedTimeMusics(int) []*masterdata.LimitedTimeMusic { return nil }

func newHandlerTestDeckController(t *testing.T) *renderdeck.Controller {
	t.Helper()

	recommendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch r.URL.Path {
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			_, _ = w.Write([]byte(`{"userdata_hash":"handler-deck-max-profile-hash"}`))
		case "/recommend":
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.1,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": 100,
							"live_score": 100,
							"mysekai_event_point": 0,
							"total_power": 200,
							"event_bonus_rate": 20,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up": 110,
							"cards": [{
								"card_id": 1001,
								"level": 60,
								"master_rank": 5,
								"skill_level": 4,
								"skill_score_up": 120,
								"event_bonus_rate": 20,
								"episode1_read": true,
								"episode2_read": true,
								"after_training": true,
								"default_image": "special_training",
								"has_canvas_bonus": false
							}]
						}]
					}
				}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(recommendServer.Close)

	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pjsk/deck/recommend" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("deck-render"))
	}))
	t.Cleanup(drawingServer.Close)

	masterdataRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(masterdataRoot, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir jp masterdata dir: %v", err)
	}
	assetRoot := t.TempDir()
	deckController := renderdeck.NewControllerWithConfig(
		&handlerDeckCardSource{
			region: renderregion.JP,
			cards: map[int]*masterdata.Card{
				1001: {
					ID:              1001,
					CharacterID:     1,
					CardRarityType:  "rarity_4",
					Attr:            "cute",
					AssetBundleName: "card_1001",
				},
			},
		},
		&handlerDeckEventSource{
			region: renderregion.JP,
			events: map[int]*masterdata.Event{
				7: {ID: 7, Name: "Deck Event", AssetBundleName: "deck_event_banner"},
			},
		},
		drawing.NewHarukiDrawingClient(drawingServer.URL),
		renderassets.NewAssetHelper(assetRoot, nil),
		nil,
		renderregion.JP,
		renderdeck.RecommendConfig{
			Enabled:        true,
			ServiceBaseURL: recommendServer.URL,
			MasterdataDir:  masterdataRoot,
			DefaultAlgs:    []string{"ga"},
		},
		handlerDeckMusicMetaSource{
			data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
		},
	)
	deckController.RegisterMusicSource(&handlerDeckMusicSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			10000: {ID: 10000, Title: "おまかせ", AssetBundleName: "omakase"},
		},
	})
	return deckController
}

func newHandlerTestMusicController(t *testing.T) *rendermusic.Controller {
	t.Helper()
	return rendermusic.NewController(&handlerDeckMusicSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			10000: {ID: 10000, Title: "おまかせ", AssetBundleName: "omakase"},
		},
	}, nil, renderassets.NewAssetHelper(t.TempDir(), nil), nil, nil)
}
