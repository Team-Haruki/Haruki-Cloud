package music

import (
	"fmt"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type rewardsSnapshotTestSource struct {
	region       renderregion.Value
	musics       map[int]*masterdata.Music
	difficulties map[int][]*masterdata.MusicDifficulty
}

func (s *rewardsSnapshotTestSource) DefaultRegion() renderregion.Value { return s.region }

func (s *rewardsSnapshotTestSource) SearchMusic(string) (*masterdata.Music, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *rewardsSnapshotTestSource) GetMusicByID(id int) (*masterdata.Music, error) {
	item, ok := s.musics[id]
	if !ok {
		return nil, fmt.Errorf("music not found: %d", id)
	}
	cloned := *item
	return &cloned, nil
}

func (s *rewardsSnapshotTestSource) GetMusicByEventID(int) (*masterdata.Music, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *rewardsSnapshotTestSource) GetMusics() []*masterdata.Music {
	result := make([]*masterdata.Music, 0, len(s.musics))
	for _, item := range s.musics {
		cloned := *item
		result = append(result, &cloned)
	}
	return result
}

func (s *rewardsSnapshotTestSource) GetBanEvents(int) []*masterdata.Event { return nil }

func (s *rewardsSnapshotTestSource) GetMusicLocalizedTitles(int) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *rewardsSnapshotTestSource) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	items, ok := s.difficulties[musicID]
	if !ok {
		return nil, fmt.Errorf("music difficulties not found: %d", musicID)
	}
	result := make([]*masterdata.MusicDifficulty, 0, len(items))
	for _, item := range items {
		cloned := *item
		result = append(result, &cloned)
	}
	return result, nil
}

func (s *rewardsSnapshotTestSource) GetMusicVocals(int) ([]*masterdata.MusicVocal, error) {
	return nil, nil
}

func (s *rewardsSnapshotTestSource) GetMusicTags(int) ([]string, error) { return nil, nil }

func (s *rewardsSnapshotTestSource) GetCharacterByID(int) (*masterdata.Character, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *rewardsSnapshotTestSource) GetOutsideCharacterByID(int) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (s *rewardsSnapshotTestSource) GetPrimaryEventByMusicID(int) (*masterdata.Event, error) {
	return nil, nil
}

func (s *rewardsSnapshotTestSource) GetLimitedTimeMusics(int) []*masterdata.LimitedTimeMusic {
	return nil
}

type musicSnapshotStub struct {
	rawValues map[string][]byte
	rawBytes  []byte
}

func (s *musicSnapshotStub) Require() error { return nil }

func (s *musicSnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return nil
}

func (s *musicSnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return nil
}

func (s *musicSnapshotStub) MusicResults(string) map[int]string { return nil }

func (s *musicSnapshotStub) GetMusicResult(int, string) string { return "" }

func (s *musicSnapshotStub) ChallengeLive() *userdata.ChallengeLiveData { return nil }

func (s *musicSnapshotStub) RawBytes() ([]byte, error) {
	return append([]byte(nil), s.rawBytes...), nil
}

func (s *musicSnapshotStub) RawValue(key string) ([]byte, error) {
	value, ok := s.rawValues[key]
	if !ok {
		return nil, fmt.Errorf("raw value %q not found", key)
	}
	return append([]byte(nil), value...), nil
}

func (s *musicSnapshotStub) RawFilePath() string { return "" }

func (s *musicSnapshotStub) RawData() *userdata.RawUserData { return nil }

func (s *musicSnapshotStub) MusicMetaBytes() []byte { return nil }

func (s *musicSnapshotStub) MusicMetaPath() string { return "" }

