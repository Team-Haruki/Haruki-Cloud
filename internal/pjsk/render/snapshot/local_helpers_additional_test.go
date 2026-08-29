package snapshot

import (
	"errors"
	"reflect"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestSnapshotProfileDeckAndCloneHelpers(t *testing.T) {
	for _, tc := range []struct {
		mode, wantMode  string
		profile, leader int
		want            int
	}{
		{mode: "custom", profile: 7, leader: 8, want: 7},
		{mode: "default", profile: 7, leader: 8, want: 8},
		{leader: 8, want: 8},
		{profile: 7, want: 7},
		{},
	} {
		if got := SelectProfileImageCardID(tc.mode, tc.profile, tc.leader); got != tc.want {
			t.Errorf("SelectProfileImageCardID(%q, %d, %d) = %d", tc.mode, tc.profile, tc.leader, got)
		}
	}
	if cloned, err := CloneRawUserData(nil); err != nil || cloned != nil {
		t.Fatalf("nil clone = %#v, %v", cloned, err)
	}
	raw := &RawUserData{UserGamedata: RawUserGamedata{UserID: 1}, UserCards: []RawUserCard{{CardID: 2, Level: 3}}}
	cloned, err := CloneRawUserData(raw)
	if err != nil || cloned == raw || !reflect.DeepEqual(cloned, raw) {
		t.Fatalf("clone = %#v, %v", cloned, err)
	}
	cloned.UserCards[0].Level = 99
	if raw.UserCards[0].Level == 99 {
		t.Fatal("clone shares card storage with source")
	}
	if _, err := EncodeRawUserData(nil); err == nil {
		t.Fatal("nil raw user data encoded")
	}
	if data, err := EncodeRawUserData(raw); err != nil || len(data) == 0 {
		t.Fatalf("encoded raw user data = %q, %v", data, err)
	}

	decks := []RawUserDeck{{DeckID: 1, Member1: 1}, {DeckID: 2, Member1: 2}}
	if got := FindActiveDeck(decks, 2); got.DeckID != 2 {
		t.Fatalf("active deck = %#v", got)
	}
	if got := FindActiveDeck(decks, 99); got.DeckID != 1 {
		t.Fatalf("fallback deck = %#v", got)
	}
	if got := FindActiveDeck(nil, 1); got.DeckID != 0 {
		t.Fatalf("empty active deck = %#v", got)
	}
	if ids, ok := UserDeckCardIDs(nil); ok || ids != nil {
		t.Fatalf("nil deck IDs = %#v, %t", ids, ok)
	}
	if ids, ok := UserDeckCardIDs(&RawUserDeck{Member1: 1}); ok || ids != nil {
		t.Fatalf("incomplete deck IDs = %#v, %t", ids, ok)
	}
	complete := &RawUserDeck{Member1: 1, Member2: 2, Member3: 3, Member4: 4, Member5: 5}
	if ids, ok := UserDeckCardIDs(complete); !ok || !reflect.DeepEqual(ids, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("complete deck IDs = %#v, %t", ids, ok)
	}

	cards := []RawUserCard{{CardID: 3}, {CardID: 4}}
	if FindUserCard(cards, 4) == nil || FindUserCard(cards, 9) != nil {
		t.Fatal("user-card lookup failed")
	}
	challengeDecks := []RawChallengeLiveDeck{{CharacterID: 6, Leader: 1}}
	if FindChallengeLiveDeck(challengeDecks, 6) == nil || FindChallengeLiveDeck(challengeDecks, 9) != nil {
		t.Fatal("challenge deck lookup failed")
	}
	if ids, ok := ChallengeLiveDeckCardIDs(nil); ok || ids != nil {
		t.Fatalf("nil challenge deck IDs = %#v, %t", ids, ok)
	}
	if ids, ok := ChallengeLiveDeckCardIDs(&RawChallengeLiveDeck{Leader: 1}); ok || ids != nil {
		t.Fatalf("incomplete challenge deck IDs = %#v, %t", ids, ok)
	}
	challenge := &RawChallengeLiveDeck{Leader: 1, Support1: 2, Support2: 3, Support3: 4, Support4: 5}
	if ids, ok := ChallengeLiveDeckCardIDs(challenge); !ok || len(ids) != 5 {
		t.Fatalf("challenge deck IDs = %#v, %t", ids, ok)
	}
	if leaderCardUsesTrainedArt(nil) || leaderCardUsesTrainedArt(&RawUserCard{DefaultImage: "normal"}) || !leaderCardUsesTrainedArt(&RawUserCard{DefaultImage: " Special_Training "}) {
		t.Fatal("trained-art detection failed")
	}
}

func TestSnapshotValueAndMySekaiMergeDecisionHelpers(t *testing.T) {
	for _, value := range []any{nil, []any{}, map[string]any{}, "  "} {
		if !isEmptySnapshotValue(value) {
			t.Errorf("value %#v was not empty", value)
		}
	}
	for _, value := range []any{[]any{1}, map[string]any{"x": 1}, "x", 0} {
		if isEmptySnapshotValue(value) {
			t.Errorf("value %#v was empty", value)
		}
	}
	base := map[string]any{"items": []any{1}, "userMysekaiFoo": map[string]any{"x": 1}}
	if !shouldSkipEmptyMySekaiOverride(base, "items", []any{}) || shouldSkipEmptyMySekaiOverride(base, "missing", []any{}) || shouldSkipEmptyMySekaiOverride(base, "items", []any{2}) {
		t.Fatal("empty MySekai override classification failed")
	}
	updated := map[string]struct{}{"items": {}}
	if !shouldSkipMySekaiTopLevelOverride(base, updated, "items", []any{2}) {
		t.Fatal("updated-resource key was not preserved")
	}
	if !shouldSkipMySekaiTopLevelOverride(base, nil, "items", []any{}) {
		t.Fatal("empty top-level override was not skipped")
	}
	if shouldSkipMySekaiTopLevelOverride(base, nil, "ordinary", 1) {
		t.Fatal("ordinary top-level key was skipped")
	}
	if !shouldSkipMySekaiTopLevelOverride(base, nil, "userMysekaiFoo", map[string]any{"y": 2}) {
		t.Fatal("nonempty MySekai suite value was overwritten")
	}
	if shouldSkipMySekaiTopLevelOverride(base, nil, "userMysekaiMissing", map[string]any{"y": 2}) {
		t.Fatal("missing MySekai suite value was skipped")
	}
}

func TestCompactSnapshotScalarHelpers(t *testing.T) {
	enums := []string{"zero", "one"}
	for _, tc := range []struct {
		value any
		want  string
	}{
		{value: 1, want: "one"},
		{value: "text", want: "text"},
		{value: json.Number("7"), want: "7"},
		{value: struct{}{}, want: ""},
	} {
		if got := compactEnumString(tc.value, enums); got != tc.want {
			t.Errorf("compactEnumString(%#v) = %q", tc.value, got)
		}
	}
	for _, tc := range []struct {
		value any
		want  int
		ok    bool
	}{
		{value: 1, want: 1, ok: true},
		{value: int64(2), want: 2, ok: true},
		{value: float64(3), want: 3, ok: true},
		{value: json.Number("4"), want: 4, ok: true},
		{value: " 5 ", want: 5, ok: true},
		{value: json.Number("bad")},
		{value: "bad"},
		{value: struct{}{}},
	} {
		got, ok := compactIntValue(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Errorf("compactIntValue(%#v) = %d, %t", tc.value, got, ok)
		}
	}
	for _, tc := range []struct {
		value any
		want  bool
		ok    bool
	}{
		{value: true, want: true, ok: true},
		{value: "true", want: true, ok: true},
		{value: json.Number("1"), want: true, ok: true},
		{value: float64(0), want: false, ok: true},
		{value: "bad"},
		{value: json.Number("bad")},
		{value: struct{}{}},
	} {
		got, ok := compactBoolValue(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Errorf("compactBoolValue(%#v) = %t, %t", tc.value, got, ok)
		}
	}
	columns := map[string][]any{"value": {json.Number("1")}}
	if got, ok := compactIntFromColumns(columns, "value", 0); !ok || got != 1 {
		t.Fatal("compact integer column failed")
	}
	if _, ok := compactIntFromColumns(columns, "missing", 0); ok {
		t.Fatal("missing compact integer column succeeded")
	}
	if got, ok := compactBoolFromColumns(columns, "value", 0); !ok || !got {
		t.Fatal("compact boolean column failed")
	}
	if _, ok := compactBoolFromColumns(columns, "value", 2); ok {
		t.Fatal("out-of-range compact boolean column succeeded")
	}
}

func TestSnapshotNormalizationScalarAndConversionHelpers(t *testing.T) {
	if got, err := normalizeExtendedJSONString("oid", "$oid"); err != nil || got != "oid" {
		t.Fatalf("normalized extended string = %q, %v", got, err)
	}
	if _, err := normalizeExtendedJSONString(1, "$oid"); err == nil {
		t.Fatal("non-string extended value was accepted")
	}
	if got, err := normalizeExtendedJSONNumber(json.Number("12"), "$numberLong"); err != nil || got != "12" {
		t.Fatalf("normalized extended number = %q, %v", got, err)
	}
	if got, err := normalizeExtendedJSONNumber(" 13 ", "$numberLong"); err != nil || got != "13" {
		t.Fatalf("normalized extended number string = %q, %v", got, err)
	}
	if _, err := normalizeExtendedJSONNumber(" ", "$numberLong"); err == nil {
		t.Fatal("empty extended number was accepted")
	}
	if _, err := normalizeExtendedJSONNumber(true, "$numberLong"); err == nil {
		t.Fatal("boolean extended number was accepted")
	}

	results := convertChallengeResults([]RawChallengeLiveResult{{CharacterID: 1, HighScore: 2}})
	stages := convertChallengeStages([]RawChallengeLiveStage{{CharacterID: 1, Rank: 3}})
	rewards := convertChallengeRewards([]RawChallengeLiveReward{{ChallengeLiveHighScoreRewardID: 4, CharacterID: 1}})
	if len(results) != 1 || results[0].HighScore != 2 || len(stages) != 1 || stages[0].Rank != 3 || len(rewards) != 1 || rewards[0].RewardID != 4 {
		t.Fatalf("challenge conversions = %#v, %#v, %#v", results, stages, rewards)
	}
}

func TestSnapshotServiceAccessorBranches(t *testing.T) {
	var nilService *Service
	if nilService.Configured() || nilService.Require() == nil || nilService.DetailedProfile(renderregion.JP) != nil || nilService.ProfileCard(renderregion.JP) != nil {
		t.Fatal("nil snapshot service reported available data")
	}
	if nilService.MusicResults("master") != nil || nilService.GetMusicResult(1, "master") != "" || nilService.ChallengeLive() != nil {
		t.Fatal("nil snapshot service returned game data")
	}
	if _, err := nilService.RawBytes(); err == nil {
		t.Fatal("nil snapshot service returned raw bytes")
	}
	if nilService.RawFilePath() != "" || nilService.RawData() != nil || nilService.MusicMetaBytes() != nil || nilService.MusicMetaView() != nil || nilService.MusicMetaPath() != "" {
		t.Fatal("nil snapshot service returned local metadata")
	}

	unconfigured := &Service{}
	if unconfigured.Configured() || unconfigured.Require() == nil {
		t.Fatal("unconfigured snapshot service passed Require")
	}
	initFailure := errors.New("init failed")
	if err := (&Service{configured: true, initErr: initFailure}).Require(); !errors.Is(err, initFailure) {
		t.Fatalf("initialization error = %v", err)
	}
	if err := (&Service{configured: true}).Require(); err == nil {
		t.Fatal("snapshot without a profile passed Require")
	}

	mode := "public"
	frame := "frame.png"
	view := &meta.View{}
	raw := &RawUserData{UserGamedata: RawUserGamedata{UserID: 1}}
	service := &Service{
		configured: true,
		baseProfile: &drawing.DetailedProfileCardRequest{
			ID: "1", Region: "JP", Nickname: "Tester", Source: "suite", UpdateTime: 2,
			Mode: &mode, FramePath: &frame, UserCards: []any{map[string]any{"cardId": 3}},
		},
		musicResult:    map[string]map[int]string{"master": {4: "ap"}},
		challenge:      &ChallengeLiveData{Results: []ChallengeLiveResult{{CharacterID: 5, HighScore: 6}}},
		rawJSON:        []byte(`{"ok":true}`),
		rawFilePath:    " raw.json ",
		rawData:        raw,
		musicMetaBytes: []byte(`[]`),
		musicMetaView:  view,
		musicMetaPath:  " meta.json ",
	}
	if !service.Configured() || service.Require() != nil {
		t.Fatal("configured snapshot service was unavailable")
	}
	detail := service.DetailedProfile(renderregion.CN)
	if detail == service.baseProfile || detail.Region != "CN" || detail.Mode == service.baseProfile.Mode || detail.FramePath == service.baseProfile.FramePath {
		t.Fatalf("cloned detailed profile = %#v", detail)
	}
	card := service.ProfileCard(renderregion.JP)
	if card == nil || card.Profile == nil || card.Profile.ID != "1" || len(card.DataSources) != 1 {
		t.Fatalf("profile card = %#v", card)
	}
	results := service.MusicResults(" MASTER ")
	if results[4] != "ap" || service.GetMusicResult(4, "master") != "ap" || service.GetMusicResult(9, "master") != "" {
		t.Fatalf("music results = %#v", results)
	}
	results[4] = "changed"
	if service.musicResult["master"][4] != "ap" {
		t.Fatal("MusicResults returned shared map storage")
	}
	challenge := service.ChallengeLive()
	if challenge == service.challenge || len(challenge.Results) != 1 {
		t.Fatalf("challenge live = %#v", challenge)
	}
	rawBytes, err := service.RawBytes()
	if err != nil || string(rawBytes) != `{"ok":true}` || service.RawFilePath() != "raw.json" || service.RawData() != raw {
		t.Fatalf("raw accessors = %q, %v", rawBytes, err)
	}
	metaBytes := service.MusicMetaBytes()
	if string(metaBytes) != `[]` || service.MusicMetaView() != view || service.MusicMetaPath() != "meta.json" {
		t.Fatal("music metadata accessors failed")
	}
	metaBytes[0] = '{'
	if string(service.musicMetaBytes) != `[]` {
		t.Fatal("MusicMetaBytes returned shared storage")
	}
}
