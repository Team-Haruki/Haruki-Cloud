package music

import (
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

func TestBuildMusicProgressRequestUsesQueryUserResults(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1:   {ID: 1, Title: "Song A", PublishedAt: now - 1000},
			2:   {ID: 2, Title: "Song B", PublishedAt: now - 1000},
			3:   {ID: 3, Title: "Song C", PublishedAt: now + 100000},
			4:   {ID: 4, Title: "Song D", PublishedAt: now - 1000},
			241: {ID: 241, Title: "Hidden Song", PublishedAt: now - 1000},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 26},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "expert", PlayLevel: 26},
			},
			3: {
				{MusicID: 3, MusicDifficulty: "expert", PlayLevel: 26},
			},
			4: {
				{MusicID: 4, MusicDifficulty: "expert", PlayLevel: 27},
			},
			241: {
				{MusicID: 241, MusicDifficulty: "expert", PlayLevel: 26},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	req, err := controller.BuildMusicProgressRequest(ProgressQuery{
		Region:     "jp",
		Difficulty: "expert",
		UserResults: map[int]string{
			1:   "clear",
			2:   "ap",
			3:   "fc",
			241: "ap",
		},
	})
	if err != nil {
		t.Fatalf("BuildMusicProgressRequest() error = %v", err)
	}
	if req.Difficulty != "expert" {
		t.Fatalf("unexpected difficulty: %q", req.Difficulty)
	}
	if len(req.Counts) != 2 {
		t.Fatalf("expected 2 level groups, got %+v", req.Counts)
	}

	if req.Counts[0].Level != 26 || req.Counts[0].Total != 2 || req.Counts[0].Clear != 2 || req.Counts[0].Fc != 1 || req.Counts[0].Ap != 1 {
		t.Fatalf("unexpected level 26 counts: %+v", req.Counts[0])
	}
	if req.Counts[1].Level != 27 || req.Counts[1].Total != 1 || req.Counts[1].NotClear != 1 || req.Counts[1].Clear != 0 {
		t.Fatalf("unexpected level 27 counts: %+v", req.Counts[1])
	}
}

func TestBuildMusicProgressRequestUsesStaticPlaceholderLeaderImage(t *testing.T) {
	controller := NewController(&lookupTestSource{
		musics:       map[int]*masterdata.Music{},
		difficulties: map[int][]*masterdata.MusicDifficulty{},
	}, nil, assets.NewAssetHelper("", nil), nil, nil)

	req, err := controller.BuildMusicProgressRequest(ProgressQuery{Region: "jp"})
	if err != nil {
		t.Fatalf("BuildMusicProgressRequest() error = %v", err)
	}
	if req.Profile.Profile == nil {
		t.Fatalf("expected placeholder profile")
	}
	if req.Profile.Profile.LeaderImagePath != "static_images/unknown.jpg" {
		t.Fatalf("unexpected placeholder leader path: %q", req.Profile.Profile.LeaderImagePath)
	}
}

type progressSnapshotStub struct {
	profile *drawing.ProfileCardRequest
	results map[string]map[int]string
}

type regionalLookupTestSource struct {
	*lookupTestSource
	region renderregion.Value
}

func (s *progressSnapshotStub) Require() error { return nil }

func (s *regionalLookupTestSource) DefaultRegion() renderregion.Value {
	if s == nil {
		return renderregion.JP
	}
	return s.region
}

func (s *progressSnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return nil
}

func (s *progressSnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return s.profile
}

func (s *progressSnapshotStub) MusicResults(diff string) map[int]string {
	source := s.results[diff]
	out := make(map[int]string, len(source))
	for musicID, result := range source {
		out[musicID] = result
	}
	return out
}

func (s *progressSnapshotStub) GetMusicResult(musicID int, diff string) string {
	return s.results[diff][musicID]
}

func (s *progressSnapshotStub) ChallengeLive() *userdata.ChallengeLiveData { return nil }

func (s *progressSnapshotStub) RawBytes() ([]byte, error) { return nil, nil }

func (s *progressSnapshotStub) RawValue(string) ([]byte, error) { return nil, nil }

func (s *progressSnapshotStub) RawFilePath() string { return "" }

