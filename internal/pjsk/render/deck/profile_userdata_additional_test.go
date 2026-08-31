package deck

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
)

type additionalEpisodeSource struct {
	episodes []snapshot.RawUserCardEpisode
	err      error
}

func (s additionalEpisodeSource) GetCardEpisodes(int) ([]snapshot.RawUserCardEpisode, error) {
	return s.episodes, s.err
}

type additionalBasicCardSource struct {
	region renderregion.Value
	card   *masterdata.Card
	err    error
}

func (s *additionalBasicCardSource) DefaultRegion() renderregion.Value { return s.region }
func (s *additionalBasicCardSource) GetCardByID(int) (*masterdata.Card, error) {
	return s.card, s.err
}

type additionalFailingAllCardSource struct {
	additionalBasicCardSource
	allErr     error
	episodeErr error
}

func (s *additionalFailingAllCardSource) GetAllCards() ([]*masterdata.Card, error) {
	if s.allErr != nil {
		return nil, s.allErr
	}
	return []*masterdata.Card{s.card}, nil
}

func (s *additionalFailingAllCardSource) GetCardEpisodes(int) ([]snapshot.RawUserCardEpisode, error) {
	return nil, s.episodeErr
}

type additionalHonorEventSource struct {
	region  renderregion.Value
	rewards []masterdata.EventRankingHonorReward
	err     error
}

func (s *additionalHonorEventSource) DefaultRegion() renderregion.Value { return s.region }
func (s *additionalHonorEventSource) GetEventByID(int) (*masterdata.Event, error) {
	return &masterdata.Event{ID: 180}, nil
}
func (s *additionalHonorEventSource) GetEvents() []*masterdata.Event { return nil }
func (s *additionalHonorEventSource) GetEventRankingHonorRewards(int) ([]masterdata.EventRankingHonorReward, error) {
	return s.rewards, s.err
}

func TestMaxProfileCardHelpersAdditional(t *testing.T) {
	assertMaxProfileCanvasHelpers(t)
	assertMaxProfileEpisodeHelpers(t)
	assertMaxProfileCardStateHelpers(t)
}

func assertMaxProfileCanvasHelpers(t *testing.T) {
	t.Helper()
	if got := buildMaxProfileCanvases(nil); got != nil {
		t.Fatalf("empty max-profile canvases = %#v", got)
	}
	canvases := buildMaxProfileCanvases([]snapshot.RawUserCard{{CardID: 0}, {CardID: 2}, {CardID: 1}})
	if !reflect.DeepEqual(canvases, []snapshot.RawUserMysekaiCanvas{{CardID: 2, Quantity: 1}, {CardID: 1, Quantity: 1}}) {
		t.Fatalf("max-profile canvases = %#v", canvases)
	}
}

func assertMaxProfileEpisodeHelpers(t *testing.T) {
	t.Helper()
	if episodes, err := maxProfileCardEpisodes(nil, 1); err != nil || episodes != nil {
		t.Fatalf("nil episode source = %#v, %v", episodes, err)
	}
	if episodes, err := maxProfileCardEpisodes(additionalEpisodeSource{}, 0); err != nil || episodes != nil {
		t.Fatalf("zero card episodes = %#v, %v", episodes, err)
	}
	wantErr := errors.New("episode failed")
	if _, err := maxProfileCardEpisodes(additionalEpisodeSource{err: wantErr}, 1); !errors.Is(err, wantErr) {
		t.Fatalf("episode error = %v", err)
	}
	originalEpisodes := []snapshot.RawUserCardEpisode{{CardEpisodeID: 1, ScenarioStatus: "read"}}
	episodes, err := maxProfileCardEpisodes(additionalEpisodeSource{episodes: originalEpisodes}, 1)
	if err != nil || !reflect.DeepEqual(episodes, originalEpisodes) {
		t.Fatalf("episodes = %#v, %v", episodes, err)
	}
	episodes[0].CardEpisodeID = 99
	if originalEpisodes[0].CardEpisodeID != 1 {
		t.Fatal("max-profile episodes were not cloned")
	}
}

func assertMaxProfileCardStateHelpers(t *testing.T) {
	t.Helper()
	for rarity, level := range map[string]int{" rarity_1 ": 20, "RARITY_2": 30, "rarity_3": 50, "rarity_4": 60, "unknown": 60} {
		if got := maxProfileCardLevel(rarity); got != level {
			t.Errorf("maxProfileCardLevel(%q) = %d", rarity, got)
		}
	}
	if cardHasAfterTraining(nil) {
		t.Fatal("nil card has after-training state")
	}
	for _, rarity := range []string{"rarity_1", "rarity_2"} {
		card := &masterdata.Card{CardRarityType: rarity}
		if cardHasAfterTraining(card) || maxProfileTrainingStatus(card) != "none" || maxProfileDefaultImage(card) != "normal" {
			t.Errorf("low-rarity training helpers mismatch for %q", rarity)
		}
	}
	for _, rarity := range []string{"rarity_3", " RARITY_4 "} {
		card := &masterdata.Card{CardRarityType: rarity}
		if !cardHasAfterTraining(card) || maxProfileTrainingStatus(card) != "done" || maxProfileDefaultImage(card) != "special_training" {
			t.Errorf("high-rarity training helpers mismatch for %q", rarity)
		}
	}
}

