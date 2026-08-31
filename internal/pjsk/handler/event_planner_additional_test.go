package handler

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestEventPlannerHumanNumberVariants(t *testing.T) {
	tests := []struct {
		raw     string
		want    int64
		wantErr bool
	}{
		{raw: "1.5w", want: 15_000},
		{raw: "2亿", want: 200_000_000},
		{raw: "2億", want: 200_000_000},
		{raw: "3k", want: 3_000},
		{raw: "12,345", want: 12_345},
		{raw: "", wantErr: true},
		{raw: "oops", wantErr: true},
		{raw: "-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := parseEventPlannerHumanNumber(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseEventPlannerHumanNumber(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("parseEventPlannerHumanNumber(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestEventPlannerPrimitiveParsers(t *testing.T) {
	if got := parseEventPlannerRank("target t123"); got != 123 {
		t.Fatalf("parse rank = %d", got)
	}
	if got := parseEventPlannerRank("456名"); got != 456 {
		t.Fatalf("parse localized rank = %d", got)
	}
	if got := parseEventPlannerRank("none"); got != 0 {
		t.Fatalf("unexpected rank = %d", got)
	}

	target, remaining := parseEventPlannerBareTargetPoint("1200w event202 #12345 23456")
	if target != 12_000_000 || !strings.Contains(remaining, "event202") || !strings.Contains(remaining, "#12345") {
		t.Fatalf("bare target = %d, remaining = %q", target, remaining)
	}
	target, remaining = parseEventPlannerBareTargetPoint("9999 5火 t100")
	if target != 0 || remaining != "9999 5火 t100" {
		t.Fatalf("small number should remain: %d %q", target, remaining)
	}

	boosts, rest, err := parseEventPlannerBoosts("歌 虾 3火 3火 10火", []int{5})
	if err != nil || !reflect.DeepEqual(boosts, []int{3, 10}) || strings.Contains(rest, "火") {
		t.Fatalf("boosts = %v, rest = %q, err = %v", boosts, rest, err)
	}
	boosts, rest, err = parseEventPlannerBoosts("歌 虾", []int{5, 10})
	if err != nil || !reflect.DeepEqual(boosts, []int{5, 10}) || rest != "歌 虾" {
		t.Fatalf("fallback boosts = %v, rest = %q, err = %v", boosts, rest, err)
	}
	if _, _, err := parseEventPlannerBoosts("0火 11火", nil); err == nil {
		t.Fatal("expected invalid boost error")
	}
}

func TestEventPlannerSongParsingVariants(t *testing.T) {
	testEventPlannerSongMarkerAndSelections(t)
	testEventPlannerSongTokenClassification(t)
	testEventPlannerDifficultyAndDigits(t)
}

func testEventPlannerSongMarkerAndSelections(t *testing.T) {
	prefix, after, ok := eventPlannerAfterSongMarker("pt100w 歌 虾ex 龙hd 5火")
	if !ok || prefix != "pt100w" || after != "虾ex 龙hd 5火" {
		t.Fatalf("marker = %q %q %v", prefix, after, ok)
	}
	if _, _, ok := eventPlannerAfterSongMarker("pt100w"); ok {
		t.Fatal("unexpected marker")
	}

	songs, remaining := eventPlannerSongSelectionsFromText("虾ex / 龙hd | #123 5火 当前")
	if len(songs) != 2 || songs[0].Query != "虾" || songs[0].Difficulty != "expert" || songs[1].Difficulty != "hard" {
		t.Fatalf("songs = %+v", songs)
	}
	if !strings.Contains(remaining, "#123") || !strings.Contains(remaining, "5火") || !strings.Contains(remaining, "当前") {
		t.Fatalf("remaining = %q", remaining)
	}

}

func testEventPlannerSongTokenClassification(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"虾ex", true},
		{"", false},
		{"#123", false},
		{"5火", false},
		{"event2", false},
		{"pt100w", false},
		{"solo", false},
		{"t100", false},
		{"当前", false},
		{"10000", false},
	}
	for _, tt := range tests {
		if got := eventPlannerLooksLikeSongToken(tt.token); got != tt.want {
			t.Errorf("eventPlannerLooksLikeSongToken(%q) = %v", tt.token, got)
		}
	}

}

func testEventPlannerDifficultyAndDigits(t *testing.T) {
	query, diff := splitEventPlannerDifficulty("野车append")
	if query != "野车" || diff != "append" {
		t.Fatalf("split = %q %q", query, diff)
	}
	query, diff = splitEventPlannerDifficulty("普通歌")
	if query != "普通歌" || diff != "" {
		t.Fatalf("unsuffixed split = %q %q", query, diff)
	}
	if !eventPlannerIsDigits("123") || eventPlannerIsDigits("") || eventPlannerIsDigits("12x") {
		t.Fatal("digit classification mismatch")
	}
}

func TestEventPlannerSongSelectionSources(t *testing.T) {
	explicit := eventPlannerCommandParams{Songs: []eventPlannerSongSelection{{Query: "龙"}}}
	got := eventPlannerSongsForRequest(explicit, renderdeck.AutoQuery{})
	if len(got) != 1 || got[0].Difficulty != "hard" || got[0].MusicID != eventPlannerLostAndFoundMusicID {
		t.Fatalf("explicit songs = %+v", got)
	}

	musicID := 42
	got = eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{MusicID: &musicID, MusicQuery: "id query"})
	if len(got) != 1 || got[0].MusicID != 42 || got[0].Difficulty != "master" {
		t.Fatalf("id song = %+v", got)
	}

	got = eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{MusicQuery: "龙"})
	if len(got) != 1 || got[0].Difficulty != "hard" || got[0].MusicID != eventPlannerLostAndFoundMusicID {
		t.Fatalf("query song = %+v", got)
	}

	got = eventPlannerSongsForRequest(eventPlannerCommandParams{}, renderdeck.AutoQuery{})
	if len(got) != 3 || got[2].MusicID != eventPlannerOmakaseMusicID {
		t.Fatalf("default songs = %+v", got)
	}
}

