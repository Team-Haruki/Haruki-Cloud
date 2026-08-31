package deck

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

type refactorCoverageRecommender struct {
	calls  int
	errors []error
}

func (*refactorCoverageRecommender) Enabled() bool { return true }
func (*refactorCoverageRecommender) Close()        {}

func (*refactorCoverageRecommender) ExpandAlgorithms(option map[string]any) []map[string]any {
	return []map[string]any{cloneRecommendOption(option)}
}

func (r *refactorCoverageRecommender) Recommend(RecommendRequest) (*RecommendResult, error) {
	r.calls++
	if r.calls <= len(r.errors) && r.errors[r.calls-1] != nil {
		return nil, r.errors[r.calls-1]
	}
	return &RecommendResult{Decks: []RecommendDeck{{Score: r.calls}}}, nil
}

func TestRunStandardRecommendExecutionFallbackBranches(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	serviceErr := errors.New("deck-service: event not found for eventId: 7")

	success := autoRecommendExecution{
		recType:     "event",
		option:      map[string]any{"event_id": 7},
		recommender: &refactorCoverageRecommender{},
	}
	result, _, err := controller.runStandardRecommendExecution(context.Background(), &success)
	if err != nil || result == nil || result.Decks[0].Score != 1 {
		t.Fatalf("standard recommendation = %#v, %v", result, err)
	}

	missingFallback := autoRecommendExecution{
		recType:     "event",
		option:      map[string]any{"event_id": 7},
		recommender: &refactorCoverageRecommender{errors: []error{serviceErr}},
	}
	if _, _, err := controller.runStandardRecommendExecution(context.Background(), &missingFallback); !errors.Is(err, serviceErr) {
		t.Fatalf("missing fallback error = %v", err)
	}

	fallback := autoRecommendExecution{
		recType: "event",
		query: AutoQuery{
			EventUnit:             "idol",
			WorldBloomEventTurn:   deckIntPtr(2),
			WorldBloomCharacterID: deckIntPtr(5),
		},
		option:      map[string]any{"event_id": 7, "event_attr": "cute"},
		recommender: &refactorCoverageRecommender{errors: []error{serviceErr}},
	}
	result, _, err = controller.runStandardRecommendExecution(context.Background(), &fallback)
	if err != nil || result == nil || result.Decks[0].Score != 2 {
		t.Fatalf("fallback recommendation = %#v, %v", result, err)
	}
	if fallback.query.EventID != nil || optionInt(fallback.option, "world_bloom_event_turn") != 2 {
		t.Fatalf("fallback state = %+v, %#v", fallback.query, fallback.option)
	}

	fallbackFailure := autoRecommendExecution{
		recType: "event",
		query: AutoQuery{
			EventUnit:             "idol",
			WorldBloomEventTurn:   deckIntPtr(2),
			WorldBloomCharacterID: deckIntPtr(5),
		},
		option:      map[string]any{"event_id": 7, "event_attr": "cute"},
		recommender: &refactorCoverageRecommender{errors: []error{serviceErr, errors.New("fallback failed")}},
	}
	if _, _, err := controller.runStandardRecommendExecution(context.Background(), &fallbackFailure); err == nil || err.Error() != "fallback failed" {
		t.Fatalf("fallback failure = %v", err)
	}
}

func TestRecommendMetadataRefactorBranches(t *testing.T) {
	for _, tc := range []struct {
		recType  string
		liveType string
		wantType string
		wantName string
	}{
		{recType: "challenge", wantType: "solo", wantName: "单人"},
		{recType: "event", liveType: "auto", wantType: "auto", wantName: "自动"},
		{recType: "mysekai", wantType: "mysekai", wantName: "烤森"},
		{recType: "event", wantType: "multi", wantName: "协力"},
	} {
		request := &drawing.DeckRequest{}
		applyRecommendLiveMetadata(request, tc.recType, map[string]any{"live_type": tc.liveType})
		if request.LiveType == nil || *request.LiveType != tc.wantType || request.LiveName == nil || *request.LiveName != tc.wantName {
			t.Fatalf("live metadata %+v = %+v", tc, request)
		}
	}

	request := &drawing.DeckRequest{}
	applyWorldBloomRecommendType(request, "bonus")
	if request.RecommendType != "wl_bonus" {
		t.Fatalf("bonus world bloom type = %q", request.RecommendType)
	}
	applyWorldBloomRecommendType(request, "other")
	if request.RecommendType != "wl_bonus" {
		t.Fatalf("unrelated world bloom type changed = %q", request.RecommendType)
	}

	if got := worldBloomMetadataCharacterID(map[string]any{"world_bloom_character_id": 3}, AutoQuery{}); got != 3 {
		t.Fatalf("option world bloom character = %d", got)
	}
	if got := worldBloomMetadataCharacterID(nil, AutoQuery{WorldBloomCharacterID: deckIntPtr(4)}); got != 4 {
		t.Fatalf("query world bloom character = %d", got)
	}
	if got := worldBloomMetadataCharacterID(nil, AutoQuery{MetadataWorldBloomCharacterID: deckIntPtr(5)}); got != 5 {
		t.Fatalf("metadata world bloom character = %d", got)
	}
	if got := worldBloomMetadataCharacterID(nil, AutoQuery{}); got != 0 {
		t.Fatalf("empty world bloom character = %d", got)
	}
}

