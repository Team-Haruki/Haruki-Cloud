package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/onebot11"
	aliases "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/utils/usererror"
)

const aliasImageThreshold = 20

type aliasMusicCoverResolver interface {
	ResolveMusicCover(query rendermusic.Query) (*rendermusic.CoverResult, error)
}

func (sekaiHandlers) MusicAliasQueryHandle() HarukiSekaiCommandHandler {
	return newEntityAliasQueryHandler(
		aliases.PjskAliasTypeMusic,
		"alias/music",
		[]string{"/pjsk alias", "/music alias", "/歌曲别名", "/查歌曲别名"}, //copylint:ignore command triggers
	)
}

func (sekaiHandlers) MusicAliasAddHandle() HarukiSekaiCommandHandler {
	return newEntityAliasAddHandler(
		aliases.PjskAliasTypeMusic,
		"alias/music/add",
		[]string{"/music alias add", "/pjsk alias add", "/pjskalias add", "/添加歌曲别名", "/歌曲别名添加"}, //copylint:ignore command triggers
	)
}

func (sekaiHandlers) MusicAliasDeleteHandle() HarukiSekaiCommandHandler {
	return newEntityAliasDeleteHandler(
		aliases.PjskAliasTypeMusic,
		"alias/music/del",
		[]string{"/music alias del", "/pjsk alias del", "/pjskalias del", "/删除歌曲别名", "/歌曲别名删除"}, //copylint:ignore command triggers
	)
}

func (sekaiHandlers) CharacterAliasQueryHandle() HarukiSekaiCommandHandler {
	return newEntityAliasQueryHandler(
		aliases.PjskAliasTypeCharacter,
		"alias/character",
		[]string{"/pjsk chara alias", "/chara alias", "/character alias", "/角色别名", "/查角色别名"}, //copylint:ignore command triggers
	)
}

func (sekaiHandlers) CharacterAliasAddHandle() HarukiSekaiCommandHandler {
	return newEntityAliasAddHandler(
		aliases.PjskAliasTypeCharacter,
		"alias/character/add",
		[]string{"/pjsk chara alias add", "/chara alias add", "/character alias add", "/添加角色别名", "/角色别名添加"}, //copylint:ignore command triggers
	)
}

func (sekaiHandlers) CharacterAliasDeleteHandle() HarukiSekaiCommandHandler {
	return newEntityAliasDeleteHandler(
		aliases.PjskAliasTypeCharacter,
		"alias/character/del",
		[]string{"/pjsk chara alias del", "/chara alias del", "/character alias del", "/删除角色别名", "/角色别名删除"}, //copylint:ignore command triggers
	)
}

func (sekaiHandlers) AliasPendingHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "alias/pending",
		Commands: []string{
			"/待审核别名", "/别名待审核",
			"/歌曲别名待审核", "/角色别名待审核",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, usererror.Misuse(i18n.M("common.no_args"))
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModePendingList, aliases.ReviewListCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
			}), nil
		},
	}, executeAlias)
}

func (sekaiHandlers) AliasSubmitterHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        "alias/submitter",
		Commands:    []string{"/查询别名提交者", "/别名提交者"},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			reviewID, err := parseAliasReviewID(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeSubmitter, aliases.SubmitterCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				ReviewID:       reviewID,
			}), nil
		},
	}, executeAlias)
}

func (sekaiHandlers) AliasBanSubmitterHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        "alias/ban_submitter",
		Commands:    []string{"/禁用别名提交", "/禁止别名提交"},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			targetPlatform, targetUserID, err := parseAliasSubmissionTarget(
				strings.TrimSpace(ctx.GetArgs()),
				ctx.GetPlatform(),
				ctx.GetAtIds(),
			)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeBanSubmitter, aliases.BanSubmitterCommandParams{
				Platform:             ctx.GetPlatform(),
				PlatformUserID:       ctx.GetUserId(),
				TargetPlatform:       targetPlatform,
				TargetPlatformUserID: targetUserID,
			}), nil
		},
	}, executeAlias)
}

func (sekaiHandlers) AliasApproveHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "alias/approve",
		Commands: []string{
			"/同意别名", "/通过别名",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			reviewIDs, err := parseAliasReviewIDs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeApprove, aliases.ApproveCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				ReviewIDs:      reviewIDs,
			}), nil
		},
	}, executeAlias)
}

func (sekaiHandlers) AliasRejectHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "alias/reject",
		Commands: []string{
			"/拒绝别名",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			reviewID, reason, err := parseAliasRejectArgs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeReject, aliases.RejectCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				ReviewID:       reviewID,
				Reason:         reason,
			}), nil
		},
	}, executeAlias)
}

func (sekaiHandlers) AliasBatchRejectHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        "alias/batch-reject",
		Commands:    []string{"/批量拒绝别名"},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			reviewIDs, err := parseAliasReviewIDs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeBatchReject, aliases.BatchRejectCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				ReviewIDs:      reviewIDs,
			}), nil
		},
	}, executeAlias)
}

