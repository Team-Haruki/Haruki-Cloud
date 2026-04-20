package deck

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"

	"github.com/klauspost/compress/zstd"
)

type testCardSource struct {
	region            renderregion.Value
	cards             map[int]*masterdata.Card
	characters        map[int]*masterdata.Character
	episodes          map[int][]snapshot.RawUserCardEpisode
	areaItemLevelCaps map[int]int
	mysekaiGates      []snapshot.RawUserMysekaiGate
	fixtureBonuses    []snapshot.RawUserFixtureBonus
}

func (s *testCardSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testCardSource) GetCardByID(id int) (*masterdata.Card, error) {
	return s.cards[id], nil
}

func (s *testCardSource) GetAllCards() ([]*masterdata.Card, error) {
	result := make([]*masterdata.Card, 0, len(s.cards))
	for _, cardInfo := range s.cards {
		result = append(result, cardInfo)
	}
	return result, nil
}

func (s *testCardSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	return s.characters[id], nil
}

func (s *testCardSource) GetUnitByCardID(cardID int) (string, error) {
	cardInfo := s.cards[cardID]
	if cardInfo == nil {
		return "", nil
	}
	if strings.TrimSpace(cardInfo.SupportUnit) != "" && !strings.EqualFold(cardInfo.SupportUnit, "none") {
		return cardInfo.SupportUnit, nil
	}
	character := s.characters[cardInfo.CharacterID]
	if character == nil {
		return "", nil
	}
	return character.Unit, nil
}

func (s *testCardSource) GetCardEpisodes(cardID int) ([]snapshot.RawUserCardEpisode, error) {
	return slices.Clone(s.episodes[cardID]), nil
}

func (s *testCardSource) AreaItemLevelCaps(limit int) map[int]int {
	if len(s.areaItemLevelCaps) == 0 {
		return nil
	}
	result := make(map[int]int, len(s.areaItemLevelCaps))
	for itemID, maxLevel := range s.areaItemLevelCaps {
		level := maxLevel
		if limit > 0 && level > limit {
			level = limit
		}
		result[itemID] = level
	}
	return result
}

func (s *testCardSource) GetMaxProfileMysekaiGates() []snapshot.RawUserMysekaiGate {
	return slices.Clone(s.mysekaiGates)
}

func (s *testCardSource) GetMaxProfileMysekaiFixtureBonuses() []snapshot.RawUserFixtureBonus {
	return slices.Clone(s.fixtureBonuses)
}

type testEventSource struct {
	region renderregion.Value
	events map[int]*masterdata.Event
}

type testMusicSource struct {
	region renderregion.Value
	musics map[int]*masterdata.Music
}

type testMusicMetaSource struct {
	data []byte
}

func (s *testMusicMetaSource) Get(string) []byte {
	return append([]byte(nil), s.data...)
}

func (s *testEventSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testMusicSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testEventSource) GetEventByID(id int) (*masterdata.Event, error) {
	return s.events[id], nil
}

