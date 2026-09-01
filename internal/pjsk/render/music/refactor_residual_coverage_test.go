package music

import (
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestMusicRefactorResidualBranches(t *testing.T) {
	withoutSource := NewController(nil, nil, nil, nil, nil)
	if _, err := withoutSource.BuildMusicRewardsDetailRequestFromAchievements(RewardsDetailQuery{Region: "jp"}, []byte("[]")); err == nil {
		t.Fatal("detail rewards without source succeeded")
	}
	if _, err := withoutSource.BuildMusicRewardsBasicEstimateRequest(RewardsBasicQuery{Region: "jp"}, nil, ""); err == nil {
		t.Fatal("basic rewards without source succeeded")
	}

	now := time.Now().UnixMilli()
	source := newRound4SearchSource()
	musicInfo := &masterdata.Music{ID: 1, Title: "Visible", AssetBundleName: "visible", PublishedAt: now - 1}
	source.musics[1] = musicInfo
	source.allMusics = []*masterdata.Music{musicInfo}
	source.difficulties = []*masterdata.MusicDifficulty{{MusicID: 1, MusicDifficulty: "master", PlayLevel: 30}}
	builder := NewBuilder(source, nil, nil)
	entry, _, _, ok := buildMusicListItemEntry(source, builder, renderregion.JP, "master", ListItemQuery{MusicID: 1}, false, now)
	if !ok || entry["difficulty_type"] != "master" {
		t.Fatalf("default-difficulty list entry = %#v, %v", entry, ok)
	}

	source.difficulties = nil
	controller := NewController(source, nil, nil, nil, nil)
	if valid := controller.validRewardMusicIDs(renderregion.JP, source, builder); len(valid) != 0 {
		t.Fatalf("music without difficulty remained reward-eligible: %v", valid)
	}
}