func TestRecommendOptionRefactorBranches(t *testing.T) {
	option := map[string]any{}
	applyRecommendProfileOverrides(option, AutoQuery{
		UseExactCardState:   true,
		MaxProfile:          true,
		SubMaxProfile:       true,
		SupportMasterMax:    true,
		SupportSkillMax:     true,
		MusicCompare:        true,
		MusicCompareQueries: []string{"a"},
		SpecificSkillOrder:  []int{1, 2},
	})
	for _, key := range []string{"max_profile", "sub_max_profile", "support_master_max", "support_skill_max", "music_compare", "keep_after_training_state"} {
		if option[key] != true {
			t.Fatalf("profile option %q = %#v", key, option[key])
		}
	}
	if option["skill_order_choose_strategy"] != "specific" {
		t.Fatalf("skill order strategy = %#v", option["skill_order_choose_strategy"])
	}

	patch := &CardConfigPatch{Disable: true, LevelMax: true, EpisodeRead: true, MasterMax: true, SkillMax: true, Canvas: true}
	applyDeckConfigPatch(option, "rarity_4_config", patch)
	cfg, ok := option["rarity_4_config"].(map[string]any)
	if !ok || len(cfg) != 6 {
		t.Fatalf("deck config patch = %#v", option["rarity_4_config"])
	}
	if got := toSingleCardConfigInterfaces([]SingleCardConfigPatch{{CardID: 0}, {CardID: 10, SkillMax: true}}); len(got) != 1 {
		t.Fatalf("single card configs = %#v", got)
	}
}

func TestRecommendRequestRefactorBranches(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	request := &drawing.DeckRequest{}
	option := map[string]any{
		"target":                          "bonus",
		"music_id":                        1,
		"music_diff":                      "expert",
		"multi_live_teammate_power":       250000,
		"multi_live_teammate_score_up":    200.5,
		"multi_live_score_up_lower_bound": 100.5,
		"boost":                           5,
		"skill_order_choose_strategy":     "specific",
		"skill_reference_choose_strategy": "max",
		"unit_filter":                     "idol",
		"attr_filter":                     "cute",
		"excluded_cards":                  []int{1},
		"fixed_cards":                     []int{2},
		"fixed_characters":                []int{3},
		"max_profile":                     true,
		"keep_after_training_state":       true,
	}
	controller.applyOptionRequestFields(request, option, AutoQuery{MusicTitle: "title", MusicCoverPath: "cover"})
	if request.MusicID == nil || *request.MusicID != 1 || request.MusicTitle == nil || *request.MusicTitle != "title" {
		t.Fatalf("music request fields = %+v", request)
	}
	if request.Boost == nil || *request.Boost != 5 || request.MultiLiveTeammatePower == nil || *request.MultiLiveTeammatePower != 250000 {
		t.Fatalf("scoring request fields = %+v", request)
	}
	if !reflect.DeepEqual(request.ExcludedCards, []int{1}) || !reflect.DeepEqual(request.FixedCardsID, []int{2}) || !reflect.DeepEqual(request.FixedCharactersID, []int{3}) {
		t.Fatalf("deck selection fields = %+v", request)
	}
	if !request.IsMaxDeck || !request.KeepAfterTrainingState {
		t.Fatalf("deck flags = %+v", request)
	}

	omakase := &drawing.DeckRequest{}
	applyRecommendMusicRequestFields(omakase, map[string]any{"music_id": 10000}, AutoQuery{})
	if omakase.MusicTitle == nil || *omakase.MusicTitle != "おまかせ (所有歌曲平均)" {
		t.Fatalf("omakase fields = %+v", omakase)
	}
}
