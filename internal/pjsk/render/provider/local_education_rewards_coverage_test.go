package provider

import (
	"context"
	"testing"
)

func TestLocalEducationChallengeRewardsLoadAndClone(t *testing.T) {
	root := t.TempDir()
	writeLocalCoverageFile(t, root, "challengeLiveHighScoreRewards.json", `[
		{"ID":1,"CharacterID":5,"HighScore":1000,"ResourceBoxID":10},
		{"ID":2,"CharacterID":5,"HighScore":2000,"ResourceBoxID":20},
		{"ID":3,"CharacterID":6,"HighScore":3000,"ResourceBoxID":30}
	]`)
	provider := NewLocalProvider(root, "").education
	ctx := context.Background()

	if got := provider.GetChallengeRewardsByCharacter(ctx, 0); got != nil {
		t.Fatalf("invalid character rewards = %+v", got)
	}
	rewards := provider.GetChallengeRewardsByCharacter(ctx, 5)
	if len(rewards) != 2 || rewards[0].HighScore != 1000 || rewards[1].HighScore != 2000 {
		t.Fatalf("character rewards = %+v", rewards)
	}
	rewards[0].HighScore = -1
	if cached := provider.GetChallengeRewardsByCharacter(ctx, 5); cached[0].HighScore != 1000 {
		t.Fatalf("cached rewards were mutated through clone: %+v", cached)
	}
	if got := provider.GetChallengeRewardsByCharacter(ctx, 99); got != nil {
		t.Fatalf("missing character rewards = %+v", got)
	}
}

func TestLocalEducationChallengeRewardsLoadFailure(t *testing.T) {
	provider := NewLocalProvider(t.TempDir(), "").education
	if got := provider.GetChallengeRewardsByCharacter(context.Background(), 5); got != nil {
		t.Fatalf("missing rewards file returned %+v", got)
	}
}
