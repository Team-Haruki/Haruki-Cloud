package handler

import (
	"fmt"
	json "github.com/bytedance/sonic"
	"log/slog"
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
	aliases "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
)

const aliasImageThreshold = 20

type aliasMusicCoverResolver interface {
	ResolveMusicCover(query rendermusic.Query) (*rendermusic.CoverResult, error)
}

func (sekaiHandlers) MusicAliasQueryHandle() HarukiSekaiCommandHandler {
	return newEntityAliasQueryHandler(
		aliases.PjskAliasTypeMusic,
		"alias/music",
		[]string{"/pjsk alias", "/music alias", "/歌曲别名", "/查歌曲别名"},
		"/music alias",
	)
}

func (sekaiHandlers) MusicAliasAddHandle() HarukiSekaiCommandHandler {
	return newEntityAliasAddHandler(
		aliases.PjskAliasTypeMusic,
		"alias/music/add",
		[]string{"/music alias add", "/pjsk alias add", "/pjskalias add", "/添加歌曲别名", "/歌曲别名添加"},
		"/music alias add",
	)
}

func (sekaiHandlers) MusicAliasDeleteHandle() HarukiSekaiCommandHandler {
	return newEntityAliasDeleteHandler(
		aliases.PjskAliasTypeMusic,
		"alias/music/del",
		[]string{"/music alias del", "/pjsk alias del", "/pjskalias del", "/删除歌曲别名", "/歌曲别名删除"},
		"/music alias del",
	)
}

func (sekaiHandlers) CharacterAliasQueryHandle() HarukiSekaiCommandHandler {
	return newEntityAliasQueryHandler(
		aliases.PjskAliasTypeCharacter,
		"alias/character",
		[]string{"/pjsk chara alias", "/chara alias", "/character alias", "/角色别名", "/查角色别名"},
		"/chara alias",
	)
}

func (sekaiHandlers) CharacterAliasAddHandle() HarukiSekaiCommandHandler {
	return newEntityAliasAddHandler(
		aliases.PjskAliasTypeCharacter,
		"alias/character/add",
		[]string{"/pjsk chara alias add", "/chara alias add", "/character alias add", "/添加角色别名", "/角色别名添加"},
		"/chara alias add",
	)
}

func (sekaiHandlers) CharacterAliasDeleteHandle() HarukiSekaiCommandHandler {
	return newEntityAliasDeleteHandler(
		aliases.PjskAliasTypeCharacter,
		"alias/character/del",
		[]string{"/pjsk chara alias del", "/chara alias del", "/character alias del", "/删除角色别名", "/角色别名删除"},
		"/chara alias del",
	)
}

func (sekaiHandlers) AliasPendingHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "alias/pending",
			Commands: []string{
				"/待审核别名", "/别名待审核",
				"/歌曲别名待审核", "/角色别名待审核",
			},
			Helper: "使用方式:\n/待审核别名",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n/待审核别名")
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModePendingList, aliases.ReviewListCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
			}), nil
		},
	}, executeAlias)
}

