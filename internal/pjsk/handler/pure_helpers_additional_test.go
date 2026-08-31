package handler

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestArrestPureHelpers(t *testing.T) {
	testArrestBindingSelectors(t)
	testArrestQueryParams(t)
	testArrestTextFormatting(t)
	testArrestValueFormatting(t)
}

func testArrestBindingSelectors(t *testing.T) {
	t.Helper()
	for _, tt := range []struct {
		value string
		want  bool
	}{
		{"u1", true}, {"U999", true}, {"u", false}, {"x1", false}, {"u1x", false},
	} {
		if got := isBindingSelector(tt.value); got != tt.want {
			t.Errorf("isBindingSelector(%q) = %v", tt.value, got)
		}
	}
}

func testArrestQueryParams(t *testing.T) {
	t.Helper()
	ctx := HarrukiSekaiHandlerContext{
		PjskHandlerContext: PjskHandlerContext{Context: context.Background(), Platform: "qq", UserId: "1"},
		originalTriggerCmd: "/注册时间",
	}
	params, err := resolveSelfOnlyQueryParams(ctx)
	if err != nil || params.Mode != "self" || params.Selector != "" {
		t.Fatalf("default self params = %+v, err = %v", params, err)
	}
	ctx.uidArg = "u2"
	params, err = resolveSelfOnlyQueryParams(ctx)
	if err != nil || params.Selector != "u2" {
		t.Fatalf("selector params = %+v, err = %v", params, err)
	}
	ctx.uidArg = "@2"
	if _, err := resolveSelfOnlyQueryParams(ctx); err == nil {
		t.Fatal("expected self-only target rejection")
	}
	ctx.uidArg = "invalid"
	if _, err := resolveUserQueryParams(ctx); err == nil {
		t.Fatal("expected invalid user query error")
	}
	ctx.uidArg = "U3"
	params, err = resolveUserQueryParams(ctx)
	if err != nil || params.Mode != "self" || params.Selector != "U3" {
		t.Fatalf("binding selector params = %+v, err = %v", params, err)
	}
}

func testArrestTextFormatting(t *testing.T) {
	t.Helper()
	diffs := defaultEnabledDiffs()
	if !reflect.DeepEqual(diffs, []sekaiapi.MusicDifficultyType{sekaiapi.MusicDifficultyMaster, sekaiapi.MusicDifficultyExpert}) {
		t.Fatalf("default difficulties = %v", diffs)
	}
	resp := &sekaiapi.GetAnotherProfileResponse{
		User: sekaiapi.AnotherUser{UserID: 1234567890, Name: "Player", Rank: 99},
		UserMusicDifficultyClearCount: []sekaiapi.AnotherUserMusicDifficultyClearCount{
			{MusicDifficultyType: sekaiapi.MusicDifficultyMaster, LiveClear: 10, FullCombo: 8, AllPerfect: 2},
		},
		UserChallengeLiveSoloResult: sekaiapi.UserChallengeLiveSoloResult{CharacterID: 21, HighScore: 1_234_567},
	}
	formatted := formatArrestText(resp, []sekaiapi.MusicDifficultyType{sekaiapi.MusicDifficultyMaster, sekaiapi.MusicDifficultyExpert}, "Miku", false)
	for _, want := range []string{"Player", "[master]", "FC:8", "Miku", "1,234,567"} {
		if !strings.Contains(formatted, want) {
			t.Errorf("formatArrestText() = %q, missing %q", formatted, want)
		}
	}
	resp.UserChallengeLiveSoloResult.HighScore = 0
	if got := formatArrestText(resp, nil, "", true); strings.Contains(got, "挑战Live") || !strings.Contains(got, "1234567890") {
		t.Fatalf("minimal arrest text = %q", got)
	}
}

