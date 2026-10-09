package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/imagecache"
	"haruki-cloud/utils/usererror"
)

func newInfoPanelDrawingServer(t *testing.T, gotPath *string, body *map[string]any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, body)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	t.Cleanup(server.Close)
	return server
}

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
		{"all", parser.ModuleMysekai, mySekaiInfoPanelAllCommand},
		{"u2 ALL", parser.ModuleMysekai, mySekaiInfoPanelAllCommand},
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
		testutil.RequireUserError(t, err, usererror.CodeUsage, "common.unrecognized_args")
	}
}

func newSuiteInfoPanelRequestContext(t *testing.T, drawingURL string) *RequestContext {
	t.Helper()
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerEducationRegionValidator{})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	return &RequestContext{
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
			Drawing:    drawing.NewHarukiDrawingClient(drawingURL),
			Bindings:   service,
			Snapshots:  rendersnapshot.NewStaticSnapshotProvider(mustBridgeEducationSnapshot(t)),
			ImageCache: imagecache.New("https://example.com", t.TempDir()),
		},
		Region:         renderregion.CN,
		RegionStr:      "cn",
		Platform:       "qq",
		PlatformUserID: "42",
	}
}

func TestExecuteSuiteInfoPanelRendersTheSuiteCard(t *testing.T) {
	var gotPath string
	var body map[string]any
	server := newInfoPanelDrawingServer(t, &gotPath, &body)
	rc := newSuiteInfoPanelRequestContext(t, server.URL)

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

func TestExecuteSuiteInfoPanelRefusesWithoutDrawingOrVisibleSuite(t *testing.T) {
	ctx := context.Background()
	if _, err := executeSuiteInfoPanel(&RequestContext{Ctx: ctx, Cmd: &CommandRequest{}, App: &renderapp.App{}}); err == nil {
		t.Fatal("executeSuiteInfoPanel without drawing error = nil")
	}

	service := newHandlerTestBindingServiceWithValidator(t, handlerEducationRegionValidator{})
	rc := &RequestContext{
		Ctx:            ctx,
		Cmd:            &CommandRequest{Mode: profileModeInfoPanel, Region: "jp"},
		App:            &renderapp.App{Drawing: drawing.NewHarukiDrawingClient("http://127.0.0.1:1"), Bindings: service},
		Region:         renderregion.JP,
		RegionStr:      "jp",
		Platform:       "qq",
		PlatformUserID: "1",
		binding:        &accountdata.ResolvedBinding{PJSKUserID: "1234567890", SuiteVisible: false},
	}
	rc.bindingOnce.Do(func() {})
	message, err := executeSuiteInfoPanel(rc)
	if err == nil || message != nil {
		t.Fatalf("hidden suite = %+v, %v; want the suite-missing error", message, err)
	}
}

func TestMysekaiInfoPanelModeRendersTheMySekaiCard(t *testing.T) {
	if fields := mysekaiSuiteFields(mySekaiInfoPanelCommand); !containsString(fields, "userMysekaiGamedata") || !containsString(fields, "userPlayerFrames") {
		t.Fatalf("suite fields = %v, want MySekai gamedata and the profile set", fields)
	}
	if opts := mysekaiRenderContextOptionsForMode(mySekaiInfoPanelCommand); !opts.NeedProfile || opts.MySekaiPayloadOnly {
		t.Fatalf("render options = %+v, want the snapshot path with a profile", opts)
	}

	var gotPath string
	var body map[string]any
	server := newInfoPanelDrawingServer(t, &gotPath, &body)
	controller := rendermysekai.NewController(drawing.NewHarukiDrawingClient(server.URL), nil, renderregion.JP, nil,
		rendermysekai.MasterdataOptions{LocalDir: t.TempDir(), AllowFallback: true}).
		WithMySekaiData([]byte(`{"upload_time":1790841600,"updatedResources":{"userMysekaiGamedata":{"mysekaiRank":7}}}`))
	rc := &RequestContext{
		Ctx: context.Background(),
		Cmd: &CommandRequest{Module: parser.ModuleMysekai, Mode: mySekaiInfoPanelCommand, Region: "jp"},
		App: &renderapp.App{ImageCache: imagecache.New("https://example.com", t.TempDir())},
	}
	message, err := executeResolvedMysekaiMode(rc, mySekaiRenderContext{
		Controller: controller,
		Region:     "jp",
		Profile: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{ID: "1", Region: "JP", Nickname: "Panel", LeaderImagePath: "leader.png"},
		},
	})
	if err != nil || len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("mysekai info panel = %+v, %v; want one image", message, err)
	}
	if gotPath != drawing.InfoPanelEndpoint {
		t.Fatalf("drawing path = %q", gotPath)
	}
	if level, _ := body["mysekai_level"].(float64); level != 7 {
		t.Fatalf("mysekai_level = %#v, want 7", body["mysekai_level"])
	}
}

