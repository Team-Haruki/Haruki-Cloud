package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	rendervlive "haruki-cloud/internal/pjsk/render/vlive"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/imagecache"
	"haruki-cloud/utils/usererror"
)

func newVLiveSoloTestApp(t *testing.T, lives []*rendervlive.Live, paths *[]string) *renderapp.App {
	t.Helper()
	drawingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(mustEncodeTestPNG(t, 8, 8))
	}))
	t.Cleanup(drawingServer.Close)
	return &renderapp.App{
		ImageCache: imagecache.New("https://image-cache.test", t.TempDir()),
		VLive: rendervlive.NewControllerWithDrawing(&bridgeVLiveSource{lives: lives},
			drawing.NewHarukiDrawingClient(drawingServer.URL), assets.NewAssetHelper("", nil), renderregion.JP),
	}
}

func runVLiveQuery(t *testing.T, app *renderapp.App, query string) onebot11.Message {
	t.Helper()
	message, err := executeVLive(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleVLive, Mode: "vlive-list", Region: "jp", Query: query,
	}, app))
	if err != nil {
		t.Fatalf("executeVLive(%q): %v", query, err)
	}
	return message
}

func TestExecuteVLiveDetailOnRegionWithoutSoloLives(t *testing.T) {
	now := time.Now()
	var paths []string
	app := newVLiveSoloTestApp(t, []*rendervlive.Live{{
		ID: 3001, Name: "Old", StartAt: now.Add(time.Hour).UnixMilli(), EndAt: now.Add(2 * time.Hour).UnixMilli(),
	}}, &paths)

	message := runVLiveQuery(t, app, "3001")
	if len(message) != 1 || message[0].Type != "text" || message[0].Data.(onebot11.TextData).Text != i18n.T("vlive.solo.none", i18n.Data{"Region": i18n.RegionLabel("jp")}) {
		t.Fatalf("unexpected old-region detail reply: %+v", message)
	}
	// Any other argument keeps the list, as before.
	message = runVLiveQuery(t, app, "abc")
	if len(message) != 1 || message[0].Type != "image" || len(paths) != 1 || paths[0] != "/api/pjsk/vlive/list" {
		t.Fatalf("non-detail argument must render the list: %+v paths=%v", message, paths)
	}
}

func TestExecuteVLiveDetailRendersSoloGroup(t *testing.T) {
	now := time.Now()
	var paths []string
	solo := func(id int) *rendervlive.Live {
		return &rendervlive.Live{ID: id, Name: "solo", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(48 * time.Hour).UnixMilli(),
			VirtualLiveType: "solo_virtual_live", GroupID: 2}
	}
	app := newVLiveSoloTestApp(t, []*rendervlive.Live{solo(491), solo(492)}, &paths)

	message := runVLiveQuery(t, app, "491")
	if len(message) != 1 || message[0].Type != "image" || len(paths) != 1 || paths[0] != "/api/pjsk/vlive/detail" {
		t.Fatalf("unexpected detail reply: %+v paths=%v", message, paths)
	}
	// An unknown Live is a typed not-found error, so the reply layer can
	// hide the query when the client did not enable parameter echo.
	_, err := executeVLive(NewRequestContext(context.Background(), &CommandRequest{
		Module: parser.ModuleVLive, Mode: "vlive-list", Region: "jp", Query: "999",
	}, app))
	typed := testutil.RequireUserError(t, err, usererror.CodeNotFound, "vlive.solo.not_found")
	if got := typed.Message.Data["UserQuery"]; got != i18n.UserText("999") {
		t.Fatalf("not-found query = %v", got)
	}
}
