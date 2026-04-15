package sekai

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/onebot11"
	aliases "haruki-cloud/internal/pjsk/alias"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"
)

func (sekaiHandlers) MusicAliasQueryHandle() SekaiCommandHandler {
	return newEntityAliasQueryHandler(
		aliases.AliasTypeMusic,
		"alias/music",
		[]string{"/pjsk alias", "/music alias", "/歌曲别名", "/查歌曲别名"},
		"/music alias",
	)
}

func (sekaiHandlers) MusicAliasAddHandle() SekaiCommandHandler {
	return newEntityAliasAddHandler(
		aliases.AliasTypeMusic,
		"alias/music/add",
		[]string{"/music alias add", "/pjsk alias add", "/pjskalias add", "/添加歌曲别名", "/歌曲别名添加"},
		"/music alias add",
	)
}

func (sekaiHandlers) MusicAliasDeleteHandle() SekaiCommandHandler {
	return newEntityAliasDeleteHandler(
		aliases.AliasTypeMusic,
		"alias/music/del",
		[]string{"/music alias del", "/pjsk alias del", "/pjskalias del", "/删除歌曲别名", "/歌曲别名删除"},
		"/music alias del",
	)
}

func (sekaiHandlers) CharacterAliasQueryHandle() SekaiCommandHandler {
	return newEntityAliasQueryHandler(
		aliases.AliasTypeCharacter,
		"alias/character",
		[]string{"/pjsk chara alias", "/chara alias", "/character alias", "/角色别名", "/查角色别名"},
		"/chara alias",
	)
}

func (sekaiHandlers) CharacterAliasAddHandle() SekaiCommandHandler {
	return newEntityAliasAddHandler(
		aliases.AliasTypeCharacter,
		"alias/character/add",
		[]string{"/pjsk chara alias add", "/chara alias add", "/character alias add", "/添加角色别名", "/角色别名添加"},
		"/chara alias add",
	)
}

func (sekaiHandlers) CharacterAliasDeleteHandle() SekaiCommandHandler {
	return newEntityAliasDeleteHandler(
		aliases.AliasTypeCharacter,
		"alias/character/del",
		[]string{"/pjsk chara alias del", "/chara alias del", "/character alias del", "/删除角色别名", "/角色别名删除"},
		"/chara alias del",
	)
}

func (sekaiHandlers) AliasPendingHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "alias/pending",
			Commands: []string{
				"/待审核别名", "/别名待审核",
				"/歌曲别名待审核", "/角色别名待审核",
			},
			Helper: "使用方式:\n/待审核别名",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, onebot11.NewReplayError("使用方式:\n/待审核别名")
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, aliases.ModePendingList, aliases.ReviewListCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
			}), nil
		},
	}
}

func (sekaiHandlers) AliasApproveHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "alias/approve",
			Commands: []string{
				"/同意别名", "/通过别名",
			},
			Helper: "使用方式:\n/同意别名 待审核ID1 待审核ID2 ...",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			reviewIDs, err := parseAliasReviewIDs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, aliases.ModeApprove, aliases.ApproveCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				ReviewIDs:      reviewIDs,
			}), nil
		},
	}
}

func (sekaiHandlers) AliasRejectHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "alias/reject",
			Commands: []string{
				"/拒绝别名",
			},
			Helper: "使用方式:\n/拒绝别名 待审核ID 原因",
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			reviewID, reason, err := parseAliasRejectArgs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, aliases.ModeReject, aliases.RejectCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				ReviewID:       reviewID,
				Reason:         reason,
			}), nil
		},
	}
}

func newEntityAliasQueryHandler(aliasType, path string, commands []string, sampleCommand string) SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path:     path,
			Commands: commands,
			Helper:   aliasQueryHelp(aliasType, sampleCommand),
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			target := strings.TrimSpace(ctx.GetArgs())
			if target == "" {
				return nil, onebot11.NewReplayError("%s", aliasQueryHelp(aliasType, sampleCommand))
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, aliases.ModeQuery, aliases.QueryCommandParams{
				AliasType: aliasType,
				Target:    target,
			}), nil
		},
	}
}

func newEntityAliasAddHandler(aliasType, path string, commands []string, sampleCommand string) SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path:     path,
			Commands: commands,
			Helper:   aliasAddHelp(aliasType, sampleCommand),
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			target, aliasValues, err := parseEntityAliasBulkArgs(strings.TrimSpace(ctx.GetArgs()), aliasAddHelp(aliasType, sampleCommand))
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, aliases.ModeAdd, aliases.AddCommandParams{
				AliasType:      aliasType,
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				Target:         target,
				Aliases:        aliasValues,
			}), nil
		},
	}
}

func newEntityAliasDeleteHandler(aliasType, path string, commands []string, sampleCommand string) SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path:     path,
			Commands: commands,
			Helper:   aliasDeleteHelp(aliasType, sampleCommand),
		},
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			target, aliasValues, err := parseEntityAliasBulkArgs(strings.TrimSpace(ctx.GetArgs()), aliasDeleteHelp(aliasType, sampleCommand))
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, aliases.ModeDelete, aliases.DeleteCommandParams{
				AliasType:      aliasType,
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				Target:         target,
				Aliases:        aliasValues,
			}), nil
		},
	}
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
	case aliases.AliasTypeMusic:
		return "歌曲"
	case aliases.AliasTypeCharacter:
		return "角色"
	default:
		return "目标"
	}
}

func aliasQueryTokenPrompt(aliasType string) string {
	switch aliasType {
	case aliases.AliasTypeMusic:
		return "歌曲ID 或 曲名 或 已审核别名"
	case aliases.AliasTypeCharacter:
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
