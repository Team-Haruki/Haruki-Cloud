package deck

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
)

func TestOptionAndNormalizationHelpersAdditional(t *testing.T) {
	if optionString(nil, "x") != "" || optionString(map[string]any{"x": 1}, "x") != "" || optionString(map[string]any{"x": "ok"}, "x") != "ok" {
		t.Fatal("unexpected optionString result")
	}
	values := []struct {
		value any
		want  int
		ok    bool
	}{
		{int(1), 1, true}, {int64(2), 2, true}, {int32(3), 3, true},
		{float64(4.9), 4, true}, {float32(5.9), 5, true},
		{json.Number("6"), 6, true}, {json.Number("7.5"), 7, true},
		{json.Number("bad"), 0, false}, {"8", 0, false},
	}
	for _, tc := range values {
		got, ok := optionIntValue(map[string]any{"x": tc.value}, "x")
		if got != tc.want || ok != tc.ok {
			t.Fatalf("optionIntValue(%T(%v)) = %d,%v", tc.value, tc.value, got, ok)
		}
	}
	if _, ok := optionIntValue(nil, "x"); ok {
		t.Fatal("nil integer option should not exist")
	}
	for _, tc := range []struct {
		value any
		want  float64
		ok    bool
	}{{float64(1.5), 1.5, true}, {float32(2.5), 2.5, true}, {3, 3, true}, {"4", 0, false}} {
		got, ok := optionFloat(map[string]any{"x": tc.value}, "x")
		if got != tc.want || ok != tc.ok {
			t.Fatalf("optionFloat(%v) = %v,%v", tc.value, got, ok)
		}
	}
	if _, ok := optionFloat(nil, "x"); ok {
		t.Fatal("nil float option should not exist")
	}

	if got := normalizeRecommendAlgorithmSubset([]string{"ga", "GA", "bad", "all"}); !reflect.DeepEqual(got, []string{"ga"}) {
		t.Fatalf("string subset = %#v", got)
	}
	if got := normalizeRecommendAlgorithmSubset([]any{"dfs", 1, "rl"}); !reflect.DeepEqual(got, []string{"dfs", "rl"}) {
		t.Fatalf("any subset = %#v", got)
	}
	if normalizeRecommendAlgorithmSubset("ga") != nil {
		t.Fatal("unexpected subset for scalar")
	}
	applyRecommendAlgorithmSubset(nil, []string{"ga"})
	option := map[string]any{recommendAlgorithmSubsetKey: []string{"ga"}}
	applyRecommendAlgorithmSubset(option, nil)
	if _, ok := option[recommendAlgorithmSubsetKey]; ok {
		t.Fatal("empty subset was not removed")
	}
	applyRecommendAlgorithmSubset(option, []string{"ga", "ga", "bad"})
	if got := selectRecommendAlgorithmSubset(option, []string{"rl", "ga"}); !reflect.DeepEqual(got, []string{"ga"}) {
		t.Fatalf("selected subset = %#v", got)
	}
	skillOption := map[string]any{"target": "skill", recommendAlgorithmSubsetKey: []any{"dfs", "dfs_ga", "bad"}}
	if got := selectRecommendAlgorithmSubset(skillOption, []string{"dfs_ga"}); !reflect.DeepEqual(got, []string{"dfs_ga"}) {
		t.Fatalf("skill subset = %#v", got)
	}
	if selectRecommendAlgorithmSubset(nil, []string{"ga"}) != nil || selectRecommendAlgorithmSubset(option, nil) != nil {
		t.Fatal("empty select subset should be nil")
	}
	if got := selectRecommendAlgorithmSubset(map[string]any{}, []string{"ga"}); got != nil {
		t.Fatalf("missing select subset = %#v", got)
	}

	cases := map[string]string{
		"challenge/auto":  "challenge_auto",
		"challenge/solo":  "challenge",
		"mysekai/multi":   "mysekai",
		"event/multi":     "multi",
		"event/challenge": "challenge",
		"event/bad":       "",
		"event/":          "",
	}
	for key, want := range cases {
		parts := filepath.SplitList(key)
		_ = parts
		var recType, raw string
		for index, char := range key {
			if char == '/' {
				recType, raw = key[:index], key[index+1:]
				break
			}
		}
		if got := normalizeRecommendLiveType(recType, raw); got != want {
			t.Fatalf("normalizeRecommendLiveType(%q,%q) = %q", recType, raw, got)
		}
	}
}

