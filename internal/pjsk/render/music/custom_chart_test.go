package music

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"reflect"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type customChartClientStub struct {
	body         []byte
	published    *sekaiapi.UserCustomMusicScorePublishedResponse
	publishedErr error
}

func (s customChartClientStub) GetCustomMusicScorePublished(_ string, _ string) (*sekaiapi.UserCustomMusicScorePublishedResponse, error) {
	if s.publishedErr != nil {
		return nil, s.publishedErr
	}
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
	region         renderregion.Value
	musicTags      []string
	customTagNames map[int]string
}

func (s *customChartDirectSource) DefaultRegion() renderregion.Value {
	if s.region != "" {
		return s.region
	}
	return renderregion.JP
}

func (s *customChartDirectSource) SearchMusic(string) (*masterdata.Music, error) {
	return nil, fmt.Errorf("music not found")
}

func (s *customChartDirectSource) GetMusicTags(int) ([]string, error) {
	return append([]string(nil), s.musicTags...), nil
}

func (s *customChartDirectSource) GetCustomMusicScoreTagNames(tagIDs []int) []string {
	result := make([]string, 0, len(tagIDs))
	seen := make(map[int]struct{}, len(tagIDs))
	for _, id := range tagIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if value := s.customTagNames[id]; value != "" {
			result = append(result, value)
		}
	}
	return result
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

func TestBuildCustomMusicChartRequestRejectsNonJPRegion(t *testing.T) {
	source := &customChartDirectSource{
		region: renderregion.CN,
		vocalBuilderTestSource: &vocalBuilderTestSource{
			music: &masterdata.Music{
				ID:              47,
				Title:           "メルト",
				AssetBundleName: "jacket_s_047",
			},
		},
	}
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	controller.SetCustomMusicScoreClient(customChartClientStub{})

	_, err := controller.BuildMusicChartRequest(ChartQuery{
		Query:  "_g5yakrvqobnfq6hafdob7ed8jwm",
		Region: "cn",
	})
	if err == nil || err.Error() != "当前服务器暂未支持自定义谱面请使用jp前缀查询" {
		t.Fatalf("BuildMusicChartRequest() error = %v", err)
	}
}

func TestBuildCustomMusicChartRequestMapsCustomScoreNotFound(t *testing.T) {
	source := &customChartDirectSource{vocalBuilderTestSource: &vocalBuilderTestSource{
		music: &masterdata.Music{
			ID:              47,
			Title:           "メルト",
			AssetBundleName: "jacket_s_047",
		},
	}}
	controller := NewController(source, nil, assets.NewAssetHelper("", nil), nil, nil)
	controller.SetCustomMusicScoreClient(customChartClientStub{
		publishedErr: &sekaiapi.APIError{
			StatusCode: 500,
			Message:    "upstream failed: status=404 body=<html>Not Found</html>",
		},
	})

	_, err := controller.BuildMusicChartRequest(ChartQuery{
		Query:  "_g5yakrvqobnfq6hafdob7ed8jwm",
		Region: "jp",
	})
	if err == nil || err.Error() != "未找到对应自定义谱面" {
		t.Fatalf("BuildMusicChartRequest() error = %v", err)
	}
}

