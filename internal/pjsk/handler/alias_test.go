package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"strings"
	"testing"

	aliases "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
)

type stubAliasMusicCoverResolver struct {
	cover     *rendermusic.CoverResult
	err       error
	lastQuery rendermusic.Query
}

func (s *stubAliasMusicCoverResolver) ResolveMusicCover(query rendermusic.Query) (*rendermusic.CoverResult, error) {
	s.lastQuery = query
	return s.cover, s.err
}

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

func TestAliasSubmitterHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.AliasSubmitterHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/查询别名提交者",
		ArgText:    "12",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil || result.Mode != aliases.ModeSubmitter {
		t.Fatalf("unexpected command request: %+v", result)
	}

	var params aliases.SubmitterCommandParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Platform != "qq" || params.PlatformUserID != "admin" || params.ReviewID != 12 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestAliasBanSubmitterHandleUsesMention(t *testing.T) {
	h := sekaiHandlers{}.AliasBanSubmitterHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/禁用别名提交",
		AtIds:      []string{"123456789"},
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil || result.Mode != aliases.ModeBanSubmitter {
		t.Fatalf("unexpected command request: %+v", result)
	}

	var params aliases.BanSubmitterCommandParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Platform != "qq" || params.PlatformUserID != "admin" || params.TargetPlatform != "qq" || params.TargetPlatformUserID != "123456789" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestParseAliasSubmissionTargetSupportsActorLabel(t *testing.T) {
	platform, userID, err := parseAliasSubmissionTarget("discord:987654", "qq", nil, "usage")
	if err != nil {
		t.Fatalf("parseAliasSubmissionTarget() error = %v", err)
	}
	if platform != "discord" || userID != "987654" {
		t.Fatalf("unexpected target: %s:%s", platform, userID)
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

func TestAliasBatchRejectHandleTreatsEveryFieldAsReviewID(t *testing.T) {
	h := sekaiHandlers{}.AliasBatchRejectHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/批量拒绝别名",
		ArgText:    "21  22\n23",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil || result.Mode != aliases.ModeBatchReject {
		t.Fatalf("unexpected command request: %+v", result)
	}

	var params aliases.BatchRejectCommandParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params.ReviewIDs) != 3 || params.ReviewIDs[0] != 21 || params.ReviewIDs[2] != 23 {
		t.Fatalf("unexpected review ids: %+v", params.ReviewIDs)
	}
}

func TestAliasBatchRejectHandleRejectsNonNumericField(t *testing.T) {
	h := sekaiHandlers{}.AliasBatchRejectHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "admin",
		TriggerCmd: "/批量拒绝别名",
		ArgText:    "21 原因",
	})
	if err == nil || !strings.Contains(err.Error(), "待审核ID必须为正整数") {
		t.Fatalf("expected numeric review ID error, got %v", err)
	}
}

func TestShouldRenderAliasQueryAsImageForMusicThreshold(t *testing.T) {
	aliasList := make([]string, aliasImageThreshold)
	for i := range aliasList {
		aliasList[i] = "alias"
	}
	if !shouldRenderAliasQueryAsImage(aliases.PjskAliasTypeMusic, aliasList) {
		t.Fatalf("expected music aliases at threshold to use image")
	}
}

func TestShouldRenderAliasQueryAsImageBelowThreshold(t *testing.T) {
	aliasList := make([]string, aliasImageThreshold-1)
	for i := range aliasList {
		aliasList[i] = "alias"
	}
	if shouldRenderAliasQueryAsImage(aliases.PjskAliasTypeMusic, aliasList) {
		t.Fatalf("expected music aliases below threshold to use text")
	}
}

func TestShouldRenderAliasQueryAsImageDoesNotApplyToCharacterAlias(t *testing.T) {
	aliasList := make([]string, aliasImageThreshold)
	for i := range aliasList {
		aliasList[i] = "alias"
	}
	if !shouldRenderAliasQueryAsImage(aliases.PjskAliasTypeCharacter, aliasList) {
		t.Fatalf("expected character aliases at threshold to use image")
	}
}

func TestShouldRenderAliasQueryAsImageDoesNotApplyToUnsupportedAlias(t *testing.T) {
	aliasList := make([]string, aliasImageThreshold)
	for i := range aliasList {
		aliasList[i] = "alias"
	}
	if shouldRenderAliasQueryAsImage("unsupported", aliasList) {
		t.Fatalf("expected unsupported alias type to remain text")
	}
}

func TestBuildAliasListImageRequestIncludesMusicJacketPath(t *testing.T) {
	resolver := &stubAliasMusicCoverResolver{
		cover: &rendermusic.CoverResult{JacketPath: "music/jacket/jacket_test/jacket_test.png"},
	}
	req, ok := buildAliasListImageRequest(resolver, aliases.PjskAliasTypeMusic, &aliases.QueryResult{
		Entity: aliases.EntityRef{
			AliasType: aliases.PjskAliasTypeMusic,
			ID:        123,
			Name:      "Song A",
		},
		Aliases: []string{"song a", "蓝歌"},
	}, "Asia/Shanghai")
	if !ok {
		t.Fatal("expected music alias request to be built")
	}
	if resolver.lastQuery.Query != "music123" {
		t.Fatalf("unexpected music cover lookup query: %+v", resolver.lastQuery)
	}
	if req.MusicJacketPath == nil || *req.MusicJacketPath != "music/jacket/jacket_test/jacket_test.png" {
		t.Fatalf("unexpected music jacket path: %+v", req.MusicJacketPath)
	}
}

func TestBuildAliasListImageRequestCharacterIncludesTrimTimezoneAndSilhouette(t *testing.T) {
	req, ok := buildAliasListImageRequest(nil, aliases.PjskAliasTypeCharacter, &aliases.QueryResult{
		Entity: aliases.EntityRef{
			AliasType: aliases.PjskAliasTypeCharacter,
			ID:        21,
			Name:      "桃井爱莉",
		},
		Aliases: []string{"爱莉", "桃桃"},
	}, "Asia/Shanghai")
	if !ok {
		t.Fatal("expected character alias request to be built")
	}
	if req.TimeZone != "Asia/Shanghai" {
		t.Fatalf("unexpected timezone: %q", req.TimeZone)
	}
	if req.DT <= 0 {
		t.Fatalf("expected dt to be populated, got %d", req.DT)
	}
	wantTrim := "asset/jp-assets/startapp/character/character_trim/chr_trim_21.png"
	if req.CharacterTrimPath == nil || *req.CharacterTrimPath != wantTrim {
		t.Fatalf("unexpected trim path: %+v", req.CharacterTrimPath)
	}
	if req.CharacterSilhouettePath == nil || *req.CharacterSilhouettePath != wantTrim {
		t.Fatalf("unexpected silhouette path: %+v", req.CharacterSilhouettePath)
	}
}