func TestPathAndConversionHelpersAdditional(t *testing.T) {
	controller := newTestDeckController(t, RecommendConfig{})
	if (&Controller{}).resolveCharacterIconPath(0) != "" || (*Controller)(nil).resolveCharacterIconPath(1) != "" {
		t.Fatal("invalid character icon should be empty")
	}
	if controller.resolveCharacterIconPath(1) == "" || controller.resolveCharacterIconPath(999) == "" {
		t.Fatal("character icon path should resolve")
	}
	if controller.resolveCharacterName(renderregion.JP, 1) != "星乃一歌" || controller.resolveCharacterName(renderregion.JP, 999) != "" {
		t.Fatal("unexpected character name")
	}
	if (*Controller)(nil).resolveCharacterName(renderregion.JP, 1) != "" {
		t.Fatal("nil controller character name should be empty")
	}
	if controller.resolveUnitIconPath("bad") != "" || controller.resolveUnitIconPath("idol") == "" {
		t.Fatal("unexpected unit icon path")
	}
	if controller.resolveAttrIconPath("bad") != "" || controller.resolveAttrIconPath("cute") == "" {
		t.Fatal("unexpected attr icon path")
	}

	if got := pickBonusTargets([]int{5}, "120"); !reflect.DeepEqual(got, []int{5}) {
		t.Fatalf("explicit bonus targets = %#v", got)
	}
	if got := pickBonusTargets(nil, "100% 加成120％ bad -1"); !reflect.DeepEqual(got, []int{100, 120}) {
		t.Fatalf("parsed bonus targets = %#v", got)
	}
	if got := pickBonusTargets(nil, "bad"); !reflect.DeepEqual(got, []int{120}) {
		t.Fatalf("default bonus targets = %#v", got)
	}
	if _, err := parseBonusTarget("bad"); err == nil {
		t.Fatal("expected invalid bonus target")
	}
	if got := toInterfaceSlice(nil); got != nil {
		t.Fatalf("nil interface slice = %#v", got)
	}
	if got := toInterfaceSlice([]string{"a", "b"}); !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Fatalf("interface slice = %#v", got)
	}
	if got := toInterfaceMap(nil); got != nil {
		t.Fatalf("nil interface map = %#v", got)
	}
	if got := toInterfaceMap(map[string]float64{"x": 1.5}); got["x"] != 1.5 {
		t.Fatalf("interface map = %#v", got)
	}
	if *float64Ptr(2.5) != 2.5 {
		t.Fatal("float64Ptr mismatch")
	}
	for _, config := range []map[string]any{defaultDeckConfig12(), defaultDeckConfig34bd(), noChangeDeckConfig()} {
		if len(config) != 6 {
			t.Fatalf("unexpected deck config = %#v", config)
		}
	}
}

