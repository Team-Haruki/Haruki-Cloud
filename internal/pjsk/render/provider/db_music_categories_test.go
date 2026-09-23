package provider

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/testutil"
)

func TestDBMusicProviderFillsCategoriesFromMusicCategoriesTable(t *testing.T) {
	ctx := context.Background()
	p := openProviderBehaviorDB(t, "music_categories")
	client := p.client

	_, err := client.Music.Create().SetGameID(1).SetTitle("no categories").SetCategories(json.RawMessage(`[]`)).SetPublishedAt(10).SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create music 1: %v", err)
	_, err = client.Music.Create().SetGameID(2).SetTitle("own categories").SetCategories(json.RawMessage(`["image"]`)).SetPublishedAt(20).SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create music 2: %v", err)
	for _, row := range []struct {
		id, musicID  int64
		name, region string
	}{
		{3, 1, "original", "jp"},
		{1, 1, "mv", "jp"},
		{2, 1, "mv_2d", "jp"},
		{4, 2, "ignored", "jp"},
		{5, 1, "other_region", "tw"},
		{6, 1, "mv_2d", "jp"}, // production repeats mv_2d for some musics
	} {
		_, err := client.Musiccategorie.Create().SetGameID(row.id).SetMusicID(row.musicID).SetMusicCategoryName(row.name).SetServerRegion(row.region).Save(ctx)
		testutil.Require(t, err == nil, "create category %d: %v", row.id, err)
	}

	first, err := p.musics.GetByID(ctx, 1)
	testutil.Require(t, err == nil, "GetByID(1): %v", err)
	testutil.Require(t, len(first.Categories) == 3 && first.Categories[0] == "mv" && first.Categories[1] == "mv_2d" && first.Categories[2] == "original",
		"categories from table (ordered by id, de-duplicated) = %v", first.Categories)
	second, err := p.musics.GetByID(ctx, 2)
	testutil.Require(t, err == nil, "GetByID(2): %v", err)
	testutil.Require(t, len(second.Categories) == 1 && second.Categories[0] == "image", "own categories must be kept: %v", second.Categories)

	p.musics.resetLocalMasterdataCache()
	all := p.musics.GetAll(ctx)
	testutil.Require(t, len(all) == 2, "GetAll = %d musics", len(all))
	for _, m := range all {
		switch m.ID {
		case 1:
			testutil.Require(t, len(m.Categories) == 3 && m.Categories[0] == "mv", "GetAll categories for 1 = %v", m.Categories)
		case 2:
			testutil.Require(t, len(m.Categories) == 1 && m.Categories[0] == "image", "GetAll categories for 2 = %v", m.Categories)
		}
	}
}

func TestLocalMusicProviderFillsCategoriesFromMusicCategoriesFile(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "musics.json", `[{"id":1,"title":"a","publishedAt":1},{"id":2,"title":"b","publishedAt":2,"categories":["image"]}]`)
	writeTestFile(t, root, "musicCategories.json", `[{"id":2,"musicId":1,"musicCategoryName":"mv_2d"},{"id":1,"musicId":1,"musicCategoryName":"mv"},{"id":3,"musicId":2,"musicCategoryName":"ignored"},{"id":4,"musicId":1,"musicCategoryName":"mv_2d"}]`)
	local := NewLocalProvider(root, renderregion.JP)

	first, err := local.Musics().GetByID(context.Background(), 1)
	testutil.Require(t, err == nil, "local GetByID(1): %v", err)
	testutil.Require(t, len(first.Categories) == 2 && first.Categories[0] == "mv" && first.Categories[1] == "mv_2d", "local categories (de-duplicated) = %v", first.Categories)
	second, err := local.Musics().GetByID(context.Background(), 2)
	testutil.Require(t, err == nil, "local GetByID(2): %v", err)
	testutil.Require(t, len(second.Categories) == 1 && second.Categories[0] == "image", "local own categories = %v", second.Categories)
}

// A failed category fill must not cache the music with empty categories.
func TestDBMusicProviderDoesNotCacheMusicsWhenCategoryFillFails(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:music_categories_fail_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client := sekaienttest.Open(t, "sqlite3", dsn)
	p := NewDatabaseProvider(client, renderregion.JP)
	_, err := client.Music.Create().SetGameID(1).SetTitle("needs categories").SetCategories(json.RawMessage(`[]`)).SetPublishedAt(10).SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create music: %v", err)

	raw, err := sql.Open("sqlite3", dsn)
	testutil.Require(t, err == nil, "open raw sqlite: %v", err)
	t.Cleanup(func() { _ = raw.Close() })
	_, err = raw.Exec(`DROP TABLE musiccategories`)
	testutil.Require(t, err == nil, "drop musiccategories: %v", err)

	served, err := p.musics.GetByID(ctx, 1)
	testutil.Require(t, err == nil && served != nil && len(served.Categories) == 0, "GetByID during outage = %+v, %v", served, err)
	all := p.musics.GetAll(ctx)
	testutil.Require(t, len(all) == 1, "GetAll during outage = %d", len(all))
	p.musics.mu.RLock()
	_, cachedByID := p.musics.musicByID[1]
	cachedList := p.musics.musicList
	p.musics.mu.RUnlock()
	testutil.Require(t, !cachedByID && cachedList == nil, "musics were cached with a failed category fill: byID=%v list=%v", cachedByID, cachedList != nil)

	testutil.Require(t, client.Schema.Create(ctx) == nil, "recreate schema")
	_, err = client.Musiccategorie.Create().SetGameID(1).SetMusicID(1).SetMusicCategoryName("mv").SetServerRegion("jp").Save(ctx)
	testutil.Require(t, err == nil, "create category: %v", err)
	retried, err := p.musics.GetByID(ctx, 1)
	testutil.Require(t, err == nil && len(retried.Categories) == 1 && retried.Categories[0] == "mv", "retry after outage = %+v, %v", retried, err)
	p.musics.mu.RLock()
	_, cachedByID = p.musics.musicByID[1]
	p.musics.mu.RUnlock()
	testutil.Require(t, cachedByID, "successful fill was not cached")
}
