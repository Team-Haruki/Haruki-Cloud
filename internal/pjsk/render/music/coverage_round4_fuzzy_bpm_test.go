//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package music

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

func TestMusicFuzzyPrimitiveBranches(t *testing.T) {
	variants := map[rune]rune{
		'達': '达', '戀': '恋', '體': '体', '驗': '验', '華': '华', '離': '离', '鈴': '铃',
		'臺': '台', '彈': '弹', '聲': '声', '夢': '梦', '愛': '爱', '類': '类', '寧': '宁',
		'遙': '遥', '穂': '穗', '絵': '绘', '鏡': '镜', '連': '连', 'x': 'x',
	}
	for input, want := range variants {
		if got := normalizeMusicFuzzyVariantRune(input); got != want {
			t.Errorf("variant %q = %q, want %q", input, got, want)
		}
	}
	if got := normalizeMusicFuzzyText(" Ａ・達！ "); got != "a达" {
		t.Fatalf("normalized fuzzy text = %q", got)
	}
	if got := normalizeMusicFuzzyHanText("A 達-B"); got != "达" {
		t.Fatalf("normalized Han text = %q", got)
	}
	if got := normalizeMusicFuzzyWidth("ＡＢＣ"); got != "ABC" {
		t.Fatalf("normalized width = %q", got)
	}

	for length, want := range map[int]int{1: 0, 2: 0, 3: 1, 5: 1, 6: 2, 10: 2, 11: 3} {
		if got := fuzzyDistanceLimit(length); got != want {
			t.Errorf("distance limit %d = %d, want %d", length, got, want)
		}
	}
	if got := levenshteinDistance(nil, []rune("abc")); got != 3 {
		t.Fatalf("empty-left distance = %d", got)
	}
	if got := levenshteinDistance([]rune("abc"), nil); got != 3 {
		t.Fatalf("empty-right distance = %d", got)
	}
	if got := levenshteinDistance([]rune("kitten"), []rune("sitting")); got != 3 {
		t.Fatalf("levenshtein distance = %d", got)
	}
	if minFuzzyInt(1, 2) != 1 || minFuzzyInt(2, 1) != 1 || maxFuzzyInt(1, 2) != 2 || maxFuzzyInt(2, 1) != 2 || absInt(-3) != 3 || absInt(3) != 3 {
		t.Fatal("fuzzy min/max/abs helper mismatch")
	}

	if _, ok := scoreNormalizedMusicFuzzyCandidate("", "abc"); ok {
		t.Fatal("empty fuzzy query matched")
	}
	if score, ok := scoreNormalizedMusicFuzzyCandidate("abc", "abc"); !ok || score.matchType != 0 {
		t.Fatalf("exact score = %+v, %v", score, ok)
	}
	if score, ok := scoreNormalizedMusicFuzzyCandidate("abc", "xxabcxx"); !ok || score.matchType != 1 {
		t.Fatalf("contains score = %+v, %v", score, ok)
	}
	if score, ok := scoreNormalizedMusicFuzzyCandidate("abcd", "xxabxdyy"); !ok || score.matchType != 2 {
		t.Fatalf("substring score = %+v, %v", score, ok)
	}
	if score, ok := scoreNormalizedMusicFuzzyCandidate("abcd", "abce"); !ok || score.matchType != 3 {
		t.Fatalf("distance score = %+v, %v", score, ok)
	}
	if _, ok := scoreNormalizedMusicFuzzyCandidate("ab", "zz"); ok {
		t.Fatal("distant short candidate matched")
	}
	if _, ok := scoreMusicFuzzySubstring([]rune("abc"), []rune("xabcx")); ok {
		t.Fatal("short substring matched")
	}
	if _, ok := scoreMusicFuzzySubstring([]rune("abcd"), []rune("zzzzzz")); ok {
		t.Fatal("distant substring matched")
	}
	if score, ok := scoreMusicFuzzySubstring([]rune("abcd"), []rune("xxabxdyy")); !ok || score.matchType != 2 {
		t.Fatalf("fuzzy substring = %+v, %v", score, ok)
	}

	compareCases := []struct {
		a, b musicFuzzyScore
	}{
		{a: musicFuzzyScore{matchType: 1}, b: musicFuzzyScore{matchType: 2}},
		{a: musicFuzzyScore{distance: 1}, b: musicFuzzyScore{distance: 2}},
		{a: musicFuzzyScore{lengthGap: 1}, b: musicFuzzyScore{lengthGap: 2}},
		{a: musicFuzzyScore{textLen: 1}, b: musicFuzzyScore{textLen: 2}},
	}
	for _, tt := range compareCases {
		if compareMusicFuzzyScore(tt.a, tt.b) >= 0 {
			t.Errorf("compare scores %+v >= %+v", tt.a, tt.b)
		}
	}
	if min3Int(3, 1, 2) != 1 || min3Int(3, 4, 2) != 2 {
		t.Fatal("min3Int mismatch")
	}
}

