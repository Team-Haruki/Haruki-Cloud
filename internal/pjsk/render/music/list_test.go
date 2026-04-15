package music

import (
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestBuildMusicListRequestFiltersByApprovedAlias(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {ID: 1, Title: "Song A", AssetBundleName: "jacket_a"},
			2: {ID: 2, Title: "Song B", AssetBundleName: "jacket_b"},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 31},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "master", PlayLevel: 30},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	controller.SetAliasResolver(&lookupTestAliasResolver{
		ids: map[string]int{"blue song": 1},
	})

	req, err := controller.BuildMusicListRequest(ListQuery{
		Region:     "jp",
		Difficulty: "master",
		Keyword:    "blue song",
	})
	if err != nil {
		t.Fatalf("BuildMusicListRequest() error = %v", err)
	}
	if len(req.MusicList) != 1 {
		t.Fatalf("expected 1 music item, got %d", len(req.MusicList))
	}
	if id, ok := req.MusicList[0]["id"].(int); !ok || id != 1 {
		t.Fatalf("unexpected music list item: %+v", req.MusicList[0])
	}
}

func TestBuildMusicListRequestSortsByLevelSeqAndID(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			10: {ID: 10, Seq: 30, Title: "Song A", AssetBundleName: "jacket_a", PublishedAt: 1000},
			11: {ID: 11, Seq: 10, Title: "Song B", AssetBundleName: "jacket_b", PublishedAt: 3000},
			12: {ID: 12, Seq: 20, Title: "Song C", AssetBundleName: "jacket_c", PublishedAt: 2000},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			10: {
				{MusicID: 10, MusicDifficulty: "master", PlayLevel: 31},
			},
			11: {
				{MusicID: 11, MusicDifficulty: "master", PlayLevel: 30},
			},
			12: {
				{MusicID: 12, MusicDifficulty: "master", PlayLevel: 31},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	req, err := controller.BuildMusicListRequest(ListQuery{
		Region:     "jp",
		Difficulty: "master",
	})
	if err != nil {
		t.Fatalf("BuildMusicListRequest() error = %v", err)
	}
	if len(req.MusicList) != 3 {
		t.Fatalf("expected 3 music items, got %d", len(req.MusicList))
	}

	gotIDs := []int{
		req.MusicList[0]["id"].(int),
		req.MusicList[1]["id"].(int),
		req.MusicList[2]["id"].(int),
	}
	wantIDs := []int{11, 12, 10}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("unexpected sorted ids: got=%v want=%v", gotIDs, wantIDs)
		}
	}

	gotOrders := []int64{
		req.MusicList[0]["release_at"].(int64),
		req.MusicList[1]["release_at"].(int64),
		req.MusicList[2]["release_at"].(int64),
	}
	wantOrders := []int64{10, 20, 30}
	for i := range wantOrders {
		if gotOrders[i] != wantOrders[i] {
			t.Fatalf("unexpected display orders: got=%v want=%v", gotOrders, wantOrders)
		}
	}
}

func TestBuildMusicListRequestSupportsExplicitItems(t *testing.T) {
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			10: {ID: 10, Seq: 30, Title: "Song A", AssetBundleName: "jacket_a", PublishedAt: 1000},
			11: {ID: 11, Seq: 10, Title: "Song B", AssetBundleName: "jacket_b", PublishedAt: 3000},
			12: {ID: 12, Seq: 20, Title: "Song C", AssetBundleName: "jacket_c", PublishedAt: 2000},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			10: {
				{MusicID: 10, MusicDifficulty: "expert", PlayLevel: 27},
			},
			11: {
				{MusicID: 11, MusicDifficulty: "expert", PlayLevel: 26},
			},
			12: {
				{MusicID: 12, MusicDifficulty: "expert", PlayLevel: 27},
			},
		},
	}

	title := "BPM 200 EXPERT 匹配结果"
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	req, err := controller.BuildMusicListRequest(ListQuery{
		Region:     "jp",
		Difficulty: "expert",
		Items: []ListItemQuery{
			{MusicID: 10, Difficulty: "expert", Level: 27},
			{MusicID: 11, Difficulty: "expert", Level: 26},
			{MusicID: 12, Difficulty: "expert"},
		},
		Title: &title,
	})
	if err != nil {
		t.Fatalf("BuildMusicListRequest() error = %v", err)
	}
	if req.Title == nil || *req.Title != title {
		t.Fatalf("unexpected title: %+v", req.Title)
	}
	if req.RequiredDifficulties != "expert" {
		t.Fatalf("unexpected required difficulty: %q", req.RequiredDifficulties)
	}
	if len(req.MusicList) != 3 {
		t.Fatalf("expected 3 music items, got %d", len(req.MusicList))
	}

	gotIDs := []int{
		req.MusicList[0]["id"].(int),
		req.MusicList[1]["id"].(int),
		req.MusicList[2]["id"].(int),
	}
	wantIDs := []int{11, 12, 10}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("unexpected sorted ids: got=%v want=%v", gotIDs, wantIDs)
		}
	}
}
