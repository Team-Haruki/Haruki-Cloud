package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"slices"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestProfileUploadBGHandleExtractsImageURL(t *testing.T) {
	h := sekaiHandlers{}.ProfileUploadBGHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/上传个人背景",
		Message: onebot11.Message{
			onebot11.Text("/上传个人背景"),
			onebot11.Image("", "https://example.com/bg.png"),
		},
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeBGUpload {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Platform != "qq" || params.PlatformUserID != "42" || params.Server != "jp" || params.ImageURL != "https://example.com/bg.png" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestProfileUploadBGHandleParsesSelector(t *testing.T) {
	h := sekaiHandlers{}.ProfileUploadBGHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/上传个人背景",
		ArgText:    "u2",
		Message: onebot11.Message{
			onebot11.Text("/上传个人背景 u2"),
			onebot11.Image("", "https://example.com/bg2.png"),
		},
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeBGUpload {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Selector != "u2" {
		t.Fatalf("unexpected selector: %q", params.Selector)
	}
	if params.ImageURL != "https://example.com/bg2.png" {
		t.Fatalf("unexpected image url: %q", params.ImageURL)
	}
}

func TestProfileAdjustBGHandleParsesArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileAdjustBGHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/调整个人信息背景",
		ArgText:    "竖屏 模糊 6 透明 70",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeBGAdjust {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Blur == nil || *params.Blur != 6 {
		t.Fatalf("unexpected blur: %+v", params.Blur)
	}
	if params.Alpha == nil || *params.Alpha != 70 {
		t.Fatalf("unexpected alpha: %+v", params.Alpha)
	}
	if params.Vertical == nil || !*params.Vertical {
		t.Fatalf("unexpected vertical: %+v", params.Vertical)
	}
}

func TestProfileAdjustBGHandleParsesSelectorAndArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileAdjustBGHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/调整个人信息背景",
		ArgText:    "u2 竖屏 模糊 6 透明 70",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeBGAdjust {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Selector != "u2" {
		t.Fatalf("unexpected selector: %q", params.Selector)
	}
	if params.Blur == nil || *params.Blur != 6 {
		t.Fatalf("unexpected blur: %+v", params.Blur)
	}
	if params.Alpha == nil || *params.Alpha != 70 {
		t.Fatalf("unexpected alpha: %+v", params.Alpha)
	}
	if params.Vertical == nil || !*params.Vertical {
		t.Fatalf("unexpected vertical: %+v", params.Vertical)
	}
}

func TestProfileHandleParsesVerticalArg(t *testing.T) {
	h := sekaiHandlers{}.ProfileHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/个人信息",
		ArgText:    "u2 竖屏",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeRender {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params UserQueryParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" {
		t.Fatalf("unexpected mode: %q", params.Mode)
	}
	if params.Selector != "u2" {
		t.Fatalf("unexpected selector: %q", params.Selector)
	}
	if params.ProfileVertical == nil || !*params.ProfileVertical {
		t.Fatalf("unexpected profile vertical: %+v", params.ProfileVertical)
	}
}

func TestProfileCustomProfileCardHandleParsesSeq(t *testing.T) {
	h := sekaiHandlers{}.ProfileCustomProfileCardHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if !containsString(h.GetCommands(), "/cp") {
		t.Fatalf("expected custom profile commands to include /cp, got %v", h.GetCommands())
	}
	for _, removed := range []string{"/自定义档案", "/pjsk custom profile", "/custom-profile"} {
		if containsString(h.GetCommands(), removed) {
			t.Fatalf("expected custom profile commands to omit %s, got %v", removed, h.GetCommands())
		}
	}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/自定义个人信息",
		ArgText:    "1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != profileModeCustomProfileCard {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params profileCustomProfileCardParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" {
		t.Fatalf("unexpected mode: %q", params.Mode)
	}
	if params.Seq != 1 {
		t.Fatalf("unexpected seq: %d", params.Seq)
	}
	if params.CustomProfileID != 0 || params.CustomProfileCardID != 0 {
		t.Fatalf("unexpected custom profile ids: %+v", params)
	}
}

func TestProfileCustomProfileCardHandleParsesSeqAndSelector(t *testing.T) {
	h := sekaiHandlers{}.ProfileCustomProfileCardHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/自定义个人信息",
		ArgText:    "u2 3",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != profileModeCustomProfileCard {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params profileCustomProfileCardParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Selector != "u2" {
		t.Fatalf("unexpected selector: %q", params.Selector)
	}
	if params.Seq != 3 {
		t.Fatalf("unexpected seq: %d", params.Seq)
	}
	if params.CustomProfileID != 0 || params.CustomProfileCardID != 0 {
		t.Fatalf("unexpected custom profile ids: %+v", params)
	}
}

func TestProfileCustomProfileCardHandleRequiresOneSeqArg(t *testing.T) {
	h := sekaiHandlers{}.ProfileCustomProfileCardHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/自定义个人信息",
	})
	if err == nil {
		t.Fatal("expected usage error")
	}
}

func TestProfileCustomProfileCardHandleAllowsArbitraryPositiveSeq(t *testing.T) {
	h := sekaiHandlers{}.ProfileCustomProfileCardHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/自定义个人信息",
		ArgText:    "99",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	var params profileCustomProfileCardParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Seq != 99 {
		t.Fatalf("unexpected seq: %d", params.Seq)
	}
}

func TestResolveCustomProfileCardReturnsErrorForMissingSeq(t *testing.T) {
	_, err := resolveCustomProfileCard([]sekaiapi.UserCustomProfileCard{
		{CustomProfileID: 1, CustomProfileCardID: 1, Seq: 1},
	}, profileCustomProfileCardParams{Seq: 99})
	if err == nil {
		t.Fatal("expected missing seq error")
	}
	if !strings.Contains(err.Error(), "未找到序号为99的自定义档案") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProfileBindListHandleOmitsServerWhenRegionImplicit(t *testing.T) {
	h := sekaiHandlers{}.ProfileBindListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/绑定列表",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeBindList {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileBindingCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Server != "" {
		t.Fatalf("unexpected server for implicit region: %q", params.Server)
	}
}

func TestProfileBindListHandleKeepsServerWhenRegionExplicit(t *testing.T) {
	h := sekaiHandlers{}.ProfileBindListHandle()
	h.Regions = []renderregion.Value{renderregion.JP, renderregion.CN}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/cn绑定列表",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeBindList {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileBindingCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Server != "cn" {
		t.Fatalf("unexpected server for explicit region: %q", params.Server)
	}
}

func TestProfileBindSwapHandleParsesArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileBindSwapHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/绑定交换",
		ArgText:    "u1 u2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeBindSwap {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileBindingCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Selector != "u1" || params.SelectorOther != "u2" {
		t.Fatalf("unexpected swap params: %+v", params)
	}
	if params.Server != "" {
		t.Fatalf("unexpected implicit server: %q", params.Server)
	}
}

func TestProfileBindHandleRedirectsSwapArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileBindHandle()
	h.Regions = []renderregion.Value{renderregion.JP, renderregion.CN}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/cn绑定",
		ArgText:    "交换 u1 u2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeBindSwap {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileBindingCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Selector != "u1" || params.SelectorOther != "u2" || params.Server != "cn" {
		t.Fatalf("unexpected redirected swap params: %+v", params)
	}
}

func TestProfileTimeZoneHandleParsesArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileTimeZoneHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/pjsktz",
		ArgText:    "+28800",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetTimeZone {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.TimeZone != "+28800" {
		t.Fatalf("unexpected timezone param: %q", params.TimeZone)
	}
}

func TestProfileTimeZoneHandleIncludesCompactAliases(t *testing.T) {
	h := sekaiHandlers{}.ProfileTimeZoneHandle()
	if !slices.Contains(h.GetCommands(), "/pjsktzhkt") {
		t.Fatalf("expected compact alias /pjsktzhkt in commands, got %v", h.GetCommands())
	}
}

func TestProfileTimeZoneHandleParsesCompactTriggerArg(t *testing.T) {
	h := sekaiHandlers{}.ProfileTimeZoneHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/pjsktzhkt",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetTimeZone {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.TimeZone != "hkt" {
		t.Fatalf("unexpected timezone param: %q", params.TimeZone)
	}
}

func TestProfileTimeZoneHandleRequiresArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileTimeZoneHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/pjsktz",
	})
	if err == nil {
		t.Fatal("expected missing args error, got nil")
	}
	if !strings.Contains(err.Error(), "<时区名|偏移量>") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProfileChartStyleHandleParsesArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileChartStyleHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/设置谱面样式",
		ArgText:    "white",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetChartStyle {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ChartStyle != "white" {
		t.Fatalf("unexpected chart style param: %q", params.ChartStyle)
	}
}

func TestProfileChartStyleHandleRequiresArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileChartStyleHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/设置谱面样式",
	})
	if err == nil {
		t.Fatal("expected missing args error, got nil")
	}
	if !strings.Contains(err.Error(), "<white|black>") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProfileArrestDifficultyHandleParsesArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileArrestDifficultyHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/逮捕难度",
		ArgText:    "easy关闭 normal关闭 hard关闭 expert关闭 master开启 append开启",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetArrestDiff {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.DifficultyToggles) != 6 {
		t.Fatalf("unexpected toggle count: %d", len(params.DifficultyToggles))
	}
	if params.DifficultyToggles[0].Difficulty != "easy" || params.DifficultyToggles[0].Enabled {
		t.Fatalf("unexpected first toggle: %+v", params.DifficultyToggles[0])
	}
	if params.DifficultyToggles[4].Difficulty != "master" || !params.DifficultyToggles[4].Enabled {
		t.Fatalf("unexpected master toggle: %+v", params.DifficultyToggles[4])
	}
	if params.DifficultyToggles[5].Difficulty != "append" || !params.DifficultyToggles[5].Enabled {
		t.Fatalf("unexpected append toggle: %+v", params.DifficultyToggles[5])
	}
}

func TestProfileArrestDifficultyHandleParsesAliases(t *testing.T) {
	h := sekaiHandlers{}.ProfileArrestDifficultyHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/逮捕难度",
		ArgText:    "hd关闭 ex开启 apd开启",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != accountdata.ProfileModeSetArrestDiff {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params accountdata.ProfileSettingsCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.DifficultyToggles) != 3 {
		t.Fatalf("unexpected toggle count: %d", len(params.DifficultyToggles))
	}
	if params.DifficultyToggles[0].Difficulty != "hard" || params.DifficultyToggles[0].Enabled {
		t.Fatalf("unexpected hard alias toggle: %+v", params.DifficultyToggles[0])
	}
	if params.DifficultyToggles[1].Difficulty != "expert" || !params.DifficultyToggles[1].Enabled {
		t.Fatalf("unexpected expert alias toggle: %+v", params.DifficultyToggles[1])
	}
	if params.DifficultyToggles[2].Difficulty != "append" || !params.DifficultyToggles[2].Enabled {
		t.Fatalf("unexpected append alias toggle: %+v", params.DifficultyToggles[2])
	}
}

func TestProfileArrestDifficultyHandleRequiresArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileArrestDifficultyHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/逮捕难度",
	})
	if err == nil {
		t.Fatal("expected missing args error, got nil")
	}
	if !strings.Contains(err.Error(), "easy关闭") {
		t.Fatalf("unexpected error: %v", err)
	}
}
