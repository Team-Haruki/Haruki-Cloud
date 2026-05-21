package music

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type customChartClientStub struct {
	body      []byte
	published *sekaiapi.UserCustomMusicScorePublishedResponse
}

func (s customChartClientStub) GetCustomMusicScorePublished(_ string, _ string) (*sekaiapi.UserCustomMusicScorePublishedResponse, error) {
	if s.published == nil {
		return nil, sekaiapi.ErrUserNotFound
	}
	return s.published, nil
}

func (s customChartClientStub) GetCustomMusicScore(_ string, scorePath string) ([]byte, error) {
	return append([]byte(nil), s.body...), nil
}

type customChartDirectSource struct {
	*vocalBuilderTestSource
}

func (s *customChartDirectSource) SearchMusic(string) (*masterdata.Music, error) {
	return nil, fmt.Errorf("music not found")
}

func TestBuildCustomMusicChartRequestUsesDirectPublishedIDWithoutSnapshot(t *testing.T) {
	chartJSON := `{"MusicScoreEventDataList":[{"id":1,"ticks":0,"eventType":0,"changeValue":180}],"NoteList":[]}`
	scoreBody := gzipBytes(t, []byte(chartJSON))
	source := &customChartDirectSource{vocalBuilderTestSource: &vocalBuilderTestSource{
		music: &masterdata.Music{
			ID:              47,
			Title:           "メルト",
			Composer:        "ryo",
			Arranger:        "ryo",
			AssetBundleName: "jacket_s_047",
		},
		difficulties: []*masterdata.MusicDifficulty{{MusicID: 47, MusicDifficulty: "master", PlayLevel: 30}},
	}}
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	controller.SetCustomMusicScoreClient(customChartClientStub{
		body: scoreBody,
		published: &sekaiapi.UserCustomMusicScorePublishedResponse{
			UserCustomMusicScoreID: "_g5yakrvqobnfq6hafdob7ed8jwm",
			UserName:               "Maker",
			MusicID:                47,
			MusicDifficultyType:    "master",
			PlayLevel:              31,
			UserCustomMusicScoreInfoJSON: &sekaiapi.UserCustomMusicScoreInfo{
				MusicID:                  47,
				Title:                    "Direct Custom",
				UserCustomMusicScorePath: "hash-a/hash-b",
			},
		},
	})

	req, err := controller.BuildMusicChartRequest(ChartQuery{
		Query:  "_g5yakrvqobnfq6hafdob7ed8jwm",
		Region: "jp",
		Style:  "white",
	})
	if err != nil {
		t.Fatalf("BuildMusicChartRequest() error = %v", err)
	}
	if req.ChartJSON == nil || *req.ChartJSON != chartJSON {
		t.Fatalf("ChartJSON = %q, want %q", valueOrEmpty(req.ChartJSON), chartJSON)
	}
	if req.MusicID != "_g5yakrvqobnfq6hafdob7ed8jwm" {
		t.Fatalf("MusicID = %#v", req.MusicID)
	}
	if req.Title != "メルト/Direct Custom" || req.Artist != "Maker/_g5yakrvqobnfq6hafdob7ed8jwm" {
		t.Fatalf("unexpected custom chart meta: title=%q artist=%q", req.Title, req.Artist)
	}
	if req.Difficulty != "master" || req.PlayLevel != 31 {
		t.Fatalf("unexpected difficulty meta: difficulty=%q playLevel=%#v", req.Difficulty, req.PlayLevel)
	}
}

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
