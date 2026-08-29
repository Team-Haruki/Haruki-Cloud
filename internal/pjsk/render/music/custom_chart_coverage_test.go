package music

import (
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type customChartErrorClient struct {
	published *sekaiapi.UserCustomMusicScorePublishedResponse
	infoErr   error
	score     []byte
	scoreErr  error
}

func (s customChartErrorClient) GetCustomMusicScorePublished(string, string) (*sekaiapi.UserCustomMusicScorePublishedResponse, error) {
	return s.published, s.infoErr
}

func (s customChartErrorClient) GetCustomMusicScore(string, string) ([]byte, error) {
	return append([]byte(nil), s.score...), s.scoreErr
}

func TestCustomChartMetadataAndErrorHelpers(t *testing.T) {
	artistCases := []struct {
		entry customChartEntry
		want  string
	}{
		{customChartEntry{UserName: " Maker ", ID: " score "}, "Maker/score"},
		{customChartEntry{UserName: "Maker"}, "Maker"},
		{customChartEntry{ID: "score"}, "score"},
		{customChartEntry{}, "自制谱"},
	}
	for _, tc := range artistCases {
		if got := buildCustomChartArtist(tc.entry); got != tc.want {
			t.Fatalf("buildCustomChartArtist(%+v) = %q, want %q", tc.entry, got, tc.want)
		}
	}

	titleCases := []struct {
		original string
		custom   string
		want     string
	}{
		{" Original ", " Custom ", "Original/Custom"},
		{"Same", "Same", "Same"},
		{"", "Custom", "Custom"},
		{"Original", "", "Original"},
	}
	for _, tc := range titleCases {
		if got := buildCustomChartTitle(tc.original, tc.custom); got != tc.want {
			t.Fatalf("buildCustomChartTitle(%q, %q) = %q, want %q", tc.original, tc.custom, got, tc.want)
		}
	}

	notFoundCases := []struct {
		err  error
		want bool
	}{
		{sekaiapi.ErrUserNotFound, true},
		{&sekaiapi.APIError{StatusCode: 404}, true},
		{&sekaiapi.APIError{Message: "status=404"}, true},
		{&sekaiapi.APIError{Message: "status 404"}, true},
		{&sekaiapi.APIError{Message: "NOT FOUND"}, true},
		{&sekaiapi.APIError{StatusCode: 500, Message: "boom"}, false},
		{errors.New("not found"), false},
	}
	for _, tc := range notFoundCases {
		if got := isCustomChartNotFoundError(tc.err); got != tc.want {
			t.Fatalf("isCustomChartNotFoundError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}

	if got := customChartEntryFromPublishedResponse(nil); !reflect.DeepEqual(got, customChartEntry{}) {
		t.Fatalf("nil response produced %+v", got)
	}
	response := &sekaiapi.UserCustomMusicScorePublishedResponse{
		UserCustomMusicScoreID: " score ",
		MusicID:                0,
		MusicDifficultyType:    " append ",
		PlayLevel:              35,
		UserName:               " author ",
		Description:            " description ",
		CustomMusicScoreTags:   []int{1, 2},
		UserCustomMusicScoreInfoJSON: &sekaiapi.UserCustomMusicScoreInfo{
			MusicID:                  9,
			Title:                    " custom ",
			UserCustomMusicScorePath: " path ",
		},
	}
	entry := customChartEntryFromPublishedResponse(response)
	response.CustomMusicScoreTags[0] = 99
	if entry.ID != "score" || entry.MusicID != 9 || entry.Title != "custom" || entry.Path != "path" || entry.TagIDs[0] != 1 {
		t.Fatalf("unexpected converted entry: %+v", entry)
	}
	if customChartCacheID(customChartEntry{}) != "custom_unknown" || customChartCacheID(entry) != "score" {
		t.Fatal("custom chart cache ID fallback failed")
	}

	if got := resolveCustomChartTags(nil, []int{1}); got != nil {
		t.Fatalf("nil source tags = %v", got)
	}
	if got := resolveCustomChartTags(&lookupTestSource{}, nil); got != nil {
		t.Fatalf("empty tag IDs = %v", got)
	}
	tagSource := &customChartDirectSource{customTagNames: map[int]string{1: " One ", 2: ""}}
	if got := resolveCustomChartTags(tagSource, []int{1, 2, 1}); !reflect.DeepEqual(got, []string{"One"}) {
		t.Fatalf("resolved tags = %v", got)
	}
	if got := compactCustomChartStrings([]string{" a ", "", "  ", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("compact strings = %v", got)
	}
}

func TestCustomChartScalarAndNoteHelpers(t *testing.T) {
	raw := map[string]stdjson.RawMessage{
		"integer": []byte(`"42"`),
		"rounded": []byte(`2.6`),
		"bad":     []byte(`{`),
		"null":    []byte(`null`),
		"true":    []byte(`true`),
		"one":     []byte(`1`),
		"false":   []byte(`" FALSE "`),
		"badbool": []byte(`"maybe"`),
	}
	if got, ok := customChartRawInt(raw, "integer", -1); !ok || got != 42 {
		t.Fatalf("raw integer = %d, %v", got, ok)
	}
	if got, ok := customChartRawInt(raw, "rounded", -1); !ok || got != 3 {
		t.Fatalf("raw rounded = %d, %v", got, ok)
	}
	for _, key := range []string{"missing", "bad", "null"} {
		if got, ok := customChartRawInt(raw, key, -7); ok || got != -7 {
			t.Fatalf("raw int fallback for %q = %d, %v", key, got, ok)
		}
	}
	boolCases := []struct {
		key  string
		want bool
		ok   bool
	}{
		{"true", true, true}, {"one", true, true}, {"false", false, true},
		{"badbool", true, false}, {"missing", true, false}, {"null", true, false}, {"bad", true, false},
	}
	for _, tc := range boolCases {
		got, ok := customChartRawBool(raw, tc.key, true)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("raw bool %q = %v, %v; want %v, %v", tc.key, got, ok, tc.want, tc.ok)
		}
	}

	floatCases := []struct {
		value any
		want  float64
		ok    bool
	}{
		{float64(1.5), 1.5, true}, {float32(2.5), 2.5, true}, {int(3), 3, true},
		{int64(4), 4, true}, {stdjson.Number("5.5"), 5.5, true}, {" 6.5 ", 6.5, true},
		{stdjson.Number("bad"), 0, false}, {"bad", 0, false}, {true, 0, false},
	}
	for _, tc := range floatCases {
		got, ok := customChartFloatValue(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("float value %#v = %v, %v; want %v, %v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
	intCases := []struct {
		value any
		want  int
		ok    bool
	}{
		{int(1), 1, true}, {int64(2), 2, true}, {float64(2.6), 3, true},
		{stdjson.Number("4"), 4, true}, {stdjson.Number("4.6"), 5, true}, {" 6 ", 6, true},
		{stdjson.Number("bad"), 0, false}, {"bad", 0, false}, {true, 0, false},
	}
	for _, tc := range intCases {
		got, ok := customChartIntValue(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("int value %#v = %v, %v; want %v, %v", tc.value, got, ok, tc.want, tc.ok)
		}
	}

	var note customChartNote
	if err := note.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("invalid note JSON unexpectedly succeeded")
	}
	if err := stdjson.Unmarshal([]byte(`{"id":"7","ticks":"240","laneStart":-2,"laneEnd":20,"previousConnectionId":null,"nextConnectionId":"8","type":"1","isSkip":1}`), &note); err != nil {
		t.Fatalf("unmarshal note: %v", err)
	}
	if note.ID != 7 || note.PreviousConnectionID != -1 || note.NextConnectionID != 8 || !note.Critical || !note.IsSkip || customChartLane(note) != 0 || customChartWidth(note) != 12 {
		t.Fatalf("unexpected note: %+v", note)
	}
	if clampCustomChartInt(-1, 0, 3) != 0 || clampCustomChartInt(4, 0, 3) != 3 || clampCustomChartInt(2, 0, 3) != 2 {
		t.Fatal("clamp branches failed")
	}
	if absCustomChartInt(-3) != 3 || absCustomChartInt(3) != 3 {
		t.Fatal("absolute value branches failed")
	}
	if customChartBoolInt(true) != 1 || customChartBoolInt(false) != 0 {
		t.Fatal("bool-int conversion failed")
	}
}

func TestCustomChartConversionBranches(t *testing.T) {
	slideCases := []struct {
		note  customChartNote
		last  bool
		kind  customChartSlideKind
		flick customChartFlickType
		step  customChartHoldStepType
		end   customChartHoldNoteType
		decor bool
	}{
		{customChartNote{NoteBaseType: 2}, false, customChartSlideStart, customChartFlickNone, customChartHoldStepNormal, customChartHoldNoteNormal, false},
		{customChartNote{NoteBaseType: 1, Direction: 1}, false, customChartSlideEnd, customChartFlickLeft, customChartHoldStepNormal, customChartHoldNoteNormal, false},
		{customChartNote{NoteBaseType: 6, Direction: 2}, false, customChartSlideInvisible, customChartFlickRight, customChartHoldStepHidden, customChartHoldNoteNormal, false},
		{customChartNote{NoteBaseType: 5, IsSkip: true, Category: 3}, false, customChartSlideRelay, customChartFlickDefault, customChartHoldStepSkip, customChartHoldNoteNormal, false},
		{customChartNote{NoteBaseType: 10}, true, customChartSlideEnd, customChartFlickNone, customChartHoldStepNormal, customChartHoldNoteGuide, true},
		{customChartNote{NoteBaseType: 9}, false, customChartSlideStart, customChartFlickNone, customChartHoldStepNormal, customChartHoldNoteHidden, false},
	}
	for _, tc := range slideCases {
		if got := customChartSlideKindFor(tc.note, tc.last); got != tc.kind {
			t.Fatalf("slide kind for %+v = %v, want %v", tc.note, got, tc.kind)
		}
		if got := customChartFlickTypeFor(tc.note); got != tc.flick {
			t.Fatalf("flick type for %+v = %v, want %v", tc.note, got, tc.flick)
		}
		if got := customChartStepTypeFor(tc.note, tc.kind); got != tc.step {
			t.Fatalf("step type for %+v = %v, want %v", tc.note, got, tc.step)
		}
		if got := customChartEndpointTypeFor(tc.note, tc.decor); got != tc.end {
			t.Fatalf("endpoint type for %+v = %v, want %v", tc.note, got, tc.end)
		}
	}
	if !customChartIsVisibleRelayAttachment(customChartNote{NoteBaseType: 5, IsSkip: true}) {
		t.Fatal("visible relay attachment not recognized")
	}
	for _, noteType := range []customChartNoteType{customChartNoteHold, customChartNoteHoldMid, customChartNoteHoldEnd} {
		if !customChartNoteRequiresHold(noteType) {
			t.Fatalf("note type %v should require a hold", noteType)
		}
	}
	if customChartNoteRequiresHold(customChartNoteTap) {
		t.Fatal("tap unexpectedly requires a hold")
	}

	score := newCustomChartScore()
	addCustomChartTap(&score, customChartNote{NoteBaseType: 9}, false)
	if len(score.notes) != 0 {
		t.Fatal("cancel tap was added")
	}
	addCustomChartTap(&score, customChartNote{Ticks: 1, LaneStart: 2, LaneEnd: 3}, true)
	if len(score.notes) != 1 || !score.notes[1].Critical {
		t.Fatalf("tap conversion failed: %+v", score.notes)
	}

	duplicateA := &customChartNote{Ticks: 10, NoteBaseType: 5}
	duplicateB := &customChartNote{Ticks: 9, NoteBaseType: 5}
	plain := &customChartNote{Ticks: 20}
	filtered := removeAdjacentCustomChartVisibleRelayDuplicates([]*customChartNote{duplicateA, duplicateB, plain})
	if len(filtered) != 2 || filtered[0] != duplicateB {
		t.Fatalf("visible relay de-duplication = %+v", filtered)
	}
	if !customChartChainHasDecoration([]*customChartNote{nil, {Category: 9}}) || customChartChainHasDecoration([]*customChartNote{plain}) {
		t.Fatal("decoration detection failed")
	}

	chainScore := newCustomChartScore()
	addCustomChartChain(&chainScore, []*customChartNote{nil})
	addCustomChartChain(&chainScore, []*customChartNote{
		{ID: 1, Ticks: 1, LaneStart: 1, LaneEnd: 2},
		{ID: 2, Ticks: 241, LaneStart: 2, LaneEnd: 3, NoteBaseType: 6},
		{ID: 3, Ticks: 481, LaneStart: 3, LaneEnd: 4},
	})
	if len(chainScore.holdNotes) != 1 || calculateCustomChartScoreComboCount(chainScore) == 0 {
		t.Fatalf("chain conversion failed: notes=%+v holds=%+v", chainScore.notes, chainScore.holdNotes)
	}

	manual := newCustomChartScore()
	manual.notes[1] = customChartConvertedNote{ID: 1, ParentID: -1, Type: customChartNoteHold, Tick: 1, Lane: 1, Width: 1}
	manual.notes[2] = customChartConvertedNote{ID: 2, ParentID: 1, Type: customChartNoteHoldMid, Tick: 241, Lane: 1, Width: 1}
	manual.notes[3] = customChartConvertedNote{ID: 3, ParentID: 1, Type: customChartNoteHoldEnd, Tick: 481, Lane: 1, Width: 1}
	manual.notes[4] = customChartConvertedNote{ID: 4, ParentID: 999, Type: customChartNoteHoldEnd}
	manual.notes[5] = customChartConvertedNote{ID: 5, ParentID: -1, Type: customChartNoteTap, Tick: 1, Lane: 1, Width: 1}
	manual.notes[6] = manual.notes[5]
	manual.notes[6] = customChartConvertedNote{ID: 6, ParentID: -1, Type: customChartNoteTap, Tick: 1, Lane: 1, Width: 1}
	manual.holdNotes[1] = customChartHold{
		Start: customChartHoldStep{ID: 1},
		Steps: []customChartHoldStep{{ID: 2, Type: customChartHoldStepHidden}},
		End:   3,
	}
	if got := calculateCustomChartScoreComboCount(manual); got != 5 {
		t.Fatalf("manual combo count = %d, want 5", got)
	}

	guide := manual
	guide.holdNotes = map[int]customChartHold{1: {Start: customChartHoldStep{ID: 1}, End: 3, StartType: customChartHoldNoteGuide}}
	if got := calculateCustomChartScoreComboCount(guide); got < 1 {
		t.Fatalf("guide score should still count independent taps, got %d", got)
	}

	missingEndpoints := newCustomChartScore()
	missingEndpoints.holdNotes[1] = customChartHold{Start: customChartHoldStep{ID: 1}, End: 2}
	_ = calculateCustomChartScoreComboCount(missingEndpoints)
	missingEndpoints.notes[1] = customChartConvertedNote{ID: 1, Type: customChartNoteHold}
	_ = calculateCustomChartScoreComboCount(missingEndpoints)

	if _, ok := customChartHoldForNote(manual, manual.notes[1]); !ok {
		t.Fatal("hold start lookup failed")
	}
	if _, ok := customChartHoldForNote(manual, manual.notes[2]); !ok {
		t.Fatal("hold child lookup failed")
	}
	if _, ok := customChartHoldForNote(manual, manual.notes[5]); ok {
		t.Fatal("tap unexpectedly resolved a hold")
	}
	if customChartComboDedupKey(manual.notes[5]) == "" || customChartHoldHalfBeatDedupKey(1, manual.holdNotes[1], manual, 240) == "" {
		t.Fatal("combo de-duplication keys are empty")
	}
}

func TestCustomChartBPMAndDecodingBranches(t *testing.T) {
	events := []struct {
		EventType   int `json:"eventType"`
		ChangeValue any `json:"changeValue"`
	}{
		{EventType: 1, ChangeValue: 10},
		{EventType: 0, ChangeValue: "bad"},
		{EventType: 0, ChangeValue: 0},
		{EventType: 0, ChangeValue: 180},
		{EventType: 0, ChangeValue: 180.0},
		{EventType: 0, ChangeValue: 120.25},
		{EventType: 0, ChangeValue: 240},
		{EventType: 0, ChangeValue: 90},
	}
	if got := formatCustomChartBPMs(events); got != "90-240（4段）" {
		t.Fatalf("formatted BPMs = %q", got)
	}
	if got := formatCustomChartBPMs(events[:3]); got != "" {
		t.Fatalf("invalid BPM events = %q", got)
	}
	if got := formatCustomChartBPMs(events[3:6]); got != "180 / 120.25" {
		t.Fatalf("short BPM events = %q", got)
	}

	rawJSON := []byte(` {"chart":[]} `)
	gzipJSON := gzipBytes(t, rawJSON)
	base64JSON := base64.RawStdEncoding.EncodeToString(rawJSON)
	base64Gzip := base64.StdEncoding.EncodeToString(gzipJSON)
	envelope := []byte(`{"userCustomMusicScoreJsonGzipBase64":"` + base64Gzip + `"}`)
	previewEnvelope := []byte(`{"userCustomMusicScorePreviewJsonGzipBase64":"` + base64JSON + `"}`)
	for name, input := range map[string][]byte{
		"raw": rawJSON, "gzip": gzipJSON, "base64": []byte(base64JSON),
		"base64_gzip": []byte(base64Gzip), "envelope": envelope, "preview": previewEnvelope,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeCustomMusicScoreJSONBytes(input)
			if err != nil || !stdjson.Valid(got) {
				t.Fatalf("decode = %q, %v", got, err)
			}
		})
	}

	if got, ok, err := decodeCustomMusicScoreEnvelope([]byte(`not-json`)); ok || err != nil || got != nil {
		t.Fatalf("non-JSON envelope = %q, %v, %v", got, ok, err)
	}
	if _, ok, err := decodeCustomMusicScoreEnvelope([]byte(`{"x":1}`)); !ok || err != nil {
		t.Fatalf("plain JSON envelope = %v, %v", ok, err)
	}
	if _, ok, err := decodeCustomMusicScoreEnvelope([]byte(`{`)); !ok || err == nil {
		t.Fatalf("malformed envelope = %v, %v", ok, err)
	}
	if _, ok, err := decodeCustomMusicScoreEnvelope([]byte(`{"userCustomMusicScoreJsonGzipBase64":12}`)); !ok || err != nil {
		t.Fatalf("non-string envelope = %v, %v", ok, err)
	}

	if _, ok, err := gunzipMaybe([]byte("plain")); ok || err != nil {
		t.Fatalf("plain gunzip = %v, %v", ok, err)
	}
	if _, ok, err := gunzipMaybe([]byte{0x1f, 0x8b, 0}); !ok || err == nil {
		t.Fatalf("invalid gzip = %v, %v", ok, err)
	}
	truncated := gzipBytes(t, rawJSON)
	truncated = truncated[:len(truncated)-4]
	if _, ok, err := gunzipMaybe(truncated); !ok || err == nil {
		t.Fatalf("truncated gzip = %v, %v", ok, err)
	}

	for _, value := range []string{
		base64.RawStdEncoding.EncodeToString([]byte("raw")),
		base64.URLEncoding.EncodeToString([]byte{0xfb, 0xff}),
		base64.RawURLEncoding.EncodeToString([]byte{0xfb, 0xff}),
	} {
		if _, ok, err := base64DecodeMaybe(value); !ok || err != nil {
			t.Fatalf("base64 %q = %v, %v", value, ok, err)
		}
	}
	if got, ok, err := base64DecodeMaybe(" "); ok || err != nil || got != nil {
		t.Fatalf("empty base64 = %q, %v, %v", got, ok, err)
	}
	if got, ok, err := base64DecodeMaybe("%%%!"); ok || err != nil || got != nil {
		t.Fatalf("invalid base64 = %q, %v, %v", got, ok, err)
	}

	invalidCases := [][]byte{
		nil,
		[]byte(`not json`),
		[]byte(`{"x":`),
		make([]byte, customChartMaxEncodedBytes+1),
		append([]byte(`{"x":"`), append(make([]byte, customChartMaxDecodedBytes), []byte(`"}`)...)...),
	}
	for _, input := range invalidCases {
		if _, err := decodeCustomMusicScoreJSONBytes(input); err == nil {
			t.Fatalf("invalid custom chart payload of %d bytes succeeded", len(input))
		}
	}
	if got, err := decodeCustomMusicScoreJSON(rawJSON); err != nil || !strings.Contains(got, "chart") {
		t.Fatalf("decode string = %q, %v", got, err)
	}
}

func TestCustomChartRequestFailureAndFallbackBranches(t *testing.T) {
	const scoreID = "_g5yakrvqobnfq6hafdob7ed8jwm"
	source := &customChartDirectSource{vocalBuilderTestSource: &vocalBuilderTestSource{
		music: &masterdata.Music{ID: 47, Title: "Original", AssetBundleName: "jacket"},
	}}
	builder := NewBuilder(source, nil, assets.NewAssetHelper("", nil))
	query := ChartQuery{Query: scoreID, Region: "jp", Difficulty: "expert"}
	var nilController *Controller
	if _, err := nilController.buildCustomMusicChartRequest(query, source, builder, renderregion.JP); err == nil {
		t.Fatal("nil controller custom chart succeeded")
	}
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	if _, err := controller.buildCustomMusicChartRequest(query, source, builder, renderregion.JP); err == nil {
		t.Fatal("missing custom score client succeeded")
	}
	controller.SetCustomMusicScoreClient(customChartErrorClient{})
	if _, err := controller.buildCustomMusicChartRequest(ChartQuery{Query: "bad id"}, source, builder, renderregion.JP); err == nil {
		t.Fatal("invalid custom score ID succeeded")
	}

	controller.SetCustomMusicScoreClient(customChartErrorClient{infoErr: errors.New("upstream")})
	if _, err := controller.BuildMusicChartRequest(query); err == nil || !strings.Contains(err.Error(), "获取自定义谱面信息失败") {
		t.Fatalf("published error = %v", err)
	}
	controller.SetCustomMusicScoreClient(customChartErrorClient{published: &sekaiapi.UserCustomMusicScorePublishedResponse{UserCustomMusicScoreID: scoreID}})
	if _, err := controller.BuildMusicChartRequest(query); err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("missing path error = %v", err)
	}

	published := &sekaiapi.UserCustomMusicScorePublishedResponse{
		UserCustomMusicScoreID: scoreID,
		MusicID:                47,
		UserCustomMusicScoreInfoJSON: &sekaiapi.UserCustomMusicScoreInfo{
			MusicID:                  47,
			UserCustomMusicScorePath: "path",
		},
	}
	controller.SetCustomMusicScoreClient(customChartErrorClient{published: published, scoreErr: errors.New("score")})
	if _, err := controller.BuildMusicChartRequest(query); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("score fetch error = %v", err)
	}
	controller.SetCustomMusicScoreClient(customChartErrorClient{published: published, score: []byte("bad")})
	if _, err := controller.BuildMusicChartRequest(query); err == nil || !strings.Contains(err.Error(), "格式无效") {
		t.Fatalf("score decode error = %v", err)
	}

	missingSource := &customChartDirectSource{vocalBuilderTestSource: &vocalBuilderTestSource{}}
	missingController := NewController(missingSource, nil, assets.NewAssetHelper("", nil), nil, nil)
	missingController.SetCustomMusicScoreClient(customChartErrorClient{published: published, score: []byte(`{}`)})
	if _, err := missingController.BuildMusicChartRequest(query); err == nil || !strings.Contains(err.Error(), "原曲数据不存在") {
		t.Fatalf("missing original music error = %v", err)
	}

	published.MusicDifficultyType = ""
	published.PlayLevel = 0
	controller.SetCustomMusicScoreClient(customChartErrorClient{published: published, score: []byte(`{}`)})
	req, err := controller.BuildMusicChartRequest(query)
	if err != nil {
		t.Fatalf("fallback chart request: %v", err)
	}
	if req.Difficulty != "expert" || req.PlayLevel != "?" {
		t.Fatalf("fallback difficulty/level = %q/%v", req.Difficulty, req.PlayLevel)
	}

	if _, err := nilController.buildCustomMusicDetailRequest(Query{Query: scoreID}, source, builder, renderregion.JP); err == nil {
		t.Fatal("nil controller custom detail succeeded")
	}
	noClient := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	if _, err := noClient.buildCustomMusicDetailRequest(Query{Query: scoreID}, source, builder, renderregion.JP); err == nil {
		t.Fatal("missing custom detail client succeeded")
	}
	noClient.SetCustomMusicScoreClient(customChartErrorClient{})
	if _, err := noClient.buildCustomMusicDetailRequest(Query{Query: "bad id"}, source, builder, renderregion.JP); err == nil {
		t.Fatal("invalid detail score ID succeeded")
	}
}