func TestBuildAndApplyMaxProfileAdditional(t *testing.T) {
	assertBuildMaxProfileErrors(t)
	controller, now := assertBuildMaxProfileCards(t)
	assertApplyMaxProfile(t, controller, now)
}

func assertBuildMaxProfileErrors(t *testing.T) {
	t.Helper()
	if _, err := (&Controller{}).buildMaxProfileCards(renderregion.JP, 1); err == nil {
		t.Fatal("max profile without card source unexpectedly succeeded")
	}
	basic := &additionalBasicCardSource{region: renderregion.JP, card: &masterdata.Card{ID: 1}}
	controller := NewController(basic, nil, nil, nil, nil, renderregion.JP)
	if _, err := controller.buildMaxProfileCards(renderregion.JP, 1); err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("basic card source error = %v", err)
	}
	allErr := errors.New("all cards failed")
	failing := &additionalFailingAllCardSource{additionalBasicCardSource: *basic, allErr: allErr}
	controller = NewController(failing, nil, nil, nil, nil, renderregion.JP)
	if _, err := controller.buildMaxProfileCards(renderregion.JP, 1); !errors.Is(err, allErr) {
		t.Fatalf("all-card error = %v", err)
	}
	failing.allErr = nil
	failing.episodeErr = errors.New("episodes failed")
	if _, err := controller.buildMaxProfileCards(renderregion.JP, 1); !errors.Is(err, failing.episodeErr) {
		t.Fatalf("max-profile episode error = %v", err)
	}
}

func assertBuildMaxProfileCards(t *testing.T) (*Controller, int64) {
	t.Helper()
	now := time.Now().UnixMilli()
	source := &testCardSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			0:  nil,
			1:  {ID: 1, CardRarityType: "rarity_1"},
			2:  {ID: 2, CardRarityType: "rarity_2"},
			3:  {ID: 3, CardRarityType: "rarity_3"},
			4:  {ID: 4, CardRarityType: "rarity_4"},
			5:  {ID: 5, CardRarityType: "rarity_4", ReleaseAt: now + 100_000},
			-1: {ID: -1},
		},
		episodes: map[int][]snapshot.RawUserCardEpisode{
			3: {{CardEpisodeID: 30, ScenarioStatus: "read"}},
		},
		areaItemLevelCaps: map[int]int{1: 20, 2: 18, 3: 0},
		mysekaiGates:      []snapshot.RawUserMysekaiGate{{MysekaiGateID: 1, MysekaiGateLevel: 40}},
		fixtureBonuses:    []snapshot.RawUserFixtureBonus{{GameCharacterID: 1, TotalBonusRate: 10}},
	}
	controller := NewController(source, nil, nil, nil, nil, renderregion.JP)
	cards, err := controller.buildMaxProfileCards(renderregion.JP, now)
	if err != nil || len(cards) != 4 || cards[0].CardID != 1 || cards[3].CardID != 4 || cards[2].Level != 50 || len(cards[2].Episodes) != 1 {
		t.Fatalf("max-profile cards = %#v, %v", cards, err)
	}
	if _, err := controller.buildMaxProfileCards(renderregion.JP, 0); err != nil {
		t.Fatalf("max profile with wall-clock now failed: %v", err)
	}
	return controller, now
}

func assertApplyMaxProfile(t *testing.T, controller *Controller, now int64) {
	t.Helper()
	if err := controller.applyProfilePreset(renderregion.JP, nil, AutoQuery{MaxProfile: true}); err != nil {
		t.Fatalf("nil profile preset = %v", err)
	}
	raw := &snapshot.RawUserData{Now: now}
	if err := controller.applyProfilePreset(renderregion.JP, raw, AutoQuery{}); err != nil || len(raw.UserCards) != 0 {
		t.Fatalf("disabled profile preset = %#v, %v", raw, err)
	}
	if err := controller.applyProfilePreset(renderregion.JP, raw, AutoQuery{MaxProfile: true}); err != nil {
		t.Fatalf("max profile preset failed: %v", err)
	}
	if len(raw.UserCards) != 4 || len(raw.UserMysekaiCanvases) != 4 || len(raw.UserMysekaiGates) != 1 || len(raw.UserMysekaiFixtureGameCharacterPerformanceBonuses) != 1 || len(raw.UserAreas) != 1 {
		t.Fatalf("applied max profile = %#v", raw)
	}
	subRaw := &snapshot.RawUserData{Now: now}
	if err := controller.applyProfilePreset(renderregion.JP, subRaw, AutoQuery{SubMaxProfile: true}); err != nil {
		t.Fatalf("sub-max profile preset failed: %v", err)
	}
	for _, item := range subRaw.UserAreas[0].AreaItems {
		if item.Level > 15 {
			t.Fatalf("sub-max area item level = %d", item.Level)
		}
	}
}

