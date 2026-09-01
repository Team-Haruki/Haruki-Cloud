package music

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
)

type rewardsCoverageSource struct {
	*rewardsSnapshotTestSource
	limited map[int][]*masterdata.LimitedTimeMusic
}

func (s *rewardsCoverageSource) GetMusics() []*masterdata.Music {
	result := make([]*masterdata.Music, 0, len(s.musics))
	for _, item := range s.musics {
		if item == nil {
			result = append(result, nil)
			continue
		}
		result = append(result, new(*item))
	}
	return result
}

func (s *rewardsCoverageSource) GetLimitedTimeMusics(id int) []*masterdata.LimitedTimeMusic {
	return append([]*masterdata.LimitedTimeMusic(nil), s.limited[id]...)
}

type failingMusicSnapshot struct {
	*musicSnapshotStub
	requireErr  error
	rawBytesErr error
}

func (s *failingMusicSnapshot) Require() error { return s.requireErr }

func (s *failingMusicSnapshot) RawBytes() ([]byte, error) {
	if s.rawBytesErr != nil {
		return nil, s.rawBytesErr
	}
	return s.musicSnapshotStub.RawBytes()
}

func buildRewardsCoverageController(t *testing.T) (*Controller, *rewardsCoverageSource) {
	t.Helper()
	now := time.Now().UnixMilli()
	source := &rewardsCoverageSource{
		rewardsSnapshotTestSource: &rewardsSnapshotTestSource{
			region: renderregion.JP,
			musics: map[int]*masterdata.Music{
				1:   {ID: 1, Title: "All difficulties", PublishedAt: now - 1},
				2:   {ID: 2, Title: "Hard only", PublishedAt: now - 1},
				3:   {ID: 3, Title: "Future", PublishedAt: now + 60_000},
				4:   {ID: 4, Title: "Expired limited", PublishedAt: now - 1},
				5:   {ID: 5, Title: "Current limited", PublishedAt: now - 1},
				6:   {ID: 6, Title: "Missing difficulties", PublishedAt: now - 1},
				7:   {ID: 7, Title: "Zero difficulties", PublishedAt: now - 1},
				241: {ID: 241, Title: "Hidden", PublishedAt: now - 1},
				8:   nil,
			},
			difficulties: map[int][]*masterdata.MusicDifficulty{
				1: {
					{MusicID: 1, MusicDifficulty: "easy", PlayLevel: 5},
					{MusicID: 1, MusicDifficulty: "hard", PlayLevel: 15},
					{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 25},
					{MusicID: 1, MusicDifficulty: "master", PlayLevel: 30},
					{MusicID: 1, MusicDifficulty: "append", PlayLevel: 35},
				},
				2:   {{MusicID: 2, MusicDifficulty: "hard", PlayLevel: 16}},
				4:   {{MusicID: 4, MusicDifficulty: "hard", PlayLevel: 17}},
				5:   {{MusicID: 5, MusicDifficulty: "master", PlayLevel: 31}},
				7:   {{MusicID: 7, MusicDifficulty: "hard", PlayLevel: 0}},
				241: {{MusicID: 241, MusicDifficulty: "master", PlayLevel: 99}},
			},
		},
		limited: map[int][]*masterdata.LimitedTimeMusic{
			4: {nil, {MusicID: 4, StartAt: now - 1000, EndAt: now - 1}},
			5: {{MusicID: 5, StartAt: now - 1000, EndAt: now + 1000}},
		},
	}
	return NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil), source
}