func TestEventPlannerQueryAndSimulationHelpers(t *testing.T) {
	eventID := 12
	params := deckAutoQueryParams{
		EventID:        &eventID,
		UseCurrentDeck: true,
		FixedCards:     []int{1, 2},
		Algorithm:      "dfs",
		LiveType:       "solo",
		Target:         "power",
	}
	query := buildEventPlannerBaseDeckQuery(renderregion.EN, params)
	if query.Region != "en" || !query.UseExactCardState || query.Algorithm != "dfs" || query.LiveType != "solo" || query.Target != "power" {
		t.Fatalf("query = %+v", query)
	}
	params.FixedCards[0] = 99
	if query.FixedCards[0] != 1 {
		t.Fatal("fixed cards were not cloned")
	}

	turn := 3
	simulated, warning, err := resolveEventPlannerEventFromQuery(context.Background(), nil, renderregion.JP, renderdeck.AutoQuery{WorldBloomEventTurn: &turn})
	if err != nil || simulated.EventType != "world_bloom" || simulated.Name != "WL3模拟活动" || warning == "" {
		t.Fatalf("simulated event = %+v, warning = %q, err = %v", simulated, warning, err)
	}
	simulated, _, err = resolveEventPlannerEventFromQuery(context.Background(), nil, renderregion.JP, renderdeck.AutoQuery{EventAttr: "cool"})
	if err != nil || simulated.Name != "模拟活动" || simulated.EventType != "" {
		t.Fatalf("attribute simulation = %+v, err = %v", simulated, err)
	}
	if _, _, err := resolveEventPlannerEventFromQuery(context.Background(), nil, renderregion.JP, renderdeck.AutoQuery{}); err == nil {
		t.Fatal("expected missing provider error")
	}
	if got := eventPlannerEventBannerPath(nil, renderregion.JP, simulated); got != "" {
		t.Fatalf("nil app banner = %q", got)
	}
	if got := eventPlannerEventBannerPath(&renderapp.App{}, renderregion.JP, nil); got != "" {
		t.Fatalf("nil event banner = %q", got)
	}
}

