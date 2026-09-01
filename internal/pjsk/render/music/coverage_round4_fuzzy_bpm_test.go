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
	"haruki-cloud/internal/testutil"
)

func TestMusicFuzzyPrimitiveBranches(t *testing.T) {
	variants := map[rune]rune{
		'達': '达', '戀': '恋', '體': '体', '驗': '验', '華': '华', '離': '离', '鈴': '铃',
		'臺': '台', '彈': '弹', '聲': '声', '夢': '梦', '愛': '爱', '類': '类', '寧': '宁',
		'遙': '遥', '穂': '穗', '絵': '绘', '鏡': '镜', '連': '连', 'x': 'x',
	}
	for input, want := range variants {
		{
			got := normalizeMusicFuzzyVariantRune(input)
			testutil.Check(t, !(got != want), "variant %q = %q, want %q", input, got, want)
		}

	}
	{
		got := normalizeMusicFuzzyText(" Ａ・達！ ")
		testutil.Require(t, !(got != "a达"), "normalized fuzzy text = %q", got)
	}
	{

		got := normalizeMusicFuzzyHanText("A 達-B")
		testutil.Require(t, !(got != "达"), "normalized Han text = %q", got)
	}
	{

		got := normalizeMusicFuzzyWidth("ＡＢＣ")
		testutil.Require(t, !(got != "ABC"), "normalized width = %q", got)
	}

	for length, want := range map[int]int{1: 0, 2: 0, 3: 1, 5: 1, 6: 2, 10: 2, 11: 3} {
		{
			got := fuzzyDistanceLimit(length)
			testutil.Check(t, !(got != want), "distance limit %d = %d, want %d", length, got, want)
		}

	}
	{
		got := levenshteinDistance(nil, []rune("abc"))
		testutil.Require(t, !(got != 3), "empty-left distance = %d", got)
	}
	{

		got := levenshteinDistance([]rune("abc"), nil)
		testutil.Require(t, !(got != 3), "empty-right distance = %d", got)
	}
	{

		got := levenshteinDistance([]rune("kitten"), []rune("sitting"))
		testutil.Require(t, !(got != 3), "levenshtein distance = %d", got)
	}
	{
		testutil.RequireArgs(t, !(minFuzzyInt(1, 2) != 1), "fuzzy min/max/abs helper mismatch")
		testutil.RequireArgs(t, !(minFuzzyInt(2, 1) != 1), "fuzzy min/max/abs helper mismatch")
		testutil.RequireArgs(t, !(maxFuzzyInt(1, 2) != 2), "fuzzy min/max/abs helper mismatch")
		testutil.RequireArgs(t, !(maxFuzzyInt(2, 1) != 2), "fuzzy min/max/abs helper mismatch")
		testutil.RequireArgs(t, !(absInt(-3) != 3), "fuzzy min/max/abs helper mismatch")
		testutil.RequireArgs(t, !(absInt(3) != 3), "fuzzy min/max/abs helper mismatch")
	}
	{

		_, ok := scoreNormalizedMusicFuzzyCandidate("", "abc")
		testutil.RequireArgs(t, !(ok), "empty fuzzy query matched")
	}
	{

		score, ok := scoreNormalizedMusicFuzzyCandidate("abc", "abc")
		{
			testutil.Require(t, ok, "exact score = %+v, %v", score, ok)
			testutil.Require(t, !(score.matchType != 0), "exact score = %+v, %v", score, ok)
		}
	}
	{

		score, ok := scoreNormalizedMusicFuzzyCandidate("abc", "xxabcxx")
		{
			testutil.Require(t, ok, "contains score = %+v, %v", score, ok)
			testutil.Require(t, !(score.matchType != 1), "contains score = %+v, %v", score, ok)
		}
	}
	{

		score, ok := scoreNormalizedMusicFuzzyCandidate("abcd", "xxabxdyy")
		{
			testutil.Require(t, ok, "substring score = %+v, %v", score, ok)
			testutil.Require(t, !(score.matchType != 2), "substring score = %+v, %v", score, ok)
		}
	}
	{

		score, ok := scoreNormalizedMusicFuzzyCandidate("abcd", "abce")
		{
			testutil.Require(t, ok, "distance score = %+v, %v", score, ok)
			testutil.Require(t, !(score.matchType != 3), "distance score = %+v, %v", score, ok)
		}
	}
	{

		_, ok := scoreNormalizedMusicFuzzyCandidate("ab", "zz")
		testutil.RequireArgs(t, !(ok), "distant short candidate matched")
	}
	{

		_, ok := scoreMusicFuzzySubstring([]rune("abc"), []rune("xabcx"))
		testutil.RequireArgs(t, !(ok), "short substring matched")
	}
	{

		_, ok := scoreMusicFuzzySubstring([]rune("abcd"), []rune("zzzzzz"))
		testutil.RequireArgs(t, !(ok), "distant substring matched")
	}
	{

		score, ok := scoreMusicFuzzySubstring([]rune("abcd"), []rune("xxabxdyy"))
		{
			testutil.Require(t, ok, "fuzzy substring = %+v, %v", score, ok)
			testutil.Require(t, !(score.matchType != 2), "fuzzy substring = %+v, %v", score, ok)
		}
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
		testutil.Check(t, !(compareMusicFuzzyScore(tt.a, tt.b) >= 0), "compare scores %+v >= %+v", tt.a, tt.b)

	}
	{
		testutil.RequireArgs(t, !(min3Int(3, 1, 2) != 1), "min3Int mismatch")
		testutil.RequireArgs(t, !(min3Int(3, 4, 2) != 2), "min3Int mismatch")
	}

}