func newEntityAliasQueryHandler(aliasType, path string, commands []string) HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        path,
		Commands:    commands,
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			target := strings.TrimSpace(ctx.GetArgs())
			if target == "" {
				return nil, aliasTargetRequiredError(aliasType)
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeQuery, aliases.QueryCommandParams{
				AliasType: aliasType,
				Target:    target,
			}), nil
		},
	}, executeAlias)
}

func newEntityAliasAddHandler(aliasType, path string, commands []string) HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        path,
		Commands:    commands,
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			target, aliasValues, err := parseEntityAliasBulkArgs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeAdd, aliases.AddCommandParams{
				AliasType:      aliasType,
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				Target:         target,
				Aliases:        aliasValues,
			}), nil
		},
	}, executeAlias)
}

func newEntityAliasDeleteHandler(aliasType, path string, commands []string) HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        path,
		Commands:    commands,
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			target, aliasValues, err := parseEntityAliasBulkArgs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeDelete, aliases.DeleteCommandParams{
				AliasType:      aliasType,
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				Target:         target,
				Aliases:        aliasValues,
			}), nil
		},
	}, executeAlias)
}

// aliasTargetRequiredError is the reply when an alias query names no song
// or character.
func aliasTargetRequiredError(aliasType string) error {
	if aliasType == aliases.PjskAliasTypeCharacter {
		return usererror.Misuse(i18n.M("alias.target_required.character"))
	}
	return usererror.Misuse(i18n.M("alias.target_required.music"))
}

func parseEntityAliasBulkArgs(args string) (string, []string, error) {
	args = strings.TrimSpace(strings.ReplaceAll(args, "\r\n", "\n"))
	if args == "" {
		return "", nil, usererror.Misuse(i18n.M("alias.bulk_usage"))
	}

	lines := strings.Split(args, "\n")
	target := strings.TrimSpace(lines[0])
	if target == "" {
		return "", nil, usererror.Misuse(i18n.M("alias.bulk_usage"))
	}

	aliasValues := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		aliasText := strings.TrimSpace(line)
		if aliasText == "" {
			continue
		}
		aliasValues = append(aliasValues, aliasText)
	}
	if len(aliasValues) == 0 {
		return "", nil, usererror.Misuse(i18n.M("alias.aliases_required"))
	}
	return target, aliasValues, nil
}

func parseAliasReviewIDs(args string) ([]int64, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil, usererror.Misuse(i18n.M("alias.review_ids_required"))
	}
	result := make([]int64, 0, len(fields))
	for _, field := range fields {
		reviewID, err := strconv.ParseInt(field, 10, 64)
		if err != nil || reviewID <= 0 {
			return nil, usererror.Invalid(i18n.M("alias.review_id_positive"))
		}
		result = append(result, reviewID)
	}
	return result, nil
}

func parseAliasReviewID(args string) (int64, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) != 1 {
		return 0, usererror.Misuse(i18n.M("alias.single_review_id"))
	}
	reviewID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || reviewID <= 0 {
		return 0, usererror.Invalid(i18n.M("alias.review_id_positive"))
	}
	return reviewID, nil
}

func parseAliasSubmissionTarget(args, currentPlatform string, atIDs []string) (string, string, error) {
	currentPlatform = strings.TrimSpace(currentPlatform)
	if len(atIDs) > 0 && strings.TrimSpace(atIDs[0]) != "" {
		return currentPlatform, strings.TrimSpace(atIDs[0]), nil
	}
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) != 1 {
		return "", "", usererror.Misuse(i18n.M("alias.submitter_target_usage"))
	}
	target := fields[0]
	if platform, userID, ok := strings.Cut(target, ":"); ok {
		platform = strings.TrimSpace(platform)
		userID = strings.TrimSpace(userID)
		if platform == "" || userID == "" {
			return "", "", usererror.Misuse(i18n.M("alias.submitter_target_usage"))
		}
		return platform, userID, nil
	}
	if currentPlatform == "" {
		return "", "", usererror.Misuse(i18n.M("alias.submitter_target_usage"))
	}
	return currentPlatform, target, nil
}

func parseAliasRejectArgs(args string) (int64, string, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return 0, "", usererror.Misuse(i18n.M("alias.reject_usage"))
	}
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return 0, "", usererror.Misuse(i18n.M("alias.reject_usage"))
	}
	reviewID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || reviewID <= 0 {
		return 0, "", usererror.Invalid(i18n.M("alias.review_id_positive"))
	}
	reason := strings.TrimSpace(strings.TrimPrefix(args, parts[0]))
	if reason == "" {
		return 0, "", usererror.Misuse(i18n.M("alias.reject_reason_required"))
	}
	return reviewID, reason, nil
}

