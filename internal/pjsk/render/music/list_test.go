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
