package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestCardAllCacheSharesLoadsAndOwnsReturns(t *testing.T) {
	ctx := t.Context()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:card_all_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	for _, region := range []string{"jp", "tw"} {
		for id := int64(1); id <= 12; id++ {
			_, err := client.Card.Create().SetGameID(id).SetCharacterID(1).SetReleaseAt(13 - id).SetPrefix(region).SetServerRegion(region).Save(ctx)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var queries atomic.Int32
	client.Card.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			queries.Add(1)
			return next.Query(ctx, q)
		})
	}))
	p := &dbCardProvider{client: client, region: renderregion.JP}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			cards, err := p.Filter(ctx, &CardFilter{})
			if err != nil || len(cards) != 12 {
				t.Errorf("cards=%v err=%v", cards, err)
				return
			}
			if cards[0].ID != 12 || cards[0].Prefix != "jp" {
				t.Error("ordering or region changed")
			}
			cards[0].Prefix = "mutated"
		})
	}
	wg.Wait()
	cards, err := p.Filter(ctx, &CardFilter{Limit: 2})
	if err != nil || len(cards) != 2 || cards[0].Prefix != "jp" {
		t.Fatalf("limited read = %v, %v", cards, err)
	}
	if _, err := p.GetByID(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if queries.Load() != 1 {
		t.Fatalf("queries = %d, want 1", queries.Load())
	}
	other := &dbCardProvider{client: client, region: renderregion.TW}
	cards, err = other.Filter(ctx, &CardFilter{})
	if err != nil || cards[0].Prefix != "tw" {
		t.Fatal("cross-region cache reuse")
	}
	if _, err := client.Card.Update().SetPrefix("updated").Save(ctx); err != nil {
		t.Fatal(err)
	}
	p.cardMu.Lock()
	p.allCardsLoadedAt = time.Now().Add(-dbBulkIndexTTL)
	p.cardMu.Unlock()
	cards, err = p.Filter(ctx, &CardFilter{})
	if err != nil || cards[0].Prefix != "updated" || queries.Load() != 3 {
		t.Fatalf("TTL refresh failed: %v, queries %d", err, queries.Load())
	}
	p.resetMasterdataCache()
	if _, err := p.Filter(ctx, &CardFilter{}); err != nil {
		t.Fatal(err)
	}
	if queries.Load() != 4 {
		t.Fatal("reset did not invalidate all-card cache")
	}
}

func TestCardAllCacheDiscardsResetGenerationAndAllowsCancel(t *testing.T) {
	ctx := t.Context()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:card_reset_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	row, err := client.Card.Create().SetGameID(1).SetCharacterID(1).SetPrefix("old").SetServerRegion("jp").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var queries atomic.Int32
	client.Card.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			value, err := next.Query(ctx, q)
			if queries.Add(1) == 1 {
				close(started)
				<-release
			}
			return value, err
		})
	}))
	p := &dbCardProvider{client: client, region: renderregion.JP}
	waiter, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := p.Filter(waiter, &CardFilter{}); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("load did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation blocked")
	}
	p.resetMasterdataCache()
	if _, err := client.Card.UpdateOneID(row.ID).SetPrefix("new").Save(ctx); err != nil {
		t.Fatal(err)
	}
	cards, err := p.Filter(ctx, &CardFilter{})
	if err != nil || cards[0].Prefix != "new" {
		t.Fatal("new generation did not load")
	}
	unblock()
	p.cardLoads.Do("0", func() (any, error) { return nil, nil })
	cards, err = p.Filter(ctx, &CardFilter{})
	if err != nil || cards[0].Prefix != "new" || queries.Load() != 2 {
		t.Fatal("old load overwrote new generation")
	}
}