func (sekaiHandlers) AliasSubmitterHandle() HarukiSekaiCommandHandler {
	const usage = "使用方式:\n/查询别名提交者 待审核ID"
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path:     "alias/submitter",
			Commands: []string{"/查询别名提交者", "/别名提交者"},
			Helper:   usage,
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			reviewID, err := parseAliasReviewID(strings.TrimSpace(ctx.GetArgs()), usage)
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
	const usage = "使用方式:\n/禁用别名提交 用户ID\n/禁用别名提交 @用户"
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path:     "alias/ban_submitter",
			Commands: []string{"/禁用别名提交", "/禁止别名提交"},
			Helper:   usage,
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			targetPlatform, targetUserID, err := parseAliasSubmissionTarget(
				strings.TrimSpace(ctx.GetArgs()),
				ctx.GetPlatform(),
				ctx.GetAtIds(),
				usage,
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
		CommandHandlerBase: CommandHandlerBase{
			Path: "alias/approve",
			Commands: []string{
				"/同意别名", "/通过别名",
			},
			Helper: "使用方式:\n/同意别名 待审核ID1 待审核ID2 ...",
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
		CommandHandlerBase: CommandHandlerBase{
			Path: "alias/reject",
			Commands: []string{
				"/拒绝别名",
			},
			Helper: "使用方式:\n/拒绝别名 待审核ID 原因",
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

func newEntityAliasQueryHandler(aliasType, path string, commands []string, sampleCommand string) HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path:     path,
			Commands: commands,
			Helper:   aliasQueryHelp(aliasType, sampleCommand),
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			target := strings.TrimSpace(ctx.GetArgs())
			if target == "" {
				return nil, onebot11.NewReplayError("%s", aliasQueryHelp(aliasType, sampleCommand))
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAlias, aliases.ModeQuery, aliases.QueryCommandParams{
				AliasType: aliasType,
				Target:    target,
			}), nil
		},
	}, executeAlias)
}

func newEntityAliasAddHandler(aliasType, path string, commands []string, sampleCommand string) HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path:     path,
			Commands: commands,
			Helper:   aliasAddHelp(aliasType, sampleCommand),
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			target, aliasValues, err := parseEntityAliasBulkArgs(strings.TrimSpace(ctx.GetArgs()), aliasAddHelp(aliasType, sampleCommand))
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

func newEntityAliasDeleteHandler(aliasType, path string, commands []string, sampleCommand string) HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path:     path,
			Commands: commands,
			Helper:   aliasDeleteHelp(aliasType, sampleCommand),
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			target, aliasValues, err := parseEntityAliasBulkArgs(strings.TrimSpace(ctx.GetArgs()), aliasDeleteHelp(aliasType, sampleCommand))
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

func aliasQueryHelp(aliasType, sampleCommand string) string {
	return fmt.Sprintf(`使用方式:
%s %s

说明:
- 按 %s 的顺序查找
- 只能查询已审核通过的%s别名`, sampleCommand, aliasQueryTokenPrompt(aliasType), aliasQueryTokenPrompt(aliasType), aliasTypeLabel(aliasType))
}

func aliasAddHelp(aliasType, sampleCommand string) string {
	return fmt.Sprintf(`使用方式:
%s
%s
别名1
别名2
...

说明:
- 第二行开始每行填写一个别名
- 至少提供一个非空别名
- 新别名会进入待审核列表，不会直接生效`, sampleCommand, aliasQueryTokenPrompt(aliasType))
}

func aliasDeleteHelp(aliasType, sampleCommand string) string {
	return fmt.Sprintf(`使用方式:
%s
%s
别名1
别名2
...

说明:
- 只能删除已审核通过的%s别名
- 第二行开始每行填写一个要删除的别名
- 仅别名审核管理员可用`, sampleCommand, aliasQueryTokenPrompt(aliasType), aliasTypeLabel(aliasType))
}

func aliasTypeLabel(aliasType string) string {
	switch aliasType {
	case aliases.PjskAliasTypeMusic:
		return "歌曲"
	case aliases.PjskAliasTypeCharacter:
		return "角色"
	default:
		return "目标"
	}
}

func aliasQueryTokenPrompt(aliasType string) string {
	switch aliasType {
	case aliases.PjskAliasTypeMusic:
		return "歌曲ID 或 曲名 或 已审核别名"
	case aliases.PjskAliasTypeCharacter:
		return "角色ID 或 角色名 或 已审核别名"
	default:
		return "ID 或 名称 或 已审核别名"
	}
}

func parseEntityAliasBulkArgs(args, usage string) (string, []string, error) {
	args = strings.TrimSpace(strings.ReplaceAll(args, "\r\n", "\n"))
	if args == "" {
		return "", nil, onebot11.NewReplayError("%s", usage)
	}

	lines := strings.Split(args, "\n")
	target := strings.TrimSpace(lines[0])
	if target == "" {
		return "", nil, onebot11.NewReplayError("%s", usage)
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
		return "", nil, onebot11.NewReplayError("请至少提供一个非空别名\n\n%s", usage)
	}
	return target, aliasValues, nil
}