func TestInfoPanelGuards(t *testing.T) {
	if _, err := executeInfoPanel(nil); err == nil {
		t.Fatal("executeInfoPanel(nil) error = nil")
	}
	// the MySekai source runs through the MySekai executor, which needs its controller
	rc := &RequestContext{Ctx: context.Background(), Cmd: &CommandRequest{Mode: mySekaiInfoPanelCommand}, App: &renderapp.App{}}
	if _, err := executeInfoPanel(rc); err == nil {
		t.Fatal("mysekai info panel without a MySekai controller error = nil")
	}

	h := sekaiHandlers{}.ProfileInfoPanelHandle()
	h.Regions = AllRegions
	if _, err := h.Handle(&PjskHandlerContext{
		Context: context.Background(), Platform: "qq", UserId: "12345",
		TriggerCmd: "/信息面板", ArgText: "12345678901234 su",
	}); err == nil {
		t.Fatal("another player's UID error = nil, want the self-only error")
	}
}

func TestExecuteSuiteInfoPanelSurfacesDrawingFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	if message, err := executeSuiteInfoPanel(newSuiteInfoPanelRequestContext(t, server.URL)); err == nil || message != nil {
		t.Fatalf("failed render = %+v, %v; want the drawing error", message, err)
	}
}

// newAllInfoPanelController is a MySekai controller over a merged Suite+MySekai snapshot,
// the shape the MySekai snapshot path hands the combined panel.
func newAllInfoPanelController(t *testing.T, drawingURL string) *rendermysekai.Controller {
	t.Helper()
	snap, err := rendersnapshot.NewFromBytes(nil, nil, renderregion.JP, []byte(`{"upload_time":1790841600,"source":"toolbox","userGamedata":{"userId":1},"userMysekaiGamedata":{"mysekaiRank":9}}`), nil, nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return rendermysekai.NewController(drawing.NewHarukiDrawingClient(drawingURL), nil, renderregion.JP, nil,
		rendermysekai.MasterdataOptions{LocalDir: t.TempDir(), AllowFallback: true}).WithSnapshot(snap)
}

func allInfoPanelRequestContext(t *testing.T, suiteVisible bool) *RequestContext {
	t.Helper()
	rc := &RequestContext{
		Ctx:            context.Background(),
		Cmd:            &CommandRequest{Module: parser.ModuleMysekai, Mode: mySekaiInfoPanelAllCommand, Region: "jp"},
		App:            &renderapp.App{ImageCache: imagecache.New("https://example.com", t.TempDir())},
		Platform:       "qq",
		PlatformUserID: "42",
		binding:        &accountdata.ResolvedBinding{PJSKUserID: "1", SuiteVisible: suiteVisible, MySekaiVisible: true},
	}
	rc.bindingOnce.Do(func() {})
	return rc
}

func allInfoPanelProfile() *drawing.ProfileCardRequest {
	suiteTime := int64(1790838000000)
	return &drawing.ProfileCardRequest{
		Profile:     &drawing.BasicProfile{ID: "1", Region: "JP", Nickname: "Panel", LeaderImagePath: "leader.png"},
		DataSources: []drawing.ProfileDataSource{{Name: "Suite数据", UpdateTime: &suiteTime}},
	}
}

func TestInfoPanelAllRendersBothSourcesWithMySekaiLevel(t *testing.T) {
	if fields := mysekaiSuiteFields(mySekaiInfoPanelAllCommand); !containsString(fields, "userMysekaiGamedata") || !containsString(fields, "userPlayerFrames") {
		t.Fatalf("suite fields = %v, want MySekai gamedata and the profile set", fields)
	}
	if opts := mysekaiRenderContextOptionsForMode(mySekaiInfoPanelAllCommand); !opts.NeedProfile || opts.MySekaiPayloadOnly || opts.PreferMySekaiPayload {
		t.Fatalf("render options = %+v, want the merged snapshot path with a profile", opts)
	}
	var gotPath string
	var body map[string]any
	server := newInfoPanelDrawingServer(t, &gotPath, &body)
	rc := allInfoPanelRequestContext(t, true)
	message, err := executeResolvedMysekaiMode(rc, mySekaiRenderContext{
		Controller: newAllInfoPanelController(t, server.URL), Region: "jp", Profile: allInfoPanelProfile(),
	})
	if err != nil || len(message) != 1 || message[0].Type != onebot11.TypeImage {
		t.Fatalf("all info panel = %+v, %v; want one image", message, err)
	}
	if gotPath != drawing.InfoPanelEndpoint {
		t.Fatalf("drawing path = %q", gotPath)
	}
	sources, _ := body["data_sources"].([]any)
	var names []string
	for _, source := range sources {
		name, _ := source.(map[string]any)["name"].(string)
		names = append(names, name)
	}
	if strings.Join(names, ",") != "Suite数据,Mysekai数据" {
		t.Fatalf("data sources = %v, want Suite then MySekai", names)
	}
	if level, _ := body["mysekai_level"].(float64); level != 9 {
		t.Fatalf("mysekai_level = %#v, want 9", body["mysekai_level"])
	}
}

func TestInfoPanelAllRequiresVisibleSuite(t *testing.T) {
	var gotPath string
	var body map[string]any
	server := newInfoPanelDrawingServer(t, &gotPath, &body)
	rc := allInfoPanelRequestContext(t, false)
	message, err := executeResolvedMysekaiMode(rc, mySekaiRenderContext{
		Controller: newAllInfoPanelController(t, server.URL), Region: "jp", Profile: allInfoPanelProfile(),
	})
	if message != nil {
		t.Fatalf("hidden suite = %+v, %v; want the suite reply", message, err)
	}
	testutil.RequireUserError(t, err, usererror.CodeSetup, "")
	if want := suiteDataNotFoundError(rc.binding); err.Error() != want.Error() {
		t.Fatalf("reply = %q, want the /信息面板 su reply %q", err, want)
	}
	if gotPath != "" {
		t.Fatalf("rendered %q although the suite is hidden", gotPath)
	}
}

func TestInfoPanelAllFollowsTheMySekaiCNGate(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() { harukiConfig.Cfg.PJSK.AllowCNMySekai = original })

	run := func(mode string) onebot11.Message {
		message, err := executeInfoPanel(NewRequestContext(context.Background(), &CommandRequest{
			Module: parser.ModuleMysekai, Mode: mode, Region: "cn",
		}, &renderapp.App{
			MySekai: rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{AllowFallback: true}),
		}))
		if err != nil {
			t.Fatalf("%s: error = %v", mode, err)
		}
		return message
	}
	ms, all := run(mySekaiInfoPanelCommand), run(mySekaiInfoPanelAllCommand)
	if got := rejectionText(t, all); got != cnMySekaiNotice() || got != rejectionText(t, ms) {
		t.Fatalf("all = %q, ms = %q; want the same CN MySekai warning", got, rejectionText(t, ms))
	}
}
