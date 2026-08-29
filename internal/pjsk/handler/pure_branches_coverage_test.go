package handler

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
)

func TestSKParsingBranchCoverage(t *testing.T) {
	eventID, characterID, characterQuery, full, ranks := extractSKMetaArgs("--full e12 cid:21 3 1", false, false)
	if eventID != 12 || characterID != 21 || characterQuery != "" || !full || ranks != "3 1" {
		t.Fatalf("unexpected parsed meta: %d %d %q %v %q", eventID, characterID, characterQuery, full, ranks)
	}
	eventID, characterID, characterQuery, full, ranks = extractSKMetaArgs("event34 chara:Miku 100", true, false)
	if eventID != 34 || characterID != 0 || characterQuery != "Miku" || !full || ranks != "100" {
		t.Fatalf("unexpected query meta: %d %d %q %v %q", eventID, characterID, characterQuery, full, ranks)
	}
	_, _, characterQuery, _, ranks = extractSKMetaArgs("", false, true)
	if characterQuery != "wl" || ranks != "" {
		t.Fatalf("empty WL meta = %q, %q", characterQuery, ranks)
	}
	_, _, characterQuery, _, ranks = extractSKMetaArgs("wl3 bad rank", false, false)
	if characterQuery != "" || ranks != "wl3 bad rank" {
		t.Fatalf("invalid leading selector unexpectedly split: %q, %q", characterQuery, ranks)
	}

	tokenCases := []struct {
		input string
		id    int
		query string
		ok    bool
	}{
		{"", 0, "", false},
		{"cid:21", 21, "", true},
		{"cidMiku", 0, "", false},
		{"wl21", 0, "", false},
		{"wl:Miku", 0, "Miku", true},
		{"char:Rin", 0, "Rin", true},
		{"chara:", 0, ":", true},
		{"unknown", 0, "", false},
	}
	for _, tt := range tokenCases {
		id, query, ok := parseSKWorldBloomCharacterToken(tt.input)
		if id != tt.id || query != tt.query || ok != tt.ok {
			t.Errorf("parseSKWorldBloomCharacterToken(%q) = %d, %q, %v", tt.input, id, query, ok)
		}
	}

	query, rankArgs := splitSKWorldBloomCharacterAndRanks(nil)
	if query != "" || rankArgs != "" {
		t.Fatalf("empty split = %q, %q", query, rankArgs)
	}
	query, rankArgs = splitSKWorldBloomCharacterAndRanks([]string{"100", "200"})
	if query != "" || rankArgs != "100 200" {
		t.Fatalf("rank-only split = %q, %q", query, rankArgs)
	}
	query, rankArgs = splitSKWorldBloomCharacterAndRanks([]string{"Hatsune", "Miku", "100", "200"})
	if query != "Hatsune Miku" || rankArgs != "100 200" {
		t.Fatalf("query/rank split = %q, %q", query, rankArgs)
	}
	query, rankArgs = splitSKWorldBloomCharacterAndRanks([]string{"Hatsune", "Miku"})
	if query != "Hatsune Miku" || rankArgs != "" {
		t.Fatalf("query-only split = %q, %q", query, rankArgs)
	}

	if _, _, ok := splitLeadingSKWorldBloomSelectorAndRanks(nil); ok {
		t.Fatal("empty leading selector unexpectedly accepted")
	}
	if _, _, ok := splitLeadingSKWorldBloomSelectorAndRanks([]string{"Miku", "100"}); ok {
		t.Fatal("non-WL leading selector unexpectedly accepted")
	}
	if _, _, ok := splitLeadingSKWorldBloomSelectorAndRanks([]string{"wl2", "bad"}); ok {
		t.Fatal("invalid leading ranks unexpectedly accepted")
	}
	if q, r, ok := splitLeadingSKWorldBloomSelectorAndRanks([]string{"WL2", "200", "100"}); !ok || q != "WL2" || r != "200 100" {
		t.Fatalf("valid leading selector = %q, %q, %v", q, r, ok)
	}
	for input, want := range map[string]bool{"": false, "wl": true, "WL2": true, "miku": false} {
		if got := isSKWorldBloomSelector(input); got != want {
			t.Errorf("isSKWorldBloomSelector(%q) = %v", input, got)
		}
	}
	if !isValidSKRankExpression("1 2") || isValidSKRankExpression("bad rank") {
		t.Fatal("rank expression validation mismatch")
	}

	if _, _, err := parseSKRanks("bad", true); err == nil {
		t.Fatal("invalid ranks unexpectedly accepted")
	}
	if got, _, err := parseSKRanks("", true); err != nil || !slices.Equal(got, defaultSKRanks) {
		t.Fatalf("self ranks = %v, %v", got, err)
	}
	if got, _, err := parseSKRanks("9", true); err != nil || !slices.Equal(got, []int{9}) {
		t.Fatalf("single rank = %v, %v", got, err)
	}
	if got, _, err := parseSKRanks("3 1 3 0 -2", true); err != nil || !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("normalized ranks = %v, %v", got, err)
	}
	many := make([]string, 21)
	for i := range many {
		many[i] = fmt.Sprint(i + 1)
	}
	if _, _, err := parseSKRanks(strings.Join(many, " "), true); err == nil {
		t.Fatal("more than 20 ranks unexpectedly accepted")
	}
	if _, _, err := parseSKRanks("0-1", true); err == nil {
		t.Fatal("zero range unexpectedly accepted")
	}
	if _, _, err := parseSKRanks("1-21", true); err == nil {
		t.Fatal("large range unexpectedly accepted")
	}
	if got, _, err := parseSKRanks("2-4", true); err != nil || !slices.Equal(got, []int{2, 3, 4}) {
		t.Fatalf("range ranks = %v, %v", got, err)
	}
	if _, uid, err := parseSKRanks("1234567890", true); err != nil || uid == nil || *uid != 1234567890 {
		t.Fatalf("uid ranks = %v, %v", uid, err)
	}
	if _, _, err := parseSKRanks("1234567890", false); err == nil {
		t.Fatal("disallowed uid unexpectedly accepted")
	}
	if _, _, err := parseSKRanks("999999999999999999999999999999", true); err == nil {
		t.Fatal("overflow uid unexpectedly accepted")
	}
	if _, _, err := parseSKRanks("@123", true); err == nil {
		t.Fatal("mention unexpectedly accepted")
	}
	if _, _, err := parseSKRanks("unbind", true); err == nil {
		t.Fatal("unsupported parser command unexpectedly accepted")
	}
	if got := normalizeRanks(nil); got != nil {
		t.Fatalf("nil ranks = %v", got)
	}
	if got := normalizeRanks([]int{3, -1, 3, 1, 0}); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("normalizeRanks = %v", got)
	}
	if normal, wl := defaultSKRanksByMode(false), defaultSKRanksByMode(true); len(normal) == 0 || len(wl) == 0 || &normal[0] == &defaultSKRanksNormal[0] {
		t.Fatal("default ranks were not cloned")
	}
	for input, want := range map[string]bool{"": false, "12": true, "1x": false, "１２": false} {
		if got := isDigits(input); got != want {
			t.Errorf("isDigits(%q) = %v", input, got)
		}
	}
}

