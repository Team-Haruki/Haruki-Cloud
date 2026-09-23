package provider

import (
	"context"
	"encoding/json"
	"testing"

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
	} {
		_, err := client.Musiccategorie.Create().SetGameID(row.id).SetMusicID(row.musicID).SetMusicCategoryName(row.name).SetServerRegion(row.region).Save(ctx)
		testutil.Require(t, err == nil, "create category %d: %v", row.id, err)
	}

	first, err := p.musics.GetByID(ctx, 1)
	testutil.Require(t, err == nil, "GetByID(1): %v", err)
	testutil.Require(t, len(first.Categories) == 3 && first.Categories[0] == "mv" && first.Categories[1] == "mv_2d" && first.Categories[2] == "original",
		"categories from table (ordered by id) = %v", first.Categories)
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
	writeTestFile(t, root, "musicCategories.json", `[{"id":2,"musicId":1,"musicCategoryName":"mv_2d"},{"id":1,"musicId":1,"musicCategoryName":"mv"},{"id":3,"musicId":2,"musicCategoryName":"ignored"}]`)
	local := NewLocalProvider(root, renderregion.JP)

	first, err := local.Musics().GetByID(context.Background(), 1)
	testutil.Require(t, err == nil, "local GetByID(1): %v", err)
	testutil.Require(t, len(first.Categories) == 2 && first.Categories[0] == "mv" && first.Categories[1] == "mv_2d", "local categories = %v", first.Categories)
	second, err := local.Musics().GetByID(context.Background(), 2)
	testutil.Require(t, err == nil, "local GetByID(2): %v", err)
	testutil.Require(t, len(second.Categories) == 1 && second.Categories[0] == "image", "local own categories = %v", second.Categories)
}
