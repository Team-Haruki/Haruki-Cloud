package profile

import (
	"context"
	"fmt"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
	"haruki-cloud/utils/sekai"
)

type profileContextKey string

type contextAwareProfileSource struct {
	*testProfileSource
	ctx       context.Context
	wantKey   profileContextKey
	wantValue string
}

func (s *contextAwareProfileSource) WithContext(ctx context.Context) DataSource {
	clone := *s
	clone.ctx = ctx
	return &clone
}

func (s *contextAwareProfileSource) GetCardByID(id int) (*masterdata.Card, error) {
	if s.ctx == nil {
		return nil, fmt.Errorf("missing request context")
	}
	value, _ := s.ctx.Value(s.wantKey).(string)
	if value != s.wantValue {
		return nil, fmt.Errorf("unexpected request context")
	}
	return s.testProfileSource.GetCardByID(id)
}

type testProfileSource struct {
	region      renderregion.Value
	cards       map[int]*masterdata.Card
	honors      map[int]*masterdata.Honor
	honorGroups map[int]*masterdata.HonorGroup
	frames      map[int]*masterdata.PlayerFrame
	frameGroups map[int]*masterdata.PlayerFrameGroup
}

func (s *testProfileSource) DefaultRegion() renderregion.Value { return s.region }

func (s *testProfileSource) GetHonorByID(id int) (*masterdata.Honor, error) {
	item, ok := s.honors[id]
	if !ok {
		return nil, fmt.Errorf("honor not found: %d", id)
	}
	cloned := *item
	return &cloned, nil
}

func (s *testProfileSource) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	item, ok := s.honorGroups[id]
	if !ok {
		return nil, fmt.Errorf("honor group not found: %d", id)
	}
	cloned := *item
	return &cloned, nil
}

func (s *testProfileSource) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	return nil, fmt.Errorf("bonds honor not found: %d", id)
}

func (s *testProfileSource) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	return nil, false
}

func (s *testProfileSource) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	item, ok := s.frames[id]
	if !ok {
		return nil, fmt.Errorf("player frame not found: %d", id)
	}
	cloned := *item
	return &cloned, nil
}

func (s *testProfileSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	item, ok := s.frameGroups[id]
	if !ok {
		return nil, fmt.Errorf("player frame group not found: %d", id)
	}
	cloned := *item
	return &cloned, nil
}

func (s *testProfileSource) GetCardByID(id int) (*masterdata.Card, error) {
	item, ok := s.cards[id]
	if !ok {
		return nil, fmt.Errorf("card not found: %d", id)
	}
	cloned := *item
	return &cloned, nil
}

func (s *testProfileSource) GetEventIDByHonorID(honorID int) int {
	return 0
}

func TestBuildProfileRequestFromAPIUsesRequestedRegionAssetPaths(t *testing.T) {
	source := &testProfileSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				CharacterID:     1,
				AssetBundleName: "res001_no001",
			},
		},
		honors: map[int]*masterdata.Honor{
			10052: {
				ID:              10052,
				GroupID:         100016,
				HonorRarity:     "low",
				AssetBundleName: "honor_exp_1",
				Levels: []masterdata.HonorLevel{
					{Level: 1},
				},
			},
		},
		honorGroups: map[int]*masterdata.HonorGroup{
			100016: {
				ID:        100016,
				HonorType: "achievement",
				Name:      "EXP",
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	controller.RegisterSource(&testProfileSource{
		region:      renderregion.CN,
		cards:       source.cards,
		honors:      source.honors,
		honorGroups: source.honorGroups,
	})

	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "CN User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserDeck:    sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
		UserProfileHonors: []sekai.UserProfileHonor{
			{Seq: 1, ProfileHonorType: "normal", HonorID: 10052, HonorLevel: 1},
		},
		UserHonors: []sekai.UserHonor{
			{HonorID: 10052, Level: 1},
		},
	}

	payload, err := controller.BuildProfileRequestFromAPI(Query{Region: "cn", Visible: true}, resp, nil)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPI failed: %v", err)
	}

	wantLeader := "asset/cn-assets/startapp/thumbnail/chara/res001_no001_after_training.png"
	if payload.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("unexpected leader image path: %q", payload.Profile.LeaderImagePath)
	}
	if len(payload.Honors) != 1 {
		t.Fatalf("expected 1 honor, got %d", len(payload.Honors))
	}
	wantHonor := "asset/cn-assets/startapp/honor/honor_exp_1/degree_main.png"
	if payload.Honors[0].HonorImgPath == nil || *payload.Honors[0].HonorImgPath != wantHonor {
		t.Fatalf("unexpected honor image path: %#v", payload.Honors[0].HonorImgPath)
	}
}

func TestBuildProfileRequestFromAPIUsesConfiguredProfileImageCard(t *testing.T) {
	source := &testProfileSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				CharacterID:     1,
				AssetBundleName: "res001_no001",
			},
			1002: {
				ID:              1002,
				CharacterID:     21,
				AssetBundleName: "res021_no002",
			},
		},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)

	resp := &sekai.GetAnotherProfileResponse{
		User: sekai.AnotherUser{UserID: 12345, Name: "Miku User", Rank: 100},
		UserProfile: sekai.UserProfile{
			ProfileImageType: "normal",
			ProfileImageID:   1002,
		},
		UserDeck: sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
			{CardID: 1002, Level: 50, MasterRank: 0, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
	}

	payload, err := controller.BuildProfileRequestFromAPI(Query{Region: "jp", Visible: true}, resp, nil)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPI failed: %v", err)
	}

	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res021_no002_after_training.png"
	if payload.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("unexpected custom profile image path: %q", payload.Profile.LeaderImagePath)
	}
}