func TestMusicFuzzyResolutionEdgeBranches(t *testing.T) {
	source := newRound4SearchSource()
	{
		_, err := resolveFuzzyMusicQuery(source, " ", false)
		testutil.RequireArgs(t, !(err == nil), "empty fuzzy query error = nil")
	}
	{

		_, err := resolveFuzzyMusicQuery(source, "!!!", false)
		testutil.RequireArgs(t, !(err == nil), "punctuation-only fuzzy query error = nil")
	}
	{

		_, err := resolveFuzzyMusicQuery(source, "missing", true)
		testutil.RequireArgs(t, !(err == nil), "allow-unreleased missing fuzzy query error = nil")
	}

	now := time.Now().UnixMilli()
	source.musics = map[int]*masterdata.Music{
		1: {ID: 1, Title: "Future Match", PublishedAt: now + 100_000},
	}
	if _, err := resolveFuzzyMusicQuery(source, "Future Matc", false); err == nil {
		t.Fatal("unreleased fuzzy match error = nil")
	} else {
		var unreleased *releasecheck.UnreleasedError
		testutil.Require(t, errors.As(err, &unreleased), "unreleased fuzzy error = %T %v", err, err)

	}
	{
		_, err := resolveFuzzyMusicQuery(source, "unrelated", false)
		{
			testutil.Require(t, !(err == nil), "missing fuzzy error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "not found"), "missing fuzzy error = %v", err)
		}
	}

	source.musics = map[int]*masterdata.Music{
		1: {ID: 1, Title: "abcd", PublishedAt: 1},
		2: {ID: 2, Title: "xxabcdy", PublishedAt: 1},
		3: {ID: 3, Title: "abce", PublishedAt: 1},
	}
	got, err := resolveFuzzyMusicQuery(source, "abcd", false)
	{
		testutil.Require(t, !(err != nil), "ranked fuzzy match = %#v, %v", got, err)
		testutil.Require(t, !(got.ID != 1), "ranked fuzzy match = %#v, %v", got, err)
	}
	testutil.RequireArgs(t, !(musicFuzzyCandidates(source, nil) != nil), "nil music fuzzy candidates should be nil")

	source.localizedErr = errors.New("localized unavailable")
	{
		got := musicFuzzyCandidates(source, source.musics[1])
		testutil.Require(t, !(len(got) != 2), "localized error candidates = %#v", got)
	}
	{

		score, ok := scoreMusicFuzzyCandidate("abc达", "xx達")
		{
			testutil.Require(t, ok, "Han fallback score = %+v, %v", score, ok)
			testutil.Require(t, !(score.matchType < 1), "Han fallback score = %+v, %v", score, ok)
		}
	}

}

func TestMusicBPMValidationAndPathHelpers(t *testing.T) {
	{
		_, err := (*Controller)(nil).ResolveMusicCover(Query{})
		testutil.RequireArgs(t, !(err == nil), "nil ResolveMusicCover() error = nil")
	}
	{

		_, err := NewController(nil, nil, nil, nil, nil).ResolveMusicCover(Query{Query: "song", Region: "jp"})
		testutil.RequireArgs(t, !(err == nil), "missing source cover error = nil")
	}

	source := newRound4SearchSource()
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	{
		_, err := controller.ResolveMusicCover(Query{Query: "missing", Region: "jp"})
		{
			testutil.Require(t, !(err == nil), "cover search error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "failed to search"), "cover search error = %v", err)
		}
	}

	source.musics[1] = &masterdata.Music{ID: 1, Title: "No Jacket", PublishedAt: 1}
	{
		_, err := controller.ResolveMusicCover(Query{Query: "No Jacket", Region: "jp"})
		{
			testutil.Require(t, !(err == nil), "missing jacket error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "jacket"), "missing jacket error = %v", err)
		}
	}
	{

		_, err := (*Controller)(nil).FindMusicChartsByBPM(BPMQuery{BPM: 100})
		testutil.RequireArgs(t, !(err == nil), "nil FindMusicChartsByBPM() error = nil")
	}
	{

		_, err := controller.FindMusicChartsByBPM(BPMQuery{BPM: 0})
		testutil.RequireArgs(t, !(err == nil), "zero BPM error = nil")
	}
	{

		_, err := NewController(nil, nil, nil, nil, nil).FindMusicChartsByBPM(BPMQuery{BPM: 100, Region: "jp"})
		testutil.RequireArgs(t, !(err == nil), "missing BPM source error = nil")
	}
	{

		_, err := controller.FindMusicChartsByBPM(BPMQuery{BPM: 123.5, Region: "jp", Difficulty: "expert"})
		{
			testutil.Require(t, !(err == nil), "no BPM matches error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "123.5"), "no BPM matches error = %v", err)
		}
	}
	{

		_, err := (*Controller)(nil).ResolveMusicBPM(Query{})
		testutil.RequireArgs(t, !(err == nil), "nil ResolveMusicBPM() error = nil")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	{
		_, err := controller.WithContext(canceled).ResolveMusicBPM(Query{})
		testutil.Require(t, errors.Is(err, context.Canceled), "canceled ResolveMusicBPM() error = %v", err)
	}
	{

		_, err := NewController(nil, nil, nil, nil, nil).ResolveMusicBPM(Query{Query: "song", Region: "jp"})
		testutil.RequireArgs(t, !(err == nil), "missing BPM builder error = nil")
	}
	{

		_, err := controller.ResolveMusicBPM(Query{Query: "missing", Region: "jp"})
		{
			testutil.Require(t, !(err == nil), "BPM search error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "failed to search"), "BPM search error = %v", err)
		}
	}
	{

		_, err := controller.ResolveMusicBPM(Query{Query: "master No Jacket", Region: "jp"})
		{
			testutil.Require(t, !(err == nil), "missing chart error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "本地谱面"), "missing chart error = %v", err)
		}
	}

	var nilController *Controller
	{
		testutil.RequireArgs(t, !(nilController.resolveLocalMusicJacket("x") != ""), "invalid local jacket lookup was non-empty")
		testutil.RequireArgs(t, !(controller.resolveLocalMusicJacket(" ") != ""), "invalid local jacket lookup was non-empty")
	}
	{
		testutil.RequireArgs(t, !(nilController.resolveLocalChartPath("jp", 1, "expert") != ""), "invalid local chart lookup was non-empty")
		testutil.RequireArgs(t, !(controller.resolveLocalChartPath("jp", 0, "expert") != ""), "invalid local chart lookup was non-empty")
		testutil.RequireArgs(t, !(controller.resolveLocalChartPath("jp", 1, " ") != ""), "invalid local chart lookup was non-empty")
	}
	{

		got := controller.resolveLocalChartPath("", 1, "expert")
		testutil.Require(t, !(got != ""), "missing default-region chart path = %q", got)
	}
	{

		got := controller.collectBPMSearchDifficulties(source, 1, " MASTER ")
		{
			testutil.Require(t, !(len(got) != 1), "preferred BPM difficulties = %#v", got)
			testutil.Require(t, !(got[0] != "master"), "preferred BPM difficulties = %#v", got)
		}
	}

	source.difficultyErr = errors.New("difficulties unavailable")
	{
		got := controller.collectBPMSearchDifficulties(source, 1, "")
		testutil.RequireArgs(t, !(len(got) == 0), "fallback BPM difficulties are empty")
	}

	source.difficultyErr = nil
	source.difficulties = []*masterdata.MusicDifficulty{nil, {MusicDifficulty: "master"}, {MusicDifficulty: "expert"}, {MusicDifficulty: "master"}}
	{
		got := controller.collectBPMSearchDifficulties(source, 1, "")
		{
			testutil.Require(t, !(len(got) != 2), "deduplicated BPM difficulties = %#v", got)
			testutil.Require(t, !(got[0] != "expert"), "deduplicated BPM difficulties = %#v", got)
			testutil.Require(t, !(got[1] != "master"), "deduplicated BPM difficulties = %#v", got)
		}
	}
	{
		testutil.RequireArgs(t, !(buildLookupMusic(nil, NewBuilder(source, nil, nil), renderregion.JP) != nil), "BPM nil/no-match helper mismatch")
		testutil.RequireArgs(t, !(chartContainsBPM(nil, 100)), "BPM nil/no-match helper mismatch")
		testutil.RequireArgs(t, !(chartContainsBPM(&parsedChartBPM{Events: []BPMEvent{{BPM: 120}}}, 100)), "BPM nil/no-match helper mismatch")
	}
	{
		testutil.RequireArgs(t, !(formatLookupBPMValue(120) != "120"), "BPM formatting mismatch")
		testutil.RequireArgs(t, !(formatLookupBPMValue(120.5) != "120.5"), "BPM formatting mismatch")
	}

}

func TestParseChartBPMMalformedAndDuplicateBranches(t *testing.T) {
	{
		_, err := parseChartBPM(nil, filepath.Join(t.TempDir(), "missing.txt"))
		{
			testutil.Require(t, !(err == nil), "missing chart error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "open"), "missing chart error = %v", err)
		}
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	{
		_, err := parseChartBPM(canceled, "missing")
		testutil.Require(t, errors.Is(err, context.Canceled), "canceled chart error = %v", err)
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
	{
		err := os.WriteFile(invalidPath, []byte(invalid), 0o644)
		testutil.Require(t, !(err != nil), "write invalid chart: %v", err)
	}
	{

		_, err := parseChartBPM(nil, invalidPath)
		{
			testutil.Require(t, !(err == nil), "invalid BPM chart error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "没有可用"), "invalid BPM chart error = %v", err)
		}
	}

	validPath := filepath.Join(root, "valid.txt")
	valid := strings.Join([]string{
		"#BPM01:120",
		"#BPM02:180",
		"#00008:0101",
		"#00108:0200",
	}, "\n")
	{
		err := os.WriteFile(validPath, []byte(valid), 0o644)
		testutil.Require(t, !(err != nil), "write valid chart: %v", err)
	}

	parsed, err := parseChartBPM(nil, validPath)
	testutil.Require(t, !(err != nil), "parse duplicate BPM chart: %v", err)
	{
		testutil.Require(t, !(len(parsed.Events) != 2), "parsed duplicate BPM chart = %+v", parsed)
		testutil.Require(t, !(parsed.Events[0].BPM != 120), "parsed duplicate BPM chart = %+v", parsed)
		testutil.Require(t, !(parsed.Events[1].BPM != 180), "parsed duplicate BPM chart = %+v", parsed)
		testutil.Require(t, !(parsed.Duration <= 0), "parsed duplicate BPM chart = %+v", parsed)
	}

}