func testArrestValueFormatting(t *testing.T) {
	t.Helper()
	if arrestChallengeCharacterLabel(21, " Miku ") != "Miku" || arrestChallengeCharacterLabel(21, "") != "角色ID:21" {
		t.Fatal("challenge character label mismatch")
	}
	regions := []struct {
		region string
		want   int
	}{{"jp", 0}, {"cn", 1}, {"tw", 2}, {"en", 3}, {"kr", 4}, {"bad", 999}}
	for _, tt := range regions {
		if got := arrestCharacterRegionRank(tt.region); got != tt.want {
			t.Errorf("arrestCharacterRegionRank(%q) = %d", tt.region, got)
		}
	}
	for _, tt := range []struct {
		value int
		want  string
	}{{0, "0"}, {12, "12"}, {1234, "1,234"}, {-1234567, "-1,234,567"}} {
		if got := formatInt(tt.value); got != tt.want {
			t.Errorf("formatInt(%d) = %q", tt.value, got)
		}
	}
	if arrestDisplayUID(1234567890, true) != "1234567890" || arrestDisplayUID(1234567890, false) == "1234567890" {
		t.Fatal("UID visibility formatting mismatch")
	}
}
func TestRegistrationTimeValidation(t *testing.T) {
	for _, tt := range []struct {
		uid     string
		server  string
		wantErr bool
	}{
		{uid: "12345678901234", server: "jp"},
		{uid: "12345678901234", server: "en"},
		{uid: "12345678901234", server: "tw"},
		{uid: "12345678901234", server: "kr"},
		{uid: "12345678901234", server: "cn"},
		{uid: "12", server: "jp", wantErr: true},
		{uid: "abc999", server: "en", wantErr: true},
		{uid: "invalid", server: "cn", wantErr: true},
		{uid: "123", server: "invalid", wantErr: true},
	} {
		t.Run(tt.server+tt.uid, func(t *testing.T) {
			value, err := calcRegistrationTime(tt.uid, tt.server)
			if (err != nil) != tt.wantErr {
				t.Fatalf("calcRegistrationTime() value = %d, err = %v", value, err)
			}
			if !tt.wantErr && value <= 0 {
				t.Fatalf("calcRegistrationTime() = %d", value)
			}
		})
	}
}

func TestMusicFormattingAndListHelpers(t *testing.T) {
	testMusicBPMFormatting(t)
	testMusicDifficultyFormatting(t)
	testMusicAmbiguousTitles(t)
	testMusicMatchHelpers(t)
}

func testMusicBPMFormatting(t *testing.T) {
	t.Helper()
	if got := formatMusicBPMResult(nil); got != "未找到 BPM 信息" {
		t.Fatalf("nil BPM result = %q", got)
	}
	result := &rendermusic.BPMResult{
		Music: &masterdata.Music{ID: 7, Title: "Song"}, Difficulty: "expert", MainBPM: 120,
		Events:   []rendermusic.BPMEvent{{BPM: 120}, {BPM: 120}, {BPM: 150.5}, {BPM: 0}},
		Duration: 125.4, BarCount: 42,
	}
	formatted := formatMusicBPMResult(result)
	for _, want := range []string{"【7】Song", "EXPERT", "主 BPM：120", "120 / 150.5", "2:05", "42"} {
		if !strings.Contains(formatted, want) {
			t.Errorf("formatMusicBPMResult() = %q, missing %q", formatted, want)
		}
	}
	result.Music = nil
	result.Difficulty = "unknown"
	result.MainBPM = 0
	result.Events = nil
	result.Duration = 0
	result.BarCount = 0
	if got := formatMusicBPMResult(result); got != "歌曲 BPM" {
		t.Fatalf("minimal BPM result = %q", got)
	}
	if got := formatMusicBPMSequence(nil); got != "" {
		t.Fatalf("empty BPM sequence = %q", got)
	}
	if formatMusicDuration(-1) != "0:00" || formatMusicDuration(65.6) != "1:06" {
		t.Fatal("duration formatting mismatch")
	}
}

func testMusicDifficultyFormatting(t *testing.T) {
	t.Helper()
	for _, tt := range []struct {
		diff string
		want string
	}{{"easy", "EASY"}, {"normal", "NORMAL"}, {"hard", "HARD"}, {"expert", "EXPERT"}, {"master", "MASTER"}, {"append", "APPEND"}, {"bad", ""}} {
		if got := formatMusicDifficultyLabel(tt.diff); got != tt.want {
			t.Errorf("formatMusicDifficultyLabel(%q) = %q", tt.diff, got)
		}
	}
	if formatMusicBPM(120) != "120" || formatMusicBPM(120.5) != "120.5" {
		t.Fatal("BPM formatting mismatch")
	}
}

func testMusicAmbiguousTitles(t *testing.T) {
	t.Helper()
	fallback := "匹配到多个歌曲，请使用 /查歌 <id> 查询："
	if got := buildAmbiguousMusicDetailListTitle(nil); got != fallback {
		t.Fatalf("detail fallback = %q", got)
	}
	if got := buildAmbiguousMusicDetailListTitle(errors.New("failed to search music: 请改用 music<id>")); got != "请使用 /查歌 <id>" {
		t.Fatalf("detail rewrite = %q", got)
	}
	if got := buildAmbiguousMusicDetailListTitle(errors.New("匹配到多个歌曲：1,2")); got != fallback {
		t.Fatalf("detail ambiguous fallback = %q", got)
	}
	bpmFallback := "匹配到多个歌曲，请使用 /查BPM <id> 查询："
	if got := buildAmbiguousMusicBPMListTitle(nil); got != bpmFallback {
		t.Fatalf("BPM fallback = %q", got)
	}
	if got := buildAmbiguousMusicBPMListTitle(errors.New("failed to search music: 请使用查BPM music<id>")); !strings.Contains(got, "/查BPM") {
		t.Fatalf("BPM rewrite = %q", got)
	}
}

