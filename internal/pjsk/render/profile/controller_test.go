package profile

import (
	"fmt"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/sekai"
)

type testProfileSource struct {
	region       renderregion.Value
	cards        map[int]*masterdata.Card
	honors       map[int]*masterdata.Honor
	honorGroups  map[int]*masterdata.HonorGroup
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
	return nil, fmt.Errorf("player frame not found: %d", id)
}

func (s *testProfileSource) GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	return nil, fmt.Errorf("player frame group not found: %d", id)
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
