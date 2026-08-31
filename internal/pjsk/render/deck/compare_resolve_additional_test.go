package deck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestMusicComparePureHelpersAdditional(t *testing.T) {
	assertMusicComparePayloadHelpers(t)
	assertMusicCompareFixedDeckHelpers(t)
	assertMusicCompareCandidateHelpers(t)
	assertMusicCompareConversionHelpers(t)
}

func assertMusicComparePayloadHelpers(t *testing.T) {
	t.Helper()
	payload := []byte("inline")
	if got, err := resolveMusicCompareMetaPayload(payload, ""); err != nil || string(got) != "inline" {
		t.Fatalf("inline metadata = %q,%v", got, err)
	}
	if _, err := resolveMusicCompareMetaPayload(nil, ""); err == nil {
		t.Fatal("expected missing metadata error")
	}
	if _, err := resolveMusicCompareMetaPayload(nil, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected metadata read error")
	}
	path := filepath.Join(t.TempDir(), "music.json")
	if err := os.WriteFile(path, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveMusicCompareMetaPayload(nil, " "+path+" "); err != nil || string(got) != "file" {
		t.Fatalf("file metadata = %q,%v", got, err)
	}
}

func assertMusicCompareFixedDeckHelpers(t *testing.T) {
	t.Helper()
	if hasCompleteFixedDeckOption(nil) || hasCompleteFixedDeckOption(map[string]any{"fixed_cards": []int{1}}) {
		t.Fatal("incomplete deck considered complete")
	}
	if !hasCompleteFixedDeckOption(map[string]any{"fixed_cards": []int{1, 2, 3, 4, 5}}) || !hasCompleteFixedDeckOption(map[string]any{"use_current_deck": true}) {
		t.Fatal("complete deck considered incomplete")
	}
	if cloneMusicCompareSelections(nil) != nil {
		t.Fatal("nil selections clone should be nil")
	}
	original := []MusicCompareSelection{{MusicID: 1}}
	cloned := cloneMusicCompareSelections(original)
	cloned[0].MusicID = 2
	if original[0].MusicID != 1 {
		t.Fatal("selection clone mutated original")
	}
}

func assertMusicCompareCandidateHelpers(t *testing.T) {
	t.Helper()
	controller := newTestDeckController(t, RecommendConfig{})
	if _, err := controller.buildMusicCompareCandidateSelections(renderregion.JP, "event", map[string]any{}, []byte("bad")); err == nil {
		t.Fatal("expected invalid candidate metadata error")
	}
	if _, err := controller.buildMusicCompareCandidateSelections(renderregion.JP, "event", map[string]any{}, []byte(`[{"music_id":0}]`)); err == nil {
		t.Fatal("expected no candidates error")
	}
	metadata := []byte(`[
		{"music_id":2,"difficulty":"EXPERT","base_score":10,"skill_score_multi":[1,2],"fever_score":4,"event_rate":120},
		{"music_id":1,"difficulty":"","base_score":20,"skill_score_multi":[1],"fever_score":2,"event_rate":100},
		{"music_id":"bad","base_score":100}
	]`)
	selections, err := controller.buildMusicCompareCandidateSelections(renderregion.JP, "event", map[string]any{"live_type": "multi", "target": "score"}, metadata)
	if err != nil || len(selections) != 2 || selections[0].MusicID != 1 || selections[0].MusicDiff != "master" {
		t.Fatalf("candidate selections = %+v,%v", selections, err)
	}
	if value := compareMusicCandidateValue(map[string]any{
		"base_score_auto":  10.0,
		"skill_score_auto": []any{1.0, 2},
	}, "challenge_auto", false); value != 13 {
		t.Fatalf("auto candidate value = %v", value)
	}
	if value := compareMusicCandidateValue(map[string]any{
		"base_score":       10.0,
		"skill_score_solo": []float64{1, 2},
	}, "solo", false); value != 13 {
		t.Fatalf("solo candidate value = %v", value)
	}
}

func assertMusicCompareConversionHelpers(t *testing.T) {
	t.Helper()
	if normalizeMusicCompareDifficulty(" ") != "master" || normalizeMusicCompareDifficulty(" EXPERT ") != "expert" {
		t.Fatal("difficulty normalization mismatch")
	}
	if compareMetaString(" x ") != "x" || compareMetaString(1) != "" {
		t.Fatal("metadata string mismatch")
	}
	for _, tc := range []struct {
		value any
		want  float64
	}{{float64(1), 1}, {float32(2), 2}, {3, 3}, {int64(4), 4}, {"5", 0}} {
		if got := compareMetaFloat(tc.value); got != tc.want {
			t.Fatalf("compareMetaFloat(%v) = %v", tc.value, got)
		}
	}
	if compareMetaInt(float64(5.9)) != 5 {
		t.Fatal("metadata integer mismatch")
	}
	if got := compareMetaFloatSlice([]float64{1, 2}); !reflect.DeepEqual(got, []float64{1, 2}) {
		t.Fatalf("float slice = %#v", got)
	}
	if got := compareMetaFloatSlice([]any{float64(1), 2, "bad"}); !reflect.DeepEqual(got, []float64{1, 2, 0}) {
		t.Fatalf("any float slice = %#v", got)
	}
	if compareMetaFloatSlice("bad") != nil {
		t.Fatal("scalar float slice should be nil")
	}
}

func TestResolveAndJSONHelpersAdditional(t *testing.T) {
	controller := newTestDeckControllerWithMeta(t, RecommendConfig{}, &testMusicMetaSource{data: []byte("meta")})
	assertDeckResolveProfileHelpers(t, controller)
	assertDeckResolveEventHelpers(t)
	assertDeckResolveSourceHelpers(t, controller)
	assertDeckJSONHelpers(t)
}

func assertDeckResolveProfileHelpers(t *testing.T, controller *Controller) {
	t.Helper()
	mode := "x"
	override := &drawing.DetailedProfileCardRequest{ID: "1", Source: "source", Mode: &mode}
	profile := controller.resolveProfile(renderregion.JP, override, "fallback")
	if profile.Source != "" || profile.Mode != nil || override.Source != "source" {
		t.Fatalf("sanitized profile = %+v", profile)
	}
	if sanitizeDeckProfile(nil) != nil {
		t.Fatal("nil profile should remain nil")
	}
	empty := (&Controller{assets: assets.NewAssetHelper("", nil)}).resolveProfile(renderregion.JP, nil, "fallback")
	if empty.Nickname != "Unknown" || empty.Source != "" || !empty.IsHideUID {
		t.Fatalf("fallback profile = %+v", empty)
	}
	if string(controller.resolveMusicMeta(renderregion.JP)) != "meta" {
		t.Fatal("loader music metadata not used")
	}
	if (&Controller{}).resolveMusicMeta(renderregion.JP) != nil || (*Controller)(nil).resolveMusicMeta(renderregion.JP) != nil {
		t.Fatal("missing music metadata should be nil")
	}
	if (*Controller)(nil).resolveUserDataFilePath() != "" || (*Controller)(nil).resolveMusicMetaFilePath() != "" {
		t.Fatal("nil snapshot paths should be empty")
	}
	if controller.resolveUserDataFilePath() == "" {
		t.Fatal("snapshot user-data path missing")
	}
}

func assertDeckResolveEventHelpers(t *testing.T) {
	t.Helper()
	now := time.Now().UnixMilli()
	events := &testEventSource{region: renderregion.JP, events: map[int]*masterdata.Event{
		1: {ID: 1, StartAt: now - 1000, AggregateAt: now + 1000},
		2: {ID: 2, StartAt: now + 2000, AggregateAt: now + 3000},
		3: {ID: 3, StartAt: now - 5000, AggregateAt: now - 4000},
	}}
	eventController := NewController(nil, events, nil, nil, nil, renderregion.JP)
	if got := eventController.pickCurrentOrNextEventID(renderregion.JP); got != 1 {
		t.Fatalf("current event = %d", got)
	}
	events.events[1].AggregateAt = now - 1
	if got := eventController.pickCurrentOrNextEventID(renderregion.JP); got != 2 {
		t.Fatalf("next event = %d", got)
	}
	delete(events.events, 2)
	if got := eventController.pickCurrentOrNextEventID(renderregion.JP); got != 1 {
		t.Fatalf("latest event = %d", got)
	}
	if (&Controller{}).pickCurrentOrNextEventID(renderregion.JP) != 0 {
		t.Fatal("missing event source should return zero")
	}
}

func assertDeckResolveSourceHelpers(t *testing.T, controller *Controller) {
	t.Helper()
	assertDeckCardAndEventSources(t, controller)
	assertDeckMusicSources(t, controller)
}

func assertDeckCardAndEventSources(t *testing.T, controller *Controller) {
	t.Helper()
	if _, _, err := (&Controller{}).resolveCardSource(renderregion.JP); err == nil {
		t.Fatal("expected missing card source error")
	}
	if _, _, err := controller.resolveCardSource(renderregion.CN); err == nil {
		t.Fatal("expected missing region card source error")
	}
	if region, _, err := controller.resolveCardSource(renderregion.Unknown); err != nil || region != renderregion.JP {
		t.Fatalf("default card source = %s,%v", region, err)
	}
	if _, _, ok := (&Controller{}).resolveEventSource(renderregion.JP); ok {
		t.Fatal("missing event source should fail")
	}
	if _, _, ok := controller.resolveEventSource(renderregion.CN); ok {
		t.Fatal("missing region event source should fail")
	}
	if region, _, ok := controller.resolveEventSource(renderregion.Unknown); !ok || region != renderregion.JP {
		t.Fatalf("default event source = %s,%v", region, ok)
	}
}

func assertDeckMusicSources(t *testing.T, controller *Controller) {
	t.Helper()
	if (*Controller)(nil).resolveEventBannerPath("x", renderregion.JP) != "" || controller.resolveEventBannerPath("", renderregion.JP) != "" || controller.resolveEventBannerPath("banner", renderregion.JP) == "" {
		t.Fatal("event banner path mismatch")
	}
	if _, _, ok := (&Controller{}).resolveMusicSource(renderregion.JP); ok {
		t.Fatal("missing music source should fail")
	}
	if _, _, ok := controller.resolveMusicSource(renderregion.CN); ok {
		t.Fatal("missing region music source should fail")
	}
	if title, cover := controller.resolveCompareMusicMetadata(renderregion.JP, 0); title != "" || cover != "" {
		t.Fatal("invalid music metadata should be empty")
	}
	if title, cover := controller.resolveCompareMusicMetadata(renderregion.JP, 1); title != "Song A" || cover == "" {
		t.Fatalf("resolved music metadata = %q,%q", title, cover)
	}
}

func assertDeckJSONHelpers(t *testing.T) {
	t.Helper()
	if _, err := structToJSONObject(make(chan int)); err == nil {
		t.Fatal("expected object marshal error")
	}
	if _, err := structToJSONObject(1); err == nil {
		t.Fatal("expected scalar object decode error")
	}
	if object, err := structToJSONObject(nil); err != nil || len(object) != 0 {
		t.Fatalf("nil object = %#v,%v", object, err)
	}
	if object, err := structToJSONObject(struct {
		A int `json:"a"`
	}{A: 1}); err != nil || object["a"] == nil {
		t.Fatalf("struct object = %#v,%v", object, err)
	}
	if jsonArrayToObjects(nil) != nil || jsonArrayToObjects([]any{}) != nil {
		t.Fatal("empty JSON array should be nil")
	}
	objects := jsonArrayToObjects([]any{map[string]any{"a": 1}, "skip"})
	if len(objects) != 1 || objectAt(objects, -1) != nil || objectAt(objects, 1) != nil || objectAt(objects, 0)["a"] != 1 {
		t.Fatalf("JSON object helpers = %#v", objects)
	}
	if got := mergeJSONObjects(nil, map[string]any{"a": 1}); got["a"] != 1 {
		t.Fatalf("merge empty = %#v", got)
	}
	if got := mergeJSONObjects(map[string]any{"a": 1}, map[string]any{"a": 2, "b": 3}); got["a"] != 2 || got["b"] != 3 {
		t.Fatalf("merge objects = %#v", got)
	}
	if len(copyJSONObject(nil)) != 0 {
		t.Fatal("nil copy should be empty")
	}
	assertDeckJSONNumberHelpers(t)
}

func assertDeckJSONNumberHelpers(t *testing.T) {
	t.Helper()
	for _, tc := range []struct {
		value any
		want  int
		ok    bool
	}{{json.Number("1"), 1, true}, {json.Number("bad"), 0, false}, {float64(2.5), 2, true}, {3, 3, true}, {int64(4), 4, true}, {"5", 0, false}} {
		got, ok := jsonNumberToInt(tc.value)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("jsonNumberToInt(%v) = %d,%v", tc.value, got, ok)
		}
	}
}