func TestEventPlannerTargetAndCurrentPointGuardPaths(t *testing.T) {
	testEventPlannerTargetAndCurrentPoints(t)
	testEventPlannerRankingAndBindingHelpers(t)
	testEventPlannerWorldBloomHelpers(t)
}

func testEventPlannerTargetAndCurrentPoints(t *testing.T) {
	rc := &RequestContext{Ctx: context.Background(), App: &renderapp.App{}}
	event := &masterdata.Event{ID: 123, EventType: "world_bloom"}

	point, source, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, event, renderdeck.AutoQuery{}, eventPlannerCommandParams{TargetPoint: 99})
	if err != nil || point != 99 || source != "直接输入" {
		t.Fatalf("direct target = %d %q %v", point, source, err)
	}
	if _, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, event, renderdeck.AutoQuery{}, eventPlannerCommandParams{}); err == nil {
		t.Fatal("expected missing target error")
	}
	if _, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, &masterdata.Event{}, renderdeck.AutoQuery{}, eventPlannerCommandParams{TargetRank: 100}); err == nil {
		t.Fatal("expected simulated rank error")
	}
	if _, _, err := resolveEventPlannerTargetPoint(rc, renderregion.JP, event, renderdeck.AutoQuery{}, eventPlannerCommandParams{TargetRank: 100}); err == nil {
		t.Fatal("expected missing tracker error")
	}

	current, known, warning := resolveEventPlannerCurrentPoint(rc, nil, renderregion.JP, event, renderdeck.AutoQuery{}, eventPlannerCommandParams{CurrentPoint: 77, CurrentPointSet: true})
	if current != 77 || !known || warning != "" {
		t.Fatalf("explicit current = %d %v %q", current, known, warning)
	}
	current, known, warning = resolveEventPlannerCurrentPoint(rc, nil, renderregion.JP, event, renderdeck.AutoQuery{}, eventPlannerCommandParams{})
	if current != 0 || !known || !strings.Contains(warning, "未配置 Tracker") {
		t.Fatalf("missing tracker current = %d %v %q", current, known, warning)
	}

}

func testEventPlannerRankingAndBindingHelpers(t *testing.T) {
	event := &masterdata.Event{ID: 123, EventType: "world_bloom"}
	if eventPlannerUseWorldBloomRanking(nil, eventPlannerCommandParams{}) || eventPlannerUseWorldBloomRanking(event, eventPlannerCommandParams{TotalRanking: true}) || !eventPlannerUseWorldBloomRanking(event, eventPlannerCommandParams{}) {
		t.Fatal("world bloom ranking classification mismatch")
	}
	if _, ok := eventPlannerBindingUID(nil); ok {
		t.Fatal("nil binding should not resolve")
	}
	if _, ok := eventPlannerBindingUID(&accountdata.ResolvedBinding{PJSKUserID: "invalid"}); ok {
		t.Fatal("invalid binding should not resolve")
	}
	if uid, ok := eventPlannerBindingUID(&accountdata.ResolvedBinding{PJSKUserID: " 123 "}); !ok || uid != 123 {
		t.Fatalf("binding uid = %d %v", uid, ok)
	}

}

func testEventPlannerWorldBloomHelpers(t *testing.T) {
	primary := 9
	metadata := 10
	if id, ok := eventPlannerWorldBloomCharacterID(renderdeck.AutoQuery{WorldBloomCharacterID: &primary}); !ok || id != 9 {
		t.Fatalf("primary character = %d %v", id, ok)
	}
	if id, ok := eventPlannerWorldBloomCharacterID(renderdeck.AutoQuery{MetadataWorldBloomCharacterID: &metadata}); !ok || id != 10 {
		t.Fatalf("metadata character = %d %v", id, ok)
	}
	if _, ok := eventPlannerWorldBloomCharacterID(renderdeck.AutoQuery{}); ok {
		t.Fatal("unexpected character")
	}
	if !strings.Contains(eventPlannerCurrentPointTrackerWarning(sekaiapi.ErrRankingNotFound, true), "WL") {
		t.Fatal("expected world bloom tracker warning")
	}
	if !strings.Contains(eventPlannerCurrentPointTrackerWarning(errors.New("offline"), false), "当前活动") {
		t.Fatal("expected normal tracker warning")
	}
}