func TestMusicFuzzyResolutionEdgeBranches(t *testing.T) {
	source := newRound4SearchSource()
	if _, err := resolveFuzzyMusicQuery(source, " ", false); err == nil {
		t.Fatal("empty fuzzy query error = nil")
	}
	if _, err := resolveFuzzyMusicQuery(source, "!!!", false); err == nil {
		t.Fatal("punctuation-only fuzzy query error = nil")
	}
	if _, err := resolveFuzzyMusicQuery(source, "missing", true); err == nil {
		t.Fatal("allow-unreleased missing fuzzy query error = nil")
	}

	now := time.Now().UnixMilli()
	source.musics = map[int]*masterdata.Music{
		1: {ID: 1, Title: "Future Match", PublishedAt: now + 100_000},
	}
	if _, err := resolveFuzzyMusicQuery(source, "Future Matc", false); err == nil {
		t.Fatal("unreleased fuzzy match error = nil")
	} else {
		var unreleased *releasecheck.UnreleasedError
		if !errors.As(err, &unreleased) {
			t.Fatalf("unreleased fuzzy error = %T %v", err, err)
		}
	}
	if _, err := resolveFuzzyMusicQuery(source, "unrelated", false); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing fuzzy error = %v", err)
	}

	source.musics = map[int]*masterdata.Music{
		1: {ID: 1, Title: "abcd", PublishedAt: 1},
		2: {ID: 2, Title: "xxabcdy", PublishedAt: 1},
		3: {ID: 3, Title: "abce", PublishedAt: 1},
	}
	got, err := resolveFuzzyMusicQuery(source, "abcd", false)
	if err != nil || got.ID != 1 {
		t.Fatalf("ranked fuzzy match = %#v, %v", got, err)
	}
	if musicFuzzyCandidates(source, nil) != nil {
		t.Fatal("nil music fuzzy candidates should be nil")
	}
	source.localizedErr = errors.New("localized unavailable")
	if got := musicFuzzyCandidates(source, source.musics[1]); len(got) != 2 {
		t.Fatalf("localized error candidates = %#v", got)
	}
	if score, ok := scoreMusicFuzzyCandidate("abc达", "xx達"); !ok || score.matchType < 1 {
		t.Fatalf("Han fallback score = %+v, %v", score, ok)
	}
}

