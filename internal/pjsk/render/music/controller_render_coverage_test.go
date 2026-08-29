package music

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/sekai"
)

func newRenderCoverageController(t *testing.T, drawingClient *drawing.HarukiDrawingClient) *Controller {
	t.Helper()
	source := &lookupTestSource{
		musics: map[int]*masterdata.Music{
			1: {
				ID: 1, Seq: 1, Title: "Render Song", Composer: "Composer", Arranger: "Arranger",
				AssetBundleName: "render_song", PublishedAt: time.Now().Add(-time.Hour).UnixMilli(),
				Categories: []string{"mv_3d"},
			},
		},
		difficulties: map[int][]*masterdata.MusicDifficulty{
			1: {
				{MusicID: 1, MusicDifficulty: "hard", PlayLevel: 18, TotalNoteCount: 400},
				{MusicID: 1, MusicDifficulty: "master", PlayLevel: 30, TotalNoteCount: 800},
			},
		},
	}
	return NewController(source, drawingClient, assets.NewAssetHelper("", nil), nil, nil)
}

func TestMusicControllerRenderEntrypointsSuccess(t *testing.T) {
	paths := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths[r.URL.Path]++
		if r.Method != http.MethodPost {
			t.Errorf("method for %s = %s", r.URL.Path, r.Method)
		}
		if r.URL.Path == "/api/pjsk/music/list" {
			if r.URL.Query().Get("show_id") != "true" || r.URL.Query().Get("show_leak") != "false" {
				t.Errorf("music-list query = %s", r.URL.RawQuery)
			}
		}
		_, _ = w.Write([]byte("rendered"))
	}))
	defer server.Close()

	controller := newRenderCoverageController(t, drawing.NewHarukiDrawingClient(server.URL))
	wantBody := []byte("rendered")
	assertRender := func(name string, got []byte, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, wantBody) {
			t.Fatalf("%s = %q, %v", name, got, err)
		}
	}

	got, err := controller.RenderMusicDetail(Query{Query: "music1", Region: "jp", Difficulty: "master"})
	assertRender("detail", got, err)
	got, err = controller.RenderMusicBriefList(BriefListQuery{MusicIDs: []int{1}, Difficulty: "master", Region: "jp"})
	assertRender("brief-list", got, err)
	got, err = controller.RenderMusicList(ListQuery{Difficulty: "master", Region: "jp", ShowID: true})
	assertRender("list", got, err)
	got, err = controller.RenderMusicRewardsDetail(RewardsDetailQuery{Region: "jp"})
	assertRender("rewards-detail", got, err)
	got, err = controller.RenderMusicRewardsDetailFromAchievements(RewardsDetailQuery{Region: "jp"}, []byte(`[]`))
	assertRender("rewards-detail-achievements", got, err)
	got, err = controller.RenderMusicRewardsDetailFromSnapshot(
		RewardsDetailQuery{Region: "jp"},
		&musicSnapshotStub{rawValues: map[string][]byte{"userMusicAchievements": []byte(`[]`)}},
	)
	assertRender("rewards-detail-snapshot", got, err)
	got, err = controller.RenderMusicRewardsBasic(RewardsBasicQuery{Region: "jp"})
	assertRender("rewards-basic", got, err)
	got, err = controller.RenderMusicRewardsBasicEstimate(
		RewardsBasicQuery{Region: "jp"},
		[]sekai.AnotherUserMusicDifficultyClearCount{{MusicDifficultyType: sekai.MusicDifficultyMaster}},
		"",
	)
	assertRender("rewards-basic-estimate", got, err)
	got, err = controller.RenderMusicChart(ChartQuery{Query: "music1", Region: "jp", Difficulty: "master"})
	assertRender("chart", got, err)
	got, err = controller.RenderMusicChartRequest(&drawing.GenerateMusicChartRequest{MusicID: 1})
	assertRender("chart-request", got, err)
	got, err = controller.RenderMusicProgress(ProgressQuery{Region: "jp", Difficulty: "master"})
	assertRender("progress", got, err)
	got, err = controller.RenderMusicProgressFromSnapshot(ProgressQuery{Region: "jp", Difficulty: "master"}, &musicSnapshotStub{}, nil)
	assertRender("progress-snapshot", got, err)

	wantPaths := map[string]int{
		"/api/pjsk/music/detail":         1,
		"/api/pjsk/music/brief-list":     1,
		"/api/pjsk/music/list":           1,
		"/api/pjsk/music/rewards/detail": 2,
		"/api/pjsk/music/rewards/basic":  2,
		"/api/pjsk/chart":                2,
		"/api/pjsk/music/progress":       1,
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("drawing paths = %#v, want %#v", paths, wantPaths)
	}
}