func TestSKParameterBranchCoverage(t *testing.T) {
	ctx := HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{Context: context.Background(), Platform: "QQ", UserId: "88", ArgText: "-f e7 cid:21 0"},
		region:             renderregion.TW,
		explicitRegion:     true,
	}
	if _, err := buildSKTrackerParamsWithDefaultRanks(ctx, false, false, false, []int{2, 4}); err == nil {
		t.Fatal("zero-only rank unexpectedly accepted")
	}
	ctx.ArgText = "--full event7 cid:21"
	params, err := buildSKTrackerParamsWithDefaultRanks(ctx, false, false, false, []int{2, 4})
	if err != nil || !slices.Equal(params["ranks"].([]int), []int{2, 4}) || params["full"] != true || params["event_id"] != 7 || params["wl_character_id"] != 21 {
		t.Fatalf("override tracker params = %#v, %v", params, err)
	}

	ctx.ArgText = "event9 not-a-period"
	params, err = buildSKSpeedTrackerParams(ctx, "", 0, 0)
	if err != nil || params["speed_unit"] != "h" || params["speed_period_seconds"] != int64(3600) || params["event_id"] != 9 {
		t.Fatalf("default speed params = %#v, %v", params, err)
	}
	ctx.prefixArg = "wl"
	ctx.ArgText = "cid:22 2"
	params, err = buildSKSpeedTrackerParams(ctx, "m", 5, 60)
	if err != nil || params["wl_character_id"] != 22 || params["speed_period_seconds"] != int64(120) {
		t.Fatalf("WL speed params = %#v, %v", params, err)
	}

	ctx.prefixArg = ""
	ctx.ArgText = "#"
	if _, err := buildSKPlayerTraceParams(ctx); err == nil {
		t.Fatal("empty compare rank unexpectedly accepted")
	}
	ctx.ArgText = "#1 #2"
	if _, err := buildSKPlayerTraceParams(ctx); err == nil {
		t.Fatal("duplicate compare rank unexpectedly accepted")
	}
	ctx.ArgText = "1 2 3"
	if _, err := buildSKPlayerTraceParams(ctx); err == nil {
		t.Fatal("three trace ranks unexpectedly accepted")
	}
	ctx.ArgText = "event8 wl:Miku 1 2 #100"
	params, err = buildSKPlayerTraceParams(ctx)
	if err != nil || params["event_id"] != 8 || params["wl_character_query"] != "Miku" || params["compare_rank"] != 100 {
		t.Fatalf("trace params = %#v, %v", params, err)
	}
	ctx.ArgText = ""
	ctx.uidArg = "@123"
	params, err = buildSKPlayerTraceParams(ctx)
	if err != nil || params["target_user_id"] != "123" {
		t.Fatalf("mention trace params = %#v, %v", params, err)
	}
	ctx.uidArg = "u2"
	params, err = buildSKPlayerTraceParams(ctx)
	if err != nil || params["target_selector"] != "u2" || params["target_user_id"] != "88" {
		t.Fatalf("selector trace params = %#v, %v", params, err)
	}
	ctx.uidArg = "123"
	params, err = buildSKPlayerTraceParams(ctx)
	if err != nil || !slices.Equal(params["ranks"].([]int), []int{123}) {
		t.Fatalf("numeric uid-arg trace params = %#v, %v", params, err)
	}
	ctx.uidArg = "1234567890"
	params, err = buildSKPlayerTraceParams(ctx)
	if err != nil || params["user_id"] != int64(1234567890) {
		t.Fatalf("game uid trace params = %#v, %v", params, err)
	}
	ctx.uidArg = ""
	ctx.prefixArg = "wl"
	params, err = buildSKPlayerTraceParams(ctx)
	if err != nil || params["wl_character_query"] != "wl" {
		t.Fatalf("default WL trace params = %#v, %v", params, err)
	}

	for _, args := range []string{"#x", "#0", "#999999999999999999999999999999", "#1 #2"} {
		if _, _, err := extractSKCompareRankArg(args); err == nil {
			t.Errorf("extractSKCompareRankArg(%q) unexpectedly succeeded", args)
		}
	}
	if remaining, rank, err := extractSKCompareRankArg("event9 #123 Miku"); err != nil || remaining != "event9 Miku" || rank != 123 {
		t.Fatalf("compare extraction = %q, %d, %v", remaining, rank, err)
	}
}

