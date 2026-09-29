package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderstamp "haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/utils/imagecache"
)

func configureHandlerArtifactHit(t *testing.T, app *renderapp.App) string {
	t.Helper()
	const path = "pjsk/api/pjsk/test/result.png"
	cache := drawing.NewRenderCacheClient(drawing.RenderCacheConfig{
		TTL: time.Hour,
		Index: handlerFakeIndex{entry: imagecache.RenderIndexEntry{
			ContentHash: strings.Repeat("c", 64),
			Entry:       imagecache.ImageEntry{CDNPath: path, StorageBackend: imagecache.BackendGarage},
		}},
	})
	t.Cleanup(func() { _ = cache.Close() })
	app.Drawing.SetRenderCache(cache)
	// No byte fetcher or image store is configured. A successful image reply
	// therefore requires preserving the artifact throughout the command.
	app.ImageCache = nil
	app.ImageHosts = urlhost.Single("https://ic.example")
	return "https://ic.example/" + path
}

func TestCommandImagePipelinePreservesArtifact(t *testing.T) {
	cases := []struct {
		mode    string
		params  any
		execute func(*RequestContext) (onebot11.Message, error)
	}{
		{"score-control", drawing.ScoreControlRequest{MusicID: 1, TargetPoint: 1000, ValidScores: []drawing.ScoreData{{EventBonus: 100, Boost: 5, ScoreMin: 1, ScoreMax: 2}}}, executeScore},
		{"score-custom-room", drawing.CustomRoomScoreRequest{TargetPoint: 1000, CandidatePairs: [][]int{{100, 200}}, MusicListMap: map[int][]map[string]any{1: {{"music_cover": "jacket/a/a.png"}}}}, executeScore},
		{"score-music-board", drawing.MusicBoardRequest{Items: []drawing.MusicBoardItem{{MusicID: 1, Difficulty: "master"}}}, executeScore},
		{"sk-winrate", drawing.WinRateRequest{TeamInfo: []drawing.TeamInfo{{TeamID: 1, TeamName: "one", WinRate: 0.5}}}, executeSK},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			app, _ := newExecutionCoverageApp(t)
			want := configureHandlerArtifactHit(t, app)
			message, err := tc.execute(executionCoverageContext(t, app, tc.mode, tc.params))
			if err != nil {
				t.Fatal(err)
			}
			if got := imageSegmentFile(t, message); got != want {
				t.Fatalf("image URL = %q, want %q", got, want)
			}
		})
	}
}

func TestStampPagesPreserveArtifacts(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)
	want := configureHandlerArtifactHit(t, app)
	stamps := make([]masterdata.Stamp, 26)
	for i := range stamps {
		stamps[i] = masterdata.Stamp{ID: i + 1, AssetBundleName: "stamp_a"}
	}
	app.Stamps = renderstamp.NewController(&handlerStampSource{region: renderregion.JP, stamps: stamps}, app.Drawing, app.Assets)
	message, err := executeStamp(executionCoverageContext(t, app, "stamp-list", renderstamp.ListQuery{All: true}))
	if err != nil {
		t.Fatal(err)
	}
	if len(message) != 2 {
		t.Fatalf("stamp pages = %d, want 2", len(message))
	}
	for _, segment := range message {
		if got := imageSegmentFile(t, onebot11.Message{segment}); got != want {
			t.Fatalf("image URL = %q, want %q", got, want)
		}
	}
}

func TestCommandHelpPreservesArtifactWithoutImageStore(t *testing.T) {
	app, _ := newExecutionCoverageApp(t)
	want := configureHandlerArtifactHit(t, app)
	message, err := commandHelpMessage(t.Context(), &CommandRequest{CommandPath: "music/list"}, app)
	if err != nil {
		t.Fatal(err)
	}
	if got := imageSegmentFile(t, message); got != want {
		t.Fatalf("image URL = %q, want %q", got, want)
	}
}

func TestMysekaiImageResultPreservesArtifactAndContext(t *testing.T) {
	rc := &RequestContext{Ctx: t.Context(), App: &renderapp.App{ImageHosts: urlhost.Single("https://ic.example")}}
	image := drawing.ImageArtifact(&drawing.ArtifactRef{CDNPath: "pjsk/mysekai.png"})
	message, err := mysekaiRenderedImageResult(rc, image, nil)
	if err != nil || imageSegmentFile(t, message) != "https://ic.example/pjsk/mysekai.png" {
		t.Fatalf("MySekai image = %+v, %v", message, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mysekaiRenderedImageResultWithContext(ctx, rc, image, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled child context error = %v", err)
	}
	wantErr := errors.New("render failed")
	if _, err := mysekaiRenderedImageResult(rc, image, wantErr); err != wantErr {
		t.Fatalf("render error = %v", err)
	}
}

func TestRenderedImageMessageStoresByteResult(t *testing.T) {
	root := t.TempDir()
	app := &renderapp.App{ImageCache: imagecache.New("https://ic.example", root)}
	t.Cleanup(func() { _ = app.ImageCache.Close() })
	rc := &RequestContext{Ctx: t.Context(), App: app}
	message, err := rc.RenderedImageMessage(drawing.ImageBytes([]byte("rendered-image")))
	if err != nil {
		t.Fatal(err)
	}
	url := imageSegmentFile(t, message)
	if !strings.HasPrefix(url, "https://ic.example/pjsk/") {
		t.Fatalf("image URL = %q", url)
	}
	data, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(url, "https://ic.example/")))
	if err != nil || string(data) != "rendered-image" {
		t.Fatalf("stored image = %q, %v", data, err)
	}
}
