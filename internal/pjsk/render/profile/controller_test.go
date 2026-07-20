package profile

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
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
	return new(*item), nil
}

func (s *testProfileSource) GetHonorGroupByID(id int) (*masterdata.HonorGroup, error) {
	item, ok := s.honorGroups[id]
	if !ok {
		return nil, fmt.Errorf("honor group not found: %d", id)
	}
	return new(*item), nil
}

func (s *testProfileSource) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	return nil, fmt.Errorf("bonds honor not found: %d", id)
}

func (s *testProfileSource) GetBondsHonorWordByID(id int) (*masterdata.BondsHonorWord, error) {
	return nil, fmt.Errorf("bonds honor word not found: %d", id)
}

func (s *testProfileSource) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	return nil, false
}

func (s *testProfileSource) GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error) {
	item, ok := s.frames[id]
	if !ok {
		return nil, fmt.Errorf("player frame not found: %d", id)
	}
	return new(*item), nil
}

func (s *testProfileSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	item, ok := s.frameGroups[id]
	if !ok {
		return nil, fmt.Errorf("player frame group not found: %d", id)
	}
	return new(*item), nil
}

func (s *testProfileSource) GetCardByID(id int) (*masterdata.Card, error) {
	item, ok := s.cards[id]
	if !ok {
		return nil, fmt.Errorf("card not found: %d", id)
	}
	return new(*item), nil
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

func TestBuildProfileRequestFromAPIFollowsLeaderCardInsteadOfCustomProfileImage(t *testing.T) {
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

	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_after_training.png"
	if payload.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("expected leader image path %q, got %q", wantLeader, payload.Profile.LeaderImagePath)
	}
}

func TestBuildProfileRequestFromAPIIncludesMultiLiveTopScoreCounts(t *testing.T) {
	source := &testProfileSource{
		region:      renderregion.JP,
		cards:       map[int]*masterdata.Card{},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)

	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "Multi User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserMultiLiveTopScoreCount: sekai.AnotherUserMultiLiveTopScoreCount{
			MVP:       12,
			SuperStar: 34,
		},
	}

	payload, err := controller.BuildProfileRequestFromAPI(Query{Region: "jp", Visible: true}, resp, nil)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPI failed: %v", err)
	}

	if payload.MultiLive == nil {
		t.Fatalf("expected multi live stats, got nil")
	}
	if payload.MultiLive.MVP != 12 || payload.MultiLive.SuperStar != 34 {
		t.Fatalf("unexpected multi live stats: %+v", payload.MultiLive)
	}
}

func TestBuildProfileRequestFromAPIUsesCurrentCardDisplayState(t *testing.T) {
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
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "Before Art User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "leader"},
		UserDeck:    sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "normal"},
		},
	}

	payload, err := controller.BuildProfileRequestFromAPI(Query{Region: "jp", Visible: true}, resp, nil)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPI failed: %v", err)
	}

	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_normal.png"
	if payload.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("expected leader image path %q, got %q", wantLeader, payload.Profile.LeaderImagePath)
	}
}

func TestBuildProfileRequestFromAPIUsesActualFcApCountsForHonorBadges(t *testing.T) {
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
			3013: {
				ID:              3013,
				GroupID:         701,
				HonorRarity:     "high",
				AssetBundleName: "honor_3013_700",
				Levels: []masterdata.HonorLevel{
					{Level: 20, AssetBundleName: "honor_3013_700", HonorRarity: "high"},
				},
			},
		},
		honorGroups: map[int]*masterdata.HonorGroup{
			701: {
				ID:        701,
				HonorType: "achievement",
				Name:      "MASTER FC",
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "Honor User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserDeck:    sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
		UserProfileHonors: []sekai.UserProfileHonor{
			{Seq: 1, ProfileHonorType: "normal", HonorID: 3013, HonorLevel: 20},
		},
		UserHonors: []sekai.UserHonor{
			{HonorID: 3013, Level: 20},
		},
		UserMusicDifficultyClearCount: []sekai.AnotherUserMusicDifficultyClearCount{
			{MusicDifficultyType: "master", LiveClear: 30, FullCombo: 15, AllPerfect: 2},
		},
	}

	payload, err := controller.BuildProfileRequestFromAPI(Query{Region: "jp", Visible: true}, resp, nil)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPI failed: %v", err)
	}
	if len(payload.Honors) != 1 {
		t.Fatalf("expected 1 honor, got %d", len(payload.Honors))
	}
	if payload.Honors[0].HonorLevel == nil || *payload.Honors[0].HonorLevel != 20 {
		t.Fatalf("unexpected visual honor level: %#v", payload.Honors[0].HonorLevel)
	}
	if payload.Honors[0].FcOrApLevel == nil || *payload.Honors[0].FcOrApLevel != "15" {
		t.Fatalf("expected displayed FC/AP level 15, got %#v", payload.Honors[0].FcOrApLevel)
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
	rawData *snapshot.RawUserData
	detail  *drawing.DetailedProfileCardRequest
	profile *drawing.ProfileCardRequest
}