func TestProfileCardFiltersAndUnitResolutionAdditional(t *testing.T) {
	if err := (&Controller{}).applyUserCardFilters(renderregion.JP, nil, AutoQuery{UnitFilter: "idol"}); err != nil {
		t.Fatalf("nil card filtering failed: %v", err)
	}
	raw := &snapshot.RawUserData{UserCards: []snapshot.RawUserCard{{CardID: 1}}}
	if err := (&Controller{}).applyUserCardFilters(renderregion.JP, raw, AutoQuery{}); err != nil || len(raw.UserCards) != 1 {
		t.Fatalf("no-op card filtering = %#v, %v", raw.UserCards, err)
	}
	if err := (&Controller{}).applyUserCardFilters(renderregion.JP, raw, AutoQuery{ExcludedCards: []int{1}}); err == nil {
		t.Fatal("card filtering without source unexpectedly succeeded")
	}

	source := &testCardSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1: {ID: 1, CharacterID: 1, Attr: "cute", SupportUnit: "idol"},
			2: {ID: 2, CharacterID: 2, Attr: "cool", SupportUnit: "none"},
			3: nil,
		},
		characters: map[int]*masterdata.Character{
			1: {ID: 1, Unit: "idol"},
			2: {ID: 2, Unit: "piapro"},
		},
	}
	controller := NewController(source, nil, nil, nil, nil, renderregion.JP)
	raw = &snapshot.RawUserData{UserCards: []snapshot.RawUserCard{{CardID: 1}, {CardID: 2}, {CardID: 3}, {CardID: 99}}}
	if err := controller.applyUserCardFilters(renderregion.JP, raw, AutoQuery{UnitFilter: "mmj", AttrFilter: "cute", ExcludedCards: []int{-1, 2}}); err != nil {
		t.Fatalf("card filtering failed: %v", err)
	}
	if len(raw.UserCards) != 1 || raw.UserCards[0].CardID != 1 {
		t.Fatalf("filtered cards = %#v", raw.UserCards)
	}
	raw = &snapshot.RawUserData{UserCards: []snapshot.RawUserCard{{CardID: 3}, {CardID: 99}}}
	if err := controller.applyUserCardFilters(renderregion.JP, raw, AutoQuery{ExcludedCards: []int{-1}}); err != nil || len(raw.UserCards) != 2 {
		t.Fatalf("missing-card exclusion-only filtering = %#v, %v", raw.UserCards, err)
	}

	if got := controller.resolveCardUnit(nil, &masterdata.Card{}, "idol"); got != "" {
		t.Fatalf("nil source unit = %q", got)
	}
	if got := controller.resolveCardUnit(source, nil, "idol"); got != "" {
		t.Fatalf("nil card unit = %q", got)
	}
	if got := controller.resolveCardUnit(source, source.cards[1], "idol"); got != "idol" {
		t.Fatalf("card-unit resolver unit = %q", got)
	}
	if got := controller.resolveCardUnit(source, source.cards[2], "piapro"); got != "piapro" {
		t.Fatalf("character fallback unit = %q", got)
	}
	basic := &additionalBasicCardSource{region: renderregion.JP}
	if got := controller.resolveCardUnit(basic, &masterdata.Card{SupportUnit: "street"}, "street"); got != "street" {
		t.Fatalf("support-unit fallback = %q", got)
	}
	if got := controller.resolveCardUnit(basic, &masterdata.Card{}, "piapro"); got != "" {
		t.Fatalf("unresolved virtual-singer unit = %q", got)
	}
}

func TestMaxProfileHonorHelpersAdditional(t *testing.T) {
	rewards := assertMaxProfileHonorSelection(t)
	assertMaxProfileHonorOrdering(t)
	assertMaxProfileHonorUpserts(t)
	assertApplyMaxProfileHonors(t, rewards)
}

func assertMaxProfileHonorSelection(t *testing.T) []masterdata.EventRankingHonorReward {
	t.Helper()
	if _, ok := pickMaxProfileRankingHonorReward(nil, 1000); ok {
		t.Fatal("empty honor rewards unexpectedly selected")
	}
	rewards := []masterdata.EventRankingHonorReward{
		{HonorID: 0, FromRank: 1, ToRank: 1000},
		{HonorID: 5, FromRank: 2000, ToRank: 3000},
		{HonorID: 4, FromRank: 1, ToRank: 2000},
		{HonorID: 3, FromRank: 1, ToRank: 1000},
		{HonorID: 2, FromRank: 1, ToRank: 1000},
	}
	reward, ok := pickMaxProfileRankingHonorReward(rewards, 1000)
	if !ok || reward.HonorID != 2 {
		t.Fatalf("selected honor reward = %+v, %v", reward, ok)
	}
	if _, ok := pickMaxProfileRankingHonorReward([]masterdata.EventRankingHonorReward{{HonorID: 0}}, 1000); ok {
		t.Fatal("invalid honor reward unexpectedly selected")
	}
	return rewards
}