func TestDeckTargetExtractionBranchCoverage(t *testing.T) {
	for _, tt := range []struct {
		args    string
		wantErr bool
	}{
		{"plain query", false},
		{"query #", true},
		{"query ##", true},
		{"query #0", true},
		{"query #1 #1", true},
		{"query #miku #miku", true},
		{"query #1 #2 #3 #4 #5 #6", true},
		{"song #1 #miku #unknown", false},
	} {
		params := deckAutoQueryParams{}
		_, err := extractDeckFixedTargets(tt.args, &params)
		if (err != nil) != tt.wantErr {
			t.Errorf("extractDeckFixedTargets(%q) err = %v", tt.args, err)
		}
	}

	selectionCases := []struct {
		args    string
		wantErr bool
	}{
		{"wl1 终章", true},
		{"wl2 终章 miku", false},
		{"event123 wl2", true},
		{"cool", true},
		{"event123 miku", false},
		{"wl1 miku song master", false},
		{"wl2", true},
		{"wl", false},
		{"plain song", false},
	}
	for _, tt := range selectionCases {
		params := deckAutoQueryParams{}
		_, err := extractDeckEventSelection(tt.args, &params, "/组卡")
		if (err != nil) != tt.wantErr {
			t.Errorf("extractDeckEventSelection(%q) err = %v", tt.args, err)
		}
	}

	params := deckAutoQueryParams{}
	if _, err := extractDeckExplicitEventSelection("wl2", intPtr(123), &params, ""); err == nil {
		t.Fatal("explicit deprecated turn unexpectedly accepted")
	}
	params = deckAutoQueryParams{}
	if remaining, err := extractDeckExplicitEventSelection("miku", intPtr(180), &params, "/组卡"); err != nil || remaining != "" || params.ForcedLeaderCharacterID == nil {
		t.Fatalf("finale leader selection = %q, %+v, %v", remaining, params, err)
	}
	params = deckAutoQueryParams{}
	if remaining, err := extractDeckExplicitEventSelection("unknown", intPtr(123), &params, "/组卡"); err != nil || remaining != "" || params.WorldBloomCharacterQuery != "unknown" {
		t.Fatalf("explicit query selection = %q, %+v, %v", remaining, params, err)
	}
	params = deckAutoQueryParams{}
	if remaining, err := extractDeckFinaleLeaderSelection("song master", &params); err != nil || remaining != "song master" || params.ForcedLeaderCharacterQuery != "" {
		t.Fatalf("inline finale query = %q, %+v, %v", remaining, params, err)
	}
	params = deckAutoQueryParams{}
	if remaining, err := extractDeckFinaleLeaderSelection("unknown", &params); err != nil || remaining != "" || params.ForcedLeaderCharacterQuery != "unknown" {
		t.Fatalf("fallback finale query = %q, %+v, %v", remaining, params, err)
	}

	for _, args := range []string{"plain", "终章 plain", "wl0 终章", "wl2 终章 song"} {
		_, _, _ = extractDeckWorldBloomFinaleTurn(args)
	}
	for _, args := range []string{"", "plain", "wl", "song WL3", "wl0", "wlx"} {
		_, _ = extractDeckWorldBloomSelectorCandidate(args)
		_, _ = parseDeckWorldBloomTurn(args)
	}
	for _, args := range []string{"plain", "wl0 miku", "wl2", "wl2 miku song", "wl2 unknown"} {
		_, _, _, _, _ = extractDeckSimulatedWorldBloom(args)
	}
	_ = invalidDeckWorldBloomMixedSelectorError()
	_ = invalidDeckWorldBloomTurnUsageError("")
	_ = invalidDeckWorldBloomTurnUsageError("/组卡")

	for _, args := range []string{"", "终章 song", "event123 song", "event0 song", "event999999999999999999999 song"} {
		_, _ = extractDeckExplicitEventID(args)
	}
	for _, args := range []string{"", "123456 song", "abc song", "123 ex", "123 song", "0 song"} {
		_, _ = extractDeckEventID(args)
	}
	for _, args := range []string{"cool mmj song", "cool", "vs", "piapro", "25", "t25", "12325", "ln", "plain"} {
		_, _, _, _ = extractDeckSimulatedEvent(args)
		_, _ = extractDeckSimulatedEventUnit(args)
		_, _ = extractDeckAttribute(args)
		_, _ = extractDeckUnit(args)
	}
	for _, args := range []string{"", "miku song", "unknown"} {
		_, _ = extractDeckCharacter(args)
		_, _, _ = extractDeckCharacterCandidate(args, false)
		_, _, _ = extractDeckCharacterCandidate(args, true)
	}
	for input, want := range map[string]bool{"": false, "song": false, "song master": true, "master": false} {
		if got := looksLikeInlineMusicQuery(input); got != want {
			t.Errorf("looksLikeInlineMusicQuery(%q) = %v", input, got)
		}
	}
	if err := validateNoEventDeckArgs("plain", "/长草组卡"); err != nil {
		t.Fatalf("plain no-event args rejected: %v", err)
	}
	if err := validateNoEventDeckArgs("cool song", "/长草组卡"); err != nil {
		t.Fatalf("mixed no-event args rejected: %v", err)
	}
	if err := validateNoEventDeckArgs("cool mmj", "/最强长草组卡"); err == nil {
		t.Fatal("filter-only no-event args unexpectedly accepted")
	}
	if got := normalizeNoEventDeckHintTrigger("/最强长草组卡"); got != "/组卡" {
		t.Fatalf("normalized trigger = %q", got)
	}
}

