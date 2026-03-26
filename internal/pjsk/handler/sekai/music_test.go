package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/musicalias"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestAliasSetHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.AliasSetHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/music alias add",
		ArgText:    "5201\n蓝歌\n群青歌",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleAlias || resolved.Mode != musicalias.ModeAdd {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params musicalias.AddCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Platform != "qq" || params.PlatformUserID != "42" || params.Target != "5201" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if len(params.Aliases) != 2 || params.Aliases[0] != "蓝歌" || params.Aliases[1] != "群青歌" {
		t.Fatalf("unexpected aliases: %+v", params.Aliases)
	}
}

func TestAliasDeleteHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.AliasDelHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/music alias del",
		ArgText:    "5201\n蓝歌\n群青歌",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleAlias || resolved.Mode != musicalias.ModeDelete {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params musicalias.DeleteCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Platform != "qq" || params.PlatformUserID != "admin" || params.Target != "5201" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if len(params.Aliases) != 2 || params.Aliases[0] != "蓝歌" || params.Aliases[1] != "群青歌" {
		t.Fatalf("unexpected aliases: %+v", params.Aliases)
	}
}

func TestApproveAliasHandleParsesReviewIDs(t *testing.T) {
	h := sekaiHandlers{}.ApproveAliasHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/同意别名",
		ArgText:    "12 15 18",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Mode != musicalias.ModeApprove {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params musicalias.ApproveCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.ReviewIDs) != 3 || params.ReviewIDs[0] != 12 || params.ReviewIDs[2] != 18 {
		t.Fatalf("unexpected review ids: %+v", params.ReviewIDs)
	}
}

func TestRejectAliasHandleParsesReason(t *testing.T) {
	h := sekaiHandlers{}.RejectAliasHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/拒绝别名",
		ArgText:    "21 与现有别名冲突",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Mode != musicalias.ModeReject {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params musicalias.RejectCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ReviewID != 21 || params.Reason != "与现有别名冲突" {
		t.Fatalf("unexpected params: %+v", params)
	}
}
