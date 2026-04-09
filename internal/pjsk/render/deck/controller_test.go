package deck

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"

	"github.com/klauspost/compress/zstd"
)

type testCardSource struct {
	region     renderregion.Value
	cards      map[int]*masterdata.Card
	characters map[int]*masterdata.Character
}

func (s *testCardSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	return s.cards[id], nil
}

func (s *testCardSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return s.characters[id], nil
}

type testEventSource struct {
	region renderregion.Value
	events map[int]*masterdata.Event
}

type testMusicMetaSource struct {
	data []byte
}

func (s *testMusicMetaSource) Get(string) []byte {
	return append([]byte(nil), s.data...)
}

func (s *testEventSource) DefaultRegion() renderregion.Value { return s.region }

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

func TestBuildAutoRecommendRequestRemoteServiceUsesExplicitEvent(t *testing.T) {
	server, masterdataRoot := newDeckRecommendStubServer(t)
	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})
	defer server.Close()

	eventID := 7
	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		Algorithm:     "ga",
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
	if request.DeckData[0].TotalPower == nil || *request.DeckData[0].TotalPower != 345678 {
		t.Fatalf("unexpected total power: %+v", request.DeckData[0].TotalPower)
	}
	if len(request.DeckData[0].CardData) != 2 {
		t.Fatalf("unexpected card count: %d", len(request.DeckData[0].CardData))
	}
	if request.DeckData[0].CardData[0].CardThumbnail.CardID != 1002 {
		t.Fatalf("unexpected card order: %+v", request.DeckData[0].CardData)
	}
}

func TestBuildAutoRecommendRequestSetsWorldBloomCharacterMetadata(t *testing.T) {
	server, masterdataRoot := newDeckRecommendStubServer(t)
	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})
	defer server.Close()

	eventID := 7
	worldBloomCharacterID := 20
	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:                "jp",
		RecommendType:         "event",
		Algorithm:             "ga",
		EventID:               &eventID,
		WorldBloomCharacterID: &worldBloomCharacterID,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if !request.IsWl {
		t.Fatalf("expected world bloom request: %+v", request)
	}
	if request.RecommendType != "wl" {
		t.Fatalf("unexpected recommend type: %q", request.RecommendType)
	}
	if request.WlCharaIconPath == nil || *request.WlCharaIconPath == "" {
		t.Fatalf("expected wl character icon: %+v", request.WlCharaIconPath)
	}
	if request.WlCharaName == nil || *request.WlCharaName != "晓山瑞希" {
		t.Fatalf("unexpected wl character name: %+v", request.WlCharaName)
	}
	if request.CharaName == nil || *request.CharaName != "晓山瑞希" {
		t.Fatalf("unexpected shared character name: %+v", request.CharaName)
	}
}