func (s *profileSnapshotStub) Require() error { return nil }

func (s *profileSnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return s.detail
}

func (s *profileSnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest {
	return s.profile
}

func (s *profileSnapshotStub) MusicResults(string) map[int]string { return nil }

func (s *profileSnapshotStub) GetMusicResult(int, string) string { return "" }

func (s *profileSnapshotStub) ChallengeLive() *snapshot.ChallengeLiveData { return nil }

func (s *profileSnapshotStub) RawBytes() ([]byte, error) { return nil, nil }

func (s *profileSnapshotStub) RawValue(string) ([]byte, error) { return nil, nil }

func (s *profileSnapshotStub) RawFilePath() string { return "" }

func (s *profileSnapshotStub) RawData() *snapshot.RawUserData { return s.rawData }

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
	snap := &profileSnapshotStub{
		rawData: &snapshot.RawUserData{
			UserFrames: []snapshot.RawUserFrame{
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

	payload, err := controller.BuildProfileRequestFromAPIWithSnapshot(Query{Region: "jp", Visible: true}, resp, snap)
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

func TestBuildProfileRequestFromSnapshotIncludesMultiLiveTopScoreCounts(t *testing.T) {
	source := &testProfileSource{
		region:      renderregion.JP,
		cards:       map[int]*masterdata.Card{},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), &profileSnapshotStub{
		rawData: &snapshot.RawUserData{
			UserGamedata: snapshot.RawUserGamedata{
				UserID: 12345,
				Rank:   88,
			},
			UserProfile: snapshot.RawUserProfile{
				ProfileImageType: "default",
			},
			UserMultiLiveTopScoreCount: snapshot.RawUserMultiLiveTopScoreCount{
				MVP:       7,
				SuperStar: 9,
			},
		},
		detail: &drawing.DetailedProfileCardRequest{
			ID:              "12345",
			Region:          "JP",
			Nickname:        "Snapshot User",
			LeaderImagePath: "",
		},
	})

	payload, err := controller.BuildProfileRequest(Query{Region: "jp", Visible: true})
	if err != nil {
		t.Fatalf("BuildProfileRequest failed: %v", err)
	}

	if payload.MultiLive == nil {
		t.Fatalf("expected multi live stats, got nil")
	}
	if payload.MultiLive.MVP != 7 || payload.MultiLive.SuperStar != 9 {
		t.Fatalf("unexpected multi live stats: %+v", payload.MultiLive)
	}
}

func TestBuildDetailedProfileCardFromAPIWithSnapshotInheritsSnapshotMetadata(t *testing.T) {
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
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	mode := "suite"
	updateTime := int64(1776000000123)
	snap := &profileSnapshotStub{
		detail: &drawing.DetailedProfileCardRequest{
			ID:              "12345",
			Region:          "JP",
			Nickname:        "Snapshot User",
			Source:          "suite_dump",
			UpdateTime:      updateTime,
			Mode:            &mode,
			LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/res001_no001_normal.png",
		},
	}

	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "API User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserDeck:    sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
	}

	detail, err := controller.BuildDetailedProfileCardFromAPIWithSnapshot(Query{Region: "jp", Visible: true}, resp, snap)
	if err != nil {
		t.Fatalf("BuildDetailedProfileCardFromAPIWithSnapshot failed: %v", err)
	}

	if detail.Source != "suite_dump" {
		t.Fatalf("expected snapshot source metadata, got %q", detail.Source)
	}
	if detail.UpdateTime != updateTime {
		t.Fatalf("expected snapshot update time %d, got %d", updateTime, detail.UpdateTime)
	}
	if detail.Mode == nil || *detail.Mode != mode {
		t.Fatalf("expected snapshot mode %q, got %+v", mode, detail.Mode)
	}
	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_after_training.png"
	if detail.LeaderImagePath != wantLeader {
		t.Fatalf("expected API leader image path %q, got %q", wantLeader, detail.LeaderImagePath)
	}
}

func TestBuildProfileCardFromAPIWithSnapshotInheritsSnapshotDataSources(t *testing.T) {
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
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	mode := "suite"
	updateTime := int64(1776000000456)
	snapSource := "suite_dump"
	snap := &profileSnapshotStub{
		detail: &drawing.DetailedProfileCardRequest{
			ID:              "12345",
			Region:          "JP",
			Nickname:        "Snapshot User",
			Source:          snapSource,
			UpdateTime:      updateTime,
			Mode:            &mode,
			LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/res001_no001_normal.png",
		},
		profile: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{
				ID:              "12345",
				Region:          "JP",
				Nickname:        "Snapshot User",
				LeaderImagePath: "asset/jp-assets/startapp/thumbnail/chara/res001_no001_normal.png",
			},
			DataSources: []drawing.ProfileDataSource{
				{
					Name:       "Suite数据",
					Source:     &snapSource,
					UpdateTime: &updateTime,
					Mode:       &mode,
				},
			},
		},
	}

	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "API User", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserDeck:    sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
	}

	card, err := controller.BuildProfileCardFromAPIWithSnapshot(Query{Region: "jp", Visible: true}, resp, snap)
	if err != nil {
		t.Fatalf("BuildProfileCardFromAPIWithSnapshot failed: %v", err)
	}

	if len(card.DataSources) != 1 {
		t.Fatalf("expected 1 data source, got %+v", card.DataSources)
	}
	if card.DataSources[0].Name != "Suite数据" {
		t.Fatalf("expected suite data source, got %+v", card.DataSources[0])
	}
	if card.DataSources[0].UpdateTime == nil || *card.DataSources[0].UpdateTime != updateTime {
		t.Fatalf("expected suite update time %d, got %+v", updateTime, card.DataSources[0].UpdateTime)
	}
	if card.DataSources[0].Source == nil || *card.DataSources[0].Source != snapSource {
		t.Fatalf("expected suite source %q, got %+v", snapSource, card.DataSources[0].Source)
	}
	if card.DataSources[0].Mode == nil || *card.DataSources[0].Mode != mode {
		t.Fatalf("expected suite mode %q, got %+v", mode, card.DataSources[0].Mode)
	}
	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_after_training.png"
	if card.Profile == nil || card.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("expected API leader image path %q, got %+v", wantLeader, card.Profile)
	}
}

