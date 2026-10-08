package provider

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"
	"haruki-cloud/database/sekai/eventcard"
	"haruki-cloud/database/sekai/eventdeckbonuse"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestEventListPreloadQueryCountsAndData(t *testing.T) {
	ctx := t.Context()
	provider := openProviderBehaviorDB(t, "event_list_bulk")
	client := provider.client
	for _, region := range []string{"jp", "tw"} {
		for id := int64(1); id <= 30; id++ {
			if _, err := client.Event.Create().SetGameID(id).SetEventType("marathon").SetStartAt(id * 1000).SetServerRegion(region).Save(ctx); err != nil {
				t.Fatal(err)
			}
			for offset := int64(1); offset >= 0; offset-- {
				cardID := id*2 + offset
				if _, err := client.Card.Create().SetGameID(cardID).SetCharacterID(cardID).SetCardRarityType("rarity_4").SetAttr("cute").SetServerRegion(region).Save(ctx); err != nil {
					t.Fatal(err)
				}
				if _, err := client.Eventcard.Create().SetGameID(cardID).SetEventID(id).SetCardID(cardID).SetServerRegion(region).Save(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := client.Eventdeckbonuse.Create().SetGameID(id).SetEventID(id).SetCardAttr(region).SetBonusRate(0.5).SetServerRegion(region).Save(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	var queries atomic.Int32
	client.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			return next.Query(ctx, query)
		})
	}))
	type row struct {
		EventID, Banner int
		Cards           []*masterdata.Card
		Bonuses         []*masterdata.EventDeckBonus
	}
	readList := func() []row {
		t.Helper()
		events := provider.events.GetAll(ctx)
		ids := make([]int, len(events))
		for i, event := range events {
			ids[i] = event.ID
		}
		if err := provider.events.PreloadList(ctx, ids); err != nil {
			t.Fatal(err)
		}
		rows := make([]row, 0, len(events))
		for _, event := range events {
			cards, err := provider.events.GetCards(ctx, event.ID)
			if err != nil {
				t.Fatal(err)
			}
			banner, err := provider.events.GetBannerCharacterID(ctx, event.ID)
			if err != nil {
				t.Fatal(err)
			}
			bonuses, err := provider.events.GetDeckBonuses(ctx, event.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(cards) != 2 || cards[0].ID != event.ID*2 || cards[1].ID != event.ID*2+1 || banner != event.ID*2 || len(bonuses) != 1 || bonuses[0].CardAttr != "jp" {
				t.Fatalf("event=%d cards=%+v banner=%d bonuses=%+v", event.ID, cards, banner, bonuses)
			}
			rows = append(rows, row{event.ID, banner, cards, bonuses})
		}
		return rows
	}
	cold := readList()
	if got := queries.Swap(0); got != 4 {
		t.Fatalf("cold queries=%d, want events+links+bonuses+cards=4", got)
	}
	warm := readList()
	if got := queries.Swap(0); got != 0 {
		t.Fatalf("warm queries=%d, want 0 (event list is indexed too)", got)
	}
	if !reflect.DeepEqual(cold, warm) {
		t.Fatal("cold and warm rows differ")
	}
	cold[0].Cards[0].CharacterID = -1
	cold[0].Bonuses[0].CardAttr = "mutated"
	if got := readList(); !reflect.DeepEqual(got, warm) {
		t.Fatal("caller mutation changed cached rows")
	}
	queries.Store(0)

	if _, err := client.Eventcard.Update().Where(eventcard.ServerRegionEQ("jp"), eventcard.EventIDEQ(1), eventcard.CardIDEQ(2)).SetCardID(4).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Eventdeckbonuse.Update().Where(eventdeckbonuse.ServerRegionEQ("jp"), eventdeckbonuse.EventIDEQ(1)).SetCardAttr("updated").Save(ctx); err != nil {
		t.Fatal(err)
	}
	provider.events.resetLocalMasterdataCache()
	if err := provider.events.PreloadList(ctx, []int{1}); err != nil {
		t.Fatal(err)
	}
	cards, err := provider.events.GetCards(ctx, 1)
	if err != nil || len(cards) != 2 || cards[0].ID != 3 || cards[1].ID != 4 {
		t.Fatalf("updated links=%+v err=%v", cards, err)
	}
	bonuses, err := provider.events.GetDeckBonuses(ctx, 1)
	if err != nil || len(bonuses) != 1 || bonuses[0].CardAttr != "updated" {
		t.Fatalf("updated bonuses=%+v err=%v", bonuses, err)
	}
	if got := queries.Load(); got != 3 {
		t.Fatalf("reset relation/card queries=%d want 3", got)
	}
}

func TestEventRelationIndexesDoNotCacheFailuresAndRefreshEmptyRows(t *testing.T) {
	ctx := t.Context()
	provider := openProviderBehaviorDB(t, "event_bulk_errors")
	var fail atomic.Bool
	fail.Store(true)
	var calls atomic.Int32
	provider.client.Eventcard.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			calls.Add(1)
			if fail.Load() {
				return nil, errors.New("unavailable")
			}
			return next.Query(ctx, query)
		})
	}))
	if _, err := provider.events.GetCards(ctx, 1); err == nil {
		t.Fatal("failed query succeeded")
	}
	fail.Store(false)
	if _, err := provider.events.GetCards(ctx, 1); err == nil {
		t.Fatal("missing event must retain its error")
	}
	if calls.Load() != 2 {
		t.Fatalf("failure was cached: calls=%d", calls.Load())
	}
	if _, err := provider.events.GetCards(ctx, 2); err == nil {
		t.Fatal("missing event must retain its error")
	}
	if calls.Load() != 2 {
		t.Fatal("empty index was not cached")
	}
	if _, err := provider.client.Eventcard.Create().SetGameID(1).SetEventID(1).SetCardID(1).SetServerRegion("jp").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.client.Card.Create().SetGameID(1).SetCharacterID(1).SetCardRarityType("rarity_4").SetServerRegion("jp").Save(ctx); err != nil {
		t.Fatal(err)
	}
	provider.events.cardLinks.mu.Lock()
	provider.events.cardLinks.loadedAt = time.Now().Add(-dbBulkIndexTTL)
	provider.events.cardLinks.mu.Unlock()
	if _, err := provider.events.GetCards(ctx, 1); err == nil {
		t.Fatal("expired index must be served while it refreshes")
	}
	waitForIndexRefresh(t, func() bool {
		cards, err := provider.events.GetCards(ctx, 1)
		return err == nil && len(cards) == 1
	})
	if calls.Load() != 3 {
		t.Fatalf("expired empty index queries=%d", calls.Load())
	}
}

