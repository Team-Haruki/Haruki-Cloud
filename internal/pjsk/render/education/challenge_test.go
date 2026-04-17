package education

import (
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

type challengeTestSource struct {
	*testSource
	rewards map[int][]*ChallengeReward
}

func (s *challengeTestSource) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	if s == nil || s.rewards == nil {
		return nil
	}
	return s.rewards[charID]
}

func TestPickChallengeRewardsSumsOnlyUnclaimedChallengeRewards(t *testing.T) {
	source := &challengeTestSource{
		testSource: &testSource{
			region: renderregion.CN,
			boxes: map[string]map[int]*ResourceBox{
				"challenge_live_high_score": {
					1: {
						ID: 1,
						Details: []ResourceBoxDetail{
							{ResourceType: "jewel", ResourceQuantity: 100},
						},
					},
					2: {
						ID: 2,
						Details: []ResourceBoxDetail{
							{ResourceType: "material", ResourceID: 15, ResourceQuantity: 10},
						},
					},
					3: {
						ID: 3,
						Details: []ResourceBoxDetail{
							{ResourceType: "material", ResourceID: 7, ResourceQuantity: 20},
						},
					},
				},
			},
		},
		rewards: map[int][]*ChallengeReward{
			1: {
				{ID: 11, CharacterID: 1, HighScore: 100000, ResourceBoxID: 1},
				{ID: 12, CharacterID: 1, HighScore: 200000, ResourceBoxID: 2},
				{ID: 13, CharacterID: 1, HighScore: 300000, ResourceBoxID: 3},
			},
		},
	}

	controller := &Controller{}
	jewel, shard := controller.pickChallengeRewards(source, 1, map[int]struct{}{
		11: {},
	})
	if jewel != 0 {
		t.Fatalf("expected claimed jewel reward to be excluded, got %d", jewel)
	}
	if shard != 10 {
		t.Fatalf("expected only shard reward to remain, got %d", shard)
	}
}

func TestEstimateChallengeMaxScoreUsesRegionalFloor(t *testing.T) {
	controller := &Controller{}

	cnSource := &challengeTestSource{
		testSource: &testSource{region: renderregion.CN},
		rewards: map[int][]*ChallengeReward{
			1: {
				{HighScore: 1800000},
			},
		},
	}
	if got := controller.estimateChallengeMaxScore(renderregion.CN, cnSource); got != 2_500_000 {
		t.Fatalf("expected CN floor 250w, got %d", got)
	}

	jpSource := &challengeTestSource{
		testSource: &testSource{region: renderregion.JP},
		rewards: map[int][]*ChallengeReward{
			1: {
				{HighScore: 2200000},
			},
		},
	}
	if got := controller.estimateChallengeMaxScore(renderregion.JP, jpSource); got != 3_000_000 {
		t.Fatalf("expected JP floor 300w, got %d", got)
	}
}
