package score

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestBuildScoreControlRequestNormalizesLegacyCoverPath(t *testing.T) {
	controller := NewController(nil)
	req, err := controller.BuildScoreControlRequest(drawing.ScoreControlRequest{
		MusicID:        1,
		TargetPoint:    100,
		MusicCoverPath: "jacket/jacket_s_001_rip/jacket_s_001.png",
	})
	if err != nil {
		t.Fatalf("BuildScoreControlRequest failed: %v", err)
	}
	if req.MusicCoverPath != "music/jacket/jacket_s_001/jacket_s_001.png" {
		t.Fatalf("unexpected cover path: %s", req.MusicCoverPath)
	}
}

func TestBuildCustomRoomScoreRequestNormalizesNestedCoverPaths(t *testing.T) {
	controller := NewController(nil)
	req, err := controller.BuildCustomRoomScoreRequest(drawing.CustomRoomScoreRequest{
		TargetPoint:    100,
		CandidatePairs: [][]int{{1, 2}},
		MusicListMap: map[int][]map[string]any{
			10: {
				{"music_cover": "jacket/jacket_s_002_rip/jacket_s_002.png"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildCustomRoomScoreRequest failed: %v", err)
	}
	got, _ := req.MusicListMap[10][0]["music_cover"].(string)
	if got != "music/jacket/jacket_s_002/jacket_s_002.png" {
		t.Fatalf("unexpected nested cover path: %s", got)
	}
}

func TestBuildMusicMetaRequestRejectsEmptyList(t *testing.T) {
	controller := NewController(nil)
	_, err := controller.BuildMusicMetaRequest(nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBuildMusicMetaRequestNormalizesCoverPath(t *testing.T) {
	controller := NewController(nil)
	req, err := controller.BuildMusicMetaRequest([]drawing.MusicMetaRequest{
		{MusicID: 1, MusicCoverPath: "jacket/jacket_s_004_rip/jacket_s_004.png"},
	})
	if err != nil {
		t.Fatalf("BuildMusicMetaRequest failed: %v", err)
	}
	if req[0].MusicCoverPath != "music/jacket/jacket_s_004/jacket_s_004.png" {
		t.Fatalf("unexpected meta cover path: %s", req[0].MusicCoverPath)
	}
}

func TestBuildMusicBoardRequestNormalizesItemCoverPath(t *testing.T) {
	controller := NewController(nil)
	req, err := controller.BuildMusicBoardRequest(drawing.MusicBoardRequest{
		Items: []drawing.MusicBoardItem{
			{MusicCoverPath: "jacket/jacket_s_003_rip/jacket_s_003.png"},
		},
	})
	if err != nil {
		t.Fatalf("BuildMusicBoardRequest failed: %v", err)
	}
	if req.Items[0].MusicCoverPath != "music/jacket/jacket_s_003/jacket_s_003.png" {
		t.Fatalf("unexpected board cover path: %s", req.Items[0].MusicCoverPath)
	}
}