func TestBuildCustomMusicDetailRequestUsesDirectPublishedID(t *testing.T) {
	chartJSON := `{"MusicScoreEventDataList":[{"id":1,"ticks":0,"eventType":0,"changeValue":180},{"id":2,"ticks":480,"eventType":0,"changeValue":210}],"NoteList":[{"id":1,"ticks":0,"previousConnectionId":-1,"nextConnectionId":2,"isSkip":false},{"id":2,"ticks":491760,"previousConnectionId":1,"nextConnectionId":-1,"isSkip":false}]}`
	scoreBody := gzipBytes(t, []byte(chartJSON))
	source := &customChartDirectSource{vocalBuilderTestSource: &vocalBuilderTestSource{
		music: &masterdata.Music{
			ID:              47,
			Title:           "メルト",
			Composer:        "ryo",
			Lyricist:        "ryo",
			Arranger:        "ryo",
			AssetBundleName: "jacket_s_047",
			PublishedAt:     1700000000000,
			Categories:      []string{"mv_3d"},
		},
		difficulties: []*masterdata.MusicDifficulty{{MusicID: 47, MusicDifficulty: "master", PlayLevel: 30, TotalNoteCount: 1000}},
	}, musicTags: []string{"vocaloid", "light_music_club"}, customTagNames: map[int]string{
		4: "観賞用譜面",
		6: "物量譜面",
		9: "縦連",
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
			CustomMusicScoreTags:   []int{9, 6, 4},
			Description:            "hello chart",
			PreviewStartTimeSec:    12.5,
			PublishedAt:            1710000000000,
			ReviewCount:            23,
			PlayCount:              456,
			FullComboRate:          0.125,
			UserCustomMusicScoreInfoJSON: &sekaiapi.UserCustomMusicScoreInfo{
				MusicID:                  47,
				Title:                    "Direct Custom",
				UserCustomMusicScorePath: "hash-a/hash-b",
			},
		},
	})

	req, err := controller.BuildMusicDetailRequest(Query{
		Query:  "_g5yakrvqobnfq6hafdob7ed8jwm",
		Region: "jp",
	})
	if err != nil {
		t.Fatalf("BuildMusicDetailRequest() error = %v", err)
	}
	if req.CustomChartInfo == nil {
		t.Fatal("CustomChartInfo is nil")
	}
	if req.CustomChartInfo.Title != "Direct Custom" || req.CustomChartInfo.Author != "Maker" {
		t.Fatalf("unexpected custom chart info: %+v", req.CustomChartInfo)
	}
	if req.CustomChartInfo.NoteCount != 2050 || req.CustomChartInfo.BPM != "180 / 210" {
		t.Fatalf("unexpected custom chart stats: %+v", req.CustomChartInfo)
	}
	if len(req.Difficulty.Level) != 1 || req.Difficulty.Level[0] != 31 || req.Difficulty.NoteCount[0] != 2050 {
		t.Fatalf("unexpected difficulty: %+v", req.Difficulty)
	}
	if want := []string{"縦連", "物量譜面", "観賞用譜面"}; !reflect.DeepEqual(req.CustomChartInfo.Tags, want) {
		t.Fatalf("unexpected tags: got %v want %v", req.CustomChartInfo.Tags, want)
	}
	if len(req.Alias) != 0 || req.LeaderboardMatrix != nil {
		t.Fatalf("custom detail should not include alias or leaderboard: alias=%v leaderboard=%v", req.Alias, req.LeaderboardMatrix)
	}
}

func TestParseCustomMusicScoreStatsDoesNotFallbackToNoteListLength(t *testing.T) {
	stats := parseCustomMusicScoreStats(`{"MusicScoreEventDataList":[],"NoteList":[{"id":1},{"id":2},{"id":3}]}`)
	if stats.NoteCount != 0 {
		t.Fatalf("NoteCount = %d, want 0", stats.NoteCount)
	}

	stats = parseCustomMusicScoreStats(`{"chart":{"totalNoteCount":2050,"MusicScoreEventDataList":[],"NoteList":[{"id":1}]}}`)
	if stats.NoteCount != 0 {
		t.Fatalf("NoteCount = %d, want 0", stats.NoteCount)
	}

	stats = parseCustomMusicScoreStats(`{"MusicScoreEventDataList":[],"NoteList":[{"id":1,"ticks":0,"previousConnectionId":-1,"nextConnectionId":2,"isSkip":false},{"id":2,"ticks":491760,"previousConnectionId":1,"nextConnectionId":-1,"isSkip":false}]}`)
	if stats.NoteCount != 2050 {
		t.Fatalf("NoteCount = %d, want 2050", stats.NoteCount)
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