func (s *testMusicSource) GetMusicByID(id int) (*masterdata.Music, error) {
	return s.musics[id], nil
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
	if request.SkillOrderChooseStrategy == nil || *request.SkillOrderChooseStrategy != "average" {
		t.Fatalf("unexpected skill order strategy: %+v", request.SkillOrderChooseStrategy)
	}
	if request.SkillReferenceChooseStrategy == nil || *request.SkillReferenceChooseStrategy != "average" {
		t.Fatalf("unexpected skill reference strategy: %+v", request.SkillReferenceChooseStrategy)
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

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:                "jp",
		RecommendType:         "event",
		Algorithm:             "ga",
		EventID:               new(7),
		Boost:                 new(5),
		WorldBloomCharacterID: new(20),
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
	if request.Boost == nil || *request.Boost != 5 {
		t.Fatalf("unexpected request boost: %+v", request.Boost)
	}
	if request.Profile.Source != "" || request.Profile.Mode != nil {
		t.Fatalf("expected deck profile source details to be hidden, got %+v", request.Profile)
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

func TestBuildDrawingRequestFromRecommendResultFallsBackToJPCardSource(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	controller.RegisterCardSource(&testCardSource{
		region: renderregion.CN,
		cards:  map[int]*masterdata.Card{},
	})

	request, err := controller.buildDrawingRequestFromRecommendResult(
		renderregion.CN,
		"no_event",
		AutoQuery{Region: "cn"},
		map[string]any{},
		nil,
		&RecommendResult{
			Decks: []RecommendDeck{
				{
					Cards: []RecommendCard{
						{
							CardID:       1001,
							Level:        50,
							MasterRank:   1,
							DefaultImage: "normal",
							SkillLevel:   4,
							SkillRate:    100,
						},
					},
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildDrawingRequestFromRecommendResult returned error: %v", err)
	}

	if len(request.DeckData) != 1 || len(request.DeckData[0].CardData) != 1 {
		t.Fatalf("expected one fallback card, got %+v", request.DeckData)
	}
	got := request.DeckData[0].CardData[0].CardThumbnail.CardThumbnailPath
	if !strings.Contains(got, "asset/cn-assets/") || !strings.Contains(got, "card_1001_normal.png") {
		t.Fatalf("unexpected fallback thumbnail path: %q", got)
	}
}

func TestBuildDrawingRequestFromRecommendResultNormalizesDisplayRates(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	request, err := controller.buildDrawingRequestFromRecommendResult(
		renderregion.JP,
		"no_event",
		AutoQuery{Region: "jp"},
		map[string]any{},
		nil,
		&RecommendResult{
			Decks: []RecommendDeck{
				{
					Cards: []RecommendCard{
						{
							CardID:       1001,
							Level:        50,
							MasterRank:   1,
							DefaultImage: "normal",
							SkillLevel:   4,
							SkillRate:    112.4999999,
						},
					},
					EventBonusRate:       120.0000001,
					SupportDeckBonusRate: 10.2499999,
					MultiLiveScoreUp:     233.2499999,
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildDrawingRequestFromRecommendResult returned error: %v", err)
	}

	card := request.DeckData[0].CardData[0]
	if card.SkillRate != 112.5 {
		t.Fatalf("unexpected normalized skill rate: %+v", card.SkillRate)
	}
	if request.DeckData[0].EventBonusRate == nil || *request.DeckData[0].EventBonusRate != 120 {
		t.Fatalf("unexpected normalized event bonus: %+v", request.DeckData[0].EventBonusRate)
	}
	if request.DeckData[0].SupportDeckBonusRate == nil || *request.DeckData[0].SupportDeckBonusRate != 10.2 {
		t.Fatalf("unexpected normalized support bonus: %+v", request.DeckData[0].SupportDeckBonusRate)
	}
	if request.DeckData[0].MultiLiveScoreUp == nil || *request.DeckData[0].MultiLiveScoreUp != 233.2 {
		t.Fatalf("unexpected normalized multi live score up: %+v", request.DeckData[0].MultiLiveScoreUp)
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
	if option["algorithm"] != "all" {
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

func TestBuildRecommendOptionDefaultsNoEventToAllAlgorithms(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "no_event", AutoQuery{
		Region:        "jp",
		RecommendType: "no_event",
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["algorithm"] != "all" {
		t.Fatalf("unexpected no_event algorithm: %+v", option["algorithm"])
	}
	if option["live_type"] != "multi" {
		t.Fatalf("unexpected live_type: %+v", option["live_type"])
	}
	if value, ok := option["event_id"]; ok && value != nil {
		t.Fatalf("expected no_event to clear event_id: %+v", value)
	}
}

func TestBuildRecommendOptionKeepsNoEventAllWithConfiguredAlgorithms(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{
		DefaultAlgs: []string{"dfs", "sa"},
	})

	option, err := controller.buildRecommendOption(renderregion.JP, "no_event", AutoQuery{
		Region:        "jp",
		RecommendType: "no_event",
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["algorithm"] != "all" {
		t.Fatalf("unexpected no_event algorithm with configured algs: %+v", option["algorithm"])
	}
}

func TestBuildRecommendOptionDefaultsMySekaiToAllAlgorithms(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "mysekai", AutoQuery{
		Region:        "jp",
		RecommendType: "mysekai",
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["algorithm"] != "all" {
		t.Fatalf("unexpected mysekai algorithm: %+v", option["algorithm"])
	}
	if option["live_type"] != "mysekai" {
		t.Fatalf("unexpected live_type: %+v", option["live_type"])
	}
	if option["event_id"] != 7 {
		t.Fatalf("expected mysekai to inherit current event for engine bonus calculation, got %+v", option["event_id"])
	}
}

func TestBuildRecommendOptionAppliesOverrides(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "event", AutoQuery{
		Region:                       "jp",
		RecommendType:                "event",
		EventID:                      new(123),
		Algorithm:                    "dfs-ga",
		LiveType:                     "multi",
		Target:                       "skill",
		MusicID:                      new(456),
		MusicDiff:                    "expert",
		WorldBloomCharacterID:        new(21),
		FixedCards:                   []int{1001},
		FixedCharacters:              []int{21},
		Rarity4Config:                &CardConfigPatch{MasterMax: true},
		SingleCardConfigs:            []SingleCardConfigPatch{{CardID: 777, LevelMax: true, SkillMax: true}},
		MultiLiveTeammatePower:       new(260000),
		MultiLiveTeammateScoreUp:     new(210),
		MultiLiveScoreUpLowerBound:   new(180.0),
		SkillOrderChooseStrategy:     "max",
		SkillReferenceChooseStrategy: "average",
		KeepAfterTrainingState:       true,
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["algorithm"] != "dfs_ga" {
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

	cfg, ok := option["rarity_4_config"].(map[string]any)
	if !ok || cfg["master_max"] != true {
		t.Fatalf("unexpected rarity_4_config: %+v", option["rarity_4_config"])
	}
	singleCardCfgs, ok := option["single_card_configs"].([]any)
	if !ok || len(singleCardCfgs) != 1 {
		t.Fatalf("unexpected single card configs: %+v", option["single_card_configs"])
	}
}

func TestBuildRecommendOptionAppliesExtendedOverrides(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "event", AutoQuery{
		Region:              "jp",
		RecommendType:       "event",
		Boost:               new(5),
		AreaItemLevel:       new(15),
		UnitFilter:          "idol",
		AttrFilter:          "cool",
		ExcludedCards:       []int{1001, 1002},
		UseCurrentDeck:      true,
		MaxProfile:          true,
		SubMaxProfile:       true,
		MusicCompare:        true,
		MusicCompareQueries: []string{"龙hard", "虾expert", "sage"},
		SpecificSkillOrder:  []int{0, 1, 2, 3, 4},
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["boost"] != 5 {
		t.Fatalf("unexpected boost: %+v", option["boost"])
	}
	if option["area_item_level"] != 15 {
		t.Fatalf("unexpected area_item_level: %+v", option["area_item_level"])
	}
	if option["unit_filter"] != "idol" || option["attr_filter"] != "cool" {
		t.Fatalf("unexpected filters: unit=%+v attr=%+v", option["unit_filter"], option["attr_filter"])
	}
	excludedCards, ok := option["excluded_cards"].([]int)
	if !ok || !reflect.DeepEqual(excludedCards, []int{1001, 1002}) {
		t.Fatalf("unexpected excluded_cards: %+v", option["excluded_cards"])
	}
	if option["use_current_deck"] != true {
		t.Fatalf("unexpected use_current_deck: %+v", option["use_current_deck"])
	}
	if option["max_profile"] != true || option["sub_max_profile"] != true {
		t.Fatalf("unexpected profile flags: max=%+v sub=%+v", option["max_profile"], option["sub_max_profile"])
	}
	if option["music_compare"] != true {
		t.Fatalf("unexpected music_compare: %+v", option["music_compare"])
	}
	musicCompareQueries, ok := option["music_compare_queries"].([]string)
	if !ok || !reflect.DeepEqual(musicCompareQueries, []string{"龙hard", "虾expert", "sage"}) {
		t.Fatalf("unexpected music_compare_queries: %+v", option["music_compare_queries"])
	}
	specificSkillOrder, ok := option["specific_skill_order"].([]int)
	if !ok || !reflect.DeepEqual(specificSkillOrder, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("unexpected specific_skill_order: %+v", option["specific_skill_order"])
	}
	if option["skill_order_choose_strategy"] != "specific" {
		t.Fatalf("unexpected skill_order_choose_strategy: %+v", option["skill_order_choose_strategy"])
	}
}

func TestApplyOptionRequestFieldsCarriesBoostAcrossRecommendModes(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	cases := []struct {
		name   string
		option map[string]any
	}{
		{
			name: "event_wl",
			option: map[string]any{
				"boost":     5,
				"live_type": "multi",
				"target":    "score",
			},
		},
		{
			name: "no_event_float_boost",
			option: map[string]any{
				"boost":     float64(5),
				"live_type": "multi",
				"target":    "score",
			},
		},
		{
			name: "mysekai",
			option: map[string]any{
				"boost":     5,
				"live_type": "mysekai",
				"target":    "score",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &drawing.DeckRequest{}
			controller.applyOptionRequestFields(request, tc.option, AutoQuery{})
			if request.Boost == nil || *request.Boost != 5 {
				t.Fatalf("unexpected request boost: %+v", request.Boost)
			}
		})
	}
}

func TestBuildRecommendOptionSkillOrderSpecificStrategy(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "event", AutoQuery{
		Region:                   "jp",
		RecommendType:            "event",
		UseCurrentDeck:           true,
		SpecificSkillOrder:       []int{0, 4, 1, 2, 3},
		SkillOrderChooseStrategy: "specific",
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["skill_order_choose_strategy"] != "specific" {
		t.Fatalf("unexpected skill_order_choose_strategy: %+v", option["skill_order_choose_strategy"])
	}
	specificSkillOrder, ok := option["specific_skill_order"].([]int)
	if !ok || !reflect.DeepEqual(specificSkillOrder, []int{0, 4, 1, 2, 3}) {
		t.Fatalf("unexpected specific_skill_order: %+v", option["specific_skill_order"])
	}
}

func TestBuildRecommendOptionDefaultsChallengeLiveTypeAndStrategies(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "challenge", AutoQuery{
		Region:        "jp",
		RecommendType: "challenge",
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["live_type"] != "challenge" {
		t.Fatalf("unexpected live_type: %+v", option["live_type"])
	}
	if option["skill_order_choose_strategy"] != "max" {
		t.Fatalf("unexpected skill_order_choose_strategy: %+v", option["skill_order_choose_strategy"])
	}
	if option["skill_reference_choose_strategy"] != "max" {
		t.Fatalf("unexpected skill_reference_choose_strategy: %+v", option["skill_reference_choose_strategy"])
	}
}

func TestPrepareRecommendUserDataPreservesUnknownSnapshotFields(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	raw := &snapshot.RawUserData{
		Now:          1700000000000,
		UserGamedata: snapshot.RawUserGamedata{UserID: 10001, Name: "Deck User", Deck: 1},
		UserProfile:  snapshot.RawUserProfile{ProfileImageType: "default"},
		UserCards: []snapshot.RawUserCard{
			{
				CardID:                1001,
				Level:                 50,
				SkillLevel:            4,
				MasterRank:            1,
				SpecialTrainingStatus: "not_done",
				DefaultImage:          "normal",
			},
		},
		UserAreas: []snapshot.RawUserArea{
			{AreaItems: []snapshot.RawUserAreaItem{{AreaItemID: 1, Level: 3}}},
		},
	}
	originalBytes := []byte(`{
		"now":1700000000000,
		"userGamedata":{"userId":10001,"name":"Deck User","deck":1},
		"userProfile":{"profileImageType":"default"},
		"userCards":[{"cardId":1001,"userId":10001,"level":50,"skillLevel":4,"masterRank":1,"specialTrainingStatus":"not_done","defaultImage":"normal","createdAt":"2026-04-15T17:00:00Z","duplicateCount":2,"exp":123,"skillExp":456,"totalExp":789,"totalSkillExp":987,"episodes":null}],
		"userAreas":[{"areaItems":[{"areaItemId":1,"level":3,"updatedAt":"2026-04-15T17:00:00Z"}]}],
		"userDecks":[{"deckId":1,"leader":1001,"subLeader":1002,"member1":1001,"member2":1002,"member3":1003,"member4":1004,"member5":1005}],
		"userCustomUnknown":{"keep":"me"}
	}`)

	userBytes, err := encodePreparedRecommendUserData(originalBytes, raw, raw)
	if err != nil {
		t.Fatalf("encodePreparedRecommendUserData() error = %v", err)
	}

	text := string(userBytes)
	if !strings.Contains(text, `"userCustomUnknown":{"keep":"me"}`) {
		t.Fatalf("expected unknown snapshot fields to be preserved: %s", text)
	}
	if !strings.Contains(text, `"userDecks":[{"deckId":1`) {
		t.Fatalf("expected original unmodeled top-level fields to remain: %s", text)
	}
	for _, fragment := range []string{
		`"createdAt":"2026-04-15T17:00:00Z"`,
		`"duplicateCount":2`,
		`"exp":123`,
		`"skillExp":456`,
		`"totalExp":789`,
		`"totalSkillExp":987`,
		`"userId":10001`,
		`"updatedAt":"2026-04-15T17:00:00Z"`,
		`"userAreaStatus":{}`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected nested snapshot field %s to be preserved: %s", fragment, text)
		}
	}
	if strings.Contains(text, `"episodes":null`) {
		t.Fatalf("expected nil nested collections to be omitted or normalized: %s", text)
	}
	if !strings.Contains(text, `"userCards":[{`) || !strings.Contains(text, `"userAreas":[{`) {
		t.Fatalf("expected prepared userCards/userAreas to remain populated: %s", text)
	}

	_ = controller
}

func TestBuildRecommendOptionMapsChallengeAutoLiveType(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "challenge", AutoQuery{
		Region:        "jp",
		RecommendType: "challenge",
		LiveType:      "auto",
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["live_type"] != "challenge_auto" {
		t.Fatalf("unexpected live_type: %+v", option["live_type"])
	}
	if option["skill_order_choose_strategy"] != "average" {
		t.Fatalf("unexpected skill_order_choose_strategy: %+v", option["skill_order_choose_strategy"])
	}
	if option["skill_reference_choose_strategy"] != "average" {
		t.Fatalf("unexpected skill_reference_choose_strategy: %+v", option["skill_reference_choose_strategy"])
	}
}

func TestBuildRecommendOptionSimulatedWorldBloomClearsEventID(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.JP, "event", AutoQuery{
		Region:                "jp",
		RecommendType:         "event",
		EventUnit:             "piapro",
		WorldBloomCharacterID: new(21),
		WorldBloomEventTurn:   new(2),
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

	option, err := controller.buildRecommendOption(renderregion.JP, "event", AutoQuery{
		Region:                "jp",
		RecommendType:         "event",
		EventID:               new(112),
		EventUnit:             "school_refusal",
		WorldBloomCharacterID: new(20),
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

func TestBuildRecommendOptionExplicitEventIgnoresResolvedWorldBloomTurn(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	option, err := controller.buildRecommendOption(renderregion.CN, "event", AutoQuery{
		Region:                "cn",
		RecommendType:         "event",
		EventID:               new(170),
		EventUnit:             "school_refusal",
		WorldBloomEventTurn:   new(2),
		WorldBloomCharacterID: new(20),
	})
	if err != nil {
		t.Fatalf("buildRecommendOption returned error: %v", err)
	}

	if option["event_id"] != 170 {
		t.Fatalf("explicit event should be preserved: %+v", option["event_id"])
	}
	if value, ok := option["world_bloom_event_turn"]; ok && value != nil {
		t.Fatalf("resolved explicit event should not keep simulated world bloom turn: %+v", value)
	}
	if value, ok := option["event_type"]; ok && value != nil {
		t.Fatalf("resolved explicit event should not force fake event_type: %+v", value)
	}
	if option["world_bloom_character_id"] != 20 {
		t.Fatalf("unexpected world bloom character: %+v", option["world_bloom_character_id"])
	}
}

func TestApplyCommonRecommendMetadataDoesNotBackfillMysekaiEvent(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	request := &drawing.DeckRequest{
		Region:        "jp",
		RecommendType: "mysekai",
	}

	controller.applyCommonRecommendMetadata(request, renderregion.JP, "mysekai", map[string]any{
		"live_type": "mysekai",
	}, AutoQuery{
		Region:        "jp",
		RecommendType: "mysekai",
	})

	if request.EventID != nil {
		t.Fatalf("mysekai should not auto-fill current event: %+v", request.EventID)
	}
}

func TestBuildDrawingRequestFromRecommendResultDoesNotExposeImplicitMysekaiEventMetadata(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	request, err := controller.buildDrawingRequestFromRecommendResult(
		renderregion.JP,
		"mysekai",
		AutoQuery{Region: "jp", RecommendType: "mysekai"},
		map[string]any{"live_type": "mysekai", "event_id": 7, "target": "score"},
		nil,
		&RecommendResult{
			Decks: []RecommendDeck{{
				Cards: []RecommendCard{{
					CardID:       1001,
					Level:        50,
					MasterRank:   1,
					SkillLevel:   4,
					SkillRate:    100,
					DefaultImage: "normal",
				}},
				Score:             123,
				MysekaiEventPoint: 456,
				EventBonusRate:    25,
			}},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildDrawingRequestFromRecommendResult() error = %v", err)
	}
	if request.EventID != nil {
		t.Fatalf("implicit mysekai event should stay hidden in metadata: %+v", request.EventID)
	}
}

func TestApplyCommonRecommendMetadataKeepsMysekaiWorldBloomChapterMetadata(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	request := &drawing.DeckRequest{
		Region:        "jp",
		RecommendType: "mysekai",
	}

	controller.applyCommonRecommendMetadata(request, renderregion.JP, "mysekai", map[string]any{
		"live_type":                "mysekai",
		"event_id":                 7,
		"world_bloom_character_id": 20,
	}, AutoQuery{
		Region:                "jp",
		RecommendType:         "mysekai",
		EventID:               new(7),
		WorldBloomCharacterID: new(20),
	})

	if !request.IsWl {
		t.Fatalf("expected mysekai request to preserve wl metadata: %+v", request)
	}
	if request.RecommendType != "mysekai" {
		t.Fatalf("unexpected recommend type: %q", request.RecommendType)
	}
	if request.EventID == nil || *request.EventID != 7 {
		t.Fatalf("unexpected event id: %+v", request.EventID)
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

func TestApplyCommonRecommendMetadataKeepsImplicitMysekaiWorldBloomBadgeWithoutEventTitle(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	request := &drawing.DeckRequest{
		Region:        "jp",
		RecommendType: "mysekai",
	}

	controller.applyCommonRecommendMetadata(request, renderregion.JP, "mysekai", map[string]any{
		"live_type": "mysekai",
	}, AutoQuery{
		Region:                        "jp",
		RecommendType:                 "mysekai",
		MetadataWorldBloomCharacterID: new(20),
	})

	if !request.IsWl {
		t.Fatalf("expected mysekai request to preserve implicit wl metadata: %+v", request)
	}
	if request.EventID != nil {
		t.Fatalf("implicit mysekai WL metadata should not change title event id: %+v", request.EventID)
	}
	if request.WlCharaName == nil || *request.WlCharaName != "晓山瑞希" {
		t.Fatalf("unexpected wl character name: %+v", request.WlCharaName)
	}
}

func TestBuildAutoRecommendRequestChallengeAllFansOutCharacters(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"userdata_hash":"challenge-all-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != challengeCharacterCount {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			response := make([]map[string]any, 0, len(options))
			for index, rawOption := range options {
				option, ok := rawOption.(map[string]any)
				if !ok {
					t.Fatalf("unexpected batch option payload: %+v", rawOption)
				}
				charID, ok := option["challenge_live_character_id"].(float64)
				if !ok || int(charID) != index+1 {
					t.Fatalf("unexpected challenge_live_character_id at %d: %+v", index, option["challenge_live_character_id"])
				}
				score := 1000000 + int(charID)
				response = append(response, map[string]any{
					"alg":       "ga",
					"cost_time": 0.5,
					"wait_time": 0.0,
					"result": map[string]any{
						"decks": []map[string]any{{
							"score":                   score,
							"live_score":              score,
							"mysekai_event_point":     0,
							"total_power":             345678,
							"event_bonus_rate":        0,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up":     120,
							"cards": []map[string]any{{
								"card_id":          1001,
								"level":            50,
								"master_rank":      1,
								"skill_level":      4,
								"skill_score_up":   100,
								"event_bonus_rate": 0,
								"episode1_read":    true,
								"episode2_read":    true,
								"after_training":   false,
								"default_image":    "normal",
								"has_canvas_bonus": false,
							}},
						}},
					},
				})
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				t.Fatalf("encode recommend response: %v", err)
			}
			_, _ = w.Write(encoded)
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

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "challenge",
		Algorithm:     "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if recommendCalls.Load() != 1 {
		t.Fatalf("expected 1 recommend call, got %d", recommendCalls.Load())
	}
	if len(request.DeckData) != 26 {
		t.Fatalf("unexpected challenge-all deck count: %d", len(request.DeckData))
	}
	if request.DeckData[0].ChallengeScoreDelta == nil || *request.DeckData[0].ChallengeScoreDelta != 1 {
		t.Fatalf("unexpected first challenge score delta: %+v", request.DeckData[0].ChallengeScoreDelta)
	}
	if request.DeckData[20].ChallengeScoreDelta == nil || *request.DeckData[20].ChallengeScoreDelta != -99979 {
		t.Fatalf("unexpected miku challenge score delta: %+v", request.DeckData[20].ChallengeScoreDelta)
	}
	if request.CharaName != nil {
		t.Fatalf("challenge-all request should not set single character metadata: %+v", request.CharaName)
	}
}

func TestBuildAutoRecommendRequestChallengeCurrentRequiresCharacter(t *testing.T) {
	server, masterdataRoot := newDeckRecommendStubServer(t)
	defer server.Close()

	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	_, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:         "jp",
		RecommendType:  "challenge",
		UseCurrentDeck: true,
	})
	if err == nil {
		t.Fatalf("expected challenge current to require a character")
	}
	if !strings.Contains(err.Error(), "需要指定挑战组卡角色") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildAutoRecommendRequestChallengeCurrentUsesSnapshotDeck(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"userdata_hash":"challenge-current-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			option, ok := options[0].(map[string]any)
			if !ok {
				t.Fatalf("unexpected batch option payload: %+v", options[0])
			}
			if option["challenge_live_character_id"] != 21.0 {
				t.Fatalf("unexpected challenge_live_character_id: %+v", option["challenge_live_character_id"])
			}
			fixedCards, ok := option["fixed_cards"].([]any)
			if !ok || len(fixedCards) != 5 {
				t.Fatalf("unexpected fixed_cards: %+v", option["fixed_cards"])
			}
			expected := []float64{1001, 1002, 1003, 1004, 1005}
			for index, value := range fixedCards {
				if value != expected[index] {
					t.Fatalf("unexpected fixed card %d: %+v", index, value)
				}
			}
			if option["best_skill_as_leader"] != false {
				t.Fatalf("expected best_skill_as_leader to be disabled: %+v", option["best_skill_as_leader"])
			}
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": 1200000,
							"live_score": 1200000,
							"mysekai_event_point": 0,
							"total_power": 456789,
							"event_bonus_rate": 0,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up": 120,
							"cards": [
								{"card_id": 1001, "level": 50, "master_rank": 1, "skill_level": 4, "skill_score_up": 100, "event_bonus_rate": 0, "episode1_read": true, "episode2_read": true, "after_training": false, "default_image": "normal", "has_canvas_bonus": false},
								{"card_id": 1002, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 0, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false},
								{"card_id": 1003, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 0, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false},
								{"card_id": 1004, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 0, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false},
								{"card_id": 1005, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 0, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false}
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
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:                   "jp",
		RecommendType:            "challenge",
		ChallengeLiveCharacterID: new(21),
		UseCurrentDeck:           true,
		Algorithm:                "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if recommendCalls.Load() != 1 {
		t.Fatalf("expected one recommend call, got %d", recommendCalls.Load())
	}
	if !reflect.DeepEqual(request.FixedCardsID, []int{1001, 1002, 1003, 1004, 1005}) {
		t.Fatalf("unexpected fixed cards in request: %+v", request.FixedCardsID)
	}
	if request.CharaName == nil || *request.CharaName != "初音未来" {
		t.Fatalf("unexpected challenge character metadata: %+v", request.CharaName)
	}
	if len(request.DeckData) != 1 || request.DeckData[0].ChallengeScoreDelta == nil || *request.DeckData[0].ChallengeScoreDelta != 100000 {
		t.Fatalf("unexpected challenge current delta: %+v", request.DeckData)
	}
}

func TestBuildAutoRecommendRequestChallengeMusicCompareRequiresCharacter(t *testing.T) {
	server, masterdataRoot := newDeckRecommendStubServer(t)
	defer server.Close()

	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	_, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "challenge",
		MusicCompare:  true,
	})
	if err == nil {
		t.Fatalf("expected challenge music compare to require a character")
	}
	if !strings.Contains(err.Error(), "必须指定一个角色") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildAutoRecommendRequestChallengeMusicCompareCurrentDeckBuildsCandidatesAndSorts(t *testing.T) {
	var recommendCalls atomic.Int32
	seenMusicIDs := make([]int, 0, 3)

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
			_, _ = w.Write([]byte(`{"userdata_hash":"challenge-compare-current-userdata-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			option := options[0].(map[string]any)
			if option["challenge_live_character_id"] != 21.0 {
				t.Fatalf("unexpected challenge_live_character_id: %+v", option["challenge_live_character_id"])
			}
			if option["limit"] != 1.0 {
				t.Fatalf("unexpected compare limit: %+v", option["limit"])
			}
			fixedCards, ok := option["fixed_cards"].([]any)
			if !ok || len(fixedCards) != 5 {
				t.Fatalf("unexpected fixed_cards: %+v", option["fixed_cards"])
			}
			if option["best_skill_as_leader"] != false {
				t.Fatalf("expected compare mode to disable best_skill_as_leader: %+v", option["best_skill_as_leader"])
			}

			musicID := int(option["music_id"].(float64))
			seenMusicIDs = append(seenMusicIDs, musicID)
			scoreByMusicID := map[int]int{
				1: 1200000,
				2: 1300000,
				3: 1250000,
			}
			score := scoreByMusicID[musicID]
			_, _ = w.Write([]byte(fmt.Sprintf(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": %d,
							"live_score": %d,
							"mysekai_event_point": 0,
							"total_power": 200000,
							"event_bonus_rate": 25,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up": 110,
							"cards": [{"card_id":1001,"level":50,"master_rank":1,"skill_level":4,"skill_score_up":100,"event_bonus_rate":20,"episode1_read":true,"episode2_read":true,"after_training":false,"default_image":"normal","has_canvas_bonus":false}]
						}]
					}
				}
			]`, score, score)))
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
		data: []byte(`[
			{"music_id":1,"difficulty":"master","music_time":100,"event_rate":100,"base_score":60,"base_score_auto":60,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100},
			{"music_id":2,"difficulty":"master","music_time":100,"event_rate":120,"base_score":50,"base_score_auto":50,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100},
			{"music_id":3,"difficulty":"expert","music_time":100,"event_rate":90,"base_score":40,"base_score_auto":40,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}
		]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:                   "jp",
		RecommendType:            "challenge",
		ChallengeLiveCharacterID: new(21),
		UseCurrentDeck:           true,
		MusicCompare:             true,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if recommendCalls.Load() != 3 {
		t.Fatalf("expected three recommend calls, got %d", recommendCalls.Load())
	}
	if !reflect.DeepEqual(seenMusicIDs, []int{1, 2, 3}) {
		t.Fatalf("unexpected recommend music ids: %+v", seenMusicIDs)
	}
	if !request.MusicCompare {
		t.Fatalf("expected music_compare in request")
	}
	if !reflect.DeepEqual(request.FixedCardsID, []int{1001, 1002, 1003, 1004, 1005}) {
		t.Fatalf("unexpected fixed cards in compare request: %+v", request.FixedCardsID)
	}
	if request.CharaName == nil || *request.CharaName != "初音未来" {
		t.Fatalf("unexpected challenge character metadata: %+v", request.CharaName)
	}
	if len(request.DeckData) != 3 {
		t.Fatalf("unexpected compare deck count: %d", len(request.DeckData))
	}
	gotMusicIDs := make([]int, 0, len(request.DeckData))
	for _, item := range request.DeckData {
		if item.MusicID == nil {
			t.Fatalf("missing compare music id in deck data: %+v", item)
		}
		gotMusicIDs = append(gotMusicIDs, *item.MusicID)
	}
	if !reflect.DeepEqual(gotMusicIDs, []int{2, 3, 1}) {
		t.Fatalf("unexpected compare sort order: %+v", gotMusicIDs)
	}
	if request.DeckData[0].MusicTitle == nil || *request.DeckData[0].MusicTitle != "Song B" {
		t.Fatalf("unexpected first compare title: %+v", request.DeckData[0].MusicTitle)
	}
	if request.DeckData[0].ChallengeScoreDelta == nil || *request.DeckData[0].ChallengeScoreDelta != 200000 {
		t.Fatalf("unexpected first compare challenge delta: %+v", request.DeckData[0].ChallengeScoreDelta)
	}
}

func TestBuildDrawingRequestFromRecommendResultUsesDefaultImageForDisplayState(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	raw, err := snapshot.CloneRawUserData(controller.snapshot.RawData())
	if err != nil {
		t.Fatalf("CloneRawUserData() error = %v", err)
	}
	for i := range raw.UserCards {
		if raw.UserCards[i].CardID != 1002 {
			continue
		}
		raw.UserCards[i].SpecialTrainingStatus = "done"
		raw.UserCards[i].DefaultImage = "normal"
	}

	request, err := controller.buildDrawingRequestFromRecommendResult(
		renderregion.JP,
		"event",
		AutoQuery{Region: "jp", RecommendType: "event"},
		map[string]any{"target": "score"},
		raw,
		&RecommendResult{
			Decks: []RecommendDeck{{
				Cards: []RecommendCard{{
					CardID:          1002,
					Level:           60,
					MasterRank:      5,
					DefaultImage:    "normal",
					SkillLevel:      4,
					SkillRate:       120,
					IsAfterTraining: true,
				}},
				Score: 123,
			}},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildDrawingRequestFromRecommendResult() error = %v", err)
	}
	if len(request.DeckData) != 1 || len(request.DeckData[0].CardData) != 1 {
		t.Fatalf("unexpected deck request: %+v", request.DeckData)
	}
	cardData := request.DeckData[0].CardData[0]
	if cardData.IsAfterTraining {
		t.Fatalf("expected displayed deck card state to stay before-training, got %+v", cardData)
	}
	if cardData.CardThumbnail.IsAfterTraining == nil || *cardData.CardThumbnail.IsAfterTraining {
		t.Fatalf("expected thumbnail display state to stay before-training, got %+v", cardData.CardThumbnail.IsAfterTraining)
	}
	if !strings.Contains(cardData.CardThumbnail.CardThumbnailPath, "_normal.png") {
		t.Fatalf("expected normal art thumbnail, got %q", cardData.CardThumbnail.CardThumbnailPath)
	}
	if !strings.Contains(cardData.CardThumbnail.RareImgPath, "rare_star_after_training.png") {
		t.Fatalf("expected trained rarity marker to stay after-training, got %q", cardData.CardThumbnail.RareImgPath)
	}
}

func TestBuildDrawingRequestFromRecommendResultUsesAfterTrainingFlagWhenDefaultImageMissing(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})

	request, err := controller.buildDrawingRequestFromRecommendResult(
		renderregion.JP,
		"event",
		AutoQuery{Region: "jp", RecommendType: "event"},
		map[string]any{"target": "score"},
		nil,
		&RecommendResult{
			Decks: []RecommendDeck{{
				Cards: []RecommendCard{{
					CardID:          1002,
					Level:           60,
					MasterRank:      5,
					SkillLevel:      4,
					SkillRate:       120,
					IsAfterTraining: true,
				}},
				Score: 123,
			}},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("buildDrawingRequestFromRecommendResult() error = %v", err)
	}
	if len(request.DeckData) != 1 || len(request.DeckData[0].CardData) != 1 {
		t.Fatalf("unexpected deck request: %+v", request.DeckData)
	}
	cardData := request.DeckData[0].CardData[0]
	if !cardData.IsAfterTraining {
		t.Fatalf("expected deck card state to follow after_training flag, got %+v", cardData)
	}
	if cardData.CardThumbnail.IsAfterTraining == nil || !*cardData.CardThumbnail.IsAfterTraining {
		t.Fatalf("expected thumbnail state to follow after_training flag, got %+v", cardData.CardThumbnail.IsAfterTraining)
	}
	if !strings.Contains(cardData.CardThumbnail.CardThumbnailPath, "_after_training.png") {
		t.Fatalf("expected after-training art thumbnail, got %q", cardData.CardThumbnail.CardThumbnailPath)
	}
}

func TestBuildAutoRecommendRequestEventMusicCompareRequiresFixedDeckWhenQueriesOmitted(t *testing.T) {
	server, masterdataRoot := newDeckRecommendStubServer(t)
	defer server.Close()

	controller := newTestDeckControllerWithMeta(t, RecommendConfig{
		Enabled:        true,
		ServiceBaseURL: server.URL,
		MasterdataDir:  masterdataRoot,
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":1,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	_, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		MusicCompare:  true,
	})
	if err == nil {
		t.Fatalf("expected compare without fixed deck to fail")
	}
	if !strings.Contains(err.Error(), "必须固定一个卡组") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildAutoRecommendRequestEventMusicCompareUsesResolvedSelections(t *testing.T) {
	var recommendCalls atomic.Int32
	seenMusicIDs := make([]int, 0, 2)

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
			_, _ = w.Write([]byte(`{"userdata_hash":"compare-userdata-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			option := options[0].(map[string]any)
			musicID := int(option["music_id"].(float64))
			seenMusicIDs = append(seenMusicIDs, musicID)
			if option["best_skill_as_leader"] != false {
				t.Fatalf("expected compare mode to disable best_skill_as_leader: %+v", option["best_skill_as_leader"])
			}

			scoreByMusicID := map[int]int{
				1: 100,
				2: 300,
			}
			score := scoreByMusicID[musicID]
			_, _ = w.Write([]byte(fmt.Sprintf(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": %d,
							"live_score": %d,
							"mysekai_event_point": 0,
							"total_power": 200000,
							"event_bonus_rate": 25,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up": 110,
							"cards": [{"card_id":1001,"level":50,"master_rank":1,"skill_level":4,"skill_score_up":100,"event_bonus_rate":20,"episode1_read":true,"episode2_read":true,"after_training":false,"default_image":"normal","has_canvas_bonus":false}]
						}]
					}
				}
			]`, score, score)))
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
		data: []byte(`[{"music_id":1,"difficulty":"hard","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100},{"music_id":2,"difficulty":"expert","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		MusicCompare:  true,
		MusicCompareSelections: []MusicCompareSelection{
			{MusicID: 1, MusicDiff: "hard", MusicTitle: "Song A", MusicCoverPath: "music/jacket/song_a/song_a.png", MusicQuery: "Song Ahd"},
			{MusicID: 2, MusicDiff: "expert", MusicTitle: "Song B", MusicCoverPath: "music/jacket/song_b/song_b.png", MusicQuery: "Song Bex"},
		},
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if recommendCalls.Load() != 2 {
		t.Fatalf("expected two recommend calls, got %d", recommendCalls.Load())
	}
	if !reflect.DeepEqual(seenMusicIDs, []int{1, 2}) {
		t.Fatalf("unexpected recommend music ids: %+v", seenMusicIDs)
	}
	if !request.MusicCompare {
		t.Fatalf("expected music_compare in request")
	}
	if request.MusicTitle != nil || request.MusicID != nil || request.MusicCoverPath != nil {
		t.Fatalf("unexpected top-level music fields in compare mode: title=%+v id=%+v cover=%+v", request.MusicTitle, request.MusicID, request.MusicCoverPath)
	}
	if len(request.DeckData) != 2 {
		t.Fatalf("unexpected compare deck count: %d", len(request.DeckData))
	}
	if request.DeckData[0].MusicID == nil || *request.DeckData[0].MusicID != 2 {
		t.Fatalf("unexpected first compare deck music id: %+v", request.DeckData[0].MusicID)
	}
	if request.DeckData[0].MusicTitle == nil || *request.DeckData[0].MusicTitle != "Song B" {
		t.Fatalf("unexpected first compare deck music title: %+v", request.DeckData[0].MusicTitle)
	}
	if request.DeckData[0].MusicQuery != nil {
		t.Fatalf("expected compare deck music query to be omitted for drawing layout, got %+v", request.DeckData[0].MusicQuery)
	}
	if request.DeckData[1].MusicID == nil || *request.DeckData[1].MusicID != 1 {
		t.Fatalf("unexpected second compare deck music id: %+v", request.DeckData[1].MusicID)
	}
}

func TestBuildAutoRecommendRequestEventMusicCompareCurrentDeckBuildsCandidatesAndSorts(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"userdata_hash":"compare-current-userdata-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			option := options[0].(map[string]any)
			fixedCards, ok := option["fixed_cards"].([]any)
			if !ok || len(fixedCards) != 5 {
				t.Fatalf("unexpected fixed_cards: %+v", option["fixed_cards"])
			}
			musicID := int(option["music_id"].(float64))
			scoreByMusicID := map[int]int{
				1: 100,
				2: 400,
				3: 250,
			}
			score := scoreByMusicID[musicID]
			_, _ = w.Write([]byte(fmt.Sprintf(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": %d,
							"live_score": %d,
							"mysekai_event_point": 0,
							"total_power": 200000,
							"event_bonus_rate": 25,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up": 110,
							"cards": [{"card_id":1001,"level":50,"master_rank":1,"skill_level":4,"skill_score_up":100,"event_bonus_rate":20,"episode1_read":true,"episode2_read":true,"after_training":false,"default_image":"normal","has_canvas_bonus":false}]
						}]
					}
				}
			]`, score, score)))
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
		data: []byte(`[
			{"music_id":1,"difficulty":"master","music_time":100,"event_rate":100,"base_score":50,"base_score_auto":50,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100},
			{"music_id":2,"difficulty":"master","music_time":100,"event_rate":120,"base_score":40,"base_score_auto":40,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100},
			{"music_id":3,"difficulty":"expert","music_time":100,"event_rate":90,"base_score":30,"base_score_auto":30,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}
		]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:         "jp",
		RecommendType:  "event",
		EventID:        new(7),
		UseCurrentDeck: true,
		MusicCompare:   true,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if recommendCalls.Load() != 3 {
		t.Fatalf("expected three recommend calls, got %d", recommendCalls.Load())
	}
	if !reflect.DeepEqual(request.FixedCardsID, []int{1001, 1002, 1003, 1004, 1005}) {
		t.Fatalf("unexpected fixed cards in compare request: %+v", request.FixedCardsID)
	}
	if len(request.DeckData) != 3 {
		t.Fatalf("unexpected compare deck count: %d", len(request.DeckData))
	}
	gotMusicIDs := make([]int, 0, len(request.DeckData))
	for _, item := range request.DeckData {
		if item.MusicID == nil {
			t.Fatalf("missing compare music id in deck data: %+v", item)
		}
		gotMusicIDs = append(gotMusicIDs, *item.MusicID)
	}
	if !reflect.DeepEqual(gotMusicIDs, []int{2, 3, 1}) {
		t.Fatalf("unexpected compare sort order: %+v", gotMusicIDs)
	}
	if request.DeckData[0].MusicTitle == nil || *request.DeckData[0].MusicTitle != "Song B" {
		t.Fatalf("unexpected first compare title: %+v", request.DeckData[0].MusicTitle)
	}
}

func TestBuildAutoRecommendRequestEventCurrentUsesSnapshotDeck(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"userdata_hash":"event-current-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			option, ok := options[0].(map[string]any)
			if !ok {
				t.Fatalf("unexpected batch option payload: %+v", options[0])
			}
			fixedCards, ok := option["fixed_cards"].([]any)
			if !ok || len(fixedCards) != 5 {
				t.Fatalf("unexpected fixed_cards: %+v", option["fixed_cards"])
			}
			expected := []float64{1001, 1002, 1003, 1004, 1005}
			for index, value := range fixedCards {
				if value != expected[index] {
					t.Fatalf("unexpected fixed card %d: %+v", index, value)
				}
			}
			if option["best_skill_as_leader"] != false {
				t.Fatalf("expected best_skill_as_leader to be disabled: %+v", option["best_skill_as_leader"])
			}
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
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
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:         "jp",
		RecommendType:  "event",
		EventID:        new(7),
		UseCurrentDeck: true,
		Algorithm:      "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if recommendCalls.Load() != 1 {
		t.Fatalf("expected one recommend call, got %d", recommendCalls.Load())
	}
	if !reflect.DeepEqual(request.FixedCardsID, []int{1001, 1002, 1003, 1004, 1005}) {
		t.Fatalf("unexpected fixed cards in request: %+v", request.FixedCardsID)
	}
}

func TestBuildAutoRecommendRequestEventCurrentAreaItemLevelFallsBackForMissingSnapshotCard(t *testing.T) {
	var (
		recommendCalls atomic.Int32
		cached         snapshot.RawUserData
	)

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
			if err := json.Unmarshal(payloads[0], &cached); err != nil {
				t.Fatalf("decode cached raw user data: %v", err)
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"event-current-area-item-fallback-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			option, ok := options[0].(map[string]any)
			if !ok {
				t.Fatalf("unexpected batch option payload: %+v", options[0])
			}
			if option["area_item_level"] != 15.0 {
				t.Fatalf("unexpected area_item_level: %+v", option["area_item_level"])
			}
			fixedCards, ok := option["fixed_cards"].([]any)
			if !ok || len(fixedCards) != 5 {
				t.Fatalf("unexpected fixed_cards: %+v", option["fixed_cards"])
			}
			expected := []float64{1006, 1002, 1003, 1004, 1005}
			for index, value := range fixedCards {
				if value != expected[index] {
					t.Fatalf("unexpected fixed card %d: %+v", index, value)
				}
			}
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
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
							"cards": [{"card_id": 1006, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false}]
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
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	snapshotJSON := []byte(`{
		"now": 1700000000000,
		"userGamedata": {"userId": 10001, "name": "Deck User", "deck": 1, "rank": 123},
		"userProfile": {"profileImageType": "default"},
		"userDecks": [{"deckId": 1, "leader": 1006, "subLeader": 1002, "member1": 1006, "member2": 1002, "member3": 1003, "member4": 1004, "member5": 1005}],
		"userCards": [
			{"cardId": 1002, "level": 60, "skillLevel": 4, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []},
			{"cardId": 1003, "level": 60, "skillLevel": 4, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []},
			{"cardId": 1004, "level": 60, "skillLevel": 4, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []},
			{"cardId": 1005, "level": 60, "skillLevel": 4, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []}
		],
		"userAreas": [{"areaItems": [{"areaItemId": 1, "level": 3}, {"areaItemId": 2, "level": 7}]}]
	}`)
	customSnapshot, err := snapshot.NewFromBytes(nil, controller.assets, renderregion.JP, snapshotJSON, nil, nil)
	if err != nil {
		t.Fatalf("build custom snapshot: %v", err)
	}
	controller = controller.WithSnapshot(customSnapshot)

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:         "jp",
		RecommendType:  "event",
		EventID:        new(7),
		UseCurrentDeck: true,
		AreaItemLevel:  new(15),
		Algorithm:      "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if recommendCalls.Load() != 1 {
		t.Fatalf("expected one recommend call, got %d", recommendCalls.Load())
	}
	if !reflect.DeepEqual(request.FixedCardsID, []int{1006, 1002, 1003, 1004, 1005}) {
		t.Fatalf("unexpected fixed cards in request: %+v", request.FixedCardsID)
	}
	card1006 := snapshot.FindUserCard(cached.UserCards, 1006)
	if card1006 == nil {
		t.Fatalf("expected cached payload to include fallback current deck card 1006: %+v", cached.UserCards)
	}
	if card1006.Level != 60 || card1006.SkillLevel != 4 || card1006.MasterRank != 5 {
		t.Fatalf("unexpected fallback current deck card 1006: %+v", card1006)
	}
}

func TestBuildAutoRecommendRequestEventFixedCardFallbackUsesBaseSkillAndMaster(t *testing.T) {
	var (
		recommendCalls atomic.Int32
		cached         snapshot.RawUserData
	)

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
			if err := json.Unmarshal(payloads[0], &cached); err != nil {
				t.Fatalf("decode cached raw user data: %v", err)
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"event-fixed-card-fallback-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			var payloads [][]byte
			if err := decodeDeckMultipartPayload(r.Body, &payloads); err != nil {
				t.Fatalf("decode recommend payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			option, ok := options[0].(map[string]any)
			if !ok {
				t.Fatalf("unexpected batch option payload: %+v", options[0])
			}
			fixedCards, ok := option["fixed_cards"].([]any)
			if !ok || len(fixedCards) != 1 || fixedCards[0] != 1006.0 {
				t.Fatalf("unexpected fixed_cards: %+v", option["fixed_cards"])
			}
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
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
							"cards": [{"card_id": 1006, "level": 60, "master_rank": 0, "skill_level": 1, "skill_score_up": 100, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false}]
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
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		FixedCards:    []int{1006},
		Algorithm:     "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if recommendCalls.Load() != 1 {
		t.Fatalf("expected one recommend call, got %d", recommendCalls.Load())
	}
	if !reflect.DeepEqual(request.FixedCardsID, []int{1006}) {
		t.Fatalf("unexpected fixed cards in request: %+v", request.FixedCardsID)
	}
	card1006 := snapshot.FindUserCard(cached.UserCards, 1006)
	if card1006 == nil {
		t.Fatalf("expected cached payload to include fallback fixed card 1006: %+v", cached.UserCards)
	}
	if card1006.Level != 60 || card1006.SkillLevel != 1 || card1006.MasterRank != 0 {
		t.Fatalf("unexpected fallback fixed card 1006: %+v", card1006)
	}
}

func TestBuildAutoRecommendRequestMaxProfilePreparesSyntheticUserCards(t *testing.T) {
	var cached snapshot.RawUserData

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
			if err := json.Unmarshal(payloads[0], &cached); err != nil {
				t.Fatalf("decode cached raw user data: %v", err)
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"max-profile-hash"}`))
		case "/recommend":
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
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
							"cards": [{"card_id": 1006, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false}]
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
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		MaxProfile:    true,
		Algorithm:     "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if !request.IsMaxDeck {
		t.Fatalf("expected max profile request to set is_max_deck")
	}
	if cached.UserGamedata.UserID != 1 || cached.UserGamedata.Name != "MaxProfile" {
		t.Fatalf("expected max profile request to ignore existing snapshot identity, got %+v", cached.UserGamedata)
	}
	if len(cached.UserCards) != 7 {
		t.Fatalf("unexpected max profile user card count: %d", len(cached.UserCards))
	}
	if len(cached.UserMysekaiCanvases) != 7 {
		t.Fatalf("unexpected max profile mysekai canvas count: %d", len(cached.UserMysekaiCanvases))
	}
	if !reflect.DeepEqual(cached.UserMysekaiGates, []snapshot.RawUserMysekaiGate{
		{MysekaiGateID: 1, MysekaiGateLevel: 40},
		{MysekaiGateID: 2, MysekaiGateLevel: 40},
		{MysekaiGateID: 3, MysekaiGateLevel: 40},
		{MysekaiGateID: 4, MysekaiGateLevel: 40},
		{MysekaiGateID: 5, MysekaiGateLevel: 40},
	}) {
		t.Fatalf("unexpected max profile mysekai gates: %+v", cached.UserMysekaiGates)
	}
	if !reflect.DeepEqual(cached.UserMysekaiFixtureGameCharacterPerformanceBonuses, []snapshot.RawUserFixtureBonus{
		{GameCharacterID: 1, TotalBonusRate: 100},
		{GameCharacterID: 5, TotalBonusRate: 42},
	}) {
		t.Fatalf("unexpected max profile fixture bonuses: %+v", cached.UserMysekaiFixtureGameCharacterPerformanceBonuses)
	}
	card1006 := snapshot.FindUserCard(cached.UserCards, 1006)
	if card1006 == nil || card1006.Level != 60 || card1006.SkillLevel != 4 || card1006.MasterRank != 5 {
		t.Fatalf("unexpected max profile card 1006: %+v", card1006)
	}
	if len(card1006.Episodes) != 2 || card1006.Episodes[0].ScenarioStatus != "already_read" || card1006.Episodes[1].ScenarioStatus != "already_read" {
		t.Fatalf("expected max profile card 1006 episodes to be marked read: %+v", card1006)
	}
	card1007 := snapshot.FindUserCard(cached.UserCards, 1007)
	if card1007 == nil || card1007.Level != 50 || card1007.DefaultImage != "special_training" {
		t.Fatalf("unexpected max profile card 1007: %+v", card1007)
	}
}

func TestBuildAutoRecommendRequestMaxProfileWithoutSnapshotUsesSyntheticSnapshot(t *testing.T) {
	var cached snapshot.RawUserData
	var cachedPayload map[string]json.RawMessage

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
			if err := json.Unmarshal(payloads[0], &cachedPayload); err != nil {
				t.Fatalf("decode cached raw user data payload: %v", err)
			}
			if err := json.Unmarshal(payloads[0], &cached); err != nil {
				t.Fatalf("decode cached raw user data: %v", err)
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"max-profile-no-snapshot-hash"}`))
		case "/recommend":
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
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
							"cards": [{"card_id": 1006, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false}]
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
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	}).WithSnapshot(nil)

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "no_event",
		MaxProfile:    true,
		Algorithm:     "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if !request.IsMaxDeck {
		t.Fatalf("expected max profile request to set is_max_deck")
	}
	if cached.UserGamedata.UserID != 1 || cached.UserGamedata.Deck != 1 {
		t.Fatalf("unexpected synthetic user gamedata: %+v", cached.UserGamedata)
	}
	if len(cached.UserMysekaiCanvases) != 7 {
		t.Fatalf("unexpected synthetic max profile mysekai canvas count: %d", len(cached.UserMysekaiCanvases))
	}
	if !reflect.DeepEqual(cached.UserMysekaiGates, []snapshot.RawUserMysekaiGate{
		{MysekaiGateID: 1, MysekaiGateLevel: 40},
		{MysekaiGateID: 2, MysekaiGateLevel: 40},
		{MysekaiGateID: 3, MysekaiGateLevel: 40},
		{MysekaiGateID: 4, MysekaiGateLevel: 40},
		{MysekaiGateID: 5, MysekaiGateLevel: 40},
	}) {
		t.Fatalf("unexpected synthetic max profile mysekai gates: %+v", cached.UserMysekaiGates)
	}
	if !reflect.DeepEqual(cached.UserMysekaiFixtureGameCharacterPerformanceBonuses, []snapshot.RawUserFixtureBonus{
		{GameCharacterID: 1, TotalBonusRate: 100},
		{GameCharacterID: 5, TotalBonusRate: 42},
	}) {
		t.Fatalf("unexpected synthetic max profile fixture bonuses: %+v", cached.UserMysekaiFixtureGameCharacterPerformanceBonuses)
	}
	userCharacters, ok := cachedPayload["userCharacters"]
	if !ok {
		t.Fatalf("expected synthetic payload to include userCharacters")
	}
	var characterRanks []snapshot.RawUserCharacter
	if err := json.Unmarshal(userCharacters, &characterRanks); err != nil {
		t.Fatalf("decode synthetic userCharacters: %v", err)
	}
	if len(characterRanks) != challengeCharacterCount {
		t.Fatalf("expected %d synthetic userCharacters, got %d", challengeCharacterCount, len(characterRanks))
	}
	if characterRanks[0].CharacterID != 1 || characterRanks[0].CharacterRank != 120 {
		t.Fatalf("unexpected first synthetic userCharacter: %+v", characterRanks[0])
	}
	if characterRanks[len(characterRanks)-1].CharacterID != challengeCharacterCount || characterRanks[len(characterRanks)-1].CharacterRank != 120 {
		t.Fatalf("unexpected last synthetic userCharacter: %+v", characterRanks[len(characterRanks)-1])
	}
	canvases, ok := cachedPayload["userMysekaiCanvases"]
	if !ok {
		t.Fatalf("expected synthetic payload to include userMysekaiCanvases")
	}
	var mysekaiCanvases []snapshot.RawUserMysekaiCanvas
	if err := json.Unmarshal(canvases, &mysekaiCanvases); err != nil {
		t.Fatalf("decode synthetic userMysekaiCanvases: %v", err)
	}
	if len(mysekaiCanvases) != 7 {
		t.Fatalf("expected 7 synthetic userMysekaiCanvases, got %d", len(mysekaiCanvases))
	}
	if mysekaiCanvases[0].CardID != 1001 || mysekaiCanvases[0].Quantity != 1 {
		t.Fatalf("unexpected first synthetic mysekai canvas: %+v", mysekaiCanvases[0])
	}
	mysekaiGates, ok := cachedPayload["userMysekaiGates"]
	if !ok {
		t.Fatalf("expected synthetic payload to include userMysekaiGates")
	}
	var decodedGates []snapshot.RawUserMysekaiGate
	if err := json.Unmarshal(mysekaiGates, &decodedGates); err != nil {
		t.Fatalf("decode synthetic userMysekaiGates: %v", err)
	}
	if !reflect.DeepEqual(decodedGates, []snapshot.RawUserMysekaiGate{
		{MysekaiGateID: 1, MysekaiGateLevel: 40},
		{MysekaiGateID: 2, MysekaiGateLevel: 40},
		{MysekaiGateID: 3, MysekaiGateLevel: 40},
		{MysekaiGateID: 4, MysekaiGateLevel: 40},
		{MysekaiGateID: 5, MysekaiGateLevel: 40},
	}) {
		t.Fatalf("unexpected synthetic payload userMysekaiGates: %+v", decodedGates)
	}
	fixtureBonuses, ok := cachedPayload["userMysekaiFixtureGameCharacterPerformanceBonuses"]
	if !ok {
		t.Fatalf("expected synthetic payload to include userMysekaiFixtureGameCharacterPerformanceBonuses")
	}
	var decodedFixtureBonuses []snapshot.RawUserFixtureBonus
	if err := json.Unmarshal(fixtureBonuses, &decodedFixtureBonuses); err != nil {
		t.Fatalf("decode synthetic userMysekaiFixtureGameCharacterPerformanceBonuses: %v", err)
	}
	if !reflect.DeepEqual(decodedFixtureBonuses, []snapshot.RawUserFixtureBonus{
		{GameCharacterID: 1, TotalBonusRate: 100},
		{GameCharacterID: 5, TotalBonusRate: 42},
	}) {
		t.Fatalf("unexpected synthetic payload fixture bonuses: %+v", decodedFixtureBonuses)
	}
	if len(cached.UserCards) != 7 {
		t.Fatalf("unexpected max profile user card count without snapshot: %d", len(cached.UserCards))
	}
	card1006 := snapshot.FindUserCard(cached.UserCards, 1006)
	if card1006 == nil || card1006.Level != 60 || card1006.SkillLevel != 4 || card1006.MasterRank != 5 {
		t.Fatalf("unexpected synthetic max profile card 1006: %+v", card1006)
	}
}

func TestBuildAutoRecommendRequestSubMaxProfilePromotesAreaItemsTo15(t *testing.T) {
	var cached snapshot.RawUserData

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
			if err := json.Unmarshal(payloads[0], &cached); err != nil {
				t.Fatalf("decode cached raw user data: %v", err)
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"sub-max-profile-hash"}`))
		case "/recommend":
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
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
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		SubMaxProfile: true,
		Algorithm:     "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if request.IsMaxDeck {
		t.Fatalf("sub max profile should not masquerade as max deck")
	}
	levels := collectRawAreaItemLevels(cached.UserAreas)
	if levels[1] != 15 || levels[2] != 15 {
		t.Fatalf("unexpected sub max area item levels: %+v", levels)
	}
}

func TestBuildAutoRecommendRequestFilterAndExcludeTrimUserCards(t *testing.T) {
	var cached snapshot.RawUserData

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
			if err := json.Unmarshal(payloads[0], &cached); err != nil {
				t.Fatalf("decode cached raw user data: %v", err)
			}
			_, _ = w.Write([]byte(`{"userdata_hash":"filtered-userdata-hash"}`))
		case "/recommend":
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.5,
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
							"cards": [{"card_id": 1003, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false}]
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
		DefaultAlgs:    []string{"ga"},
	}, &testMusicMetaSource{
		data: []byte(`[{"music_id":10000,"difficulty":"master","music_time":100,"event_rate":120,"base_score":1,"base_score_auto":1,"skill_score_solo":[1,1,1,1,1,1],"skill_score_auto":[1,1,1,1,1,1],"skill_score_multi":[1,1,1,1,1,1],"fever_score":1,"fever_end_time":1,"tap_count":100}]`),
	})

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		Boost:         new(5),
		UnitFilter:    "piapro",
		ExcludedCards: []int{1004},
		Algorithm:     "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if request.UnitFilter == nil || *request.UnitFilter != "piapro" {
		t.Fatalf("unexpected request unit filter: %+v", request.UnitFilter)
	}
	if request.UnitLogoPath == nil || *request.UnitLogoPath != "static_images/icon_piapro.png" {
		t.Fatalf("unexpected request unit logo path: %+v", request.UnitLogoPath)
	}
	if request.Boost == nil || *request.Boost != 5 {
		t.Fatalf("unexpected request boost: %+v", request.Boost)
	}
	if !reflect.DeepEqual(request.ExcludedCards, []int{1004}) {
		t.Fatalf("unexpected request excluded cards: %+v", request.ExcludedCards)
	}
	if len(cached.UserCards) != 2 {
		t.Fatalf("unexpected filtered user card count: %d", len(cached.UserCards))
	}
	got := []int{cached.UserCards[0].CardID, cached.UserCards[1].CardID}
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{1003, 1005}) {
		t.Fatalf("unexpected filtered user cards: %+v", got)
	}
}

func TestBuildAutoRecommendRequestSetsAttrIconPathFromAttrFilter(t *testing.T) {
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

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		AttrFilter:    "happy",
		Algorithm:     "ga",
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}

	if request.AttrFilter == nil || *request.AttrFilter != "happy" {
		t.Fatalf("unexpected request attr filter: %+v", request.AttrFilter)
	}
	if request.AttrIconPath == nil || *request.AttrIconPath != "static_images/card/attr_icon_happy.png" {
		t.Fatalf("unexpected request attr icon path: %+v", request.AttrIconPath)
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
			var payload map[string]any
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
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode music meta request: %v", err)
			}
			data, _ := payload["data"].(string)
			var metas []map[string]any
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
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
			if !ok || len(options) != 1 {
				t.Fatalf("unexpected batch_options: %+v", payload["batch_options"])
			}
			first, ok := options[0].(map[string]any)
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
			var payload map[string]any
			if err := json.Unmarshal(payloads[0], &payload); err != nil {
				t.Fatalf("decode recommend json: %v", err)
			}
			options, ok := payload["batch_options"].([]any)
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

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		Target:        "skill",
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if recommendCalls.Load() != 1 {
		t.Fatalf("expected one batched recommend call, got %d", recommendCalls.Load())
	}
	if len(request.ModelName) != 1 || request.ModelName[0] != "DFS+GA" {
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
			var payload map[string]any
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

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
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

func TestBuildAutoRecommendRequestRemoteServiceFallsBackToLegacyWhenUserdataHashMissing(t *testing.T) {
	var binaryRecommendCalls atomic.Int32
	var legacyRecommendCalls atomic.Int32

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
			_, _ = w.Write([]byte(`{"userdata_hash":"missing-userdata-hash"}`))
		case "/recommend":
			if strings.Contains(r.Header.Get("Content-Type"), "application/octet-stream") {
				binaryRecommendCalls.Add(1)
				http.Error(w, `{"error":"User data not found for userdata_hash: missing-userdata-hash"}`, http.StatusInternalServerError)
				return
			}

			legacyRecommendCalls.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode legacy recommend request: %v", err)
			}
			if payload["algorithm"] != "ga" {
				t.Fatalf("unexpected legacy algorithm: %+v", payload["algorithm"])
			}
			if strings.TrimSpace(payload["user_data_str"].(string)) == "" {
				t.Fatalf("expected user_data_str in legacy payload")
			}
			_, _ = w.Write([]byte(`{"decks":[{"score":456,"live_score":456,"mysekai_event_point":0,"total_power":789,"event_bonus_rate":25,"support_deck_bonus_rate":0,"multi_live_score_up":110,"cards":[{"card_id":1001,"level":50,"master_rank":1,"skill_level":4,"skill_score_up":100,"event_bonus_rate":20,"episode1_read":true,"episode2_read":true,"after_training":false,"default_image":"normal","has_canvas_bonus":false}]}]}`))
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

	request, err := controller.BuildAutoRecommendRequest(AutoQuery{
		Region:        "jp",
		RecommendType: "event",
		EventID:       new(7),
		Algorithm:     "ga",
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("BuildAutoRecommendRequest returned error: %v", err)
	}
	if binaryRecommendCalls.Load() != 1 {
		t.Fatalf("expected one binary recommend call, got %d", binaryRecommendCalls.Load())
	}
	if legacyRecommendCalls.Load() != 1 {
		t.Fatalf("expected one legacy recommend call, got %d", legacyRecommendCalls.Load())
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
		"userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 1002, "member1": 1001, "member2": 1002, "member3": 1003, "member4": 1004, "member5": 1005}],
		"userCards": [
			{"cardId": 1001, "level": 50, "skillLevel": 4, "masterRank": 1, "specialTrainingStatus": "not_done", "defaultImage": "normal", "episodes": []},
			{"cardId": 1002, "level": 60, "skillLevel": 4, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []},
			{"cardId": 1003, "level": 60, "skillLevel": 4, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []},
			{"cardId": 1004, "level": 60, "skillLevel": 4, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []},
			{"cardId": 1005, "level": 60, "skillLevel": 4, "masterRank": 5, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []}
		],
		"userAreas": [{"areaItems": [{"areaItemId": 1, "level": 3}, {"areaItemId": 2, "level": 7}]}],
		"userChallengeLiveSoloDecks": [
			{"characterId": 21, "leader": 1001, "support1": 1002, "support2": 1003, "support3": 1004, "support4": 1005}
		],
		"userChallengeLiveSoloResults": [
			{"characterId": 1, "highScore": 1000000},
			{"characterId": 21, "highScore": 1100000}
		]
	}`
	if err := os.WriteFile(userJSONPath, []byte(userJSON), 0o644); err != nil {
		t.Fatalf("write user snapshot: %v", err)
	}

	assetHelper := assets.NewAssetHelper(tempDir, nil)
	snap := snapshot.NewLocalFileService(nil, assetHelper, snapshot.LocalFileConfig{
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
			1003: {
				ID:              1003,
				CharacterID:     21,
				CardRarityType:  "rarity_4",
				Attr:            "mysterious",
				AssetBundleName: "card_1003",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 210},
					{CardParameterType: "param2", Power: 210},
					{CardParameterType: "param3", Power: 210},
				},
			},
			1004: {
				ID:              1004,
				CharacterID:     22,
				CardRarityType:  "rarity_4",
				Attr:            "pure",
				AssetBundleName: "card_1004",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 220},
					{CardParameterType: "param2", Power: 220},
					{CardParameterType: "param3", Power: 220},
				},
			},
			1005: {
				ID:              1005,
				CharacterID:     23,
				CardRarityType:  "rarity_4",
				Attr:            "happy",
				AssetBundleName: "card_1005",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 230},
					{CardParameterType: "param2", Power: 230},
					{CardParameterType: "param3", Power: 230},
				},
			},
			1006: {
				ID:              1006,
				CharacterID:     5,
				CardRarityType:  "rarity_4",
				Attr:            "cool",
				AssetBundleName: "card_1006",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 240},
					{CardParameterType: "param2", Power: 240},
					{CardParameterType: "param3", Power: 240},
				},
			},
			1007: {
				ID:              1007,
				CharacterID:     6,
				CardRarityType:  "rarity_3",
				Attr:            "cute",
				AssetBundleName: "card_1007",
				CardParameters: []masterdata.CardParameter{
					{CardParameterType: "param1", Power: 180},
					{CardParameterType: "param2", Power: 180},
					{CardParameterType: "param3", Power: 180},
				},
			},
		},
		episodes: map[int][]snapshot.RawUserCardEpisode{
			1001: {{CardEpisodeID: 11001, ScenarioStatus: "already_read"}, {CardEpisodeID: 11002, ScenarioStatus: "already_read"}},
			1002: {{CardEpisodeID: 12001, ScenarioStatus: "already_read"}, {CardEpisodeID: 12002, ScenarioStatus: "already_read"}},
			1003: {{CardEpisodeID: 13001, ScenarioStatus: "already_read"}, {CardEpisodeID: 13002, ScenarioStatus: "already_read"}},
			1004: {{CardEpisodeID: 14001, ScenarioStatus: "already_read"}, {CardEpisodeID: 14002, ScenarioStatus: "already_read"}},
			1005: {{CardEpisodeID: 15001, ScenarioStatus: "already_read"}, {CardEpisodeID: 15002, ScenarioStatus: "already_read"}},
			1006: {{CardEpisodeID: 16001, ScenarioStatus: "already_read"}, {CardEpisodeID: 16002, ScenarioStatus: "already_read"}},
			1007: {{CardEpisodeID: 17001, ScenarioStatus: "already_read"}, {CardEpisodeID: 17002, ScenarioStatus: "already_read"}},
		},
		characters: map[int]*masterdata.Character{
			1:  {ID: 1, FirstName: "星乃", GivenName: "一歌", Unit: "light_sound"},
			2:  {ID: 2, FirstName: "天马", GivenName: "咲希", Unit: "light_sound"},
			5:  {ID: 5, FirstName: "花里", GivenName: "实乃理", Unit: "idol"},
			6:  {ID: 6, FirstName: "桐谷", GivenName: "遥", Unit: "idol"},
			20: {ID: 20, FirstName: "晓山", GivenName: "瑞希", Unit: "school_refusal"},
			21: {ID: 21, FirstName: "初音", GivenName: "未来", Unit: "piapro"},
			22: {ID: 22, FirstName: "镜音", GivenName: "铃", Unit: "piapro"},
			23: {ID: 23, FirstName: "镜音", GivenName: "连", Unit: "piapro"},
		},
		areaItemLevelCaps: map[int]int{
			1: 20,
			2: 20,
		},
		mysekaiGates: []snapshot.RawUserMysekaiGate{
			{MysekaiGateID: 1, MysekaiGateLevel: 40},
			{MysekaiGateID: 2, MysekaiGateLevel: 40},
			{MysekaiGateID: 3, MysekaiGateLevel: 40},
			{MysekaiGateID: 4, MysekaiGateLevel: 40},
			{MysekaiGateID: 5, MysekaiGateLevel: 40},
		},
		fixtureBonuses: []snapshot.RawUserFixtureBonus{
			{GameCharacterID: 1, TotalBonusRate: 100},
			{GameCharacterID: 5, TotalBonusRate: 42},
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
	musicSource := &testMusicSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			1:     {ID: 1, Title: "Song A", AssetBundleName: "song_a"},
			2:     {ID: 2, Title: "Song B", AssetBundleName: "song_b"},
			3:     {ID: 3, Title: "Song C", AssetBundleName: "song_c"},
			4:     {ID: 4, Title: "Song D", AssetBundleName: "song_d"},
			10000: {ID: 10000, Title: "おまかせ", AssetBundleName: "omakase"},
		},
	}

	controller := NewControllerWithConfig(cardSource, eventSource, nil, assetHelper, snap, renderregion.JP, cfg, metaLoader)
	controller.RegisterMusicSource(musicSource)
	return controller
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