func TestMusicControllerRenderEntrypointsRejectMissingClientsAndPayloads(t *testing.T) {
	controller := newRenderCoverageController(t, nil)
	assertMissingDrawing := func(name string, _ []byte, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "drawing client") {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	got, err := controller.RenderMusicDetail(Query{})
	assertMissingDrawing("detail", got, err)
	got, err = controller.RenderMusicBriefList(BriefListQuery{})
	assertMissingDrawing("brief-list", got, err)
	got, err = controller.RenderMusicList(ListQuery{})
	assertMissingDrawing("list", got, err)
	got, err = controller.RenderMusicRewardsDetail(RewardsDetailQuery{})
	assertMissingDrawing("rewards-detail", got, err)
	got, err = controller.RenderMusicRewardsDetailFromAchievements(RewardsDetailQuery{}, nil)
	assertMissingDrawing("rewards-achievements", got, err)
	got, err = controller.RenderMusicRewardsDetailFromSnapshot(RewardsDetailQuery{}, nil)
	assertMissingDrawing("rewards-snapshot", got, err)
	got, err = controller.RenderMusicRewardsBasic(RewardsBasicQuery{})
	assertMissingDrawing("rewards-basic", got, err)
	got, err = controller.RenderMusicRewardsBasicEstimate(RewardsBasicQuery{}, nil, "")
	assertMissingDrawing("rewards-estimate", got, err)
	got, err = controller.RenderMusicChart(ChartQuery{})
	assertMissingDrawing("chart", got, err)
	got, err = controller.RenderMusicChartRequest(nil)
	assertMissingDrawing("chart-request", got, err)
	got, err = controller.RenderMusicProgress(ProgressQuery{})
	assertMissingDrawing("progress", got, err)
	got, err = controller.RenderMusicProgressFromSnapshot(ProgressQuery{}, nil, nil)
	assertMissingDrawing("progress-snapshot", got, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "drawing failed", http.StatusBadGateway)
	}))
	defer server.Close()
	drawingController := newRenderCoverageController(t, drawing.NewHarukiDrawingClient(server.URL))
	if _, err := drawingController.RenderMusicRewardsBasic(RewardsBasicQuery{Region: "jp"}); err == nil {
		t.Fatal("drawing HTTP error was not returned")
	}
	if _, err := drawingController.RenderMusicChartRequest(nil); err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatalf("nil chart payload error = %v", err)
	}
}