func TestEventCardBatchPreservesDuplicatePositions(t *testing.T) {
	ctx := t.Context()
	provider := openProviderBehaviorDB(t, "event_duplicate_links")
	if _, err := provider.client.Card.Create().SetGameID(1).SetCharacterID(1).SetCardRarityType("rarity_4").SetServerRegion("jp").Save(ctx); err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 2; id++ {
		if _, err := provider.client.Eventcard.Create().SetGameID(id).SetEventID(id).SetCardID(1).SetServerRegion("jp").Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	provider.events.init()
	cold, err := provider.events.getCardsByIDs(ctx, []int64{1, 1})
	if err != nil || len(cold) != 2 || cold[0].ID != 1 || cold[1].ID != 1 {
		t.Fatalf("cold duplicate positions=%+v err=%v", cold, err)
	}
	provider.events.resetLocalMasterdataCache()
	if err := provider.events.PreloadList(ctx, []int{1, 2}); err != nil {
		t.Fatal(err)
	}
	warm, err := provider.events.getCardsByIDs(ctx, []int64{1, 1})
	if err != nil || !reflect.DeepEqual(cold, warm) {
		t.Fatalf("preloaded duplicate positions=%+v err=%v", warm, err)
	}
	warm[0].CharacterID = -1
	if warm[1].CharacterID != 1 {
		t.Fatal("duplicate result positions share mutable card data")
	}
}