func TestRecommendDeckComparisonAdditional(t *testing.T) {
	if !compareRecommendDecks("event", "power", RecommendDeck{TotalPower: 2}, RecommendDeck{TotalPower: 1}) {
		t.Fatal("power comparison failed")
	}
	if !compareRecommendDecks("event", "skill", RecommendDeck{MultiLiveScoreUp: 2}, RecommendDeck{MultiLiveScoreUp: 1}) {
		t.Fatal("skill comparison failed")
	}
	if !compareRecommendDecks("event", "bonus", RecommendDeck{EventBonusRate: 2}, RecommendDeck{EventBonusRate: 1}) ||
		!compareRecommendDecks("event", "bonus", RecommendDeck{EventBonusRate: 1, Score: 2}, RecommendDeck{EventBonusRate: 1, Score: 1}) ||
		!compareRecommendDecks("event", "bonus", RecommendDeck{EventBonusRate: 1, Score: 1, MultiLiveScoreUp: 2}, RecommendDeck{EventBonusRate: 1, Score: 1, MultiLiveScoreUp: 1}) {
		t.Fatal("bonus comparison failed")
	}
	if !compareRecommendDecks("no_event", "score", RecommendDeck{LiveScore: 2}, RecommendDeck{Score: 1}) {
		t.Fatal("no-event live score comparison failed")
	}

	base := RecommendDeck{MysekaiEventPoint: 10, TotalPower: 100, EventBonusRate: 5, SupportDeckBonusRate: 4, MultiLiveScoreUp: 3}
	if !compareMysekaiDecks(RecommendDeck{MysekaiEventPoint: 11}, base) {
		t.Fatal("mysekai event point comparison failed")
	}
	if !compareMysekaiDecks(RecommendDeck{MysekaiEventPoint: 10, TotalPower: 450000}, base) {
		t.Fatal("mysekai internal point comparison failed")
	}
	left := base
	right := base
	left.EventBonusRate = 6
	left.SupportDeckBonusRate = 3
	right.EventBonusRate = 5
	right.SupportDeckBonusRate = 4
	left.TotalPower = 101
	if !compareMysekaiDecks(left, right) {
		t.Fatal("mysekai power tie-break failed")
	}
	left, right = base, base
	left.SupportDeckBonusRate = 5
	left.EventBonusRate = 4
	if !compareMysekaiDecks(left, right) {
		t.Fatal("mysekai support tie-break failed")
	}
	left, right = base, base
	left.MultiLiveScoreUp = 4
	if !compareMysekaiDecks(left, right) {
		t.Fatal("mysekai skill tie-break failed")
	}
	if mysekaiCombinedBonusRate(base) != 9 || mysekaiInternalPoint(base) <= 0 || !floatAlmostEqual(1, 1+1e-10) || floatAlmostEqual(1, 2) {
		t.Fatal("mysekai numeric helper mismatch")
	}
}