func TestBuildMusicRewardsDetailRequestFromSnapshotUsesSnapshotAchievements(t *testing.T) {
	source := &rewardsSnapshotTestSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			1001: {
				ID:          1001,
				Title:       "Test Song",
				PublishedAt: time.Now().Add(-time.Hour).UnixMilli(),
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1001: {
				{
					MusicID:         1001,
					MusicDifficulty: "hard",
					PlayLevel:       10,
				},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	snapshot := &musicSnapshotStub{
		rawValues: map[string][]byte{
			"userMusicAchievements": []byte(`[]`),
		},
	}

	payload, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{Region: "jp"}, snapshot)
	if err != nil {
		t.Fatalf("BuildMusicRewardsDetailRequestFromSnapshot failed: %v", err)
	}

	if payload.RankRewards != 110 {
		t.Fatalf("unexpected rank rewards: %d", payload.RankRewards)
	}
	rewards := payload.ComboRewards["hard"]
	if len(rewards) != 1 || rewards[0].Level != 10 || rewards[0].Reward != 50 {
		t.Fatalf("unexpected hard combo rewards: %+v", rewards)
	}
}

func TestBuildMusicRewardsDetailRequestFromSnapshotAcceptsColumnarAchievements(t *testing.T) {
	source := &rewardsSnapshotTestSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			1001: {
				ID:          1001,
				Title:       "Test Song",
				PublishedAt: time.Now().Add(-time.Hour).UnixMilli(),
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1001: {
				{
					MusicID:         1001,
					MusicDifficulty: "hard",
					PlayLevel:       10,
				},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	snapshot := &musicSnapshotStub{
		rawValues: map[string][]byte{
			"userMusicAchievements": []byte(`{
				"musicId": [1001, 1001, 1001, 1001, 1001, 1001, 1001, 1001],
				"musicAchievementId": [1, 2, 3, 4, 13, 14, 15, 16]
			}`),
		},
	}

	payload, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{Region: "jp"}, snapshot)
	if err != nil {
		t.Fatalf("BuildMusicRewardsDetailRequestFromSnapshot failed: %v", err)
	}

	if payload.RankRewards != 0 {
		t.Fatalf("unexpected rank rewards: %d", payload.RankRewards)
	}
	if rewards := payload.ComboRewards["hard"]; len(rewards) != 0 {
		t.Fatalf("unexpected hard combo rewards: %+v", rewards)
	}
}

func TestBuildMusicRewardsDetailRequestFromSnapshotAcceptsCompactAchievements(t *testing.T) {
	source := &rewardsSnapshotTestSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			1001: {
				ID:          1001,
				Title:       "Test Song",
				PublishedAt: time.Now().Add(-time.Hour).UnixMilli(),
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1001: {
				{
					MusicID:         1001,
					MusicDifficulty: "hard",
					PlayLevel:       10,
				},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	snapshot := &musicSnapshotStub{
		rawValues: map[string][]byte{
			"compactUserMusicAchievements": []byte(`{
				"musicId": [1001, 1001, 1001, 1001, 1001, 1001, 1001, 1001],
				"musicAchievementId": [1, 2, 3, 4, 13, 14, 15, 16]
			}`),
		},
	}

	payload, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{Region: "jp"}, snapshot)
	if err != nil {
		t.Fatalf("BuildMusicRewardsDetailRequestFromSnapshot failed: %v", err)
	}

	if payload.RankRewards != 0 {
		t.Fatalf("unexpected rank rewards: %d", payload.RankRewards)
	}
	if rewards := payload.ComboRewards["hard"]; len(rewards) != 0 {
		t.Fatalf("unexpected hard combo rewards: %+v", rewards)
	}
}

func TestBuildMusicRewardsDetailRequestFromSnapshotAcceptsGroupedAchievements(t *testing.T) {
	source := &rewardsSnapshotTestSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			1001: {
				ID:          1001,
				Title:       "Test Song",
				PublishedAt: time.Now().Add(-time.Hour).UnixMilli(),
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1001: {
				{
					MusicID:         1001,
					MusicDifficulty: "hard",
					PlayLevel:       10,
				},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	snapshot := &musicSnapshotStub{
		rawValues: map[string][]byte{
			"userMusicAchievements": []byte(`{
				"1001": [1, 2, 3, 4, 13, 14, 15, 16]
			}`),
		},
	}

	payload, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{Region: "jp"}, snapshot)
	if err != nil {
		t.Fatalf("BuildMusicRewardsDetailRequestFromSnapshot failed: %v", err)
	}

	if payload.RankRewards != 0 {
		t.Fatalf("unexpected rank rewards: %d", payload.RankRewards)
	}
	if rewards := payload.ComboRewards["hard"]; len(rewards) != 0 {
		t.Fatalf("unexpected hard combo rewards: %+v", rewards)
	}
}

func TestBuildMusicRewardsDetailRequestFromSnapshotFindsNestedAchievements(t *testing.T) {
	source := &rewardsSnapshotTestSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			1001: {
				ID:          1001,
				Title:       "Test Song",
				PublishedAt: time.Now().Add(-time.Hour).UnixMilli(),
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1001: {
				{
					MusicID:         1001,
					MusicDifficulty: "hard",
					PlayLevel:       10,
				},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	snapshot := &musicSnapshotStub{
		rawValues: map[string][]byte{},
		rawBytes: []byte(`{
			"updatedResources": {
				"userMusicAchievements": {
					"1001": [1, 2, 3, 4, 13, 14, 15, 16]
				}
			}
		}`),
	}

	payload, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{Region: "jp"}, snapshot)
	if err != nil {
		t.Fatalf("BuildMusicRewardsDetailRequestFromSnapshot failed: %v", err)
	}

	if payload.RankRewards != 0 {
		t.Fatalf("unexpected rank rewards: %d", payload.RankRewards)
	}
	if rewards := payload.ComboRewards["hard"]; len(rewards) != 0 {
		t.Fatalf("unexpected hard combo rewards: %+v", rewards)
	}
}

func TestBuildMusicRewardsDetailRequestFromSnapshotFindsNestedCompactAchievements(t *testing.T) {
	source := &rewardsSnapshotTestSource{
		region: renderregion.JP,
		musics: map[int]*masterdata.Music{
			1001: {
				ID:          1001,
				Title:       "Test Song",
				PublishedAt: time.Now().Add(-time.Hour).UnixMilli(),
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1001: {
				{
					MusicID:         1001,
					MusicDifficulty: "hard",
					PlayLevel:       10,
				},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	snapshot := &musicSnapshotStub{
		rawValues: map[string][]byte{},
		rawBytes: []byte(`{
			"updatedResources": {
				"compactUserMusicAchievements": {
					"musicId": [1001, 1001, 1001, 1001, 1001, 1001, 1001, 1001],
					"musicAchievementId": [1, 2, 3, 4, 13, 14, 15, 16]
				}
			}
		}`),
	}

	payload, err := controller.BuildMusicRewardsDetailRequestFromSnapshot(RewardsDetailQuery{Region: "jp"}, snapshot)
	if err != nil {
		t.Fatalf("BuildMusicRewardsDetailRequestFromSnapshot failed: %v", err)
	}

	if payload.RankRewards != 0 {
		t.Fatalf("unexpected rank rewards: %d", payload.RankRewards)
	}
	if rewards := payload.ComboRewards["hard"]; len(rewards) != 0 {
		t.Fatalf("unexpected hard combo rewards: %+v", rewards)
	}
}