func TestMusicBPMValidationAndPathHelpers(t *testing.T) {
	if _, err := (*Controller)(nil).ResolveMusicCover(Query{}); err == nil {
		t.Fatal("nil ResolveMusicCover() error = nil")
	}
	if _, err := NewController(nil, nil, nil, nil, nil).ResolveMusicCover(Query{Query: "song", Region: "jp"}); err == nil {
		t.Fatal("missing source cover error = nil")
	}
	source := newRound4SearchSource()
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	if _, err := controller.ResolveMusicCover(Query{Query: "missing", Region: "jp"}); err == nil || !strings.Contains(err.Error(), "failed to search") {
		t.Fatalf("cover search error = %v", err)
	}
	source.musics[1] = &masterdata.Music{ID: 1, Title: "No Jacket", PublishedAt: 1}
	if _, err := controller.ResolveMusicCover(Query{Query: "No Jacket", Region: "jp"}); err == nil || !strings.Contains(err.Error(), "jacket") {
		t.Fatalf("missing jacket error = %v", err)
	}

	if _, err := (*Controller)(nil).FindMusicChartsByBPM(BPMQuery{BPM: 100}); err == nil {
		t.Fatal("nil FindMusicChartsByBPM() error = nil")
	}
	if _, err := controller.FindMusicChartsByBPM(BPMQuery{BPM: 0}); err == nil {
		t.Fatal("zero BPM error = nil")
	}
	if _, err := NewController(nil, nil, nil, nil, nil).FindMusicChartsByBPM(BPMQuery{BPM: 100, Region: "jp"}); err == nil {
		t.Fatal("missing BPM source error = nil")
	}
	if _, err := controller.FindMusicChartsByBPM(BPMQuery{BPM: 123.5, Region: "jp", Difficulty: "expert"}); err == nil || !strings.Contains(err.Error(), "123.5") {
		t.Fatalf("no BPM matches error = %v", err)
	}

	if _, err := (*Controller)(nil).ResolveMusicBPM(Query{}); err == nil {
		t.Fatal("nil ResolveMusicBPM() error = nil")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := controller.WithContext(canceled).ResolveMusicBPM(Query{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ResolveMusicBPM() error = %v", err)
	}
	if _, err := NewController(nil, nil, nil, nil, nil).ResolveMusicBPM(Query{Query: "song", Region: "jp"}); err == nil {
		t.Fatal("missing BPM builder error = nil")
	}
	if _, err := controller.ResolveMusicBPM(Query{Query: "missing", Region: "jp"}); err == nil || !strings.Contains(err.Error(), "failed to search") {
		t.Fatalf("BPM search error = %v", err)
	}
	if _, err := controller.ResolveMusicBPM(Query{Query: "master No Jacket", Region: "jp"}); err == nil || !strings.Contains(err.Error(), "本地谱面") {
		t.Fatalf("missing chart error = %v", err)
	}

	var nilController *Controller
	if nilController.resolveLocalMusicJacket("x") != "" || controller.resolveLocalMusicJacket(" ") != "" {
		t.Fatal("invalid local jacket lookup was non-empty")
	}
	if nilController.resolveLocalChartPath("jp", 1, "expert") != "" || controller.resolveLocalChartPath("jp", 0, "expert") != "" || controller.resolveLocalChartPath("jp", 1, " ") != "" {
		t.Fatal("invalid local chart lookup was non-empty")
	}
	if got := controller.resolveLocalChartPath("", 1, "expert"); got != "" {
		t.Fatalf("missing default-region chart path = %q", got)
	}

	if got := controller.collectBPMSearchDifficulties(source, 1, " MASTER "); len(got) != 1 || got[0] != "master" {
		t.Fatalf("preferred BPM difficulties = %#v", got)
	}
	source.difficultyErr = errors.New("difficulties unavailable")
	if got := controller.collectBPMSearchDifficulties(source, 1, ""); len(got) == 0 {
		t.Fatal("fallback BPM difficulties are empty")
	}
	source.difficultyErr = nil
	source.difficulties = []*masterdata.MusicDifficulty{nil, {MusicDifficulty: "master"}, {MusicDifficulty: "expert"}, {MusicDifficulty: "master"}}
	if got := controller.collectBPMSearchDifficulties(source, 1, ""); len(got) != 2 || got[0] != "expert" || got[1] != "master" {
		t.Fatalf("deduplicated BPM difficulties = %#v", got)
	}
	if buildLookupMusic(nil, NewBuilder(source, nil, nil), renderregion.JP) != nil || chartContainsBPM(nil, 100) || chartContainsBPM(&parsedChartBPM{Events: []BPMEvent{{BPM: 120}}}, 100) {
		t.Fatal("BPM nil/no-match helper mismatch")
	}
	if formatLookupBPMValue(120) != "120" || formatLookupBPMValue(120.5) != "120.5" {
		t.Fatal("BPM formatting mismatch")
	}
}

func TestParseChartBPMMalformedAndDuplicateBranches(t *testing.T) {
	if _, err := parseChartBPM(nil, filepath.Join(t.TempDir(), "missing.txt")); err == nil || !strings.Contains(err.Error(), "open") {
		t.Fatalf("missing chart error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parseChartBPM(canceled, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled chart error = %v", err)
	}

	root := t.TempDir()
	invalidPath := filepath.Join(root, "invalid.txt")
	invalid := strings.Join([]string{
		"not a SUS line",
		"#BPMXX:not-a-number",
		"#ABC01:value",
		"#A0008:01",
		"#00008:0",
		"#00108:00FF",
	}, "\n")
	if err := os.WriteFile(invalidPath, []byte(invalid), 0o644); err != nil {
		t.Fatalf("write invalid chart: %v", err)
	}
	if _, err := parseChartBPM(nil, invalidPath); err == nil || !strings.Contains(err.Error(), "没有可用") {
		t.Fatalf("invalid BPM chart error = %v", err)
	}

	validPath := filepath.Join(root, "valid.txt")
	valid := strings.Join([]string{
		"#BPM01:120",
		"#BPM02:180",
		"#00008:0101",
		"#00108:0200",
	}, "\n")
	if err := os.WriteFile(validPath, []byte(valid), 0o644); err != nil {
		t.Fatalf("write valid chart: %v", err)
	}
	parsed, err := parseChartBPM(nil, validPath)
	if err != nil {
		t.Fatalf("parse duplicate BPM chart: %v", err)
	}
	if len(parsed.Events) != 2 || parsed.Events[0].BPM != 120 || parsed.Events[1].BPM != 180 || parsed.Duration <= 0 {
		t.Fatalf("parsed duplicate BPM chart = %+v", parsed)
	}
}
