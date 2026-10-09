package handler

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
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
	for _, want := range []string{
		"Player",
		i18n.T("misc.arrest.difficulty", i18n.Data{"Difficulty": i18n.DifficultyLabel("master"), "Clear": 10, "FC": 8, "AP": 2}),
		i18n.T("misc.arrest.challenge", i18n.Data{"Character": i18n.Verbatim("Miku"), "Score": "1,234,567"}),
	} {
		if !strings.Contains(formatted, want) {
			t.Errorf("formatArrestText() = %q, missing %q", formatted, want)
		}
	}
	resp.UserChallengeLiveSoloResult.HighScore = 0
	if got := formatArrestText(resp, nil, "", true); strings.Contains(got, "\n") || !strings.Contains(got, "1234567890") {
		t.Fatalf("minimal arrest text = %q", got)
	}
}

func testArrestValueFormatting(t *testing.T) {
	t.Helper()
	if arrestChallengeCharacterLabel(21, " Miku ").String() != "Miku" || arrestChallengeCharacterLabel(21, "").ID != "misc.arrest.character_id" {
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
	if got := formatMusicBPMResult(nil); got != i18n.T("music.bpm.no_data") {
		t.Fatalf("nil BPM result = %q", got)
	}
	result := &rendermusic.BPMResult{
		Music: &masterdata.Music{ID: 7, Title: "Song"}, Difficulty: "expert", MainBPM: 120,
		Events:   []rendermusic.BPMEvent{{BPM: 120}, {BPM: 120}, {BPM: 150.5}, {BPM: 0}},
		Duration: 125.4, BarCount: 42,
	}
	formatted := formatMusicBPMResult(result)
	want := strings.Join([]string{
		i18n.T("music.caption", i18n.Data{"ID": 7, "Title": "Song"}),
		i18n.T("music.bpm.detail.difficulty", i18n.Data{"Difficulty": "EXPERT"}),
		i18n.T("music.bpm.detail.main", i18n.Data{"BPM": "120"}),
		i18n.T("music.bpm.detail.changes", i18n.Data{"Sequence": "120 / 150.5 / 0"}),
		i18n.T("music.bpm.detail.duration", i18n.Data{"Duration": i18n.FormatDuration(125 * time.Second)}),
		i18n.T("music.bpm.detail.bars", i18n.Data{"Count": 42}),
	}, "\n")
	if formatted != want {
		t.Errorf("formatMusicBPMResult() = %q, want %q", formatted, want)
	}
	result.Music = nil
	result.Difficulty = "unknown"
	result.MainBPM = 0
	result.Events = nil
	result.Duration = 0
	result.BarCount = 0
	if got := formatMusicBPMResult(result); got != i18n.T("music.bpm.detail.title") {
		t.Fatalf("minimal BPM result = %q", got)
	}
	if got := formatMusicBPMSequence(nil); got != "" {
		t.Fatalf("empty BPM sequence = %q", got)
	}
	if formatMusicDuration(-1).String() != i18n.FormatDuration(0).String() || formatMusicDuration(65.6).String() != i18n.FormatDuration(66*time.Second).String() {
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
	if got := buildAmbiguousMusicDetailListTitle(); got != i18n.T("music.ambiguous_list.detail") {
		t.Fatalf("detail title = %q", got)
	}
	if got := buildAmbiguousMusicBPMListTitle(); got != i18n.T("music.ambiguous_list.bpm") {
		t.Fatalf("BPM title = %q", got)
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
	if _, err := renderMusicLookupListMessages(rc, nil, "jp", "title", "", nil); err == nil {
		t.Fatal("expected empty detailed lookup error")
	}
	if _, err := renderMusicBriefLookupListMessages(rc, nil, "jp", "title", nil); err == nil {
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
	notBound := i18n.M("binding.target_not_bound")
	if normalizeBindingLookupError(nil, notBound) != nil {
		t.Fatal("nil binding error changed")
	}
	testutil.RequireUserError(t, normalizeBindingLookupError(accountdata.ErrNoBinding, i18n.Message{}), usererror.CodeSetup, "binding.required")
	testutil.RequireUserError(t, normalizeBindingLookupError(accountdata.ErrNoBinding, notBound), usererror.CodeNotFound, "binding.target_not_bound")
	original := errors.New("original")
	wrapped := testutil.RequireUserError(t, normalizeBindingLookupError(original, notBound), usererror.CodeUnavailable, "common.unavailable")
	if !errors.Is(wrapped, original) {
		t.Fatalf("binding storage failure must keep its cause: %v", wrapped)
	}
	typed := usererror.ReadOnly()
	if normalizeBindingLookupError(typed, notBound) != typed {
		t.Fatal("a typed error must pass through")
	}
}

func testBindingDisplayHelpers(t *testing.T) {
	t.Helper()
	if _, ok := bindingAccountLabel(nil); ok {
		t.Fatal("nil binding must have no account label")
	}
	if _, ok := bindingAccountLabel(&accountdata.ResolvedBinding{Server: "jp"}); ok {
		t.Fatal("a binding without UID must have no account label")
	}
	binding := &accountdata.ResolvedBinding{Server: "jp", PJSKUserID: "12345678901234", Visible: false}
	label, ok := bindingAccountLabel(binding)
	if !ok || label.String() != i18n.AccountLabel("jp", "12345678901234", false).String() {
		t.Fatalf("binding account label = %q, %v", label, ok)
	}
}

func testPrivateDataMessages(t *testing.T) {
	t.Helper()
	binding := &accountdata.ResolvedBinding{Server: "jp", PJSKUserID: "12345678901234", Visible: false}
	for _, tc := range []struct {
		got  i18n.Message
		id   string
		data string
	}{
		{privateDataHiddenMessage("mysekai", binding), "binding.data.hidden_mysekai", ""},
		{privateDataHiddenMessage("unknown", nil), "binding.data.hidden_suite", ""},
		{privateDataNotFoundMessage("", nil), "binding.data.not_found", "binding.data_kind.suite"},
		{privateDataNotFoundMessage("mysekai", &accountdata.ResolvedBinding{}), "binding.data.not_found", "binding.data_kind.mysekai"},
		{privateDataNotFoundMessage("mysekai", binding), "binding.data.not_found_account", "binding.data_kind.mysekai"},
		{toolboxAccessDeniedMessage("suite", nil), "binding.toolbox.access_denied", "binding.data_kind.suite"},
		{toolboxAccessDeniedMessage("suite", binding), "binding.toolbox.access_denied_account", "binding.data_kind.suite"},
	} {
		if tc.got.ID != tc.id {
			t.Errorf("message = %s, want %s", tc.got.ID, tc.id)
		}
		if tc.data != "" && tc.got.Data["Data"].(i18n.Message).ID != tc.data {
			t.Errorf("%s data kind = %+v, want %s", tc.id, tc.got.Data["Data"], tc.data)
		}
	}
	if normalizePrivateDataKind(" MySekai ") != "mysekai" || normalizePrivateDataKind("other") != "suite" {
		t.Fatal("private data kind mismatch")
	}
}

func testMaskingAndFallbackText(t *testing.T) {
	t.Helper()
	if i18n.MaskUID("", false) != "" || i18n.MaskUID("123", false) != "123" || i18n.MaskUID("1234567890", true) != "1234567890" || i18n.MaskUID("1234567890", false) != "123****890" {
		t.Fatal("game ID masking mismatch")
	}
	if stringPtr("") != nil {
		t.Fatal("blank string pointer should be nil")
	}
	if value := stringPtr(" value "); value == nil || *value != "value" {
		t.Fatalf("string pointer = %v", value)
	}
}