func TestBuildProfileRequestFromAPIWithSnapshotKeepsAPILeaderDisplayState(t *testing.T) {
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
	snap := &profileSnapshotStub{
		rawData: &snapshot.RawUserData{
			UserGamedata: snapshot.RawUserGamedata{Deck: 1},
			UserDecks: []snapshot.RawUserDeck{
				{DeckID: 1, Leader: 1001, Member1: 1001},
			},
			UserCards: []snapshot.RawUserCard{
				{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "normal"},
			},
		},
	}

	resp := &sekai.GetAnotherProfileResponse{
		User: sekai.AnotherUser{UserID: 12345, Name: "Snapshot Leader", Rank: 100},
		UserProfile: sekai.UserProfile{
			ProfileImageType: "normal",
			ProfileImageID:   1002,
		},
		UserDeck: sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
		UserCards: []sekai.AnotherUserCard{
			{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
			{CardID: 1002, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
		},
	}

	payload, err := controller.BuildProfileRequestFromAPIWithSnapshot(Query{Region: "jp", Visible: true}, resp, snap)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPIWithSnapshot failed: %v", err)
	}

	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_after_training.png"
	if payload.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("expected leader image path %q, got %q", wantLeader, payload.Profile.LeaderImagePath)
	}
	if len(payload.Pcards) != 1 || payload.Pcards[0].CardThumbnailPath != wantLeader {
		t.Fatalf("expected pcard thumbnail to follow API leader art, got %+v", payload.Pcards)
	}
}

func TestBuildProfileRequestFromAPIWithSnapshotFallsBackToSnapshotDeckWhenAPIDeckMissing(t *testing.T) {
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
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	snap := &profileSnapshotStub{
		rawData: &snapshot.RawUserData{
			UserGamedata: snapshot.RawUserGamedata{Deck: 1},
			UserDecks: []snapshot.RawUserDeck{
				{DeckID: 1, Leader: 1001, Member1: 1001},
			},
			UserCards: []snapshot.RawUserCard{
				{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "normal"},
			},
		},
	}

	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "Snapshot Fallback", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserDeck:    sekai.UserDeck{},
	}

	payload, err := controller.BuildProfileRequestFromAPIWithSnapshot(Query{Region: "jp", Visible: true}, resp, snap)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPIWithSnapshot failed: %v", err)
	}

	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_normal.png"
	if payload.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("expected snapshot fallback leader image path %q, got %q", wantLeader, payload.Profile.LeaderImagePath)
	}
	if len(payload.Pcards) != 1 || payload.Pcards[0].CardThumbnailPath != wantLeader {
		t.Fatalf("expected snapshot fallback pcard thumbnail, got %+v", payload.Pcards)
	}
	if payload.Pcards[0].IsAfterTraining == nil || *payload.Pcards[0].IsAfterTraining {
		t.Fatalf("expected pcard display state to stay before-training, got %+v", payload.Pcards[0].IsAfterTraining)
	}
	if !strings.Contains(payload.Pcards[0].RareImgPath, "rare_star_after_training.png") {
		t.Fatalf("expected trained rarity marker to stay after-training, got %q", payload.Pcards[0].RareImgPath)
	}
}

