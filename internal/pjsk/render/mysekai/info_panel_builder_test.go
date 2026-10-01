package mysekai

import (
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestBuildInfoPanelRequestKeepsOnlyTheMySekaiSource(t *testing.T) {
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: t.TempDir(), AllowFallback: true}).
		WithMySekaiData([]byte(`{"upload_time":1790841600,"source":"toolbox","updatedResources":{"userMysekaiGamedata":{"mysekaiRank":42}}}`))
	suiteTime := int64(1790838000000)
	frame := &drawing.PlayerFramePaths{Base: "asset/jp-assets/startapp/player_frame/frame_0001/10001/vertical/frame_base.png"}

	req, err := controller.BuildInfoPanelRequest(InfoPanelQuery{
		Region: "jp",
		Profile: &drawing.ProfileCardRequest{
			Profile: &drawing.BasicProfile{
				ID: "12345678901234567", Region: "JP", Nickname: "Tester",
				LeaderImagePath: "user/leader.png", HasFrame: true, FramePaths: frame,
			},
			DataSources: []drawing.ProfileDataSource{{Name: "Suite数据", UpdateTime: &suiteTime}},
		},
	})
	if err != nil {
		t.Fatalf("BuildInfoPanelRequest() error = %v", err)
	}
	if len(req.DataSources) != 1 || req.DataSources[0].Name != mySekaiDataLabel {
		t.Fatalf("data sources = %+v, want only %s", req.DataSources, mySekaiDataLabel)
	}
	if got := req.DataSources[0].UpdateTime; got == nil || *got != 1790841600000 {
		t.Fatalf("mysekai update time = %v, want the upload time in ms", got)
	}
	if req.MysekaiLevel == nil || *req.MysekaiLevel != 42 {
		t.Fatalf("mysekai level = %v, want 42", req.MysekaiLevel)
	}
	if req.Profile == nil || !req.Profile.HasFrame || req.Profile.FramePaths == nil {
		t.Fatalf("profile = %+v, want the equipped frame kept", req.Profile)
	}
}

func TestBuildInfoPanelRequestNeedsAProfile(t *testing.T) {
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: t.TempDir(), AllowFallback: true}).
		WithMySekaiData([]byte(`{"updatedResources":{}}`))
	if _, err := controller.BuildInfoPanelRequest(InfoPanelQuery{Region: "jp"}); err == nil {
		t.Fatal("BuildInfoPanelRequest() without a profile error = nil")
	}
	if _, err := (&Controller{}).RenderInfoPanelImage(InfoPanelQuery{}); err == nil {
		t.Fatal("RenderInfoPanelImage() without drawing error = nil")
	}
}

func TestRenderInfoPanelImageReportsBuildErrors(t *testing.T) {
	withDrawing := NewController(drawing.NewHarukiDrawingClient("http://127.0.0.1:1"), nil, renderregion.JP, nil,
		MasterdataOptions{LocalDir: t.TempDir(), AllowFallback: true}).WithMySekaiData([]byte(`{"updatedResources":{}}`))
	if _, err := withDrawing.RenderInfoPanelImage(InfoPanelQuery{Region: "jp"}); err == nil {
		t.Fatal("RenderInfoPanelImage() without a profile error = nil")
	}
	unconfigured := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{})
	if _, err := unconfigured.BuildInfoPanelRequest(InfoPanelQuery{Region: "jp"}); err == nil {
		t.Fatal("BuildInfoPanelRequest() without masterdata error = nil")
	}
}
