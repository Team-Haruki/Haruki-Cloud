package snapshot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"

	_ "github.com/mattn/go-sqlite3"
)

func TestNewFromBytesWithContextUsesFactoryContextForLeaderLookup(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:userdata_live_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Card.Create().
		SetServerRegion("jp").
		SetGameID(1001).
		SetCharacterID(1).
		SetAssetbundleName("res001_no001").
		Save(ctx); err != nil {
		t.Fatalf("create card: %v", err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	snapshot, err := NewFromBytesWithContext(canceledCtx, sekaiClient, nil, renderregion.JP, []byte(minimalSuiteJSON), nil, nil)
	if err != nil {
		t.Fatalf("NewFromBytesWithContext() error = %v", err)
	}
	if err := snapshot.Require(); err != nil {
		t.Fatalf("snapshot.Require() error = %v", err)
	}

	profile := snapshot.DetailedProfile(renderregion.JP)
	if profile == nil {
		t.Fatalf("expected profile")
	}
	if profile.LeaderImagePath != "static_images/unknown.jpg" {
		t.Fatalf("expected fallback leader path when build ctx is canceled, got %q", profile.LeaderImagePath)
	}
}

func TestNewFromBytesWithContextUsesCurrentLeaderDisplayState(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:userdata_live_before_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Card.Create().
		SetServerRegion("jp").
		SetGameID(1001).
		SetCharacterID(1).
		SetAssetbundleName("res001_no001").
		Save(ctx); err != nil {
		t.Fatalf("create card: %v", err)
	}

	beforeArtJSON := strings.Replace(minimalSuiteJSON, `"defaultImage": "special_training"`, `"defaultImage": "normal"`, 1)
	snapshot, err := NewFromBytesWithContext(ctx, sekaiClient, nil, renderregion.JP, []byte(beforeArtJSON), nil, nil)
	if err != nil {
		t.Fatalf("NewFromBytesWithContext() error = %v", err)
	}
	if err := snapshot.Require(); err != nil {
		t.Fatalf("snapshot.Require() error = %v", err)
	}

	profile := snapshot.DetailedProfile(renderregion.JP)
	if profile == nil {
		t.Fatalf("expected profile")
	}
	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_normal.png"
	if profile.LeaderImagePath != wantLeader {
		t.Fatalf("expected leader image path %q, got %q", wantLeader, profile.LeaderImagePath)
	}
}

func TestNewFromBytesWithContextIgnoresCustomProfileImageCard(t *testing.T) {
	ctx := context.Background()
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:userdata_live_profile_leader_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = sekaiClient.Close() })

	for _, item := range []struct {
		id     int64
		asset  string
		charID int64
	}{
		{id: 1001, asset: "res001_no001", charID: 1},
		{id: 1002, asset: "res021_no002", charID: 21},
	} {
		if _, err := sekaiClient.Card.Create().
			SetServerRegion("jp").
			SetGameID(item.id).
			SetCharacterID(item.charID).
			SetAssetbundleName(item.asset).
			Save(ctx); err != nil {
			t.Fatalf("create card %d: %v", item.id, err)
		}
	}

	customProfileJSON := `{
  "now": 1710000000,
  "userGamedata": {"userId": 123456789, "name": "SnapshotUser", "deck": 1, "rank": 100, "coin": 0},
  "userProfile": {"profileImageType": "normal", "profileImageId": 1002, "word": "", "twitterId": ""},
  "userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 1001, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
  "userCards": [
    {"cardId": 1001, "level": 60, "masterRank": 0, "specialTrainingStatus": "done", "defaultImage": "normal", "episodes": []},
    {"cardId": 1002, "level": 60, "masterRank": 0, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []}
  ],
  "userMusicResults": [],
  "userChallengeLiveSoloResults": [],
  "userChallengeLiveSoloStages": [],
  "userChallengeLiveSoloHighScoreRewards": []
}`

	snapshot, err := NewFromBytesWithContext(ctx, sekaiClient, nil, renderregion.JP, []byte(customProfileJSON), nil, nil)
	if err != nil {
		t.Fatalf("NewFromBytesWithContext() error = %v", err)
	}
	if err := snapshot.Require(); err != nil {
		t.Fatalf("snapshot.Require() error = %v", err)
	}

	profile := snapshot.DetailedProfile(renderregion.JP)
	if profile == nil {
		t.Fatalf("expected profile")
	}
	wantLeader := "asset/jp-assets/startapp/thumbnail/chara/res001_no001_normal.png"
	if profile.LeaderImagePath != wantLeader {
		t.Fatalf("expected leader image path %q, got %q", wantLeader, profile.LeaderImagePath)
	}
}
