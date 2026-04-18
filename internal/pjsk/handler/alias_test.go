package handler

import (
	"context"
	"encoding/json"
	"testing"

	aliases "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestMusicAliasAddHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.MusicAliasAddHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/music alias add",
		ArgText:    "5201\n蓝歌\n群青歌",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleAlias || resolved.Mode != aliases.ModeAdd {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params aliases.AddCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.AliasType != aliases.PjskAliasTypeMusic || params.Platform != "qq" || params.PlatformUserID != "42" || params.Target != "5201" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if len(params.Aliases) != 2 || params.Aliases[0] != "蓝歌" || params.Aliases[1] != "群青歌" {
		t.Fatalf("unexpected aliases: %+v", params.Aliases)
	}
}

func TestCharacterAliasQueryHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.CharacterAliasQueryHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/角色别名",
		ArgText:    "初音未来",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleAlias || resolved.Mode != aliases.ModeQuery {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params aliases.QueryCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.AliasType != aliases.PjskAliasTypeCharacter || params.Target != "初音未来" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestCharacterAliasDeleteHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.CharacterAliasDeleteHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/chara alias del",
		ArgText:    "1\n葱\n公主殿下",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleAlias || resolved.Mode != aliases.ModeDelete {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params aliases.DeleteCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.AliasType != aliases.PjskAliasTypeCharacter || params.PlatformUserID != "admin" || params.Target != "1" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if len(params.Aliases) != 2 || params.Aliases[0] != "葱" || params.Aliases[1] != "公主殿下" {
		t.Fatalf("unexpected aliases: %+v", params.Aliases)
	}
}

func TestAliasPendingHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.AliasPendingHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/待审核别名",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != aliases.ModePendingList {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params aliases.ReviewListCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Platform != "qq" || params.PlatformUserID != "admin" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestAliasApproveHandleParsesReviewIDs(t *testing.T) {
	h := sekaiHandlers{}.AliasApproveHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/同意别名",
		ArgText:    "12 15 18",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != aliases.ModeApprove {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params aliases.ApproveCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.ReviewIDs) != 3 || params.ReviewIDs[0] != 12 || params.ReviewIDs[2] != 18 {
		t.Fatalf("unexpected review ids: %+v", params.ReviewIDs)
	}
}

func TestAliasRejectHandleParsesReason(t *testing.T) {
	h := sekaiHandlers{}.AliasRejectHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/拒绝别名",
		ArgText:    "21 与现有别名冲突",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Mode != aliases.ModeReject {
		t.Fatalf("resolved.Mode = %q", resolved.Mode)
	}

	var params aliases.RejectCommandParams
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.ReviewID != 21 || params.Reason != "与现有别名冲突" {
		t.Fatalf("unexpected params: %+v", params)
	}
}
