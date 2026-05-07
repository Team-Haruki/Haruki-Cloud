package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/imagecache"
)

func TestResolveDeckRenderProfileAndSnapshotUsesSelectedBinding(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
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
	rc := NewRequestContext(ctx, &CommandRequest{
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

func TestResolveDeckRenderProfileSnapshotAndPublicReturnsSelectedPublicProfile(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
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

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		switch r.URL.Path {
		case "/api/cn/11111111111111/profile":
			_ = json.ConfigDefault.NewEncoder(w).Encode(sekaiapi.GetAnotherProfileResponse{
				User: sekaiapi.AnotherUser{
					UserID: 11111111111111,
					Name:   "CN User 1",
				},
				UserDeck: sekaiapi.UserDeck{
					DeckID:    9,
					Leader:    2001,
					SubLeader: 2002,
					Member1:   2001,
					Member2:   2002,
					Member3:   2003,
					Member4:   2004,
					Member5:   2005,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oldBaseURL := harukiConfig.Cfg.SekaiAPI.BaseURL
	oldToken := harukiConfig.Cfg.SekaiAPI.Token
	harukiConfig.Cfg.SekaiAPI.BaseURL = server.URL
	harukiConfig.Cfg.SekaiAPI.Token = "test-token"
	t.Cleanup(func() {
		harukiConfig.Cfg.SekaiAPI.BaseURL = oldBaseURL
		harukiConfig.Cfg.SekaiAPI.Token = oldToken
	})

	rc := NewRequestContext(ctx, &CommandRequest{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings:  service,
		Snapshots: provider,
		SekaiAPI:  sekaiapi.NewSekaiAPIClient(&harukiConfig.Cfg.SekaiAPI),
	})

	detail, snap, region, resp, err := resolveDeckRenderProfileSnapshotAndPublic(rc, "u1")
	if err != nil {
		t.Fatalf("resolveDeckRenderProfileSnapshotAndPublic() error = %v", err)
	}
	if snap == nil {
		t.Fatal("expected snapshot")
	}
	if detail == nil || detail.Nickname != "selector-snapshot" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if region != "cn" {
		t.Fatalf("expected resolved region cn, got %q", region)
	}
	if requestedPath != "/api/cn/11111111111111/profile" {
		t.Fatalf("unexpected public profile path: %q", requestedPath)
	}
	if resp == nil || resp.UserDeck.Member1 != 2001 || resp.UserDeck.Member5 != 2005 {
		t.Fatalf("unexpected public profile resp: %+v", resp)
	}
	if len(provider.selectors) != 1 {
		t.Fatalf("expected one snapshot selector, got %d", len(provider.selectors))
	}
	selector := provider.selectors[0]
	if selector.PJSKUserID != expectedBinding.PJSKUserID {
		t.Fatalf("expected selected binding uid %q, got %q", expectedBinding.PJSKUserID, selector.PJSKUserID)
	}
}

func TestExecuteDeckMySekaiAllowsCNRegion(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

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

	message, err := executeDeck(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleDeck,
		Mode:              "deck-mysekai",
		Region:            "cn",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Decks:      newHandlerTestDeckControllerForRegion(t, renderregion.CN, 640, "CN Deck Event"),
		Music:      newHandlerTestMusicControllerForRegion(t, renderregion.CN),
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

	message, err := executeDeck(NewRequestContext(context.Background(), &CommandRequest{
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

func TestExecuteDeckMySekaiResolvesFullSnapshot(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	raw := &rendersnapshot.RawUserData{
		Now:          1700000000000,
		UserGamedata: rendersnapshot.RawUserGamedata{UserID: 12345678901234, Name: "JPUser", Deck: 1, Rank: 100},
		UserProfile:  rendersnapshot.RawUserProfile{ProfileImageType: "default"},
		UserDecks: []rendersnapshot.RawUserDeck{{
			DeckID:    1,
			Leader:    1001,
			SubLeader: 1001,
			Member1:   1001,
			Member2:   1001,
			Member3:   1001,
			Member4:   1001,
			Member5:   1001,
		}},
		UserCards: []rendersnapshot.RawUserCard{{
			CardID:                1001,
			Level:                 60,
			SkillLevel:            4,
			MasterRank:            5,
			SpecialTrainingStatus: "done",
			DefaultImage:          "special_training",
		}},
	}
	rawBytes, err := rendersnapshot.EncodeRawUserData(raw)
	if err != nil {
		t.Fatalf("encode raw user data: %v", err)
	}

	provider := &runtimeSnapshotProviderStub{
		snapshot: &runtimeSnapshotStub{
			raw:      raw,
			rawBytes: rawBytes,
		},
	}

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

	message, err := executeDeck(NewRequestContext(ctx, &CommandRequest{
		Module:            parser.ModuleDeck,
		Mode:              "deck-mysekai",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings:  service,
		Snapshots: provider,
		Decks:     newHandlerTestDeckController(t),
		Music:     newHandlerTestMusicController(t),
		ImageCache: imagecache.New(
			"https://image-cache.test",
			t.TempDir(),
		),
	}))
	if err != nil {
		t.Fatalf("executeDeck() error = %v", err)
	}
	if len(message) != 2 {
		t.Fatalf("unexpected message: %+v", message)
	}
	if len(provider.resolveNeedFlags) != 1 || !provider.resolveNeedFlags[0] {
		t.Fatalf("unexpected snapshot resolve flags: %+v", provider.resolveNeedFlags)
	}
}

func TestExecuteDeckMySekaiRequiresVisibleMySekaiSnapshot(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := service.SetBindingMySekaiVisible(ctx, "qq", "42", "jp", false); err != nil {
		t.Fatalf("hide mysekai: %v", err)
	}

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

	_, err = executeDeck(NewRequestContext(ctx, &CommandRequest{
		Module:            parser.ModuleDeck,
		Mode:              "deck-mysekai",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings: service,
		Decks:    newHandlerTestDeckController(t),
		Music:    newHandlerTestMusicController(t),
	}))
	if err == nil || err.Error() != buildPrivateDataNotFoundMessage("mysekai", &accountdata.ResolvedBinding{
		Server:     "jp",
		PJSKUserID: "12345678901234",
		Visible:    false,
	}) {
		t.Fatalf("unexpected error: %v", err)
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

	message, err := executeDeck(NewRequestContext(context.Background(), &CommandRequest{
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

func TestExecuteDeckUsesSelectedBindingRegionBeforeResolvingCurrentEvent(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"jp": {
				"33333333333333": "JP User 1",
			},
			"cn": {
				"11111111111111": "CN User 1",
			},
		},
	})

	if _, err := service.Bind(ctx, "qq", "42", "33333333333333"); err != nil {
		t.Fatalf("bind jp account: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind cn account: %v", err)
	}

	sekaiClient := sekaienttest.Open(t, "sqlite3", "file:handler_test_execute_deck_selector_region?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = sekaiClient.Close() })
	now := time.Now().UnixMilli()
	seedHandlerTestWorldBloomEvent(t, ctx, sekaiClient, "cn", 640, now-int64(4*time.Hour/time.Millisecond), now+int64(4*time.Hour/time.Millisecond), []handlerTestWorldBloomChapter{
		{chapterNo: 1, startAt: now - int64(2*time.Hour/time.Millisecond), aggregateAt: now + int64(time.Hour/time.Millisecond), characterID: 21},
	})
	raw := &rendersnapshot.RawUserData{
		Now: now,
		UserGamedata: rendersnapshot.RawUserGamedata{
			UserID: 11111111111111,
			Name:   "CN User 1",
			Deck:   1,
			Rank:   100,
		},
		UserDecks: []rendersnapshot.RawUserDeck{{
			DeckID:    1,
			Leader:    1001,
			Member1:   1001,
			Member2:   1001,
			Member3:   1001,
			Member4:   1001,
			Member5:   1001,
			SubLeader: 1001,
		}},
		UserCards: []rendersnapshot.RawUserCard{{
			CardID:                1001,
			Level:                 60,
			SkillLevel:            4,
			MasterRank:            5,
			SpecialTrainingStatus: "done",
			DefaultImage:          "special_training",
		}},
		UserAreas: []rendersnapshot.RawUserArea{{AreaItems: []rendersnapshot.RawUserAreaItem{}}},
	}
	rawBytes, err := rendersnapshot.EncodeRawUserData(raw)
	if err != nil {
		t.Fatalf("encode raw snapshot: %v", err)
	}

	params, err := json.Marshal(map[string]any{
		"selector": "u2",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeDeck(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleDeck,
		Mode:              "deck-event",
		Region:            "jp",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Sekai:    sekaiClient,
		Bindings: service,
		Snapshots: &runtimeSnapshotProviderStub{
			snapshot: &runtimeSnapshotStub{
				raw:      raw,
				rawBytes: rawBytes,
			},
		},
		Decks:      newHandlerTestDeckControllerForRegion(t, renderregion.CN, 640, "CN Deck Event"),
		Music:      newHandlerTestMusicControllerForRegion(t, renderregion.CN),
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
	}))
	if err != nil {
		t.Fatalf("executeDeck() error = %v", err)
	}
	if len(message) != 2 {
		t.Fatalf("unexpected message: %+v", message)
	}
	textData, ok := message[0].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected text data: %+v", message[0].Data)
	}
	if !strings.Contains(textData.Text, "CN / 活动组卡 / event640") {
		t.Fatalf("expected selected binding region event summary, got %q", textData.Text)
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
	return newHandlerTestDeckControllerForRegion(t, renderregion.JP, 7, "Deck Event")
}

func newHandlerTestDeckControllerForRegion(t *testing.T, region renderregion.Value, eventID int, eventName string) *renderdeck.Controller {
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
			region: region,
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
			region: region,
			events: map[int]*masterdata.Event{
				eventID: {ID: eventID, Name: eventName, AssetBundleName: "deck_event_banner"},
			},
		},
		drawing.NewHarukiDrawingClient(drawingServer.URL),
		renderassets.NewAssetHelper(assetRoot, nil),
		nil,
		region,
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
		region: region,
		musics: map[int]*masterdata.Music{
			10000: {ID: 10000, Title: "おまかせ", AssetBundleName: "omakase"},
		},
	})
	if region != renderregion.JP {
		deckController.RegisterCardSource(&handlerDeckCardSource{
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
		})
		deckController.RegisterEventSource(&handlerDeckEventSource{
			region: renderregion.JP,
			events: map[int]*masterdata.Event{
				eventID: {ID: eventID, Name: eventName, AssetBundleName: "deck_event_banner"},
			},
		})
		deckController.RegisterMusicSource(&handlerDeckMusicSource{
			region: renderregion.JP,
			musics: map[int]*masterdata.Music{
				10000: {ID: 10000, Title: "おまかせ", AssetBundleName: "omakase"},
			},
		})
	}
	return deckController
}

func newHandlerTestMusicController(t *testing.T) *rendermusic.Controller {
	t.Helper()
	return newHandlerTestMusicControllerForRegion(t, renderregion.JP)
}

func newHandlerTestMusicControllerForRegion(t *testing.T, region renderregion.Value) *rendermusic.Controller {
	t.Helper()
	return rendermusic.NewController(&handlerDeckMusicSource{
		region: region,
		musics: map[int]*masterdata.Music{
			10000: {ID: 10000, Title: "おまかせ", AssetBundleName: "omakase"},
		},
	}, nil, renderassets.NewAssetHelper(t.TempDir(), nil), nil, nil)
}