func assertMaxProfileHonorOrdering(t *testing.T) {
	t.Helper()
	for _, tt := range []struct {
		reward masterdata.EventRankingHonorReward
		rank   int
		want   bool
	}{
		{masterdata.EventRankingHonorReward{}, 0, false},
		{masterdata.EventRankingHonorReward{FromRank: 10}, 9, false},
		{masterdata.EventRankingHonorReward{ToRank: 10}, 11, false},
		{masterdata.EventRankingHonorReward{FromRank: 5, ToRank: 10}, 7, true},
		{masterdata.EventRankingHonorReward{}, 7, true},
	} {
		if got := rewardContainsRank(tt.reward, tt.rank); got != tt.want {
			t.Errorf("rewardContainsRank(%+v, %d) = %v", tt.reward, tt.rank, got)
		}
	}
	if !rankingHonorRewardLess(masterdata.EventRankingHonorReward{ToRank: 10}, masterdata.EventRankingHonorReward{ToRank: 0}) {
		t.Fatal("finite reward range did not sort before open range")
	}
	if !rankingHonorRewardLess(masterdata.EventRankingHonorReward{ToRank: 10, FromRank: 1}, masterdata.EventRankingHonorReward{ToRank: 10, FromRank: 2}) {
		t.Fatal("reward from-rank tiebreak mismatch")
	}
	if !rankingHonorRewardLess(masterdata.EventRankingHonorReward{ToRank: 10, FromRank: 1, HonorID: 1}, masterdata.EventRankingHonorReward{ToRank: 10, FromRank: 1, HonorID: 2}) {
		t.Fatal("reward honor-ID tiebreak mismatch")
	}
}

func assertMaxProfileHonorUpserts(t *testing.T) {
	t.Helper()
	originalHonors := []snapshot.RawUserHonor{{Seq: 3, HonorID: 10}}
	if got := upsertMaxProfileUserHonor(originalHonors, 0); !reflect.DeepEqual(got, originalHonors) {
		t.Fatalf("invalid honor upsert = %#v", got)
	}
	gotHonors := upsertMaxProfileUserHonor(originalHonors, 10)
	if gotHonors[0].HonorLevel != 1 || !gotHonors[0].ProfilePlayer {
		t.Fatalf("updated user honor = %#v", gotHonors)
	}
	gotHonors = upsertMaxProfileUserHonor(originalHonors, 20)
	if len(gotHonors) != 2 || gotHonors[1].Seq != 4 || gotHonors[1].HonorID != 20 {
		t.Fatalf("appended user honor = %#v", gotHonors)
	}
	if originalHonors[0].HonorLevel != 0 {
		t.Fatal("user honor input was mutated")
	}

	originalProfiles := []snapshot.RawUserProfileHonor{{Seq: 2, HonorID: 20}, {Seq: 1, HonorID: 10}}
	if got := upsertMaxProfileProfileHonor(originalProfiles, 0); !reflect.DeepEqual(got, originalProfiles) {
		t.Fatalf("invalid profile honor upsert = %#v", got)
	}
	gotProfiles := upsertMaxProfileProfileHonor(originalProfiles, 30)
	if len(gotProfiles) != 2 || gotProfiles[0].Seq != 1 || gotProfiles[0].HonorID != 30 || gotProfiles[1].Seq != 2 {
		t.Fatalf("replaced profile honor = %#v", gotProfiles)
	}
	gotProfiles = upsertMaxProfileProfileHonor([]snapshot.RawUserProfileHonor{{Seq: 2, HonorID: 20}}, 30)
	if len(gotProfiles) != 2 || gotProfiles[0].Seq != 1 {
		t.Fatalf("inserted profile honor = %#v", gotProfiles)
	}
}

func assertApplyMaxProfileHonors(t *testing.T, rewards []masterdata.EventRankingHonorReward) {
	t.Helper()
	eventID := 180
	raw := &snapshot.RawUserData{}
	(&Controller{}).applyMaxProfileEventHonors(renderregion.JP, raw, AutoQuery{EventID: &eventID})
	if len(raw.UserHonors) != 0 {
		t.Fatal("honor applied without event source")
	}
	eventSource := &additionalHonorEventSource{region: renderregion.JP, rewards: rewards}
	controller := NewController(nil, eventSource, nil, nil, nil, renderregion.JP)
	controller.applyMaxProfileEventHonors(renderregion.JP, raw, AutoQuery{EventID: &eventID})
	if len(raw.UserHonors) != 1 || raw.UserHonors[0].HonorID != 2 || len(raw.UserProfileHonors) != 1 {
		t.Fatalf("applied event honors = %#v / %#v", raw.UserHonors, raw.UserProfileHonors)
	}
	eventSource.err = errors.New("rewards failed")
	controller.applyMaxProfileEventHonors(renderregion.JP, &snapshot.RawUserData{}, AutoQuery{EventID: &eventID})
	controller.applyMaxProfileEventHonors(renderregion.JP, nil, AutoQuery{EventID: &eventID})
	otherEvent := 1
	controller.applyMaxProfileEventHonors(renderregion.JP, &snapshot.RawUserData{}, AutoQuery{EventID: &otherEvent})
}