func TestMusicControllerCoverBriefListAndRewardBuilders(t *testing.T) {
	controller := newRenderCoverageController(t, nil)
	emptyController := NewController(nil, nil, assets.NewAssetHelper("", nil), nil, nil)
	if _, err := emptyController.ResolveMusicCoverByTitleOrAlias(Query{Region: "jp"}); err == nil {
		t.Fatal("cover lookup without a source succeeded")
	}
	if _, err := emptyController.BuildMusicDetailRequest(Query{Region: "jp"}); err == nil {
		t.Fatal("detail build without a source succeeded")
	}
	if _, err := emptyController.BuildMusicBriefListRequest(BriefListQuery{Region: "jp"}); err == nil {
		t.Fatal("brief-list build without a source succeeded")
	}
	if _, err := emptyController.BuildMusicListRequest(ListQuery{Region: "jp"}); err == nil {
		t.Fatal("list build without a source succeeded")
	}
	if result, err := (*Controller)(nil).ResolveMusicCoverByTitleOrAlias(Query{}); err == nil || result != nil {
		t.Fatalf("nil cover controller = %+v, %v", result, err)
	}
	cover, err := controller.ResolveMusicCoverByTitleOrAlias(Query{Query: "Render Song", Region: "jp"})
	if err != nil || cover.Music.ID != 1 || !strings.Contains(cover.JacketPath, "render_song") {
		t.Fatalf("cover = %+v, %v", cover, err)
	}
	if _, err := controller.ResolveMusicCoverByTitleOrAlias(Query{Region: "jp"}); err == nil {
		t.Fatal("empty cover query succeeded")
	}

	title := "Brief"
	brief, err := controller.BuildMusicBriefListRequest(BriefListQuery{
		MusicIDs: []int{99, 1}, Difficulty: "master", Region: "jp",
		Title: &title, TitleStyle: map[string]any{"color": "white"}, TitleShadow: true,
	})
	if err != nil || len(brief.MusicList) != 1 || brief.RequiredDifficulty != "master" || brief.Title != &title || !brief.TitleShadow {
		t.Fatalf("brief request = %+v, %v", brief, err)
	}
	if _, err := controller.BuildMusicBriefListRequest(BriefListQuery{MusicIDs: []int{99}, Region: "jp"}); err == nil {
		t.Fatal("invalid brief list succeeded")
	}
	if _, err := controller.BuildMusicBriefListRequest(BriefListQuery{Region: "jp"}); err == nil {
		t.Fatal("empty brief list succeeded")
	}
	fullList, err := controller.BuildMusicListRequest(ListQuery{
		Region: "jp", Difficulty: "master", Full: true, Level: 30, ResultFilter: "not_clear",
	})
	if err != nil || len(fullList.MusicList) != 1 || fullList.Profile != nil {
		t.Fatalf("full music list = %+v, %v", fullList, err)
	}

	explicitIcon := " explicit.png "
	detail, err := controller.BuildMusicRewardsDetailRequest(RewardsDetailQuery{
		Region: "jp", RankRewards: 10,
		ComboRewards:  map[string][]drawing.MusicComboReward{"hard": {{Level: 18, Reward: 50}}},
		JewelIconPath: &explicitIcon,
	})
	if err != nil || detail.RankRewards != 10 || detail.JewelIconPath == nil || *detail.JewelIconPath != "explicit.png" || len(detail.ComboRewards) != 4 {
		t.Fatalf("detail reward builder = %+v, %v", detail, err)
	}
	basic, err := controller.BuildMusicRewardsBasicRequest(RewardsBasicQuery{Region: "jp"})
	if err != nil || len(basic.ComboRewards) != 4 || basic.ComboRewards["master"] != "0" {
		t.Fatalf("basic reward defaults = %+v, %v", basic, err)
	}
	combo := map[string]string{"master": "10"}
	basic, err = controller.BuildMusicRewardsBasicRequest(RewardsBasicQuery{Region: "jp", ComboRewards: combo})
	if err != nil || !reflect.DeepEqual(basic.ComboRewards, combo) {
		t.Fatalf("basic reward override = %+v, %v", basic, err)
	}
}

func TestMusicControllerRenderPropagatesBuildErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("drawing server should not be called after a build error")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	controller := newRenderCoverageController(t, drawing.NewHarukiDrawingClient(server.URL))

	checks := []struct {
		name string
		err  error
	}{
		{"detail", func() error {
			_, err := controller.RenderMusicDetail(Query{Query: "missing", Region: "jp"})
			return err
		}()},
		{"brief", func() error { _, err := controller.RenderMusicBriefList(BriefListQuery{Region: "jp"}); return err }()},
		{"list", func() error {
			_, err := controller.RenderMusicList(ListQuery{Region: "jp", Difficulty: "easy"})
			return err
		}()},
		{"achievements", func() error {
			_, err := controller.RenderMusicRewardsDetailFromAchievements(RewardsDetailQuery{Region: "jp"}, []byte(`{`))
			return err
		}()},
		{"snapshot", func() error {
			_, err := controller.RenderMusicRewardsDetailFromSnapshot(RewardsDetailQuery{Region: "jp"}, nil)
			return err
		}()},
		{"chart", func() error {
			_, err := controller.RenderMusicChart(ChartQuery{Query: "missing", Region: "jp"})
			return err
		}()},
	}
	for _, check := range checks {
		if check.err == nil {
			t.Fatalf("%s build error was nil", check.name)
		}
	}
	if !errors.Is(errors.Join(checks[0].err), checks[0].err) {
		t.Fatal("unexpected error identity")
	}
}