func TestBuildProfileRequestFromAPIWithSnapshotUsesSnapshotLeaderArtWhenAPICardStateMissing(t *testing.T) {
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
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil)
	snap := &profileSnapshotStub{
		rawData: &snapshot.RawUserData{
			UserCards: []snapshot.RawUserCard{
				{CardID: 1001, Level: 60, MasterRank: 5, SpecialTrainingStatus: "done", DefaultImage: "special_training"},
			},
		},
	}

	resp := &sekai.GetAnotherProfileResponse{
		User:        sekai.AnotherUser{UserID: 12345, Name: "Leader Art Fallback", Rank: 100},
		UserProfile: sekai.UserProfile{ProfileImageType: "default"},
		UserDeck:    sekai.UserDeck{DeckID: 1, Leader: 1001, Member1: 1001},
	}

	payload, err := controller.BuildProfileRequestFromAPIWithSnapshot(Query{Region: "jp", Visible: true}, resp, snap)
	if err != nil {
		t.Fatalf("BuildProfileRequestFromAPIWithSnapshot failed: %v", err)
	}

	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_after_training.png"
	if payload.Profile.LeaderImagePath != wantLeader {
		t.Fatalf("expected snapshot-supplemented leader image path %q, got %q", wantLeader, payload.Profile.LeaderImagePath)
	}
	if len(payload.Pcards) != 1 || payload.Pcards[0].CardThumbnailPath != wantLeader {
		t.Fatalf("expected snapshot-supplemented pcard thumbnail, got %+v", payload.Pcards)
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

type overlapCensorStub struct {
	inflight   atomic.Int32
	overlapped atomic.Bool
	rejectBio  bool
	nameCalls  atomic.Int32
	bioCalls   atomic.Int32
}

func (o *overlapCensorStub) observe() {
	if o.inflight.Add(1) > 1 {
		o.overlapped.Store(true)
	}
	time.Sleep(100 * time.Millisecond)
	if o.inflight.Load() > 1 {
		o.overlapped.Store(true)
	}
	o.inflight.Add(-1)
}

func (o *overlapCensorStub) CensorName(context.Context, int, string, string, string) bool {
	o.nameCalls.Add(1)
	o.observe()
	return true
}

func (o *overlapCensorStub) CensorShortBio(context.Context, int, string, string, string) bool {
	o.bioCalls.Add(1)
	o.observe()
	return !o.rejectBio
}

func TestBuildProfileRequestRunsCensorChecksConcurrently(t *testing.T) {
	source := &testProfileSource{
		region:      renderregion.JP,
		cards:       map[int]*masterdata.Card{},
		honors:      map[int]*masterdata.Honor{},
		honorGroups: map[int]*masterdata.HonorGroup{},
	}
	stub := &overlapCensorStub{rejectBio: true}
	snap := &profileSnapshotStub{
		rawData: &snapshot.RawUserData{
			UserGamedata: snapshot.RawUserGamedata{UserID: 42, Name: "player", Rank: 10},
			UserProfile:  snapshot.RawUserProfile{Word: "hello world"},
		},
		detail: &drawing.DetailedProfileCardRequest{
			ID:       "42",
			Region:   "JP",
			Nickname: "player",
		},
	}
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), snap)
	controller.censor = stub

	payload, err := controller.BuildProfileRequest(Query{Region: "jp", Visible: true})
	if err != nil {
		t.Fatalf("BuildProfileRequest: %v", err)
	}
	if got := stub.nameCalls.Load(); got != 1 {
		t.Fatalf("CensorName calls = %d, want 1", got)
	}
	if got := stub.bioCalls.Load(); got != 1 {
		t.Fatalf("CensorShortBio calls = %d, want 1", got)
	}
	if !stub.overlapped.Load() {
		t.Fatal("censor name/bio checks should run concurrently")
	}
	if payload.Profile.Nickname != "player" {
		t.Fatalf("nickname = %q, want kept", payload.Profile.Nickname)
	}
	if payload.Word != "" {
		t.Fatalf("word = %q, want blanked by rejected bio", payload.Word)
	}
}