func TestCurrentDeckAndFixedCardHelpersAdditional(t *testing.T) {
	profile, deck := assertCurrentDeckCardHelpers(t)
	assertApplyCurrentDeckHelpers(t, profile, deck)
}

func assertCurrentDeckCardHelpers(t *testing.T) (*sekai.GetAnotherProfileResponse, *snapshot.RawUserDeck) {
	t.Helper()
	if publicProfileCurrentDeck(nil) != nil || publicProfileCurrentDeck(&sekai.GetAnotherProfileResponse{}) != nil {
		t.Fatal("empty public profile returned a current deck")
	}
	profile := &sekai.GetAnotherProfileResponse{UserDeck: sekai.UserDeck{DeckID: 1, Leader: 3, Member1: 1, Member2: 2, Member3: 3, Member4: 4, Member5: 5}}
	deck := publicProfileCurrentDeck(profile)
	if deck == nil || deck.DeckID != 1 || deck.Member5 != 5 {
		t.Fatalf("public current deck = %#v", deck)
	}
	if cards, ok := currentDeckFixedCardIDs(nil); ok || cards != nil {
		t.Fatalf("nil fixed deck = %#v, %v", cards, ok)
	}
	incomplete := &snapshot.RawUserDeck{Leader: 1, Member1: 1}
	if _, ok := currentDeckFixedCardIDs(incomplete); ok {
		t.Fatal("incomplete deck unexpectedly accepted")
	}
	ordered, ok := currentDeckFixedCardIDs(deck)
	if !ok || !reflect.DeepEqual(ordered, []int{3, 1, 2, 4, 5}) {
		t.Fatalf("leader-ordered fixed cards = %#v, %v", ordered, ok)
	}
	deck.Leader = 1
	ordered, ok = currentDeckFixedCardIDs(deck)
	if !ok || !reflect.DeepEqual(ordered, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("already-leading fixed cards = %#v, %v", ordered, ok)
	}
	return profile, deck
}

func assertApplyCurrentDeckHelpers(t *testing.T, profile *sekai.GetAnotherProfileResponse, deck *snapshot.RawUserDeck) {
	t.Helper()
	controller := &Controller{}
	option := map[string]any{"fixed_characters": []int{1}}
	if err := controller.applyCurrentDeckOption(nil, nil, "event", AutoQuery{}, option); err != nil {
		t.Fatalf("disabled current deck = %v", err)
	}
	if err := controller.applyCurrentDeckOption(nil, nil, "challenge", AutoQuery{UseCurrentDeck: true}, option); err != nil {
		t.Fatalf("challenge current deck = %v", err)
	}
	if err := controller.applyCurrentDeckOption(nil, nil, "event", AutoQuery{UseCurrentDeck: true}, option); err == nil {
		t.Fatal("nil original current deck unexpectedly succeeded")
	}
	if err := controller.applyCurrentDeckOption(nil, &snapshot.RawUserData{}, "event", AutoQuery{UseCurrentDeck: true}, option); err == nil {
		t.Fatal("missing active deck unexpectedly succeeded")
	}
	badProfile := &sekai.GetAnotherProfileResponse{UserDeck: sekai.UserDeck{DeckID: 1, Member1: 1}}
	if err := controller.applyCurrentDeckOption(nil, nil, "event", AutoQuery{UseCurrentDeck: true, PublicProfileResp: badProfile}, option); err == nil {
		t.Fatal("incomplete public current deck unexpectedly succeeded")
	}
	option = map[string]any{"fixed_characters": []int{1}}
	if err := controller.applyCurrentDeckOption(nil, nil, "event", AutoQuery{UseCurrentDeck: true, PublicProfileResp: profile}, option); err != nil {
		t.Fatalf("public current deck option failed: %v", err)
	}
	if _, ok := option["fixed_characters"]; ok || option["best_skill_as_leader"] != false || len(option["fixed_cards"].([]int)) != 5 {
		t.Fatalf("public current deck option = %#v", option)
	}
	original := &snapshot.RawUserData{UserGamedata: snapshot.RawUserGamedata{Deck: 1}, UserDecks: []snapshot.RawUserDeck{{DeckID: 1, Leader: 1, Member1: 1, Member2: 2, Member3: 3, Member4: 4, Member5: 5}}}
	option = map[string]any{"fixed_characters": []int{1}}
	if err := controller.applyCurrentDeckOption(nil, original, "event", AutoQuery{UseCurrentDeck: true}, option); err != nil {
		t.Fatalf("snapshot current deck option failed: %v", err)
	}
}