func TestDeckHelperAndConfigBranchCoverage(t *testing.T) {
	for _, tt := range []struct {
		values  []int
		limit   int
		wantErr bool
	}{{nil, 5, true}, {[]int{1, 2}, 1, true}, {[]int{1, 1}, 5, true}, {[]int{1, 2}, 5, false}} {
		if err := validateDeckUniqueIDs(tt.values, tt.limit, "目标"); (err != nil) != tt.wantErr {
			t.Errorf("validateDeckUniqueIDs(%v) = %v", tt.values, err)
		}
	}
	if strategy, consumed := resolveDeckStrategyField("技能最高", 0, []string{"技能最高"}, []string{"技能"}); strategy != "max" || consumed != 1 {
		t.Fatalf("inline strategy = %q, %d", strategy, consumed)
	}
	if strategy, consumed := resolveDeckStrategyField("技能", 0, []string{"技能", "最低"}, []string{"技能"}); strategy != "min" || consumed != 2 {
		t.Fatalf("next strategy = %q, %d", strategy, consumed)
	}
	if strategy, consumed := resolveDeckStrategyField("none", 0, []string{"none"}, []string{"技能"}); strategy != "" || consumed != 0 {
		t.Fatalf("missing strategy = %q, %d", strategy, consumed)
	}

	orderCases := []struct {
		field  string
		fields []string
	}{
		{"技能顺序最优", []string{"技能顺序最优"}},
		{"技能顺序54321", []string{"技能顺序54321"}},
		{"技能顺序", []string{"技能顺序", "平均"}},
		{"技能顺序", []string{"技能顺序", "12345"}},
		{"技能顺序bad", []string{"技能顺序bad"}},
		{"技能顺序", []string{"技能顺序", "bad"}},
		{"none", []string{"none"}},
	}
	for _, tt := range orderCases {
		_, _, _, _ = resolveDeckSkillOrderField(tt.field, 0, tt.fields)
	}
	for input, want := range map[string]bool{"12345": true, "54321": true, "1234": false, "12344": false, "1234x": false} {
		_, got := parseDeckSpecificSkillOrder(input)
		if got != want {
			t.Errorf("parseDeckSpecificSkillOrder(%q) ok = %v", input, got)
		}
	}
	for _, raw := range []string{"最高", "最低", "平均", "other"} {
		_ = resolveDeckStrategyValue(raw)
	}
	for _, raw := range []string{"", "miku", "unknown"} {
		_, _ = resolveDeckCharacterToken(raw)
	}

	parserFn := func(raw string) (int, error) { return parseDeckInt(raw) }
	_, _, _ = extractDeckKeywordNumber("none", []string{"技能"}, parserFn)
	_, _, _ = extractDeckKeywordNumber("技能", []string{"技能"}, parserFn)
	_, _, _ = extractDeckKeywordNumber("技能12%", []string{"技能"}, parserFn)
	_, _, _ = extractDeckKeywordNumber("技能bad", []string{"技能"}, parserFn)
	for _, args := range []struct {
		fields []string
		index  int
	}{{[]string{"技能", "12%"}, 0}, {[]string{"12", "技能"}, 0}, {[]string{"技能12"}, 0}, {[]string{"none"}, 0}, {[]string{"x"}, -1}, {[]string{"x"}, 2}} {
		_, _, _, _ = extractDeckKeywordNumberFromFields(args.fields, args.index, []string{"技能"}, parserFn)
	}
	for _, raw := range []string{"12", "bad"} {
		_, _ = parseDeckInt(raw)
	}
	for _, raw := range []string{"12%", "12％", "加成 12", "bad"} {
		_, _ = parseDeckBonusInt(raw)
	}
	for input, want := range map[string]bool{"": false, "%": false, "12": true, "12%": true, "12x": false} {
		if got := looksLikeDeckNumericToken(input); got != want {
			t.Errorf("looksLikeDeckNumericToken(%q) = %v", input, got)
		}
	}
	if !containsDeckKeyword("abc技能", []string{"技能"}) || containsDeckKeyword("abc", []string{"技能"}) {
		t.Fatal("containsDeckKeyword mismatch")
	}
	if got := removeDeckKeywordOnce(" a 技能 b ", []string{"技能"}); got != "a b" {
		t.Fatalf("remove keyword = %q", got)
	}
	if got := removeDeckKeywordOnce(" a  b ", []string{"技能"}); got != "a b" {
		t.Fatalf("remove missing keyword = %q", got)
	}
	for input, want := range map[string]int{"": 0, "abc": 0, "0abc": 0, "12abc": 12, "999999999999999999999999x": 0} {
		if got := deckLeadingDigits(input); got != want {
			t.Errorf("deckLeadingDigits(%q) = %d", input, got)
		}
	}
	for _, id := range []int{0, 1, 5, 9, 13, 17, 21, 27} {
		_ = deckCharacterUnit(id)
		_ = resolveDeckCharacterUnit(id)
	}
	_ = newDeckUnitAliasRules()
	if !isDeckASCIIAlias("mmj") || isDeckASCIIAlias("未来") {
		t.Fatal("ASCII alias classification mismatch")
	}

	params := deckAutoQueryParams{}
	remaining := extractDeckCardConfigs("支援满技 四星满破 123满技 禁用 满剧情 满画布 bf不变 song", &params)
	if remaining != "song" || !params.SupportSkillMax || params.Rarity4Config == nil || len(params.SingleCardConfigs) != 1 || !params.KeepAfterTrainingState {
		t.Fatalf("deck card configs = %q, %+v", remaining, params)
	}
	_ = applyDeckSupportConfig("plain", &params)
	_ = applyDeckSupportConfig("支援bad", &params)
	_ = applyDeckRarityConfig("四星bad", &params)
	_ = applyDeckSingleCardConfig("plain", &params)
	_ = applyDeckSingleCardConfig("123bad", &params)
	_, _ = extractGlobalDeckCardConfig("plain")
	_, _ = parseDeckSupportConfigPatch("bad")
	_, _ = parseDeckCardConfigPatch("bad")
	applyGlobalDeckCardConfig(&params, renderdeck.CardConfigPatch{})
	var target *renderdeck.CardConfigPatch
	mergeDeckCardConfigPatch(&target, renderdeck.CardConfigPatch{})
	mergeDeckCardConfigPatch(&target, renderdeck.CardConfigPatch{Disable: true, LevelMax: true, EpisodeRead: true, MasterMax: true, SkillMax: true, Canvas: true})
	upsertDeckSingleCardConfig(&params, 0, renderdeck.CardConfigPatch{})
	upsertDeckSingleCardConfig(&params, 123, renderdeck.CardConfigPatch{Disable: true, EpisodeRead: true, MasterMax: true, SkillMax: true, Canvas: true})
	upsertDeckSingleCardConfig(&params, 456, renderdeck.CardConfigPatch{Disable: true, EpisodeRead: true, MasterMax: true, SkillMax: true, Canvas: true})
	if target == nil || !target.LevelMax || len(params.SingleCardConfigs) != 2 {
		t.Fatalf("merged deck configs = %+v, %+v", target, params.SingleCardConfigs)
	}
}

