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
	if err != nil {
		t.Fatalf("BuildMusicRewardsBasicEstimateRequest() error = %v", err)
	}
	if payload.RankRewards != "0 (110×0)" {
		t.Fatalf("rank estimate = %q", payload.RankRewards)
	}
	wantCombo := map[string]string{
		"hard": "100 (50×2)", "expert": "210 (70×3)",
		"master": "210 (70×3)", "append": "0 (15×0)",
	}
	if !reflect.DeepEqual(payload.ComboRewards, wantCombo) {
		t.Fatalf("combo estimate = %#v, want %#v", payload.ComboRewards, wantCombo)
	}
	if payload.Profile.ErrorMessage == nil || !strings.Contains(*payload.Profile.ErrorMessage, "Suite unavailable\n") {
		t.Fatalf("estimate message = %+v", payload.Profile.ErrorMessage)
	}

	defaultMessage, err := controller.BuildMusicRewardsBasicEstimateRequest(
		RewardsBasicQuery{Region: "jp"},
		[]sekai.AnotherUserMusicDifficultyClearCount{{MusicDifficultyType: sekai.MusicDifficultyAppend, FullCombo: -2}},
		"",
	)
	if err != nil {
		t.Fatalf("default-message estimate error = %v", err)
	}
	if defaultMessage.Profile.ErrorMessage == nil || !strings.Contains(*defaultMessage.Profile.ErrorMessage, "当前未使用") {
		t.Fatalf("default estimate message = %+v", defaultMessage.Profile.ErrorMessage)
	}
	if defaultMessage.ComboRewards["append"] != "45 (15×3)" {
		t.Fatalf("negative full-combo estimate = %q", defaultMessage.ComboRewards["append"])
	}
}

func TestRewardsEligibilityAndFormattingHelpers(t *testing.T) {
	controller, source := buildRewardsCoverageController(t)
	_, _, builder, err := controller.resolveBuilder("jp")
	if err != nil {
		t.Fatal(err)
	}
	valid := controller.validRewardMusicIDs(renderregion.JP, source, builder)
	if len(valid) != 3 {
		t.Fatalf("valid reward musics = %v", valid)
	}
	now := time.Now().UnixMilli()
	if !musicRewardAvailableNow(source, 1, now, renderregion.JP) || musicRewardAvailableNow(source, 4, now, renderregion.JP) || !musicRewardAvailableNow(source, 5, now, renderregion.JP) {
		t.Fatal("limited-time reward availability mismatch")
	}
	if musicRewardAvailableNow(&rewardsCoverageSource{rewardsSnapshotTestSource: source.rewardsSnapshotTestSource, limited: map[int][]*masterdata.LimitedTimeMusic{9: {nil}}}, 9, now, renderregion.JP) {
		t.Fatal("nil-only limited reward unexpectedly available")
	}

	info := &drawing.DifficultyInfo{Order: []string{"hard", "master"}, Level: []int{15}}
	if difficultyLevelFromInfo(nil, "hard") != 0 || difficultyLevelFromInfo(info, "master") != 0 || difficultyLevelFromInfo(info, "hard") != 15 || difficultyLevelFromInfo(info, " HARD ") != 0 {
		t.Fatal("difficulty lookup branches failed")
	}
	if missingComboRewardTotal("hard", map[int]struct{}{16: {}}) != 0 {
		t.Fatal("completed hard combo reward remains")
	}
	if missingComboRewardTotal("append", nil) != 15 {
		t.Fatal("append shard reward total mismatch")
	}
	if got := sortedRewardLevels(map[int]int{30: 0, 29: 10, 28: -1, 31: 20}); !reflect.DeepEqual(got, []int{29, 31}) {
		t.Fatalf("sorted reward levels = %v", got)
	}
	if formatEstimatedReward(10, 3) != "30 (10×3)" || formatEstimatedReward(10, -1) != "0 (10×0)" {
		t.Fatal("reward estimate formatting mismatch")
	}
}

