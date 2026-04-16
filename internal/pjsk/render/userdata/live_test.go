package userdata

import (
	"context"
	"fmt"
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