func testMusicMatchHelpers(t *testing.T) {
	t.Helper()
	music1 := &masterdata.Music{ID: 1}
	music2 := &masterdata.Music{ID: 2}
	matches := dedupeBPMMatchesByMusic([]rendermusic.BPMMatch{{Music: nil}, {Music: music1}, {Music: music1}, {Music: music2}})
	if len(matches) != 2 || matches[0].Music.ID != 1 || matches[1].Music.ID != 2 {
		t.Fatalf("deduplicated matches = %+v", matches)
	}
	if dedupeBPMMatchesByMusic(nil) != nil {
		t.Fatal("nil matches should remain nil")
	}
	if got := buildMusicLookupListTitle("BPM", "120", "master"); got != "BPM 120 MASTER 匹配结果" {
		t.Fatalf("lookup title = %q", got)
	}
	if got := buildMusicLookupListTitle("BPM", "120", ""); got != "BPM 120 匹配结果" {
		t.Fatalf("plain lookup title = %q", got)
	}
}
func TestMusicLevelParserInvalidAndBoundaryCases(t *testing.T) {
	testInvalidMusicLevelTokens(t)
	testMusicLevelBoundaries(t)
}

func testInvalidMusicLevelTokens(t *testing.T) {
	t.Helper()
	for _, token := range []string{"", "0", "<=0", ">=-1", "<1", ">-1", "=0", "bad", "1-0"} {
		if _, ok := parseMusicListLevelToken(token); ok {
			t.Errorf("parseMusicListLevelToken(%q) unexpectedly succeeded", token)
		}
	}
	for _, token := range []string{"", "1", "0-2", "bad"} {
		if _, _, ok := parseMusicListRangeToken(token); ok {
			t.Errorf("parseMusicListRangeToken(%q) unexpectedly succeeded", token)
		}
	}
	if _, ok := parseMusicListExactLevelToken("0"); ok {
		t.Fatal("zero exact level unexpectedly succeeded")
	}
	if _, ok := parseMusicListExactLevelToken("bad"); ok {
		t.Fatal("invalid exact level unexpectedly succeeded")
	}
}

func testMusicLevelBoundaries(t *testing.T) {
	t.Helper()
	if got, ok := parseMusicListLevelToken("<2"); !ok || got["level_max"] != 1 {
		t.Fatalf("less-than parser = %v %v", got, ok)
	}
	if got, ok := parseMusicListLevelToken(">0"); !ok || got["level_min"] != 1 {
		t.Fatalf("greater-than parser = %v %v", got, ok)
	}
	if left, right, ok := parseMusicListRangeToken("【30～28】"); !ok || left != 30 || right != 28 {
		t.Fatalf("range parser = %d %d %v", left, right, ok)
	}
	for _, token := range []string{"-", "~", "～", ",", "，", "..", "到", "至"} {
		if !isMusicListRangeSeparatorToken(token) {
			t.Errorf("separator %q not recognized", token)
		}
	}
	if isMusicListRangeSeparatorToken("x") {
		t.Fatal("invalid separator recognized")
	}
	if got := joinMusicListTokensExcluding(nil, 0); got != "" {
		t.Fatalf("empty join = %q", got)
	}
	if got := joinMusicListTokensExcluding([]string{"a", "b", "c"}, 1); got != "a c" {
		t.Fatalf("join excluding = %q", got)
	}
}
func TestMusicEmptyRenderInputsReturnErrors(t *testing.T) {
	rc := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{Region: "jp"}}
	if message := renderMusicBPMDetailMessage(rc, &rendermusic.BPMResult{}); len(message) != 1 || message[0].Type != onebot11.TypeText {
		t.Fatalf("BPM detail message = %+v", message)
	}
	if _, err := renderMusicLookupListMessages(rc, nil, "jp", "BPM", "120", "", "", nil); err == nil {
		t.Fatal("expected empty detailed lookup error")
	}
	if _, err := renderMusicBriefLookupListMessages(rc, nil, "jp", "BPM", "120", nil); err == nil {
		t.Fatal("expected empty brief lookup error")
	}
	if _, err := renderAmbiguousMusicDetailListMessages(rc, nil, "jp", nil, nil); err == nil {
		t.Fatal("expected empty ambiguous lookup error")
	}
	if _, err := renderAmbiguousMusicIDsMessages(rc, nil, "jp", nil, nil); err == nil {
		t.Fatal("expected empty ambiguous IDs error")
	}
	if _, err := renderAmbiguousMusicBPMIDsMessages(rc, nil, "jp", nil, nil); err == nil {
		t.Fatal("expected empty ambiguous BPM IDs error")
	}
	if _, err := renderNoteCountLookupListMessages(rc, nil, rendermusic.NoteCountQuery{NoteCount: 1}, []rendermusic.NoteCountMatch{{Music: nil}}); err == nil {
		t.Fatal("expected empty note-count lookup error")
	}
	if _, err := renderBPMLookupListMessages(rc, nil, rendermusic.BPMQuery{BPM: 120}, []rendermusic.BPMMatch{{Music: nil}}); err == nil {
		t.Fatal("expected empty BPM lookup error")
	}
}

