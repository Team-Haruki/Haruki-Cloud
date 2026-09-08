package music

import (
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"testing"
	"time"
)

type countedMusicListSource struct {
	*lookupTestSource
	allCalls      int
	difficultyIDs []int
	preloads      int
}

func (s *countedMusicListSource) PreloadMusicDifficulties() error {
	s.preloads++
	return nil
}

func (s *countedMusicListSource) GetMusics() []*masterdata.Music {
	s.allCalls++
	return s.lookupTestSource.GetMusics()
}
func (s *countedMusicListSource) GetMusicDifficulties(id int) ([]*masterdata.MusicDifficulty, error) {
	s.difficultyIDs = append(s.difficultyIDs, id)
	return s.lookupTestSource.GetMusicDifficulties(id)
}

func TestMusicListSkipsUnrelatedDifficultyLookups(t *testing.T) {
	now := time.Now().UnixMilli()
	for _, byID := range []bool{true, false} {
		source := &countedMusicListSource{lookupTestSource: &lookupTestSource{
			musics:       map[int]*masterdata.Music{1: {ID: 1, Title: "Target", PublishedAt: now - 1000}, 2: {ID: 2, Title: "Other", PublishedAt: now - 1000}, 3: {ID: 3, Title: "Future", PublishedAt: now + 3600000}},
			difficulties: map[int][]*masterdata.MusicDifficulty{1: {{MusicDifficulty: "master", PlayLevel: 30}}, 2: {{MusicDifficulty: "master", PlayLevel: 31}}, 3: {{MusicDifficulty: "master", PlayLevel: 32}}},
		}}
		var id *int
		keyword := "target"
		if byID {
			id = new(1)
			keyword = ""
		}
		entries, _ := buildFilteredMusicListEntries(source, NewBuilder(source, nil, nil), renderregion.JP, musicListBuildOptions{difficulty: "master"}, id, keyword)
		if len(entries) != 1 || entries[0]["id"] != 1 {
			t.Fatalf("byID=%v entries=%v", byID, entries)
		}
		if len(source.difficultyIDs) != 1 || source.difficultyIDs[0] != 1 {
			t.Fatalf("byID=%v queried difficulties=%v", byID, source.difficultyIDs)
		}
		if byID && source.allCalls != 0 {
			t.Fatalf("ID lookup loaded full music list %d times", source.allCalls)
		}
	}
}

func TestMusicListPreloadsOnlyForFullList(t *testing.T) {
	for _, keyword := range []string{"", "target"} {
		source := &countedMusicListSource{lookupTestSource: &lookupTestSource{
			musics:       map[int]*masterdata.Music{1: {ID: 1, Title: "Target one"}, 2: {ID: 2, Title: "Target two"}},
			difficulties: map[int][]*masterdata.MusicDifficulty{1: {{MusicDifficulty: "master", PlayLevel: 30}}, 2: {{MusicDifficulty: "master", PlayLevel: 31}}},
		}}
		entries, _ := buildFilteredMusicListEntries(source, NewBuilder(source, nil, nil), renderregion.JP, musicListBuildOptions{difficulty: "master", includeLeaks: true}, nil, keyword)
		wantPreloads := 0
		if keyword == "" {
			wantPreloads = 1
		}
		if len(entries) != 2 || source.preloads != wantPreloads {
			t.Fatalf("keyword=%q entries=%d preloads=%d want=%d", keyword, len(entries), source.preloads, wantPreloads)
		}
	}
}

type orderedRewardDifficultySource struct {
	*countedMusicListSource
	t *testing.T
}

func (s *orderedRewardDifficultySource) GetMusicDifficulties(id int) ([]*masterdata.MusicDifficulty, error) {
	if s.preloads != 1 {
		s.t.Errorf("difficulty read before regional preload: id=%d preloads=%d", id, s.preloads)
	}
	return s.countedMusicListSource.GetMusicDifficulties(id)
}

func TestRewardsPreloadBeforeEligibilityDifficultyReads(t *testing.T) {
	source := &orderedRewardDifficultySource{t: t, countedMusicListSource: &countedMusicListSource{lookupTestSource: &lookupTestSource{
		musics:       map[int]*masterdata.Music{1: {ID: 1, Title: "One"}, 2: {ID: 2, Title: "Two"}},
		difficulties: map[int][]*masterdata.MusicDifficulty{1: {{MusicDifficulty: "master", PlayLevel: 30}}, 2: {{MusicDifficulty: "master", PlayLevel: 31}}},
	}}}
	controller := NewController(source, nil, nil, nil, nil)
	payload, err := controller.BuildMusicRewardsDetailRequestFromAchievements(RewardsDetailQuery{Region: "jp"}, []byte(`[]`))
	if err != nil || payload == nil {
		t.Fatalf("payload=%v err=%v", payload, err)
	}
	if source.preloads != 1 {
		t.Fatalf("preloads=%d want 1", source.preloads)
	}
}