func executeAlias(rc *RequestContext) (onebot11.Message, error) {
	if rc.App == nil || rc.App.Aliases == nil {
		return nil, usererror.Unavailable(i18n.M("alias.feature"), errors.New("alias service is not configured"))
	}
	if message, ok, err := tryRenderAliasQueryAsImage(rc); ok {
		return message, err
	}
	reply, err := aliases.ExecuteCommand(rc.Ctx, rc.App.Aliases, rc.Cmd.Mode, rc.Cmd.Params)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.LocalizedText(reply)}, nil
}

func tryRenderAliasQueryAsImage(rc *RequestContext) (onebot11.Message, bool, error) {
	if rc == nil || rc.App == nil || rc.App.Aliases == nil || rc.App.Music == nil {
		return nil, false, nil
	}
	if rc.Cmd == nil || rc.Cmd.Mode != aliases.ModeQuery {
		return nil, false, nil
	}

	var params aliases.QueryCommandParams
	if err := json.Unmarshal(rc.Cmd.Params, &params); err != nil {
		return nil, false, err
	}
	params.AliasType = strings.TrimSpace(params.AliasType)
	params.Target = strings.TrimSpace(params.Target)
	if params.Target == "" {
		return nil, false, nil
	}

	result, err := rc.App.Aliases.Query(rc.Ctx, params.AliasType, params.Target)
	if err != nil {
		return nil, false, err
	}
	if !shouldRenderAliasQueryAsImage(params.AliasType, result.Aliases) {
		return nil, false, nil
	}

	req, ok := buildAliasListImageRequest(
		rc.App.Music.WithContext(rc.Ctx),
		params.AliasType,
		result,
		resolveRequesterHarukiUserTimeZone(rc.Ctx, rc.App, rc.Platform, rc.PlatformUserID),
	)
	if !ok || rc.App.Misc == nil {
		return nil, false, nil
	}
	payload, renderErr := rc.App.Misc.WithContext(rc.Ctx).RenderAliasListImage(req)
	if renderErr != nil {
		slog.WarnContext(rc.Ctx, "alias image fallback render failed",
			"alias_type", params.AliasType,
			"entity_id", result.Entity.ID,
			"alias_count", len(result.Aliases),
			"error_type", fmt.Sprintf("%T", renderErr),
		)
		return nil, false, nil
	}
	message, err := rc.RenderedImageMessage(payload)
	if err != nil {
		return nil, false, err
	}
	return message, true, nil
}

func shouldRenderAliasQueryAsImage(aliasType string, aliasesList []string) bool {
	switch strings.TrimSpace(aliasType) {
	case aliases.PjskAliasTypeMusic, aliases.PjskAliasTypeCharacter:
		return len(aliasesList) >= aliasImageThreshold
	default:
		return false
	}
}

func buildCharacterAliasTrimPath(characterID int) *string {
	if characterID <= 0 {
		return nil
	}
	assetBase := renderassets.RegionAssetDirByMode("", renderassets.RegionAssetStartApp)
	return drawing.StringPtr(fmt.Sprintf("%s/character/character_trim/chr_trim_%d.png", assetBase, characterID))
}

func buildAliasListImageRequest(musicCtrl aliasMusicCoverResolver, aliasType string, result *aliases.QueryResult, timeZone string) (drawing.AliasListRequest, bool) {
	if result == nil {
		return drawing.AliasListRequest{}, false
	}
	trimPath := buildCharacterAliasTrimPath(result.Entity.ID)
	now := displaytime.Now(timeZone).UnixMilli()
	switch strings.TrimSpace(aliasType) {
	case aliases.PjskAliasTypeMusic:
		req := drawing.AliasListRequest{
			Title:       i18n.T("alias.image.title.music"),
			EntityLabel: i18n.T("alias.image.entity_label.music"),
			EntityType:  aliases.PjskAliasTypeMusic,
			EntityID:    result.Entity.ID,
			EntityName:  result.Entity.Name,
			TimeZone:    timeZone,
			DT:          now,
			Aliases:     result.Aliases,
		}
		if musicCtrl != nil && result.Entity.ID > 0 {
			if cover, err := musicCtrl.ResolveMusicCover(rendermusic.Query{Query: fmt.Sprintf("music%d", result.Entity.ID)}); err == nil && cover != nil && strings.TrimSpace(cover.JacketPath) != "" {
				req.MusicJacketPath = drawing.StringPtr(strings.TrimSpace(cover.JacketPath))
			}
		}
		return req, true
	case aliases.PjskAliasTypeCharacter:
		return drawing.AliasListRequest{
			Title:                   i18n.T("alias.image.title.character"),
			EntityLabel:             i18n.T("alias.image.entity_label.character"),
			EntityType:              aliases.PjskAliasTypeCharacter,
			EntityID:                result.Entity.ID,
			EntityName:              result.Entity.Name,
			TimeZone:                timeZone,
			DT:                      now,
			CharacterTrimPath:       trimPath,
			CharacterSilhouettePath: trimPath,
			Aliases:                 result.Aliases,
		}, true
	default:
		return drawing.AliasListRequest{}, false
	}
}