func TestDeckRuntimePureBranchCoverage(t *testing.T) {
	eventID, charID, leaderID, challengeID := 10, 21, 22, 23
	for _, q := range []renderdeck.AutoQuery{
		{},
		{Region: "jp", RecommendType: "event", EventID: &eventID, MusicTitle: "Title", MusicDiff: "master", WorldBloomCharacterQuery: "Miku", ForcedLeaderCharacterQuery: "Rin", ChallengeLiveCharacterQuery: "Len"},
		{RecommendType: "challenge", MusicQuery: "Query", WorldBloomCharacterID: &charID, ForcedLeaderCharacterID: &leaderID, ChallengeLiveCharacterID: &challengeID},
		{RecommendType: "no_event"},
		{RecommendType: "bonus"},
		{RecommendType: "mysekai"},
	} {
		if got := formatDeckQuerySummary(q); got == "" {
			t.Errorf("empty summary for %+v", q)
		}
	}
	applyDefaultChallengeDeckAutoQueryMusic(nil)
	for _, q := range []*renderdeck.AutoQuery{
		{RecommendType: "event"},
		{RecommendType: "challenge", MusicCompare: true},
		{RecommendType: "challenge", MusicID: &eventID},
		{RecommendType: "challenge", MusicQuery: "specified"},
		{RecommendType: "challenge"},
		{RecommendType: "challenge", MusicDiff: "expert"},
	} {
		applyDefaultChallengeDeckAutoQueryMusic(q)
	}
	if detail, snapshot, region, err := resolveDeckRenderProfileAndSnapshot(nil, ""); err != nil || detail != nil || snapshot != nil || region != "" {
		t.Fatalf("nil deck profile resolve = %v, %v, %q, %v", detail, snapshot, region, err)
	}
	if resolveDeckPublicProfileForTarget(nil, ResolvedGameTarget{}, "jp") != nil {
		t.Fatal("nil target profile unexpectedly resolved")
	}
	if buildDeckDetailedProfileForTargetWithResponse(nil, ResolvedGameTarget{}, "jp", nil, nil) != nil {
		t.Fatal("nil detailed target unexpectedly built")
	}
	if isTheoreticalDeckRequest(nil) {
		t.Fatal("nil request unexpectedly theoretical")
	}
	for _, tt := range []struct {
		values  []int
		wantErr bool
	}{{nil, false}, {[]int{1, 2, 3, 4, 5, 6}, true}, {[]int{0}, true}, {[]int{1, 1}, true}, {[]int{1, 2}, false}} {
		if err := validateDeckCharacterIDs(tt.values); (err != nil) != tt.wantErr {
			t.Errorf("validateDeckCharacterIDs(%v) = %v", tt.values, err)
		}
	}
	if isCharacterNotFoundError(nil) || !isCharacterNotFoundError(fmt.Errorf("未找到角色 Miku")) {
		t.Fatal("character error classification mismatch")
	}
	for _, unit := range []string{" light_sound ", "IDOL", "street", "theme_park", "school_refusal", "piapro", "bad"} {
		_ = normalizeDeckUnit(unit)
	}
}