func TestMysekaiRankParsingBranches(t *testing.T) {
	testMysekaiRankParts(t)
	testMysekaiRankClassifiers(t)
	testMysekaiRankMessages(t)
}

func testMysekaiRankParts(t *testing.T) {
	t.Helper()
	if parts := splitMysekaiHousingRankToken("1,2，3、4"); !reflect.DeepEqual(parts, []string{"1", "2", "3", "4"}) {
		t.Fatalf("split ranks = %v", parts)
	}
	for _, tt := range []struct {
		part string
		want []int
	}{
		{part: "", want: nil}, {part: "3", want: []int{3}}, {part: "3-1", want: []int{1, 2, 3}},
		{part: "1到3", want: []int{1, 2, 3}}, {part: "1..3", want: []int{1, 2, 3}},
	} {
		got, err := parseMysekaiHousingRankPart(tt.part)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseMysekaiHousingRankPart(%q) = %v, %v", tt.part, got, err)
		}
	}
	for _, part := range []string{"0", "bad", "1-", "-1", "1-bad"} {
		if _, err := parseMysekaiHousingRankPart(part); err == nil {
			t.Errorf("parseMysekaiHousingRankPart(%q) unexpectedly succeeded", part)
		}
	}
	if _, err := parseMysekaiHousingRankPart("1-1000"); err == nil {
		t.Fatal("expected oversized range error")
	}
	if ranks, err := parseMysekaiHousingRankTokens([]string{"1,2", "3-4"}); err != nil || !reflect.DeepEqual(ranks, []int{1, 2, 3, 4}) {
		t.Fatalf("rank tokens = %v, err = %v", ranks, err)
	}
}

func testMysekaiRankClassifiers(t *testing.T) {
	t.Helper()
	if !isPositiveIntegerToken("123") || isPositiveIntegerToken("") || isPositiveIntegerToken("1x") {
		t.Fatal("positive integer token mismatch")
	}
	for _, token := range []string{"1-2", "1~2", "1到2", "1至2", "1..2"} {
		if !isMysekaiHousingRankRangeToken(token) {
			t.Errorf("range token %q not recognized", token)
		}
	}
	if isMysekaiHousingRankRangeToken("12") {
		t.Fatal("plain rank recognized as range")
	}
	if !shouldEnforceMysekaiExpiry("mysekai-resource") || !shouldEnforceMysekaiExpiry("mysekai-map") || shouldEnforceMysekaiExpiry("mysekai-photo") {
		t.Fatal("expiry mode classification mismatch")
	}
}

func testMysekaiRankMessages(t *testing.T) {
	t.Helper()
	if message := mysekaiNoRemainingMaterialMessage("tw"); len(message) != 1 || !strings.Contains(message[0].Data.(onebot11.TextData).Text, "TW") {
		t.Fatalf("no material message = %+v", message)
	}
	if message, err := executeConcurrentMessages(context.Background()); err != nil || message != nil {
		t.Fatalf("empty concurrent messages = %+v, %v", message, err)
	}
}

func TestMessageConstructionAndMaskingHelpers(t *testing.T) {
	testMessageErrorHelpers(t)
	testBindingDisplayHelpers(t)
	testPrivateDataMessages(t)
	testMaskingAndFallbackText(t)
}