func TestRestoreFixedCardsAndAreaItemsAdditional(t *testing.T) {
	source := &testCardSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			3: {ID: 3, CardRarityType: "rarity_3"},
			4: nil,
		},
		episodes:          map[int][]snapshot.RawUserCardEpisode{3: {{CardEpisodeID: 30}}},
		areaItemLevelCaps: map[int]int{1: 20, 2: 10, 3: 0},
	}
	controller := NewController(source, nil, nil, nil, nil, renderregion.JP)
	assertRestoreFixedCards(t, controller)
	assertFallbackRecommendCards(t, controller)
	assertAreaItemLevelHelpers(t, controller)
}

func assertRestoreFixedCards(t *testing.T, controller *Controller) {
	t.Helper()
	if err := controller.restoreFixedCards(renderregion.JP, nil, &snapshot.RawUserData{}, map[string]any{}, false); err != nil {
		t.Fatalf("nil fixed-card restore failed: %v", err)
	}
	raw := &snapshot.RawUserData{Now: time.Now().UnixMilli(), UserCards: []snapshot.RawUserCard{{CardID: 1, Level: 1}, {CardID: 3, Level: 1}}}
	original := &snapshot.RawUserData{UserCards: []snapshot.RawUserCard{{CardID: 1, Level: 50}, {CardID: 2, Level: 40}}}
	if err := controller.restoreFixedCards(renderregion.JP, raw, original, map[string]any{"fixed_cards": "bad"}, false); err != nil {
		t.Fatalf("invalid fixed-card option failed: %v", err)
	}
	option := map[string]any{"fixed_cards": []int{0, 1, 2, 3}}
	if err := controller.restoreFixedCards(renderregion.JP, raw, original, option, true); err != nil {
		t.Fatalf("fixed-card restore failed: %v", err)
	}
	if len(raw.UserCards) != 3 || snapshot.FindUserCard(raw.UserCards, 1).Level != 50 || snapshot.FindUserCard(raw.UserCards, 2) == nil {
		t.Fatalf("restored fixed cards = %#v", raw.UserCards)
	}
	if card := snapshot.FindUserCard(raw.UserCards, 3); card == nil || card.SkillLevel != 0 {
		t.Fatalf("existing fallback card changed = %#v", card)
	}
	if err := controller.restoreFixedCards(renderregion.JP, &snapshot.RawUserData{}, &snapshot.RawUserData{}, map[string]any{"fixed_cards": []int{4}}, false); err == nil {
		t.Fatal("unknown fixed card unexpectedly restored")
	}
}

func assertFallbackRecommendCards(t *testing.T, controller *Controller) {
	t.Helper()
	if card, err := controller.buildFallbackRecommendUserCard(renderregion.JP, 0, 0, false); err != nil || card != nil {
		t.Fatalf("zero fallback card = %#v, %v", card, err)
	}
	card, err := controller.buildFallbackRecommendUserCard(renderregion.JP, 3, 0, false)
	if err != nil || card == nil || card.SkillLevel != 1 || card.MasterRank != 0 || card.Level != 50 || len(card.Episodes) != 1 {
		t.Fatalf("unowned fallback card = %#v, %v", card, err)
	}
	card, err = controller.buildFallbackRecommendUserCard(renderregion.JP, 3, 0, true)
	if err != nil || card.SkillLevel != 4 || card.MasterRank != 5 {
		t.Fatalf("owned fallback card = %#v, %v", card, err)
	}
	if card, err := controller.buildFallbackRecommendUserCard(renderregion.JP, 4, 0, false); err != nil || card != nil {
		t.Fatalf("missing fallback card = %#v, %v", card, err)
	}
}

func assertAreaItemLevelHelpers(t *testing.T, controller *Controller) {
	t.Helper()
	controller.applyAreaItemCaps(renderregion.JP, nil, 0)
	areaRaw := &snapshot.RawUserData{}
	controller.applyAreaItemCaps(renderregion.JP, areaRaw, 15)
	if len(areaRaw.UserAreas) != 1 || len(areaRaw.UserAreas[0].AreaItems) != 2 {
		t.Fatalf("area item caps = %#v", areaRaw.UserAreas)
	}
	if err := controller.applyAreaItemLevel(renderregion.JP, nil, 10); err != nil {
		t.Fatalf("nil area level failed: %v", err)
	}
	if err := controller.applyAreaItemLevel(renderregion.JP, areaRaw, 0); err != nil {
		t.Fatalf("zero area level failed: %v", err)
	}
	if err := controller.applyAreaItemLevel(renderregion.JP, areaRaw, 15); err == nil {
		t.Fatal("area level above regional cap unexpectedly succeeded")
	}
	if err := controller.applyAreaItemLevel(renderregion.JP, areaRaw, 10); err != nil {
		t.Fatalf("area level at caps failed: %v", err)
	}
	if levels := collectRawAreaItemLevels([]snapshot.RawUserArea{{AreaItems: []snapshot.RawUserAreaItem{{AreaItemID: 0, Level: 9}, {AreaItemID: 1, Level: 2}, {AreaItemID: 1, Level: 5}}}}); levels[1] != 5 || len(levels) != 1 {
		t.Fatalf("collected area levels = %#v", levels)
	}
	if buildRawUserAreas(nil) != nil || buildRawUserAreas(map[int]int{0: 1, 1: 0, -1: 2}) != nil {
		t.Fatal("invalid raw area levels produced areas")
	}
	areas := buildRawUserAreas(map[int]int{3: 3, 1: 1, 2: 2})
	if len(areas) != 1 || !reflect.DeepEqual(areas[0].AreaItems, []snapshot.RawUserAreaItem{{AreaItemID: 1, Level: 1}, {AreaItemID: 2, Level: 2}, {AreaItemID: 3, Level: 3}}) {
		t.Fatalf("sorted raw user areas = %#v", areas)
	}
	assertAreaItemLevelFallback(t)
}