func TestBuildMusicRewardsBasicEstimateCoversEligibilityAndClamping(t *testing.T) {
	controller, _ := buildRewardsCoverageController(t)
	payload, err := controller.BuildMusicRewardsBasicEstimateRequest(
		RewardsBasicQuery{Region: "jp"},
		[]sekai.AnotherUserMusicDifficultyClearCount{
			{MusicDifficultyType: sekai.MusicDifficultyHard, LiveClear: 99, FullCombo: 1},
			{MusicDifficultyType: sekai.MusicDifficultyAppend, FullCombo: 99},
			{MusicDifficultyType: "  ", LiveClear: 99, FullCombo: 99},
		},
		"Suite unavailable",
	)
	testutil.Require(t, !(err != nil), "BuildMusicRewardsBasicEstimateRequest() error = %v", err)
	testutil.Require(t, !(payload.RankRewards != "0 (110×0)"), "rank estimate = %q", payload.RankRewards)

	wantCombo := map[string]string{
		"hard": "100 (50×2)", "expert": "210 (70×3)",
		"master": "210 (70×3)", "append": "0 (15×0)",
	}
	testutil.Require(t, reflect.DeepEqual(payload.ComboRewards, wantCombo), "combo estimate = %#v, want %#v", payload.ComboRewards, wantCombo)
	{
		testutil.Require(t, !(payload.Profile.ErrorMessage == nil), "estimate message = %+v", payload.Profile.ErrorMessage)
		testutil.Require(t, strings.Contains(*payload.Profile.ErrorMessage, "Suite unavailable\n"), "estimate message = %+v", payload.Profile.ErrorMessage)
	}

	defaultMessage, err := controller.BuildMusicRewardsBasicEstimateRequest(
		RewardsBasicQuery{Region: "jp"},
		[]sekai.AnotherUserMusicDifficultyClearCount{{MusicDifficultyType: sekai.MusicDifficultyAppend, FullCombo: -2}},
		"",
	)
	testutil.Require(t, !(err != nil), "default-message estimate error = %v", err)
	{
		testutil.Require(t, !(defaultMessage.Profile.ErrorMessage == nil), "default estimate message = %+v", defaultMessage.Profile.ErrorMessage)
		testutil.Require(t, strings.Contains(*defaultMessage.Profile.ErrorMessage, "当前未使用"), "default estimate message = %+v", defaultMessage.Profile.ErrorMessage)
	}
	testutil.Require(t, !(defaultMessage.ComboRewards["append"] != "45 (15×3)"), "negative full-combo estimate = %q", defaultMessage.ComboRewards["append"])

}

func TestRewardsEligibilityAndFormattingHelpers(t *testing.T) {
	controller, source := buildRewardsCoverageController(t)
	_, _, builder, err := controller.resolveBuilder("jp")
	testutil.RequireArgs(t, !(err != nil), err)

	valid := controller.validRewardMusicIDs(renderregion.JP, source, builder)
	testutil.Require(t, !(len(valid) != 3), "valid reward musics = %v", valid)

	now := time.Now().UnixMilli()
	{
		testutil.RequireArgs(t, musicRewardAvailableNow(source, 1, now, renderregion.JP), "limited-time reward availability mismatch")
		testutil.RequireArgs(t, !(musicRewardAvailableNow(source, 4, now, renderregion.JP)), "limited-time reward availability mismatch")
		testutil.RequireArgs(t, musicRewardAvailableNow(source, 5, now, renderregion.JP), "limited-time reward availability mismatch")
	}
	testutil.RequireArgs(t, !(musicRewardAvailableNow(&rewardsCoverageSource{rewardsSnapshotTestSource: source.rewardsSnapshotTestSource, limited: map[int][]*masterdata.LimitedTimeMusic{9: {nil}}}, 9, now, renderregion.JP)), "nil-only limited reward unexpectedly available")

	info := &drawing.DifficultyInfo{Order: []string{"hard", "master"}, Level: []int{15}}
	{
		testutil.RequireArgs(t, !(difficultyLevelFromInfo(nil, "hard") != 0), "difficulty lookup branches failed")
		testutil.RequireArgs(t, !(difficultyLevelFromInfo(info, "master") != 0), "difficulty lookup branches failed")
		testutil.RequireArgs(t, !(difficultyLevelFromInfo(info, "hard") != 15), "difficulty lookup branches failed")
		testutil.RequireArgs(t, !(difficultyLevelFromInfo(info, " HARD ") != 0), "difficulty lookup branches failed")
	}
	testutil.RequireArgs(t, !(missingComboRewardTotal("hard", map[int]struct{}{16: {}}) != 0), "completed hard combo reward remains")
	testutil.RequireArgs(t, !(missingComboRewardTotal("append", nil) != 15), "append shard reward total mismatch")
	{

		got := sortedRewardLevels(map[int]int{30: 0, 29: 10, 28: -1, 31: 20})
		testutil.Require(t, reflect.DeepEqual(got, []int{29, 31}), "sorted reward levels = %v", got)
	}
	{
		testutil.RequireArgs(t, !(formatEstimatedReward(10, 3) != "30 (10×3)"), "reward estimate formatting mismatch")
		testutil.RequireArgs(t, !(formatEstimatedReward(10, -1) != "0 (10×0)"), "reward estimate formatting mismatch")
	}

}

