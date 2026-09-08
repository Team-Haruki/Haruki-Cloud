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

func TestMusicDifficultyBulkIndexLoadsOnceAndRefreshes(t *testing.T) {
	ctx := t.Context()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:difficulty_bulk_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	for _, region := range []string{"jp", "tw"} {
		for id := int64(1); id <= 12; id++ {
			_, err := client.Musicdifficultie.Create().SetGameID(id).SetMusicID(id).SetMusicDifficulty("master").SetPlayLevel(30).SetServerRegion(region).Save(ctx)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var queries atomic.Int32
	client.Musicdifficultie.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) { queries.Add(1); return next.Query(ctx, q) })
	}))
	p := &dbMusicProvider{client: client, region: renderregion.JP}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if err := p.PreloadDifficulties(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for id := 1; id <= 12; id++ {
		rows, err := p.GetDifficulties(ctx, id)
		if err != nil || len(rows) != 1 || rows[0].MusicID != id {
			t.Fatalf("id=%d rows=%v err=%v", id, rows, err)
		}
		rows[0].PlayLevel = 99
	}
	rows, err := p.GetDifficulties(ctx, 1)
	if err != nil || rows[0].PlayLevel != 30 {
		t.Fatalf("caller mutation escaped: %v %v", rows, err)
	}
	if _, err := p.GetDifficulties(ctx, 1000); err == nil {
		t.Fatal("missing music should retain not-found error")
	}
	if n := queries.Load(); n != 1 {
		t.Fatalf("queries=%d want one regional query", n)
	}
	if _, err := client.Musicdifficultie.Update().SetPlayLevel(31).Save(ctx); err != nil {
		t.Fatal(err)
	}
	p.difficultyMu.Lock()
	p.difficultyLoadedAt = time.Now().Add(-dbBulkIndexTTL)
	p.difficultyMu.Unlock()
	if err := p.PreloadDifficulties(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err = p.GetDifficulties(ctx, 1)
	if err != nil || rows[0].PlayLevel != 31 || queries.Load() != 2 {
		t.Fatalf("refresh rows=%v err=%v queries=%d", rows, err, queries.Load())
	}
}

func TestMusicDifficultyIndexCancellationAndReset(t *testing.T) {
	ctx := t.Context()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:difficulty_reset_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	defer client.Close()
	row, err := client.Musicdifficultie.Create().SetGameID(1).SetMusicID(1).SetMusicDifficulty("master").SetPlayLevel(30).SetServerRegion("jp").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int32
	client.Musicdifficultie.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(loadCtx context.Context, q ent.Query) (ent.Value, error) {
			result, err := next.Query(loadCtx, q)
			if calls.Add(1) == 1 {
				close(entered)
				<-release
				close(finished)
			}
			return result, err
		})
	}))
	p := &dbMusicProvider{client: client, region: renderregion.JP}
	waiting, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- p.PreloadDifficulties(waiting) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("load did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("caller cannot cancel shared index wait")
	}
	p.resetLocalMasterdataCache()
	if _, err := client.Musicdifficultie.UpdateOneID(row.ID).SetPlayLevel(32).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.PreloadDifficulties(ctx); err != nil {
		t.Fatal(err)
	}
	unblock()
	<-finished
	// Wait for the old generation's flight to finish publishing before checking.
	p.difficultyLoads.Do("0", func() (any, error) { return nil, nil })
	rows, err := p.GetDifficulties(ctx, 1)
	if err != nil || len(rows) != 1 || rows[0].PlayLevel != 32 {
		t.Fatalf("old generation replaced reset index: %v %v", rows, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("generation loads=%d", calls.Load())
	}
}