func TestRewardsDetailAchievementAndSnapshotErrors(t *testing.T) {
	controller, _ := buildRewardsCoverageController(t)
	if _, err := controller.BuildMusicRewardsDetailRequestFromAchievements(RewardsDetailQuery{Region: "jp"}, []byte(`{`)); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("invalid achievements error = %v", err)
	}
	payload, err := controller.BuildMusicRewardsDetailRequestFromAchievements(
		RewardsDetailQuery{Region: "jp"},
		[]byte(`[{"musicId":1,"musicAchievementId":1},{"musicId":1,"musicAchievementId":25},{"musicId":999,"musicAchievementId":1}]`),
	)
	if err != nil {
		t.Fatalf("detail rewards error = %v", err)
	}
	if payload.RankRewards <= 0 || len(payload.ComboRewards["append"]) != 1 || payload.ComboRewards["append"][0].Reward != 15 {
		t.Fatalf("detail rewards = %+v", payload)
	}

	emptySource := &rewardsSnapshotTestSource{region: renderregion.JP, musics: map[int]*masterdata.Music{}, difficulties: map[int][]*masterdata.MusicDifficulty{}}
	emptyController := NewController(emptySource, nil, assets.NewAssetHelper("", nil), nil, nil)
	if _, err := emptyController.BuildMusicRewardsBasicEstimateRequest(RewardsBasicQuery{Region: "jp"}, nil, ""); err == nil {
		t.Fatal("empty basic estimate succeeded")
	}
	if _, err := emptyController.BuildMusicRewardsDetailRequestFromAchievements(RewardsDetailQuery{Region: "jp"}, []byte(`[]`)); err == nil {
		t.Fatal("empty detail rewards succeeded")
	}

	if _, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{}, nil); err == nil {
		t.Fatal("nil snapshot succeeded")
	}
	requireErr := errors.New("snapshot unavailable")
	if _, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{}, &failingMusicSnapshot{musicSnapshotStub: &musicSnapshotStub{}, requireErr: requireErr}); !errors.Is(err, requireErr) {
		t.Fatalf("snapshot Require error = %v", err)
	}
	rawErr := errors.New("raw unavailable")
	if _, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{}, &failingMusicSnapshot{musicSnapshotStub: &musicSnapshotStub{}, rawBytesErr: rawErr}); !errors.Is(err, rawErr) {
		t.Fatalf("snapshot RawBytes error = %v", err)
	}
	for _, raw := range [][]byte{[]byte(`{`), []byte(`{"other":[]}`)} {
		if _, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{}, &musicSnapshotStub{rawValues: map[string][]byte{}, rawBytes: raw}); err == nil {
			t.Fatalf("invalid nested snapshot %q succeeded", raw)
		}
	}
}

func TestAchievementDecoderHelperBranches(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte(`null`), []byte(` [] `)} {
		items, err := decodeUserMusicAchievements(raw)
		if err != nil || len(items) != 0 {
			t.Fatalf("empty achievements %q = %+v, %v", raw, items, err)
		}
	}
	if _, err := decodeUserMusicAchievements([]byte(`not-json`)); err == nil {
		t.Fatal("malformed achievements succeeded")
	}
	if _, err := decodeUserMusicAchievements([]byte(`{"unsupported":true}`)); err == nil {
		t.Fatal("unsupported achievements succeeded")
	}
	items, err := decodeUserMusicAchievements([]byte(`{"nested":[{"music_id":"5","music-achievement-id":6},null,{"5":[7,0,"8"]}]}`))
	if err != nil || len(items) != 3 {
		t.Fatalf("nested achievements = %+v, %v", items, err)
	}

	if got := collectUserMusicAchievements(true); got != nil {
		t.Fatalf("scalar achievements = %+v", got)
	}
	if _, ok := parseAchievementItemMap(map[string]any{"musicId": 0, "musicAchievementId": 1}); ok {
		t.Fatal("invalid item map succeeded")
	}
	if _, ok := parseAchievementColumnsMap(map[string]any{"musicId": []any{1}, "musicAchievementId": []any{1, 2}}); ok {
		t.Fatal("mismatched columns succeeded")
	}
	if _, ok := parseAchievementColumnsMap(map[string]any{"musicId": []any{"bad"}, "musicAchievementId": []any{1}}); ok {
		t.Fatal("invalid columns succeeded")
	}
	if _, ok := parseAchievementGroupedMap(map[string]any{"bad": []any{1}, "0": []any{1}, "1": true}); ok {
		t.Fatal("invalid grouped map succeeded")
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
		if got != tc.want || ok != tc.ok {
			t.Fatalf("achievement int %#v = %d, %v", tc.value, got, ok)
		}
	}
	if got, ok := toAchievementIntSlice(7); !ok || !reflect.DeepEqual(got, []int{7}) {
		t.Fatalf("scalar int slice = %v, %v", got, ok)
	}
	if _, ok := toAchievementIntSlice([]any{1, true}); ok {
		t.Fatal("invalid int slice succeeded")
	}
	if _, ok := findNestedJSONValue([]any{map[string]any{"x": 1}}, "missing"); ok {
		t.Fatal("missing nested value found")
	}
	if _, ok := findNestedJSONValue(true, "missing"); ok {
		t.Fatal("scalar nested value found")
	}
}

var _ snapshot.Snapshot = (*failingMusicSnapshot)(nil)