func TestBuildAutoRecommendRequestUsesExplicitRegionSources(t *testing.T) {
	server, masterdataRoot := newDeckRecommendStubServer(t)
	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})
	defer server.Close()
	now := time.Now().UnixMilli()

	controller.RegisterCardSource(&testCardSource{
		region: renderregion.CN,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				CharacterID:     1,
				CardRarityType:  "rarity_4",
				Attr:            "cute",
				AssetBundleName: "cn_card_1001",
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
				AssetBundleName:                 "cn_card_1002",
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
	})
	controller.RegisterEventSource(&testEventSource{
		region: renderregion.CN,
		events: map[int]*masterdata.Event{
			200: {
				ID:              200,
				Name:            "CN Deck Event",
				AssetBundleName: "cn_deck_event_banner",
				StartAt:         now - 1_000,
				AggregateAt:     now + 1_000,
			},
		},
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "cn",
		RecommendType: "event",
		Algorithm:     "ga",
		Limit:         2,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if request.Region != "cn" {
		t.Fatalf("unexpected request region: %s", request.Region)
	}
	if request.EventID == nil || *request.EventID != 200 {
		t.Fatalf("unexpected event id: %+v", request.EventID)
	}
	if request.EventName == nil || *request.EventName != "CN Deck Event" {
		t.Fatalf("unexpected event name: %+v", request.EventName)
	}
	if request.EventBannerPath == nil || !strings.Contains(*request.EventBannerPath, "asset/cn-assets/") || !strings.Contains(*request.EventBannerPath, "cn_deck_event_banner") {
		t.Fatalf("unexpected event banner path: %+v", request.EventBannerPath)
	}
	if len(request.DeckData) != 1 || len(request.DeckData[0].CardData) != 2 {
		t.Fatalf("unexpected deck data: %+v", request.DeckData)
	}
	if !strings.Contains(request.DeckData[0].CardData[0].CardThumbnail.CardThumbnailPath, "asset/cn-assets/") {
		t.Fatalf("expected cn card thumbnail path, got %q", request.DeckData[0].CardData[0].CardThumbnail.CardThumbnailPath)
	}
}

func TestBuildAutoRecommendRequestRequiresRemoteServiceWhenEngineEnabled(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{
		Enabled: true,
	})

	_, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
	})
	if err == nil {
		t.Fatalf("expected remote service configuration error")
	}
	if !strings.Contains(err.Error(), "deck recommend service is not configured") {
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

func TestBuildRecommendOptionBonusTargetsWithKeywords(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "bonus", AutoQuery{
		Region:        "jp",
		RecommendType: "bonus",
		Limit:         3,
		Args:          "150加成 160%",
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	targets, ok := option["target_bonus_list"].([]int)
	if !ok {
		t.Fatalf("unexpected target_bonus_list type: %T", option["target_bonus_list"])
	}
	if len(targets) != 2 || targets[0] != 150 || targets[1] != 160 {
		t.Fatalf("unexpected target bonus list: %+v", targets)
	}
}

func TestBuildRecommendOptionAppliesOverrides(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	eventID := 123
	musicID := 456
	worldBloomCharacterID := 21
	teammatePower := 260000
	teammateScoreUp := 210
	scoreUpLowerBound := 180.0
	option, err := controller.buildRecommendOption(renderregion.JP, "event", AutoQuery{
		Region:                       "jp",
		RecommendType:                "event",
		EventID:                      &eventID,
		Algorithm:                    "sa",
		LiveType:                     "multi",
		Target:                       "skill",
		MusicID:                      &musicID,
		MusicDiff:                    "expert",
		WorldBloomCharacterID:        &worldBloomCharacterID,
		FixedCards:                   []int{1001},
		FixedCharacters:              []int{21},
		Rarity4Config:                &CardConfigPatch{MasterMax: true},
		SingleCardConfigs:            []SingleCardConfigPatch{{CardID: 777, LevelMax: true, SkillMax: true}},
		MultiLiveTeammatePower:       &teammatePower,
		MultiLiveTeammateScoreUp:     &teammateScoreUp,
		MultiLiveScoreUpLowerBound:   &scoreUpLowerBound,
		SkillOrderChooseStrategy:     "max",
		SkillReferenceChooseStrategy: "average",
		KeepAfterTrainingState:       true,
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["algorithm"] != "sa" {
		t.Fatalf("unexpected algorithm: %+v", option["algorithm"])
	}
	if option["live_type"] != "multi" {
		t.Fatalf("unexpected live_type: %+v", option["live_type"])
	}
	if option["target"] != "skill" {
		t.Fatalf("unexpected target: %+v", option["target"])
	}
	if option["music_id"] != 456 || option["music_diff"] != "expert" {
		t.Fatalf("unexpected music fields: music_id=%+v music_diff=%+v", option["music_id"], option["music_diff"])
	}
	if option["event_id"] != 123 {
		t.Fatalf("unexpected event id: %+v", option["event_id"])
	}
	if option["world_bloom_character_id"] != 21 {
		t.Fatalf("unexpected world bloom character: %+v", option["world_bloom_character_id"])
	}
	if option["multi_live_teammate_power"] != 260000 || option["multi_live_teammate_score_up"] != 210 {
		t.Fatalf("unexpected teammate values: power=%+v score_up=%+v", option["multi_live_teammate_power"], option["multi_live_teammate_score_up"])
	}
	if option["multi_live_score_up_lower_bound"] != 180.0 {
		t.Fatalf("unexpected score up lower bound: %+v", option["multi_live_score_up_lower_bound"])
	}
	if option["skill_order_choose_strategy"] != "max" || option["skill_reference_choose_strategy"] != "average" {
		t.Fatalf("unexpected skill strategies: %+v / %+v", option["skill_order_choose_strategy"], option["skill_reference_choose_strategy"])
	}
	if option["keep_after_training_state"] != true {
		t.Fatalf("unexpected keep_after_training_state: %+v", option["keep_after_training_state"])
	}

	cfg, ok := option["rarity_4_config"].(map[string]interface{})
	if !ok || cfg["master_max"] != true {
		t.Fatalf("unexpected rarity_4_config: %+v", option["rarity_4_config"])
	}
	singleCardCfgs, ok := option["single_card_configs"].([]interface{})
	if !ok || len(singleCardCfgs) != 1 {
		t.Fatalf("unexpected single card configs: %+v", option["single_card_configs"])
	}
}

func TestBuildRecommendOptionSimulatedWorldBloomClearsEventID(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	worldBloomCharacterID := 21
	worldBloomTurn := 2
	option, err := controller.buildRecommendOption(renderregion.JP, "event", AutoQuery{
		Region:                "jp",
		RecommendType:         "event",
		EventUnit:             "piapro",
		WorldBloomCharacterID: &worldBloomCharacterID,
		WorldBloomEventTurn:   &worldBloomTurn,
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if value, ok := option["event_id"]; ok && value != nil {
		t.Fatalf("simulated world bloom should clear event_id: %+v", value)
	}
	if option["event_type"] != "world_bloom" {
		t.Fatalf("unexpected event_type: %+v", option["event_type"])
	}
}

func TestBuildRecommendOptionExplicitEventWorldBloomCharacterKeepsEventID(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	eventID := 112
	worldBloomCharacterID := 20
	option, err := controller.buildRecommendOption(renderregion.JP, "event", AutoQuery{
		Region:                "jp",
		RecommendType:         "event",
		EventID:               &eventID,
		EventUnit:             "school_refusal",
		WorldBloomCharacterID: &worldBloomCharacterID,
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["event_id"] != 112 {
		t.Fatalf("explicit event should be preserved: %+v", option["event_id"])
	}
	if option["world_bloom_character_id"] != 20 {
		t.Fatalf("unexpected world bloom character: %+v", option["world_bloom_character_id"])
	}
	if value, ok := option["event_unit"]; ok && value != nil {
		t.Fatalf("explicit event wl filter should not forward derived event_unit: %+v", value)
	}
}

func TestApplyCommonRecommendMetadataDoesNotBackfillMysekaiEvent(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	request := &drawing.DeckRequest{
		Region:        "jp",
		RecommendType: "mysekai",
	}

	controller.applyCommonRecommendMetadata(request, renderregion.JP, "mysekai", map[string]interface{}{
		"live_type": "mysekai",
	}, AutoQuery{
		Region:        "jp",
		RecommendType: "mysekai",
	})

	if request.EventID != nil {
		t.Fatalf("mysekai should not auto-fill current event: %+v", request.EventID)
	}
}

func TestBuildAutoRecommendRequestRemoteService(t *testing.T) {
	var masterdataCalls atomic.Int32
	var musicMetaCalls atomic.Int32
	var recommendCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch r.URL.Path {
		case "/update/masterdata":
			masterdataCalls.Add(1)
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode masterdata request: %v", err)
			}
			if payload["region"] != "jp" {
				t.Fatalf("unexpected masterdata region: %+v", payload["region"])
			}
			if strings.TrimSpace(payload["base_dir"].(string)) == "" {
				t.Fatalf("expected masterdata base_dir")
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))

		case "/update/musicmetas/string":
			musicMetaCalls.Add(1)
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode music meta request: %v", err)
			}
			data, _ := payload["data"].(string)
			var metas []map[string]interface{}
			if err := json.Unmarshal([]byte(data), &metas); err != nil || len(metas) == 0 {
				http.Error(w, "invalid music meta payload", http.StatusBadRequest)
				return
			}
			if musicID, ok := metas[0]["music_id"].(float64); !ok || int(musicID) != 10000 {
				http.Error(w, "unexpected music meta payload", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))

		case "/cache_userdata":
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode cache_userdata payload: %v", err)
			}
			if len(payloads) != 1 || len(payloads[0]) == 0 {
				t.Fatalf("unexpected cache_userdata payloads: %d", len(payloads))
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))

		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			if len(payloads) != 1 {
				t.Fatalf("unexpected recommend payloads: %d", len(payloads))
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]interface{})
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			first, ok := options[0].(map[string]interface{})
			if !ok || first["algorithm"] != "ga" {
				t.Fatalf("unexpected algorithm batch: %+v", payload["batch_options"])
			}
			if payload["userdata_hash"] != "test-userdata-hash" {
				t.Fatalf("unexpected userdata_hash: %+v", payload["userdata_hash"])
			}
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": 1234567,
							"live_score": 1234567,
							"mysekai_event_point": 0,
							"total_power": 345678,
							"event_bonus_rate": 25,
							"support_deck_bonus_rate": 10,
							"multi_live_score_up": 120,
							"cards": [
								{
									"card_id": 1002,
									"level": 60,
									"master_rank": 5,
									"skill_level": 4,
									"skill_score_up": 120,
									"event_bonus_rate": 25,
									"episode1_read": true,
									"episode2_read": true,
									"after_training": true,
									"default_image": "special_training",
									"has_canvas_bonus": false
								},
								{
									"card_id": 1001,
									"level": 50,
									"master_rank": 1,
									"skill_level": 4,
									"skill_score_up": 100,
									"event_bonus_rate": 20,
									"episode1_read": true,
									"episode2_read": true,
									"after_training": false,
									"default_image": "normal",
									"has_canvas_bonus": false
								}
							]
						}]
					}
				}
			]`))

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	masterdataRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(masterdataRoot, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir jp masterdata dir: %v", err)
	}

	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
	}, &testMusicMetaSource{
		data: []byte(`[{
			"music_id": 10000,
			"difficulty": "master",
			"music_time": 100,
			"event_rate": 120,
			"base_score": 1,
			"base_score_auto": 1,
			"skill_score_solo": [1,1,1,1,1,1],
			"skill_score_auto": [1,1,1,1,1,1],
			"skill_score_multi": [1,1,1,1,1,1],
			"fever_score": 1,
			"fever_end_time": 1,
			"tap_count": 100
		}]`),
	})

	eventID := 7
	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       &eventID,
		Algorithm:     "ga",
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if len(request.DeckData) != 1 || len(request.DeckData[0].CardData) != 2 {
		t.Fatalf("unexpected deck payload: %+v", request.DeckData)
	}
	if request.DeckData[0].CardData[0].CardThumbnail.CardID != 1002 {
		t.Fatalf("unexpected remote card order: %+v", request.DeckData[0].CardData)
	}
	if masterdataCalls.Load() != 1 || musicMetaCalls.Load() != 1 || recommendCalls.Load() != 1 {
		t.Fatalf("unexpected call counts: masterdata=%d musicmeta=%d recommend=%d", masterdataCalls.Load(), musicMetaCalls.Load(), recommendCalls.Load())
	}

	_, err = controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       &eventID,
		Algorithm:     "ga",
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("second BuildAutoRecommendRequest returned error: %v", err)
	}
	if masterdataCalls.Load() != 1 || musicMetaCalls.Load() != 1 || recommendCalls.Load() != 2 {
		t.Fatalf("unexpected cached call counts: masterdata=%d musicmeta=%d recommend=%d", masterdataCalls.Load(), musicMetaCalls.Load(), recommendCalls.Load())
	}
}

func TestBuildAutoRecommendRequestRemoteServiceBatchesAllAlgorithms(t *testing.T) {
	var recommendCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch r.URL.Path {
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode cache_userdata payload: %v", err)
			}
			if len(payloads) != 1 || len(payloads[0]) == 0 {
				t.Fatalf("unexpected cache_userdata payloads: %d", len(payloads))
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			if len(payloads) != 1 {
				t.Fatalf("unexpected recommend payloads: %d", len(payloads))
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]interface{})
			if !ok || len(options) != 2 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			if payload["userdata_hash"] != "test-userdata-hash" {
				t.Fatalf("unexpected userdata_hash: %+v", payload["userdata_hash"])
			}
			_, _ = w.Write([]byte(`[
				{
					"alg": "dfs",
					"cost_time": 1.0,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": 100,
							"live_score": 100,
							"mysekai_event_point": 0,
							"total_power": 200,
							"event_bonus_rate": 25,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up": 110,
							"cards": [{"card_id": 1001, "level": 50, "master_rank": 1, "skill_level": 4, "skill_score_up": 100, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": false, "default_image": "normal", "has_canvas_bonus": false}]
						}]
					}
				},
				{
					"alg": "ga",
					"cost_time": 2.0,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": 100,
							"live_score": 100,
							"mysekai_event_point": 0,
							"total_power": 200,
							"event_bonus_rate": 25,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up": 110,
							"cards": [{"card_id": 1001, "level": 50, "master_rank": 1, "skill_level": 4, "skill_score_up": 100, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": false, "default_image": "normal", "has_canvas_bonus": false}]
						}]
					}
				}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	masterdataRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(masterdataRoot, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir jp masterdata dir: %v", err)
	}

	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
		DefaultAlgs:    []string{"dfs", "ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	eventID := 7
	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       &eventID,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if recommendCalls.Load() != 1 {
		t.Fatalf("expected one batched recommend call, got %d", recommendCalls.Load())
	}
	if len(request.ModelName) != 1 || request.ModelName[0] != "dfs+ga" {
		t.Fatalf("unexpected model names: %+v", request.ModelName)
	}
}

