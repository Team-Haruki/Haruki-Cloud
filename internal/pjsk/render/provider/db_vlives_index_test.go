package provider

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestDBVLiveProviderCachesRegionUntilReset(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:vlive_index_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	for _, live := range []struct {
		id, startAt int64
		region      renderregion.Value
	}{{2, 200, renderregion.JP}, {1, 100, renderregion.JP}, {3, 100, renderregion.JP}, {9, 50, renderregion.TW}} {
		if _, err := client.Virtuallive.Create().SetGameID(live.id).SetName(fmt.Sprintf("live %d", live.id)).
			SetStartAt(live.startAt).SetEndAt(live.startAt + 10).
			SetVirtualLiveSchedules([]byte(`[{"startAt":1,"endAt":2}]`)).
			SetServerRegion(live.region.String()).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var queries atomic.Int32
	client.Virtuallive.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			return next.Query(ctx, query)
		})
	}))

	provider := NewDatabaseProvider(client, renderregion.JP)
	lives, err := provider.vlives.GetLives(ctx, renderregion.JP)
	if err != nil || len(lives) != 3 || lives[0].ID != 1 || lives[1].ID != 3 || lives[2].ID != 2 {
		t.Fatalf("lives = %+v, %v", lives, err)
	}
	// Results are copies: a caller's mutation must not reach the index.
	lives[0].Name = "mutated"
	lives[0].Schedules[0].StartAt = -1
	again, err := provider.vlives.GetLives(ctx, renderregion.JP)
	if err != nil || again[0].Name != "live 1" || again[0].Schedules[0].StartAt != 1 {
		t.Fatalf("caller mutation reached the index: %+v, %v", again[0], err)
	}
	tw, err := provider.vlives.GetLives(ctx, renderregion.TW)
	if err != nil || len(tw) != 1 || tw[0].ID != 9 {
		t.Fatalf("tw lives = %+v, %v", tw, err)
	}
	if queries.Load() != 2 {
		t.Fatalf("queries = %d, want one per region", queries.Load())
	}

	if _, err := client.Virtuallive.Create().SetGameID(4).SetStartAt(400).SetEndAt(410).SetServerRegion(renderregion.JP.String()).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if lives, err := provider.vlives.GetLives(ctx, renderregion.JP); err != nil || len(lives) != 3 || queries.Load() != 2 {
		t.Fatalf("loaded lives must be served until reset: %d lives, %v, queries=%d", len(lives), err, queries.Load())
	}
	provider.ResetMasterdataCache()
	lives, err = provider.vlives.GetLives(ctx, renderregion.JP)
	if err != nil || len(lives) != 4 || lives[3].ID != 4 {
		t.Fatalf("reset lives = %+v, %v", lives, err)
	}
	if queries.Load() != 3 {
		t.Fatalf("queries after reset = %d, want 3", queries.Load())
	}
	(*dbVLiveProvider)(nil).resetMasterdataCache()
}

func TestCloneVLivesCopiesNestedValues(t *testing.T) {
	source := []*VLive{nil, {
		ID:                           1,
		Rewards:                      []VLiveReward{{ResourceBoxID: 5}},
		Characters:                   []VLiveCharacter{{GameCharacterUnitID: 7}},
		TotalCheerPointRewards:       []VLiveTotalCheerPointReward{{Threshold: 1, ResourceBoxID: 2}},
		TotalCheerPointSurplusReward: &VLiveSurplusReward{BasePoint: 3},
		VirtualItemOverrideCost:      &VLiveOverrideCost{CostResourceID: 4},
	}}
	clones := cloneVLives(source)
	if len(clones) != 1 {
		t.Fatalf("nil lives must be dropped: %+v", clones)
	}
	clone := clones[0]
	clone.Rewards[0].ResourceBoxID = 50
	clone.Characters[0].GameCharacterUnitID = 70
	clone.TotalCheerPointRewards[0].Threshold = 10
	clone.TotalCheerPointSurplusReward.BasePoint = 30
	clone.VirtualItemOverrideCost.CostResourceID = 40
	original := source[1]
	if original.Rewards[0].ResourceBoxID != 5 || original.Characters[0].GameCharacterUnitID != 7 ||
		original.TotalCheerPointRewards[0].Threshold != 1 || original.TotalCheerPointSurplusReward.BasePoint != 3 ||
		original.VirtualItemOverrideCost.CostResourceID != 4 {
		t.Fatalf("clone shares memory with the source: %+v", original)
	}
}