func assertAreaItemLevelFallback(t *testing.T) {
	t.Helper()
	noCaps := NewController(&additionalBasicCardSource{region: renderregion.JP}, nil, nil, nil, nil, renderregion.JP)
	noCapsRaw := &snapshot.RawUserData{UserAreas: []snapshot.RawUserArea{{AreaItems: []snapshot.RawUserAreaItem{{AreaItemID: 1, Level: 2}}}}}
	if err := noCaps.applyAreaItemLevel(renderregion.JP, noCapsRaw, 5); err != nil || noCapsRaw.UserAreas[0].AreaItems[0].Level != 5 {
		t.Fatalf("fallback area leveling = %#v, %v", noCapsRaw.UserAreas, err)
	}
	emptyRaw := &snapshot.RawUserData{}
	if err := noCaps.applyAreaItemLevel(renderregion.JP, emptyRaw, 5); err != nil || len(emptyRaw.UserAreas) != 0 {
		t.Fatalf("empty fallback area leveling = %#v, %v", emptyRaw.UserAreas, err)
	}
}

func TestPreparedUserDataMergingAdditional(t *testing.T) {
	raw := &snapshot.RawUserData{
		UserCards: []snapshot.RawUserCard{
			{CardID: 1, Level: 10, SkillLevel: 2, MasterRank: 1, SpecialTrainingStatus: "none", DefaultImage: "normal"},
			{CardID: 2, Level: 20, SkillLevel: 3, MasterRank: 2, SpecialTrainingStatus: "done", DefaultImage: "special_training", Episodes: []snapshot.RawUserCardEpisode{{CardEpisodeID: 20}}},
			{CardID: 3, Level: 30},
		},
		UserAreas: []snapshot.RawUserArea{
			{AreaItems: []snapshot.RawUserAreaItem{{AreaItemID: 1, Level: 5}, {AreaItemID: 2, Level: 6}}},
			{AreaItems: []snapshot.RawUserAreaItem{{AreaItemID: 3, Level: 7}}},
		},
		UserHonors:        []snapshot.RawUserHonor{{Seq: 1, HonorID: 10, HonorLevel: 1}, {Seq: 2, HonorID: 20, HonorLevel: 2}},
		UserProfileHonors: []snapshot.RawUserProfileHonor{{Seq: 1, HonorID: 10}, {Seq: 2, HonorID: 20}},
	}
	original := &snapshot.RawUserData{UserCards: []snapshot.RawUserCard{raw.UserCards[0], {CardID: 2, Level: 19}}}
	originalJSON := []byte(`{
		"untouched":{"x":1},
		"userCards":[
			{"cardId":0,"ignored":true},
			{"cardId":1,"level":10,"skillLevel":2,"masterRank":1,"specialTrainingStatus":"none","defaultImage":"normal","episodes":null,"custom":"keep"},
			{"cardId":2,"level":19,"custom":"merge"}
		],
		"userAreas":[{"areaItems":[{"areaItemId":0},{"areaItemId":1,"legacy":"keep"}],"custom":"area","userAreaStatus":null}],
		"userHonors":[{"honorId":0},{"honorId":10,"custom":"honor"}],
		"userProfileHonors":[{"seq":0},{"seq":1,"custom":"profile"}]
	}`)
	payload := assertPreparedUserDataEncoding(t, raw, original, originalJSON)
	assertPreparedMergedPayload(t, payload)
	assertPreparedEmptyMerges(t)
	assertPreparedCardComparison(t)
	assertPreparedJSONNormalization(t)
}