func parseAliasReviewIDs(args string) ([]int64, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil, onebot11.NewReplayError("使用方式:\n/同意别名 待审核ID1 待审核ID2 ...")
	}
	result := make([]int64, 0, len(fields))
	for _, field := range fields {
		reviewID, err := strconv.ParseInt(field, 10, 64)
		if err != nil || reviewID <= 0 {
			return nil, onebot11.NewReplayError("待审核ID必须为正整数")
		}
		result = append(result, reviewID)
	}
	return result, nil
}

func parseAliasReviewID(args, usage string) (int64, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) != 1 {
		return 0, onebot11.NewReplayError("%s", usage)
	}
	reviewID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || reviewID <= 0 {
		return 0, onebot11.NewReplayError("待审核ID必须为正整数")
	}
	return reviewID, nil
}

func parseAliasSubmissionTarget(args, currentPlatform string, atIDs []string, usage string) (string, string, error) {
	currentPlatform = strings.TrimSpace(currentPlatform)
	if len(atIDs) > 0 && strings.TrimSpace(atIDs[0]) != "" {
		return currentPlatform, strings.TrimSpace(atIDs[0]), nil
	}
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) != 1 {
		return "", "", onebot11.NewReplayError("%s", usage)
	}
	target := fields[0]
	if platform, userID, ok := strings.Cut(target, ":"); ok {
		platform = strings.TrimSpace(platform)
		userID = strings.TrimSpace(userID)
		if platform == "" || userID == "" {
			return "", "", onebot11.NewReplayError("%s", usage)
		}
		return platform, userID, nil
	}
	if currentPlatform == "" {
		return "", "", onebot11.NewReplayError("缺少平台信息")
	}
	return currentPlatform, target, nil
}

func parseAliasRejectArgs(args string) (int64, string, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return 0, "", onebot11.NewReplayError("使用方式:\n/拒绝别名 待审核ID 原因")
	}
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return 0, "", onebot11.NewReplayError("使用方式:\n/拒绝别名 待审核ID 原因")
	}
	reviewID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || reviewID <= 0 {
		return 0, "", onebot11.NewReplayError("待审核ID必须为正整数")
	}
	reason := strings.TrimSpace(strings.TrimPrefix(args, parts[0]))
	if reason == "" {
		return 0, "", onebot11.NewReplayError("请输入拒绝原因")
	}
	return reviewID, reason, nil
}

func executeAlias(rc *RequestContext) (onebot11.Message, error) {
	if rc.App == nil || rc.App.Aliases == nil {
		return nil, fmt.Errorf("别名服务未就绪，请稍后再试")
	}
	if message, ok, err := tryRenderAliasQueryAsImage(rc); ok {
		return message, err
	}
	data, err := aliases.ExecuteCommand(rc.Ctx, rc.App.Aliases, rc.Cmd.Mode, rc.Cmd.Params)
	if err != nil {
		return nil, err
	}
	return onebot11.Message{onebot11.Text(string(data))}, nil
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
	payload, renderErr := rc.App.Misc.WithContext(rc.Ctx).RenderAliasList(req)
	if renderErr != nil {
		slog.WarnContext(rc.Ctx, "alias image fallback render failed",
			"alias_type", params.AliasType,
			"entity_id", result.Entity.ID,
			"alias_count", len(result.Aliases),
			"error_type", fmt.Sprintf("%T", renderErr),
		)
		return nil, false, nil
	}
	message, err := imageMessage(rc.Ctx, payload, rc.App, BotModulePJSK)
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
			Title:       "歌曲别名",
			EntityLabel: "歌曲ID",
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
			Title:                   "角色别名",
			EntityLabel:             "角色ID",
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