func TestBuildAutoRecommendRequestRemoteServiceFallsBackToLegacyProtocol(t *testing.T) {
	var recommendCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch r.URL.Path {
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			http.NotFound(w, r)
		case "/recommend":
			recommendCalls.Add(1)
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode recommend request: %v", err)
			}
			if payload["algorithm"] != "ga" {
				t.Fatalf("unexpected legacy algorithm: %+v", payload["algorithm"])
			}
			if strings.TrimSpace(payload["user_data_str"].(string)) == "" {
				t.Fatalf("expected user_data_str in legacy payload")
			}
			_, _ = w.Write([]byte(`{"decks":[{"score":123,"live_score":123,"mysekai_event_point":0,"total_power":456,"event_bonus_rate":25,"support_deck_bonus_rate":0,"multi_live_score_up":110,"cards":[{"card_id":1001,"level":50,"master_rank":1,"skill_level":4,"skill_score_up":100,"event_bonus_rate":20,"episode1_read":true,"episode2_read":true,"after_training":false,"default_image":"normal","has_canvas_bonus":false}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	masterdataRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(masterdataRoot, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir jp masterdata dir: %v", err)
	}

	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	eventID := 7
	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       &eventID,
		Algorithm:     "ga",
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if recommendCalls.Load() != 1 {
		t.Fatalf("expected one legacy recommend call, got %d", recommendCalls.Load())
	}
	if len(request.DeckData) != 1 || len(request.DeckData[0].CardData) != 1 {
		t.Fatalf("unexpected request payload: %+v", request.DeckData)
	}
}

func newDeckRecommendStubServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch r.URL.Path {
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode cache_userdata payload: %v", err)
			}
			if len(payloads) != 1 || len(payloads[0]) == 0 {
				t.Fatalf("unexpected cache_userdata payloads: %d", len(payloads))
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			if len(payloads) != 1 {
				t.Fatalf("unexpected recommend payloads: %d", len(payloads))
			}
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": 1234567,
							"live_score": 1234567,
							"mysekai_event_point": 0,
							"total_power": 345678,
							"event_bonus_rate": 25,
							"support_deck_bonus_rate": 10,
							"multi_live_score_up": 120,
							"cards": [
								{
									"card_id": 1002,
									"level": 60,
									"master_rank": 5,
									"skill_level": 4,
									"skill_score_up": 120,
									"event_bonus_rate": 25,
									"episode1_read": true,
									"episode2_read": true,
									"after_training": true,
									"default_image": "special_training",
									"has_canvas_bonus": false
								},
								{
									"card_id": 1001,
									"level": 50,
									"master_rank": 1,
									"skill_level": 4,
									"skill_score_up": 100,
									"event_bonus_rate": 20,
									"episode1_read": true,
									"episode2_read": true,
									"after_training": false,
									"default_image": "normal",
									"has_canvas_bonus": false
								}
							]
						}]
					}
				}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))

	masterdataRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(masterdataRoot, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir jp masterdata dir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(masterdataRoot, "cn"), 0o755); err != nil {
		t.Fatalf("mkdir cn masterdata dir: %v", err)
	}

	return server, masterdataRoot
}

func newTestDeckController(t *testing.T, cfg RecommendConfig) *Controller {
	return newTestDeckControllerWithMeta(t, cfg, nil)
}

func newTestDeckControllerWithMeta(t *testing.T, cfg RecommendConfig, metaLoader MusicMetaSource) *Controller {
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
		characters: map[int]*masterdata.Character{
			1:  {ID: 1, FirstName: "星乃", GivenName: "一歌", Unit: "light_sound"},
			2:  {ID: 2, FirstName: "天马", GivenName: "咲希", Unit: "light_sound"},
			20: {ID: 20, FirstName: "晓山", GivenName: "瑞希", Unit: "school_refusal"},
		},
	}
	eventSource := &testEventSource{
		region: renderregion.JP,
		events: map[int]*masterdata.Event{
			7: {
				ID:              7,
				Name:            "Deck Event",
				AssetBundleName: "deck_event_banner",
			},
		},
	}

	return NewControllerWithConfig(cardSource, eventSource, nil, assetHelper, snapshot, renderregion.JP, cfg, metaLoader)
}

func decodeDeckMultipartPayload(body io.Reader, out *[][]byte) error {
	compressed, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	reader, err := zstd.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	buf := bytes.NewReader(raw)
	var items [][]byte
	for buf.Len() > 0 {
		var size uint32
		if err := binary.Read(buf, binary.BigEndian, &size); err != nil {
			return err
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(buf, payload); err != nil {
			return err
		}
		items = append(items, payload)
	}
	*out = items
	return nil
}