func TestBuildProfileRequestFromAPIFallsBackToJPCardMetadataForNonJPProfileCards(t *testing.T) {
	jpSource := &testProfileSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			764: {ID: 764, CharacterID: 17, AssetBundleName: "res017_no042"},
			585: {ID: 585, CharacterID: 18, AssetBundleName: "res018_no033"},
		},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
	}
	twSource := &testProfileSource{
		region:      renderregion.TW,
		cards:       map[int]*masterdata.Card{},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
	}

	controller := NewController(jpSource, nil, assets.NewAssetHelper("", nil), nil)
	controller.RegisterSource(twSource)

	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "TW User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "leader"},
		UserDeck:    sekai.UserDeck{DeckID: 4, Leader: 764, Member1: 764, Member2: 585},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 764, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
			{CardID: 585, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
	}

	payload, err := controller.BuildProfileRequestFromAPI(Query{Region: "tw", Visible: true}, resp, nil)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPI failed: %v", err)
	}

	wantLeader := "asset/tw-assets/startapp/thumbnail/chara/res017_no042_after_training.png"
	if payload.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("unexpected leader image path: %q", payload.Profile.LeaderImagePath)
	}
	if len(payload.Pcards) != 2 {
		t.Fatalf("expected 2 pcards, got %d", len(payload.Pcards))
	}
	if payload.Pcards[0].CardThumbnailPath != wantLeader {
		t.Fatalf("unexpected first pcard thumbnail path: %q", payload.Pcards[0].CardThumbnailPath)
	}
	if payload.Pcards[0].TrainRank == nil || *payload.Pcards[0].TrainRank != 5 {
		t.Fatalf("unexpected first pcard master rank: %+v", payload.Pcards[0].TrainRank)
	}
}

type profileSnapshotStub struct {
	rawData *userdata.RawUserData
}

func (s *profileSnapshotStub) Require() error { return nil }

func (s *profileSnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return nil
}

func (s *profileSnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return nil
}

func (s *profileSnapshotStub) MusicResults(string) map[int]string { return nil }

func (s *profileSnapshotStub) GetMusicResult(int, string) string { return "" }

func (s *profileSnapshotStub) ChallengeLive() *userdata.ChallengeLiveData { return nil }

func (s *profileSnapshotStub) RawBytes() ([]byte, error) { return nil, nil }

func (s *profileSnapshotStub) RawValue(string) ([]byte, error) { return nil, nil }

func (s *profileSnapshotStub) RawFilePath() string { return "" }

func (s *profileSnapshotStub) RawData() *userdata.RawUserData { return s.rawData }

func (s *profileSnapshotStub) MusicMetaBytes() []byte { return nil }

func (s *profileSnapshotStub) MusicMetaPath() string { return "" }

func TestBuildProfileRequestFromAPIWithSnapshotUsesUserFrames(t *testing.T) {
	source := &testProfileSource{
		region: renderregion.JP,
		cards: map[int]*masterdata.Card{
			1001: {
				ID:              1001,
				CharacterID:     1,
				AssetBundleName: "res001_no001",
			},
		},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
		frames: map[int]*masterdata.PlayerFrame{
			10: {ID: 10, PlayerFrameGroupID: 20},
		},
		frameGroups: map[int]*masterdata.PlayerFrameGroup{
			20: {ID: 20, AssetBundleName: "frame_group_20"},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	snapshot := &profileSnapshotStub{
		rawData: &userdata.RawUserData{
			UserFrames: []userdata.RawUserFrame{
				{PlayerFrameID: 10, PlayerFrameAttachStatus: "equipped"},
			},
		},
	}

	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "Frame User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserDeck:    sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
	}

	payload, err := controller.BuildProfileRequestFromAPIWithSnapshot(Query{Region: "jp", Visible: true}, resp, snapshot)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPIWithSnapshot failed: %v", err)
	}

	if !payload.Profile.HasFrame {
		t.Fatalf("expected frame to be rendered")
	}
	if payload.FramePaths == nil || payload.FramePaths.Base != "player_frame/frame_group_20/10/horizontal/frame_base.png" {
		t.Fatalf("unexpected frame paths: %+v", payload.FramePaths)
	}
	if payload.Profile.FramePath == nil || *payload.Profile.FramePath != payload.FramePaths.Base {
		t.Fatalf("unexpected profile frame path: %+v", payload.Profile.FramePath)
	}
}

func TestControllerWithContextClonesProfileSource(t *testing.T) {
	source := &contextAwareProfileSource{
		testProfileSource: &testProfileSource{
			region: renderregion.JP,
			cards: map[int]*masterdata.Card{
				1001: {
					ID:              1001,
					CharacterID:     1,
					AssetBundleName: "res001_no001",
				},
			},
			honors:      map[int]*masterdata.Honor{},
			honorGroups: map[int]*masterdata.HonorGroup{},
		},
		wantKey:   profileContextKey("trace"),
		wantValue: "profile-api",
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "Ctx User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserDeck:    sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
	}

	ctx := context.WithValue(context.Background(), profileContextKey("trace"), "profile-api")
	payload, err := controller.WithContext(ctx).BuildProfileRequestFromAPI(Query{Region: "jp", Visible: true}, resp, nil)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPI failed: %v", err)
	}
	if payload.Profile.LeaderImagePath == "" {
		t.Fatalf("expected leader image path to be resolved")
	}
}