func (s *progressSnapshotStub) RawData() *userdata.RawUserData { return nil }

func (s *progressSnapshotStub) MusicMetaBytes() []byte { return nil }

func (s *progressSnapshotStub) MusicMetaPath() string { return "" }

func TestBuildMusicProgressRequestFromSnapshotUsesSnapshotData(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", PublishedAt: now - 1000},
			2: {ID: 2, Title: "Song B", PublishedAt: now - 1000},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	snapshotProfile := &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{Nickname: "Snapshot User"},
	}
	fallbackProfile := &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{Nickname: "Fallback User"},
	}
	snapshot := &progressSnapshotStub{
		profile: snapshotProfile,
		results: map[string]map[int]string{
			"master": {
				1: "fc",
				2: "clear",
			},
		},
	}

	req, err := controller.BuildMusicProgressRequestFromSnapshot(ProgressQuery{
		Region:     "jp",
		Difficulty: "master",
	}, snapshot, fallbackProfile)
	if err != nil {
		t.Fatalf("BuildMusicProgressRequestFromSnapshot() error = %v", err)
	}
	if req.Profile.Profile == nil || req.Profile.Profile.Nickname != "Snapshot User" {
		t.Fatalf("expected snapshot profile, got %+v", req.Profile.Profile)
	}
	if len(req.Counts) != 1 || req.Counts[0].Level != 31 || req.Counts[0].Total != 2 || req.Counts[0].Clear != 2 || req.Counts[0].Fc != 1 {
		t.Fatalf("unexpected counts: %+v", req.Counts)
	}
}

func TestBuildMusicProgressRequestFromSnapshotUsesCompactToolboxResults(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			201: {ID: 201, Title: "Song A", PublishedAt: now - 1000},
			202: {ID: 202, Title: "Song B", PublishedAt: now - 1000},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			201: {
				{MusicID: 201, MusicDifficulty: "master", PlayLevel: 31},
			},
			202: {
				{MusicID: 202, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}

	snapshot, err := userdata.NewDefaultSnapshotFactory(nil, nil).Build(nil, userdata.BuildInput{
		Region: renderregion.CN,
		Source: "toolbox",
		SuiteJSON: []byte(`{
  "now": 1710000000,
  "userGamedata": {"userId": 123456789, "name": "SnapshotUser", "deck": 1, "rank": 100, "coin": 0},
  "userProfile": {"profileImageType": "default", "profileImageId": 0, "word": "", "twitterId": ""},
  "userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 1001, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
  "userCards": [{"cardId": 1001, "level": 60, "masterRank": 0, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []}],
  "userMusicResults": [],
  "compactUserMusicResults": {
    "__ENUM__": {
      "musicDifficulty": ["easy", "normal", "hard", "expert", "master", "append"],
      "playType": ["solo", "multi"],
      "playResult": ["full_perfect", "full_combo", "clear", "not_clear"]
    },
    "musicId": [201, 202],
    "musicDifficulty": [4, 4],
    "playType": [0, 1],
    "playResult": [2, 1],
    "fullComboFlg": [false, true],
    "fullPerfectFlg": [false, false]
  },
  "userChallengeLiveSoloResults": [],
  "userChallengeLiveSoloStages": [],
  "userChallengeLiveSoloHighScoreRewards": []
}`),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	controller := NewController(&regionalLookupTestSource{
		lookupTestSource: source,
		region:           renderregion.CN,
	}, nil, assets.NewAssetHelper("", nil), nil, nil)
	req, err := controller.BuildMusicProgressRequestFromSnapshot(ProgressQuery{
		Region:     "cn",
		Difficulty: "master",
	}, snapshot, nil)
	if err != nil {
		t.Fatalf("BuildMusicProgressRequestFromSnapshot() error = %v", err)
	}
	if len(req.Counts) != 1 {
		t.Fatalf("expected 1 level group, got %+v", req.Counts)
	}
	if req.Counts[0].Level != 31 || req.Counts[0].Total != 2 || req.Counts[0].Clear != 2 || req.Counts[0].Fc != 1 || req.Counts[0].NotClear != 0 {
		t.Fatalf("unexpected counts: %+v", req.Counts[0])
	}
}