func TestRewardsDetailAchievementAndSnapshotErrors(t *testing.T) {
	controller, _ := buildRewardsCoverageController(t)
	{
		_, err := controller.BuildMusicRewardsDetailRequestFromAchievements(RewardsDetailQuery{Region: "jp"}, []byte(`{`))
		{
			testutil.Require(t, !(err == nil), "invalid achievements error = %v", err)
			testutil.Require(t, strings.Contains(err.Error(), "decode"), "invalid achievements error = %v", err)
		}
	}

	payload, err := controller.BuildMusicRewardsDetailRequestFromAchievements(
		RewardsDetailQuery{Region: "jp"},
		[]byte(`[{"musicId":1,"musicAchievementId":1},{"musicId":1,"musicAchievementId":25},{"musicId":999,"musicAchievementId":1}]`),
	)
	testutil.Require(t, !(err != nil), "detail rewards error = %v", err)
	{
		testutil.Require(t, !(payload.RankRewards <= 0), "detail rewards = %+v", payload)
		testutil.Require(t, !(len(payload.ComboRewards["append"]) != 1), "detail rewards = %+v", payload)
		testutil.Require(t, !(payload.ComboRewards["append"][0].Reward != 15), "detail rewards = %+v", payload)
	}

	emptySource := &rewardsSnapshotTestSource{region: renderregion.JP, musics: map[int]*masterdata.Music{}, difficulties: map[int][]*masterdata.MusicDifficulty{}}
	emptyController := NewController(emptySource, nil, assets.NewAssetHelper("", nil), nil, nil)
	{
		_, err := emptyController.BuildMusicRewardsBasicEstimateRequest(RewardsBasicQuery{Region: "jp"}, nil, "")
		testutil.RequireArgs(t, !(err == nil), "empty basic estimate succeeded")
	}
	{

		_, err := emptyController.BuildMusicRewardsDetailRequestFromAchievements(RewardsDetailQuery{Region: "jp"}, []byte(`[]`))
		testutil.RequireArgs(t, !(err == nil), "empty detail rewards succeeded")
	}
	{

		_, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{}, nil)
		testutil.RequireArgs(t, !(err == nil), "nil snapshot succeeded")
	}

	requireErr := errors.New("snapshot unavailable")
	{
		_, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{}, &failingMusicSnapshot{musicSnapshotStub: &musicSnapshotStub{}, requireErr: requireErr})
		testutil.Require(t, errors.Is(err, requireErr), "snapshot Require error = %v", err)
	}

	rawErr := errors.New("raw unavailable")
	{
		_, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{}, &failingMusicSnapshot{musicSnapshotStub: &musicSnapshotStub{}, rawBytesErr: rawErr})
		testutil.Require(t, errors.Is(err, rawErr), "snapshot RawBytes error = %v", err)
	}

	for _, raw := range [][]byte{[]byte(`{`), []byte(`{"other":[]}`)} {
		{
			_, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{}, &musicSnapshotStub{rawValues: map[string][]byte{}, rawBytes: raw})
			testutil.Require(t, !(err == nil), "invalid nested snapshot %q succeeded", raw)
		}

	}
}

