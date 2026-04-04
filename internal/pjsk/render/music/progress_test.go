package music

import (
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func TestBuildMusicProgressRequestUsesQueryUserResults(t *testing.T) {
	now := time.Now().UnixMilli()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1:   {ID: 1, Title: "Song A", PublishedAt: now - 1000},
			2:   {ID: 2, Title: "Song B", PublishedAt: now - 1000},
			3:   {ID: 3, Title: "Song C", PublishedAt: now + 100000},
			4:   {ID: 4, Title: "Song D", PublishedAt: now - 1000},
			241: {ID: 241, Title: "Hidden Song", PublishedAt: now - 1000},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "expert", PlayLevel: 26},
			},
			2: {
				{MusicID: 2, MusicDifficulty: "expert", PlayLevel: 26},
			},
			3: {
				{MusicID: 3, MusicDifficulty: "expert", PlayLevel: 26},
			},
			4: {
				{MusicID: 4, MusicDifficulty: "expert", PlayLevel: 27},
			},
			241: {
				{MusicID: 241, MusicDifficulty: "expert", PlayLevel: 26},
			},
		},
	}

	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	req, err := controller.BuildMusicProgressRequest(ProgressQuery{
		Region:     "jp",
		Difficulty: "expert",
		UserResults: map[int]string{
			1:   "clear",
			2:   "ap",
			3:   "fc",
			241: "ap",
		},
	})
	if err != nil {
		t.Fatalf("BuildMusicProgressRequest() error = %v", err)
	}
	if req.Difficulty != "expert" {
		t.Fatalf("unexpected difficulty: %q", req.Difficulty)
	}
	if len(req.Counts) != 2 {
		t.Fatalf("expected 2 level groups, got %+v", req.Counts)
	}

	if req.Counts[0].Level != 26 || req.Counts[0].Total != 2 || req.Counts[0].Clear != 2 || req.Counts[0].Fc != 1 || req.Counts[0].Ap != 1 {
		t.Fatalf("unexpected level 26 counts: %+v", req.Counts[0])
	}
	if req.Counts[1].Level != 27 || req.Counts[1].Total != 1 || req.Counts[1].NotClear != 1 || req.Counts[1].Clear != 0 {
		t.Fatalf("unexpected level 27 counts: %+v", req.Counts[1])
	}
}