func testMessageErrorHelpers(t *testing.T) {
	t.Helper()
	if got := unsupportedModeError("music", "bad").Error(); !strings.Contains(got, "unsupported music mode") {
		t.Fatalf("unsupported error = %q", got)
	}
	original := errors.New("original")
	if normalizeBindingLookupError(nil, "fallback") != nil {
		t.Fatal("nil binding error changed")
	}
	if got := normalizeBindingLookupError(accountdata.ErrNoBinding, "fallback"); got != accountdata.ErrNoBinding {
		t.Fatalf("no binding error changed: %v", got)
	}
	if got := normalizeBindingLookupError(original, ""); got != original {
		t.Fatalf("empty fallback changed error: %v", got)
	}
	if got := normalizeBindingLookupError(original, "lookup failed"); !strings.Contains(got.Error(), "lookup failed") || !errors.Is(got, original) {
		t.Fatalf("wrapped lookup error = %v", got)
	}
}

func testBindingDisplayHelpers(t *testing.T) {
	t.Helper()
	bindings := []*accountdata.ResolvedBinding{
		nil,
		{Server: "jp"},
		{PJSKUserID: "12345678901234", Visible: true},
		{Server: "jp", PJSKUserID: "12345678901234", Visible: false},
	}
	wants := []string{"", "JP服", "12345678901234", "JP服123********234"}
	for i, binding := range bindings {
		if got := formatUserFacingBindingAccount(binding); got != wants[i] {
			t.Errorf("binding account %d = %q, want %q", i, got, wants[i])
		}
	}
}

func testPrivateDataMessages(t *testing.T) {
	t.Helper()
	binding := &accountdata.ResolvedBinding{Server: "jp", PJSKUserID: "12345678901234", Visible: false}
	if got := buildPrivateDataHiddenMessage("mysekai", binding); !strings.Contains(got, "/展示烤森抓包") || !strings.Contains(got, "mysekai") {
		t.Fatalf("hidden MySekai message = %q", got)
	}
	if got := buildPrivateDataHiddenMessage("unknown", nil); !strings.Contains(got, "/展示抓包") || !strings.Contains(got, "suite") {
		t.Fatalf("hidden suite message = %q", got)
	}
	if got := buildPrivateDataNotFoundMessage("", nil); !strings.Contains(got, "suite") {
		t.Fatalf("nil binding not found message = %q", got)
	}
	if got := buildPrivateDataNotFoundMessage("mysekai", &accountdata.ResolvedBinding{}); !strings.Contains(got, "mysekai") {
		t.Fatalf("incomplete binding not found message = %q", got)
	}
	if got := buildPrivateDataNotFoundMessage("mysekai", binding); !strings.Contains(got, "JP服") {
		t.Fatalf("bound not found message = %q", got)
	}
	if got := buildToolboxAccessDeniedMessage("suite", nil); !strings.Contains(got, "当前QQ号") {
		t.Fatalf("anonymous toolbox denial = %q", got)
	}
	if got := buildToolboxAccessDeniedMessage("suite", binding); !strings.Contains(got, "查询账号") {
		t.Fatalf("bound toolbox denial = %q", got)
	}
	if normalizeToolboxDataLabel(" MySekai ") != "mysekai" || normalizeToolboxDataLabel("other") != "suite" {
		t.Fatal("toolbox data label mismatch")
	}
}

func testMaskingAndFallbackText(t *testing.T) {
	t.Helper()
	if maskUserFacingGameID("", false) != "" || maskUserFacingGameID("123", false) != "123" || maskUserFacingGameID("1234567890", true) != "1234567890" || maskUserFacingGameID("1234567890", false) != "123****890" {
		t.Fatal("game ID masking mismatch")
	}
	if stringPtr("") != nil {
		t.Fatal("blank string pointer should be nil")
	}
	if value := stringPtr(" value "); value == nil || *value != "value" {
		t.Fatalf("string pointer = %v", value)
	}
	if got := sanitizeUserFacingText(" safe message "); got != "safe message" {
		t.Fatalf("sanitized safe text = %q", got)
	}
	if sanitizeUserFacingText("") != genericUserFacingErrorText || sanitizeUserFacingText("failed at http://localhost/private/token") != genericUserFacingErrorText {
		t.Fatal("sensitive text was not replaced")
	}
	if fallbackCommandHelpMarkdown("", "", "body") != "# 指令帮助\n\nbody" {
		t.Fatal("generic fallback help mismatch")
	}
	if fallbackCommandHelpMarkdown("/cmd", "path", "body") != "# /cmd\n\nbody" {
		t.Fatal("trigger fallback help mismatch")
	}
}
