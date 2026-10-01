package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/utils/imagecache"
)

func TestProfileInfoPanelHandleRoutesEachSource(t *testing.T) {
	h := sekaiHandlers{}.ProfileInfoPanelHandle()
	h.Regions = AllRegions

	cases := []struct {
		args   string
		module parser.TargetModule
		mode   string
	}{
		{"su", parser.ModuleProfile, profileModeInfoPanel},
		{"SUITE", parser.ModuleProfile, profileModeInfoPanel},
		{"u2 ms", parser.ModuleMysekai, mySekaiInfoPanelCommand},
		{"mysekai", parser.ModuleMysekai, mySekaiInfoPanelCommand},
	}
	for _, tc := range cases {
		request, err := h.Handle(&PjskHandlerContext{
			Context: context.Background(), Platform: "qq", UserId: "12345",
			TriggerCmd: "/信息面板", ArgText: tc.args,
		})
		if err != nil || request == nil {
			t.Fatalf("Handle(%q) = %+v, %v", tc.args, request, err)
		}
		if request.Module != tc.module || request.Mode != tc.mode {
			t.Fatalf("Handle(%q) = module %v mode %q", tc.args, request.Module, request.Mode)
		}
		var params userQueryParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatalf("params: %v", err)
		}
		if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "12345" {
			t.Fatalf("Handle(%q) params = %+v", tc.args, params)
		}
		if strings.HasPrefix(tc.args, "u2") && params.Selector != "u2" {
			t.Fatalf("Handle(%q) selector = %q", tc.args, params.Selector)
		}
	}
}

func TestProfileInfoPanelHandleRequiresASource(t *testing.T) {
	h := sekaiHandlers{}.ProfileInfoPanelHandle()
	h.Regions = AllRegions
	for _, args := range []string{"", "suite ms", "其他"} {
		_, err := h.Handle(&PjskHandlerContext{
			Context: context.Background(), Platform: "qq", UserId: "12345",
			TriggerCmd: "/信息面板", ArgText: args,
		})
		var replay onebot11.ReplayError
		if !errors.As(err, &replay) || !strings.Contains(string(replay), "/信息面板 su") {
			t.Fatalf("Handle(%q) error = %v, want usage", args, err)
		}
	}
}

func TestExecuteSuiteInfoPanelRendersTheSuiteCard(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerEducationRegionValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	var gotPath string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	t.Cleanup(server.Close)

	rc := &RequestContext{
		Ctx: ctx,
		Cmd: &CommandRequest{
			Module:            parser.ModuleProfile,
			Mode:              profileModeInfoPanel,
			Params:            []byte(`{"mode":"self","platform":"qq","platform_user_id":"42"}`),
			RequesterPlatform: "qq",
			RequesterUserID:   "42",
		},
		App: &renderapp.App{
			Config:     renderapp.Config{UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true}},
			Drawing:    drawing.NewHarukiDrawingClient(server.URL),
			Bindings:   service,
			Snapshots:  rendersnapshot.NewStaticSnapshotProvider(mustBridgeEducationSnapshot(t)),
			ImageCache: imagecache.New("https://example.com", t.TempDir()),
		},
		Region:         renderregion.CN,
		RegionStr:      "cn",
		Platform:       "qq",
		PlatformUserID: "42",
	}

	message, err := executeInfoPanel(rc)
	if err != nil || len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("executeInfoPanel = %+v, %v; want one image", message, err)
	}
	if gotPath != drawing.InfoPanelEndpoint {
		t.Fatalf("drawing path = %q, want %q", gotPath, drawing.InfoPanelEndpoint)
	}
	sources, _ := body["data_sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("data_sources = %#v, want the Suite source", body["data_sources"])
	}
	if name, _ := sources[0].(map[string]any)["name"].(string); name != "Suite数据" {
		t.Fatalf("data source = %q, want Suite数据", name)
	}
	if _, ok := body["dt"]; !ok {
		t.Fatal("request carries no dt for the watermark")
	}
}

func TestExecuteInfoPanelRejectsUnknownModes(t *testing.T) {
	rc := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{Mode: "profile-render"}}
	if _, err := executeInfoPanel(rc); err == nil {
		t.Fatal("executeInfoPanel(unknown mode) error = nil")
	}
}