func TestEventPlannerFormattingAndGuardPaths(t *testing.T) {
	testEventPlannerDailyAndDeckFormatting(t)
	testEventPlannerPointerFormatting(t)
	testEventPlannerDependencyGuards(t)
}

func testEventPlannerDailyAndDeckFormatting(t *testing.T) {
	day := int64(24 * time.Hour / time.Millisecond)
	if got := eventPlannerDailyPoint(0, 0, 0, day, 0, false); got != 0 {
		t.Fatalf("zero target daily point = %d", got)
	}
	if got := eventPlannerDailyPoint(100, 0, day, day, 0, false); got != 0 {
		t.Fatalf("invalid interval daily point = %d", got)
	}
	if got := eventPlannerDailyPoint(100, 200, 0, day, day/2, true); got != 0 {
		t.Fatalf("completed target daily point = %d", got)
	}

	cards := eventPlannerDeckCards([]drawing.DeckCardData{{
		CardThumbnail:  drawing.CardFullThumbnailRequest{CardID: 1, CardThumbnailPath: "a"},
		SkillLevel:     "2",
		SkillRate:      3.5,
		EventBonusRate: 4.5,
	}})
	if len(cards) != 1 || cards[0].CardThumbnail.CardThumbnailPath != "a" || cards[0].SkillLevel != "2" {
		t.Fatalf("deck cards = %+v", cards)
	}
	for _, tt := range []struct {
		query renderdeck.AutoQuery
		label string
	}{
		{query: renderdeck.AutoQuery{FixedCards: []int{1}}, label: "指定卡组"},
		{query: renderdeck.AutoQuery{UseCurrentDeck: true}, label: "当前主队"},
		{query: renderdeck.AutoQuery{MaxProfile: true}, label: "顶配组卡"},
		{query: renderdeck.AutoQuery{SubMaxProfile: true}, label: "次顶配组卡"},
		{query: renderdeck.AutoQuery{}, label: "最优组卡"},
	} {
		got := buildEventPlannerDeckSummary(tt.query, 123456, 12.34, 45)
		if !strings.Contains(got, tt.label) || !strings.Contains(got, "123,456") || !strings.Contains(got, "12.3%") {
			t.Errorf("summary = %q", got)
		}
	}

}

func testEventPlannerPointerFormatting(t *testing.T) {
	value := 7
	rate := 1.25
	textValue := " title "
	if eventPlannerIntValue(nil) != 0 || eventPlannerIntValue(&value) != 7 {
		t.Fatal("int pointer helper mismatch")
	}
	if eventPlannerFloatValue(nil) != 0 || eventPlannerFloatValue(&rate) != 1.25 {
		t.Fatal("float pointer helper mismatch")
	}
	if eventPlannerStringValue(nil, "fallback") != "fallback" || eventPlannerStringValue(new(string), "fallback") != "fallback" || eventPlannerStringValue(&textValue, "fallback") != textValue {
		t.Fatal("string pointer helper mismatch")
	}
	if formatEventPlannerPlainInt(12) != "12" || formatEventPlannerPlainInt(1_234_567) != "1,234,567" {
		t.Fatal("plain integer formatting mismatch")
	}
	if formatEventPlannerRate(12) != "12" || formatEventPlannerRate(12.34) != "12.3" {
		t.Fatal("rate formatting mismatch")
	}

}

func testEventPlannerDependencyGuards(t *testing.T) {
	if _, err := buildEventPlannerDrawingRequest(nil, renderregion.JP, nil, nil, renderdeck.AutoQuery{}, eventPlannerCommandParams{}, nil, 0, "", 0, false); err == nil {
		t.Fatal("expected nil event error")
	}
	cmd := &CommandRequest{}
	for _, app := range []*renderapp.App{
		nil,
		{Decks: &renderdeck.Controller{}},
		{Decks: &renderdeck.Controller{}, Music: &rendermusic.Controller{}},
	} {
		rc := &RequestContext{Ctx: context.Background(), Cmd: cmd, App: app}
		if _, err := executeEventPlanner(rc); err == nil {
			t.Fatal("expected dependency guard error")
		}
	}
}
