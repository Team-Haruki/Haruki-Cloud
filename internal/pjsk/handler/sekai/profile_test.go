package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	accountdata "haruki-cloud/internal/pjsk/userdata"
)

func TestProfileUploadBGHandleExtractsImageURL(t *testing.T) {
	h := sekaiHandlers{}.ProfileUploadBGHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/上传个人背景",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]string{"text": "/上传个人背景"}},
			{Type: "image", Data: map[string]string{"url": "https://example.com/bg.png"}},
		},
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
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

func TestProfileAdjustBGHandleParsesArgs(t *testing.T) {
	h := sekaiHandlers{}.ProfileAdjustBGHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/调整个人信息背景",
		ArgText:    "竖屏 模糊 6 透明 70",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
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