func TestAchievementDecoderHelperBranches(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte(`null`), []byte(` [] `)} {
		items, err := decodeUserMusicAchievements(raw)
		{
			testutil.Require(t, !(err != nil), "empty achievements %q = %+v, %v", raw, items, err)
			testutil.Require(t, !(len(items) != 0), "empty achievements %q = %+v, %v", raw, items, err)
		}

	}
	{
		_, err := decodeUserMusicAchievements([]byte(`not-json`))
		testutil.RequireArgs(t, !(err == nil), "malformed achievements succeeded")
	}
	{

		_, err := decodeUserMusicAchievements([]byte(`{"unsupported":true}`))
		testutil.RequireArgs(t, !(err == nil), "unsupported achievements succeeded")
	}

	items, err := decodeUserMusicAchievements([]byte(`{"nested":[{"music_id":"5","music-achievement-id":6},null,{"5":[7,0,"8"]}]}`))
	{
		testutil.Require(t, !(err != nil), "nested achievements = %+v, %v", items, err)
		testutil.Require(t, !(len(items) != 3), "nested achievements = %+v, %v", items, err)
	}
	{

		got := collectUserMusicAchievements(true)
		testutil.Require(t, !(got != nil), "scalar achievements = %+v", got)
	}
	{

		_, ok := parseAchievementItemMap(map[string]any{"musicId": 0, "musicAchievementId": 1})
		testutil.RequireArgs(t, !(ok), "invalid item map succeeded")
	}
	{

		_, ok := parseAchievementColumnsMap(map[string]any{"musicId": []any{1}, "musicAchievementId": []any{1, 2}})
		testutil.RequireArgs(t, !(ok), "mismatched columns succeeded")
	}
	{

		_, ok := parseAchievementColumnsMap(map[string]any{"musicId": []any{"bad"}, "musicAchievementId": []any{1}})
		testutil.RequireArgs(t, !(ok), "invalid columns succeeded")
	}
	{

		_, ok := parseAchievementGroupedMap(map[string]any{"bad": []any{1}, "0": []any{1}, "1": true})
		testutil.RequireArgs(t, !(ok), "invalid grouped map succeeded")
	}

	numbers := []struct {
		value any
		want  int
		ok    bool
	}{
		{int(1), 1, true}, {int64(2), 2, true}, {float64(3), 3, true},
		{json.Number("4"), 4, true}, {" 5 ", 5, true}, {json.Number("bad"), 0, false}, {"bad", 0, false}, {true, 0, false},
	}
	for _, tc := range numbers {
		got, ok := toAchievementInt(tc.value)
		{
			testutil.Require(t, !(got != tc.want), "achievement int %#v = %d, %v", tc.value, got, ok)
			testutil.Require(t, !(ok != tc.ok), "achievement int %#v = %d, %v", tc.value, got, ok)
		}

	}
	{
		got, ok := toAchievementIntSlice(7)
		{
			testutil.Require(t, ok, "scalar int slice = %v, %v", got, ok)
			testutil.Require(t, reflect.DeepEqual(got, []int{7}), "scalar int slice = %v, %v", got, ok)
		}
	}
	{

		_, ok := toAchievementIntSlice([]any{1, true})
		testutil.RequireArgs(t, !(ok), "invalid int slice succeeded")
	}
	{

		_, ok := findNestedJSONValue([]any{map[string]any{"x": 1}}, "missing")
		testutil.RequireArgs(t, !(ok), "missing nested value found")
	}
	{

		_, ok := findNestedJSONValue(true, "missing")
		testutil.RequireArgs(t, !(ok), "scalar nested value found")
	}

}

var _ snapshot.Snapshot = (*failingMusicSnapshot)(nil)