func assertPreparedUserDataEncoding(t *testing.T, raw, original *snapshot.RawUserData, originalJSON []byte) map[string]any {
	t.Helper()
	if _, err := encodePreparedRecommendUserData(nil, nil, nil); err == nil {
		t.Fatal("nil prepared raw data unexpectedly encoded")
	}
	encoded, err := encodePreparedRecommendUserData(nil, nil, raw)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("empty-original encode = %s, %v", encoded, err)
	}
	encoded, err = encodePreparedRecommendUserData([]byte("null"), nil, raw)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("null-original encode = %s, %v", encoded, err)
	}
	if _, err := encodePreparedRecommendUserData([]byte("{"), nil, raw); err == nil {
		t.Fatal("malformed original snapshot unexpectedly encoded")
	}
	encoded, err = encodePreparedRecommendUserData(originalJSON, original, raw)
	if err != nil {
		t.Fatalf("rich prepared snapshot encode: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode prepared snapshot: %v", err)
	}
	return payload
}

func assertPreparedMergedPayload(t *testing.T, payload map[string]any) {
	t.Helper()
	cards := payload["userCards"].([]any)
	if len(cards) != 3 || cards[0].(map[string]any)["custom"] != "keep" || cards[1].(map[string]any)["custom"] != "merge" {
		t.Fatalf("merged prepared cards = %#v", cards)
	}
	if _, ok := cards[0].(map[string]any)["episodes"]; ok {
		t.Fatal("nil episodes field was not normalized")
	}
	areas := payload["userAreas"].([]any)
	if len(areas) != 2 || areas[0].(map[string]any)["custom"] != "area" || areas[0].(map[string]any)["userAreaStatus"] == nil {
		t.Fatalf("merged prepared areas = %#v", areas)
	}
}

func assertPreparedEmptyMerges(t *testing.T) {
	t.Helper()
	if got, err := mergePreparedUserCards(nil, nil, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty merged cards = %#v, %v", got, err)
	}
	if got, err := mergePreparedUserAreas(nil, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty merged areas = %#v, %v", got, err)
	}
	if got, err := mergePreparedAreaItems(nil, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty merged area items = %#v, %v", got, err)
	}
	if got, err := mergePreparedUserHonors(nil, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty merged honors = %#v, %v", got, err)
	}
	if got, err := mergePreparedUserProfileHonors(nil, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty merged profile honors = %#v, %v", got, err)
	}
}

func assertPreparedCardComparison(t *testing.T) {
	t.Helper()
	card := snapshot.RawUserCard{CardID: 1, Level: 1, Episodes: []snapshot.RawUserCardEpisode{{CardEpisodeID: 1}}}
	if samePreparedUserCard(nil, &card) || samePreparedUserCard(&card, nil) {
		t.Fatal("nil prepared card comparison matched")
	}
	other := card
	other.CardID = 2
	if samePreparedUserCard(&card, &other) {
		t.Fatal("different card IDs matched")
	}
	other = card
	other.Level = 2
	if samePreparedUserCard(&card, &other) {
		t.Fatal("different card fields matched")
	}
	other = card
	other.Episodes = nil
	if samePreparedUserCard(&card, &other) {
		t.Fatal("different episode lengths matched")
	}
	other = card
	other.Episodes = []snapshot.RawUserCardEpisode{{CardEpisodeID: 2}}
	if samePreparedUserCard(&card, &other) {
		t.Fatal("different episodes matched")
	}
	if !samePreparedUserCard(&card, &card) {
		t.Fatal("identical prepared card did not match")
	}
}

func assertPreparedJSONNormalization(t *testing.T) {
	t.Helper()
	normalizePreparedUserCardJSON(nil)
	cardMap := map[string]any{"episodes": nil}
	normalizePreparedUserCardJSON(cardMap)
	if _, ok := cardMap["episodes"]; ok {
		t.Fatal("nil episode JSON was not removed")
	}
	normalizePreparedUserAreaJSON(nil)
	areaMap := map[string]any{}
	normalizePreparedUserAreaJSON(areaMap)
	if areaMap["userAreaStatus"] == nil {
		t.Fatal("missing user area status was not added")
	}
	areaMap = map[string]any{"userAreaStatus": map[string]any{"ok": true}}
	normalizePreparedUserAreaJSON(areaMap)
	if areaMap["userAreaStatus"].(map[string]any)["ok"] != true {
		t.Fatal("existing user area status changed")
	}
}

func TestShouldPrepareRecommendUserDataAdditional(t *testing.T) {
	positive := 1
	zero := 0
	queries := []AutoQuery{
		{MaxProfile: true}, {SubMaxProfile: true}, {FixedCards: []int{1}}, {UnitFilter: "idol"},
		{AttrFilter: "cute"}, {ExcludedCards: []int{1}}, {AreaItemLevel: &positive}, {UseCurrentDeck: true},
	}
	for _, query := range queries {
		if !shouldPrepareRecommendUserData(query) {
			t.Errorf("query %#v did not require preparation", query)
		}
	}
	if shouldPrepareRecommendUserData(AutoQuery{}) || shouldPrepareRecommendUserData(AutoQuery{AreaItemLevel: &zero}) {
		t.Fatal("empty query unexpectedly required preparation")
	}
}
