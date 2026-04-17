package sekai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/onebot11"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestProfileUploadBGHandleExtractsImageURL(t *testing.T) {
	h := sekaiHandlers{}.ProfileUploadBGHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
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

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
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

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
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

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
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

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
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

func TestProfileBindListHandleOmitsServerWhenRegionImplicit(t *testing.T) {
	h := sekaiHandlers{}.ProfileBindListHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
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

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
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

func TestProfileTimeZoneHandleParsesArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileTimeZoneHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.PjskHandlerContext{
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
		t.Fatal("expected resolved command, got nil")
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

func TestProfileTimeZoneHandleRequiresArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileTimeZoneHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	_, err := h.Handle(&handler.PjskHandlerContext{
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
