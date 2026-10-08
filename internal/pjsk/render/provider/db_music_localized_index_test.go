package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent"
	"haruki-cloud/database/sekai/music"
)

func TestMusicLocalizedTitlesUseOneNarrowIndexQuery(t *testing.T) {
	ctx := t.Context()
	provider := openProviderBehaviorDB(t, "music_title_bulk")
	for _, region := range []string{"jp", "tw"} {
		for id := int64(1); id <= 30; id++ {
			title := fmt.Sprintf(" Song %d ", id)
			if region == "tw" {
				title = fmt.Sprintf("譯名 %d", id)
			}
			if _, err := provider.client.Music.Create().SetGameID(id).SetTitle(title).SetPronunciation(fmt.Sprintf("song %d", id)).SetServerRegion(region).Save(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := provider.client.Musicdifficultie.Create().SetGameID(id).SetMusicID(id).SetMusicDifficulty("master").SetPlayLevel(30).SetServerRegion(region).Save(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	var queries, titleQueries atomic.Int32
	provider.client.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries.Add(1)
			queryCtx := ent.QueryFromContext(ctx)
			if queryCtx.Type == "Music" && queryCtx.Op == ent.OpQuerySelect {
				titleQueries.Add(1)
				want := []string{music.FieldGameID, music.FieldTitle, music.FieldPronunciation}
				if !reflect.DeepEqual(queryCtx.Fields, want) {
					t.Errorf("localized title fields=%v want %v", queryCtx.Fields, want)
				}
			}
			return next.Query(ctx, query)
		})
	}))
	readList := func() {
		t.Helper()
		all := provider.musics.GetAll(ctx)
		if len(all) != 30 {
			t.Fatalf("music count=%d", len(all))
		}
		if err := provider.musics.PreloadDifficulties(ctx); err != nil {
			t.Fatal(err)
		}
		for _, row := range all {
			if _, err := provider.musics.GetByID(ctx, row.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := provider.musics.GetDifficulties(ctx, row.ID); err != nil {
				t.Fatal(err)
			}
			titles, err := provider.musics.GetLocalizedTitles(ctx, row.ID)
			want := []string{fmt.Sprintf("Song %d", row.ID), fmt.Sprintf("譯名 %d", row.ID)}
			if err != nil || !reflect.DeepEqual(titles, want) {
				t.Fatalf("id=%d titles=%v want=%v err=%v", row.ID, titles, want, err)
			}
			titles[0] = "mutated"
		}
	}
	readList()
	if queries.Load() != 4 || titleQueries.Load() != 1 {
		t.Fatalf("cold queries=%d titles=%d want 4/1", queries.Load(), titleQueries.Load())
	}
	queries.Store(0)
	readList()
	if queries.Load() != 0 {
		t.Fatalf("warm queries=%d want 0", queries.Load())
	}
	missing, err := provider.musics.GetLocalizedTitles(ctx, 1000)
	if err != nil || missing == nil || len(missing) != 0 {
		t.Fatalf("missing titles=%#v err=%v", missing, err)
	}
	if queries.Load() != 0 {
		t.Fatal("missing title was not served from the index")
	}

	if _, err := provider.client.Music.Update().Where(music.GameIDEQ(1), music.ServerRegionEQ("tw")).SetTitle("新譯名").Save(ctx); err != nil {
		t.Fatal(err)
	}
	provider.musics.resetLocalMasterdataCache()
	titles, err := provider.musics.GetLocalizedTitles(ctx, 1)
	if err != nil || !reflect.DeepEqual(titles, []string{"Song 1", "新譯名"}) {
		t.Fatalf("reset titles=%v err=%v", titles, err)
	}
	if queries.Load() != 1 {
		t.Fatalf("reset query count=%d", queries.Load())
	}
}

func TestMusicTitleIndexRetriesFailureAndExpires(t *testing.T) {
	ctx := t.Context()
	provider := openProviderBehaviorDB(t, "music_title_retry")
	var fail atomic.Bool
	fail.Store(true)
	var calls atomic.Int32
	provider.client.Music.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			calls.Add(1)
			if fail.Load() {
				return nil, errors.New("unavailable")
			}
			return next.Query(ctx, query)
		})
	}))
	if _, err := provider.musics.GetLocalizedTitles(ctx, 1); err == nil {
		t.Fatal("failed query succeeded")
	}
	fail.Store(false)
	if _, err := provider.musics.GetLocalizedTitles(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("failed index was cached: calls=%d", calls.Load())
	}
	if _, err := provider.client.Music.Create().SetGameID(1).SetTitle("new title").SetServerRegion("jp").Save(ctx); err != nil {
		t.Fatal(err)
	}
	provider.musics.localizedTitles.mu.Lock()
	provider.musics.localizedTitles.loadedAt = time.Now().Add(-dbBulkIndexTTL)
	provider.musics.localizedTitles.mu.Unlock()
	if titles, err := provider.musics.GetLocalizedTitles(ctx, 1); err != nil || len(titles) != 0 {
		t.Fatalf("expired title index was not served while refreshing: %v, %v", titles, err)
	}
	waitForIndexRefresh(t, func() bool {
		titles, err := provider.musics.GetLocalizedTitles(ctx, 1)
		return err == nil && reflect.DeepEqual(titles, []string{"new title"})
	})
	if calls.Load() != 3 {
		t.Fatalf("expired title index calls=%d", calls.Load())
	}
}

func TestLocalizedTitleIndexMatchesLegacyQueryCandidates(t *testing.T) {
	ctx := t.Context()
	provider := openProviderBehaviorDB(t, "music_title_legacy_order")
	rows := []struct {
		id                           int64
		region, title, pronunciation string
	}{
		{1, "tw", " 圓舞曲 ", "walTZ"},
		{1, "kr", "왈츠", "WALTZ"},
		{1, "cn", "圆舞曲", " waltz "},
		{1, "en", "Waltz", "Waltz"},
		{1, "jp", "ワルツ", "わるつ"},
		{2, "tw", " FIRST ", "Common"},
		{2, "jp", "First", "common"},
		{2, "en", "FIRST", "COMMON"},
		{2, "cn", "first", "Common"},
		{3, "jp", " ", ""},
	}
	for _, row := range rows {
		if _, err := provider.client.Music.Create().SetGameID(row.id).SetTitle(row.title).SetPronunciation(row.pronunciation).SetServerRegion(row.region).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int{1, 2, 3, 999} {
		// Pin the old query and deduplication together: title order affects
		// which language-specific candidate the music renderer selects.
		entities, err := provider.client.Music.Query().Where(music.GameIDEQ(int64(id))).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := make([]string, 0, len(entities)*2)
		seen := make(map[string]struct{})
		for _, entity := range entities {
			for _, raw := range []string{entity.Title, entity.Pronunciation} {
				title := strings.TrimSpace(raw)
				key := strings.ToLower(title)
				if title == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				want = append(want, title)
			}
		}
		got, err := provider.musics.GetLocalizedTitles(ctx, id)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("music %d indexed titles=%v legacy=%v err=%v", id, got, want, err)
		}
	}
}