func TestMasterdataPathHelpersAdditional(t *testing.T) {
	if got, err := resolveDeckMasterdataDir("", "jp"); err != nil || got != "" {
		t.Fatalf("empty masterdata dir = %q, %v", got, err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if got, err := resolveDeckMasterdataDir(missing, "jp"); err != nil || got != missing {
		t.Fatalf("missing root = %q, %v", got, err)
	}
	root := t.TempDir()
	jp := filepath.Join(root, "jp")
	if err := os.Mkdir(jp, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveDeckMasterdataDir(root, ""); err != nil || got != jp {
		t.Fatalf("region root = %q, %v", got, err)
	}
	if _, err := resolveDeckMasterdataDir(root, "cn"); err == nil {
		t.Fatal("expected missing region error")
	}
	flat := t.TempDir()
	if err := os.WriteFile(filepath.Join(flat, "cards.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveDeckMasterdataDir(flat, "cn"); err != nil || got != flat {
		t.Fatalf("flat root = %q, %v", got, err)
	}
	emptyRoot := t.TempDir()
	if got, err := resolveDeckMasterdataDir(emptyRoot, "jp"); err != nil || got != emptyRoot {
		t.Fatalf("empty root = %q, %v", got, err)
	}
	if got := resolveDeckRemoteMasterdataDir(""); got != "" {
		t.Fatalf("empty remote root = %q", got)
	}
	if got := resolveDeckRemoteMasterdataDir(jp); got != root {
		t.Fatalf("remote region root = %q", got)
	}
	if got := resolveDeckRemoteMasterdataDir(flat); got != flat {
		t.Fatalf("remote flat root = %q", got)
	}

	if _, ok := resolveDeckMasterdataContentDir("", "jp"); ok {
		t.Fatal("empty content dir should fail")
	}
	if err := os.WriteFile(filepath.Join(jp, "areaItemLevels.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolveDeckMasterdataContentDir(root, ""); !ok || got != jp {
		t.Fatalf("content dir = %q,%v", got, ok)
	}
	repoRoot := t.TempDir()
	repoMaster := filepath.Join(repoRoot, deckMasterdataRepoDirs["cn"], "master")
	if err := os.MkdirAll(repoMaster, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoMaster, "areaItemLevels.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolveDeckMasterdataContentDir(repoRoot, "cn"); !ok || got != repoMaster {
		t.Fatalf("repo content dir = %q,%v", got, ok)
	}
	if _, ok := resolveDeckMasterdataContentDir(t.TempDir(), "jp"); ok {
		t.Fatal("missing marker should fail")
	}

	if found, checked := deckMasterdataContainsEvent(root, "jp", 0); found || checked {
		t.Fatal("invalid event should be unchecked")
	}
	if found, checked := deckMasterdataContainsEvent(root, "jp", 7); found || checked {
		t.Fatal("missing events should be unchecked")
	}
	eventsPath := filepath.Join(jp, "events.json")
	if err := os.WriteFile(eventsPath, []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if found, checked := deckMasterdataContainsEvent(root, "jp", 7); found || checked {
		t.Fatal("invalid events should be unchecked")
	}
	if err := os.WriteFile(eventsPath, []byte(`[{"id":7},{"id":8}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if found, checked := deckMasterdataContainsEvent(root, "jp", 7); !found || !checked {
		t.Fatalf("event 7 = %v,%v", found, checked)
	}
	if found, checked := deckMasterdataContainsEvent(root, "jp", 9); found || !checked {
		t.Fatalf("event 9 = %v,%v", found, checked)
	}
	if _, ok := resolveDeckMasterdataEventsFile("", "jp"); ok {
		t.Fatal("empty events root should fail")
	}
	if got, ok := resolveDeckMasterdataEventsFile(root, ""); !ok || got != eventsPath {
		t.Fatalf("events file = %q,%v", got, ok)
	}
	if !hasDeckRegionSubdirs(root) || hasDeckRegionSubdirs(t.TempDir()) {
		t.Fatal("unexpected region subdir detection")
	}
	if !dirExists(jp) || dirExists(eventsPath) || !fileExists(eventsPath) || fileExists(jp) || fileExists(filepath.Join(root, "none")) {
		t.Fatal("unexpected file/dir detection")
	}
}

func TestMasterdataSignatureRefreshAdditional(t *testing.T) {
	if _, err := deckMasterdataDirSignature(t.TempDir(), "jp"); err == nil {
		t.Fatal("expected missing content dir error")
	}
	root := t.TempDir()
	jp := filepath.Join(root, "jp")
	if err := os.MkdirAll(filepath.Join(jp, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jp, "areaItemLevels.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jp, "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jp, ".git", "ignored.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	sig, err := deckMasterdataDirSignature(root, "jp")
	if err != nil || sig.Files != 1 || sig.Hash == "" {
		t.Fatalf("signature = %+v, %v", sig, err)
	}

	state := &remoteTargetState{masterdataReady: true}
	remote := &RemoteDeckRecommender{
		masterdataDir: root,
		region:        "jp",
		targetStates:  map[string]*remoteTargetState{"one": state},
	}
	remote.captureMasterdataSignature()
	if remote.masterdataSig == "" {
		t.Fatal("initial signature missing")
	}
	remote.refreshMasterdataSignature()
	if !state.masterdataReady {
		t.Fatal("unchanged signature invalidated state")
	}
	if err := os.WriteFile(filepath.Join(jp, "cards.json"), []byte(`[1,2,3]`), 0o644); err != nil {
		t.Fatal(err)
	}
	remote.refreshMasterdataSignature()
	if state.masterdataReady {
		t.Fatal("changed signature did not invalidate state")
	}
	remote.masterdataSig = ""
	state.masterdataReady = true
	remote.refreshMasterdataSignature()
	if remote.masterdataSig == "" || !state.masterdataReady {
		t.Fatal("empty previous signature should initialize only")
	}

	provider := &remoteEngineProvider{recommenders: map[string]PjskDeckRecommender{"jp": remote}}
	provider.refreshMasterdataSignatures()
	(*remoteEngineProvider)(nil).refreshMasterdataSignatures()
	(*RemoteDeckRecommender)(nil).captureMasterdataSignature()
	(*RemoteDeckRecommender)(nil).refreshMasterdataSignature()
	(&remoteEngineProvider{}).startMasterdataRefreshLoop()
}

func TestControllerEntryHelpersAdditional(t *testing.T) {
	var nilController *Controller
	nilController.RegisterCardSource(nil)
	nilController.RegisterEventSource(nil)
	nilController.RegisterMusicSource(nil)
	if nilController.WithSnapshot(nil) != nil || nilController.contextOrBackground() == nil {
		t.Fatal("nil controller helper mismatch")
	}
	c := &Controller{defaultRegion: renderregion.JP, assets: assets.NewAssetHelper("", nil)}
	c.RegisterCardSource(nil)
	c.RegisterEventSource(nil)
	c.RegisterMusicSource(nil)
	if c.cardSources == nil || c.eventSources == nil || c.musicSources == nil {
		t.Fatal("registries were not initialized")
	}
	if _, err := c.BuildRecommendRequest(drawing.DeckRequest{}); err == nil {
		t.Fatal("expected missing region error")
	}
	if _, err := c.BuildRecommendRequest(drawing.DeckRequest{Region: "jp"}); err == nil {
		t.Fatal("expected empty deck data error")
	}
	req := drawing.DeckRequest{Region: "jp", DeckData: []drawing.DeckData{{}}}
	if got, err := c.BuildRecommendRequest(req); err != nil || got.Region != "jp" {
		t.Fatalf("valid request = %+v, %v", got, err)
	}
	if _, err := nilController.RenderRecommend(req); err == nil {
		t.Fatal("expected missing drawing error")
	}
	if _, err := nilController.RenderAutoRecommend(AutoQuery{}); err == nil {
		t.Fatal("expected missing drawing error")
	}
	if nilController.recommendTimeoutMs() != 60000 || (&Controller{}).recommendTimeoutMs() != 60000 {
		t.Fatal("default timeout mismatch")
	}
	c.recommendCfg.Timeout = 1500 * time.Millisecond
	if c.recommendTimeoutMs() != 1500 {
		t.Fatal("configured timeout mismatch")
	}
	for _, recType := range []string{"", "event", "challenge", "no_event", "bonus", "mysekai"} {
		_, gotType, err := c.normalizeAutoQuery(AutoQuery{Region: "jp", RecommendType: recType})
		if err != nil || (recType != "" && gotType != recType) || (recType == "" && gotType != "event") {
			t.Fatalf("normalize rec type %q = %q,%v", recType, gotType, err)
		}
	}
	if _, _, err := c.normalizeAutoQuery(AutoQuery{RecommendType: "bad"}); err == nil {
		t.Fatal("expected unsupported recommend type")
	}
	c.ctx = context.Background()
	if c.contextOrBackground() == nil {
		t.Fatal("controller context missing")
	}
	if _, err := (*Controller)(nil).resolveAutoRecommendSnapshot(AutoQuery{}); err == nil {
		t.Fatal("expected nil controller snapshot error")
	}
	if _, err := (&Controller{}).resolveAutoRecommendSnapshot(AutoQuery{}); err == nil {
		t.Fatal("expected missing snapshot error")
	}
	if _, err := encodeSyntheticAutoRecommendRawUserData(nil); err == nil {
		t.Fatal("expected nil synthetic raw error")
	}
	if got := sliceOrEmpty([]int(nil)); got == nil || len(got) != 0 {
		t.Fatalf("nil slice = %#v", got)
	}
	if got := sliceOrEmpty([]int{1}); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("nonempty slice = %#v", got)
	}
}
