package deck

import (
	"context"
	"errors"
	"fmt"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

type additionalSequentialRecommender struct {
	calls int
	errAt int
	empty bool
}

func (*additionalSequentialRecommender) Enabled() bool { return true }
func (*additionalSequentialRecommender) Close()        {}

func (*additionalSequentialRecommender) ExpandAlgorithms(option map[string]any) []map[string]any {
	return []map[string]any{cloneRecommendOption(option)}
}

func (r *additionalSequentialRecommender) Recommend(req RecommendRequest) (*RecommendResult, error) {
	r.calls++
	if r.errAt > 0 && r.calls == r.errAt {
		return nil, errors.New("recommend failed")
	}
	result := &RecommendResult{
		CostTimes: map[string]float64{"GA": 2},
		WaitTimes: map[string]float64{"GA": 1},
	}
	if r.empty {
		return result, nil
	}
	charID := optionInt(req.BatchOption[0], "challenge_live_character_id")
	result.Decks = []RecommendDeck{{Score: 2_000_000 + charID}}
	if charID%2 == 0 {
		result.DeckAlgs = []string{"GA"}
	}
	return result, nil
}

func TestChallengeSequentialAdditional(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	recommender := &additionalSequentialRecommender{}
	result, err := controller.recommendChallengeAll(context.Background(), recommender, RecommendRequest{Region: "jp"}, map[string]any{"algorithm": "ga"})
	if err != nil {
		t.Fatalf("recommendChallengeAll sequential: %v", err)
	}
	if recommender.calls != challengeCharacterCount || len(result.Decks) != challengeCharacterCount || len(result.DeckAlgs) != challengeCharacterCount {
		t.Fatalf("sequential result = calls=%d decks=%d algs=%d", recommender.calls, len(result.Decks), len(result.DeckAlgs))
	}
	if result.Decks[0].ChallengeScoreDelta != 1_000_001 {
		t.Fatalf("character 1 delta = %d", result.Decks[0].ChallengeScoreDelta)
	}
	if result.DeckAlgs[0] != "" || result.DeckAlgs[1] != "GA" {
		t.Fatalf("deck algorithms = %#v", result.DeckAlgs[:2])
	}
	if result.CostTimes["GA"] != 2 || result.WaitTimes["GA"] != 1 {
		t.Fatalf("timing averages = %#v, %#v", result.CostTimes, result.WaitTimes)
	}

	if _, err := controller.recommendChallengeAll(context.Background(), nil, RecommendRequest{}, nil); err == nil {
		t.Fatal("expected nil recommender error")
	}
	if _, err := controller.recommendChallengeAllSequential(context.Background(), &additionalSequentialRecommender{errAt: 1}, RecommendRequest{}, nil); err == nil {
		t.Fatal("expected sequential recommendation error")
	}
	if _, err := controller.recommendChallengeAllSequential(context.Background(), &additionalSequentialRecommender{empty: true}, RecommendRequest{}, nil); err == nil {
		t.Fatal("expected no deck result error")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := controller.recommendChallengeAllSequential(canceled, &additionalSequentialRecommender{}, RecommendRequest{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sequential = %v", err)
	}
}

func TestChallengeAndWorldBloomBranchesAdditional(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	if err := controller.prepareChallengeRecommend(AutoQuery{}, nil); err != nil {
		t.Fatalf("nil challenge option = %v", err)
	}
	if err := controller.prepareChallengeRecommend(AutoQuery{MusicCompare: true}, map[string]any{}); err == nil {
		t.Fatal("expected compare character error")
	}
	if err := controller.prepareChallengeRecommend(AutoQuery{}, map[string]any{}); err != nil {
		t.Fatalf("non-current challenge = %v", err)
	}
	if err := controller.prepareChallengeRecommend(AutoQuery{UseCurrentDeck: true}, map[string]any{}); err == nil {
		t.Fatal("expected current challenge character error")
	}
	if !shouldRunChallengeAll(nil) || shouldRunChallengeAll(map[string]any{"challenge_live_character_id": 1}) {
		t.Fatal("unexpected challenge-all decision")
	}
	if averageChallengeSamples(nil) != 0 || averageChallengeSamples([]float64{1, 2, 3}) != 2 {
		t.Fatal("unexpected challenge average")
	}
	applyChallengeScoreDelta(nil, 1, nil)
	result := &RecommendResult{Decks: []RecommendDeck{{Score: 10}}}
	applyChallengeScoreDelta(result, 0, nil)
	if challengeHighScore(nil, 1) != 0 || challengeHighScore(controller.snapshot.RawData(), 999) != 0 {
		t.Fatal("unexpected missing challenge high score")
	}

	query := AutoQuery{EventUnit: "idol", WorldBloomEventTurn: deckIntPtr(2), WorldBloomCharacterID: deckIntPtr(5)}
	option := map[string]any{"event_id": 7, "event_attr": "cute"}
	fallbackQuery, fallbackOption, ok := buildWorldBloomSimulationFallback(query, option, " event ")
	if !ok || fallbackQuery.EventID != nil || optionInt(fallbackOption, "world_bloom_event_turn") != 2 || optionString(fallbackOption, "event_unit") != "idol" {
		t.Fatalf("world bloom fallback = %+v, %#v, %v", fallbackQuery, fallbackOption, ok)
	}
	if _, exists := fallbackOption["event_attr"]; exists {
		t.Fatal("fallback retained event_attr")
	}
	if _, _, ok := buildWorldBloomSimulationFallback(query, option, "challenge"); ok {
		t.Fatal("non-event fallback should fail")
	}
	if _, _, ok := buildWorldBloomSimulationFallback(query, map[string]any{}, "event"); ok {
		t.Fatal("missing event id fallback should fail")
	}
	if _, _, ok := buildWorldBloomSimulationFallback(AutoQuery{}, option, "event"); ok {
		t.Fatal("missing turn fallback should fail")
	}
	if _, _, ok := buildWorldBloomSimulationFallback(AutoQuery{WorldBloomEventTurn: deckIntPtr(3)}, option, "event"); ok {
		t.Fatal("missing character fallback should fail")
	}
	if _, _, ok := buildWorldBloomSimulationFallback(AutoQuery{WorldBloomEventTurn: deckIntPtr(2), WorldBloomCharacterID: deckIntPtr(1)}, option, "event"); ok {
		t.Fatal("early turn without unit fallback should fail")
	}
	turn := 3
	character := 6
	metadataQuery := AutoQuery{MetadataWorldBloomEventTurn: &turn, MetadataWorldBloomCharacterID: &character}
	if worldBloomSimulationTurn(metadataQuery) != 3 || worldBloomSimulationCharacterID(metadataQuery, nil) != 6 {
		t.Fatal("metadata world bloom fields not preferred")
	}
	if worldBloomSimulationTurn(AutoQuery{}) != 0 || worldBloomSimulationCharacterID(AutoQuery{}, map[string]any{"world_bloom_character_id": 8}) != 8 {
		t.Fatal("world bloom fallback values mismatch")
	}
	if _, _, ok := buildWorldBloomSimulationFallbackOnError(query, option, "event", nil); ok {
		t.Fatal("nil error fallback should fail")
	}
	err := fmt.Errorf("deck-service: event not found for eventId: 7")
	if _, _, ok := buildWorldBloomSimulationFallbackOnError(query, option, "event", err); !ok {
		t.Fatal("matching event error should fall back")
	}
	if _, _, ok := buildWorldBloomSimulationFallbackOnError(query, option, "event", errors.New("other")); ok {
		t.Fatal("unrelated error should not fall back")
	}
	if isDeckServiceEventNotFoundForID(nil, 7) || isDeckServiceEventNotFoundForID(err, 0) || isDeckServiceEventNotFoundForID(errors.New("other"), 7) || isDeckServiceEventNotFoundForID(err, 8) {
		t.Fatal("unexpected event-not-found match")
	}
	if !isDeckServiceEventNotFoundForID(err, 7) || *deckIntPtr(9) != 9 {
		t.Fatal("event-not-found helper mismatch")
	}

	controller.recommendCfg.MasterdataDir = t.TempDir()
	gotQuery, gotOption := controller.applyWorldBloomSimulationFallbackIfMasterdataMissing(renderregion.JP, "event", query, option)
	if gotQuery.EventUnit != query.EventUnit || optionInt(gotOption, "event_id") != 7 {
		t.Fatal("unchecked masterdata should preserve request")
	}
}
