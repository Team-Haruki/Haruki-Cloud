package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/musicalias"
	"haruki-cloud/internal/pjsk/parser"
	"strconv"
	"strings"
)

const (
	MUSIC_ALIAS_QUERY_HELP = `使用方式:
/music alias 歌曲ID 或 曲名 或 已审核别名

说明:
- 按 歌曲ID -> 曲名 -> 已审核别名 的顺序查找
- 只能查询已审核通过的歌曲别名`

	MUSIC_ALIAS_ADD_HELP = `使用方式:
/music alias add
歌曲ID 或 曲名 或 已审核别名
别名1
别名2
...

说明:
- 第二行开始每行填写一个别名
- 至少提供一个非空别名
- 新别名会进入待审核列表，不会直接生效`

	MUSIC_ALIAS_DELETE_HELP = `使用方式:
/music alias del
歌曲ID 或 曲名 或 已审核别名
别名1
别名2
...

说明:
- 只能删除已审核通过的歌曲别名
- 第二行开始每行填写一个要删除的别名
- 仅歌曲别名审核管理员可用`

	MUSIC_ALIAS_PENDING_HELP = `使用方式:
/待审核别名`

	MUSIC_ALIAS_APPROVE_HELP = `使用方式:
/同意别名 待审核ID1 待审核ID2 ...`

	MUSIC_ALIAS_REJECT_HELP = `使用方式:
/拒绝别名 待审核ID 原因`
)

func (sekaiHandlers) MusicDetailHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music",
			Commands: []string{
				"/查曲", "/查歌", "/查乐", "/查音乐", "/查询乐曲", "/查歌曲", "/歌曲", "/乐曲", "/song", "/music",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if diff, cleaned := extractMusicDifficulty(args); diff != "" {
				ctx.SetArgs(cleaned)
				return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-detail", map[string]any{
					"difficulty": diff,
				}), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-detail"), nil
		},
	}
}

func (sekaiHandlers) MusicListHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/list",
			Commands: []string{
				"/歌曲列表", "/歌曲一览", "/乐曲列表", "/乐曲一览", "/难度排行", "/定数表", "/歌曲定数", "/查乐曲", "/music-list", "/pjsk music list",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if diff, cleaned := extractMusicDifficulty(args); diff != "" {
				ctx.SetArgs(cleaned)
				return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-list", map[string]any{
					"difficulty": diff,
				}), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-list"), nil
		},
	}
}

func (sekaiHandlers) MusicRewardsHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/rewards",
			Commands: []string{
				"/曲目奖励", "/歌曲奖励", "/music rewards", "/music-rewards", "/pjsk music rewards",
				"/打歌奖励", "/歌曲挖矿", "/打歌挖矿", "/pjsk 曲目奖励",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-rewards"), nil
		},
	}
}

func (sekaiHandlers) MusicProgressHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/progress",
			Commands: []string{
				"/打歌进度", "/歌曲进度", "/打歌信息", "/pjsk进度", "/progress", "/music-progress", "/pjsk music progress",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-progress"), nil
		},
	}
}

func (sekaiHandlers) AliasDelHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/alias/del",
			Commands: []string{
				"/music alias del",
				"/pjsk alias del", "/pjskalias del",
				"/删除歌曲别名", "/歌曲别名删除",
			},
			Helper: MUSIC_ALIAS_DELETE_HELP,
		},
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			target, aliases, err := parseMusicAliasDeleteArgs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, musicalias.ModeDelete, musicalias.DeleteCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				Target:         target,
				Aliases:        aliases,
			}), nil
		},
	}
}

func (sekaiHandlers) SongHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music",
			Commands: []string{
				"/pjsk song", "/pjsk music", "/song", "/music",
				"/查曲", "/查歌", "/歌曲", "/查歌曲",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			query := strings.TrimSpace(ctx.GetArgs())
			if query == "" {
				return nil, fmt.Errorf("请输入要查询的歌曲名或ID")
			}
			if diff, cleaned := extractMusicDifficulty(query); diff != "" {
				ctx.SetArgs(cleaned)
				return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-detail", map[string]any{
					"difficulty": diff,
				}), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-detail"), nil
		},
	}
}

func (sekaiHandlers) NoteNumHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/note-count",
			Commands: []string{
				"/pjsk note num", "/pjsk note count",
				"/物量", "/查物量",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			noteCount, err := strconv.Atoi(args)
			if err != nil {
				return nil, fmt.Errorf("请输入物量数值")
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-note-count", map[string]any{
				"note_count": noteCount,
			}), nil
		},
	}
}

func (sekaiHandlers) PlayProgressHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/progress",
			Commands: []string{
				"/pjsk progress",
				"/pjsk进度", "/打歌进度", "/歌曲进度", "/打歌信息",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if diff, cleaned := extractMusicDifficulty(args); diff != "" {
				ctx.SetArgs(cleaned)
				return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-progress", map[string]any{
					"difficulty": diff,
				}), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-progress"), nil
		},
	}
}

func (sekaiHandlers) BPMHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/bpm",
			Commands: []string{
				"/pjsk bpm", "/查bpm", "/查BPM",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			query := strings.TrimSpace(ctx.GetArgs())
			if query == "" {
				return nil, fmt.Errorf("请输入要查询的歌曲名或ID")
			}
			if diff, cleaned := extractMusicDifficulty(query); diff != "" {
				ctx.SetArgs(cleaned)
				return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-bpm", map[string]any{
					"difficulty": diff,
				}), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-bpm"), nil
		},
	}
}

func (sekaiHandlers) MusicCoverHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/cover",
			Commands: []string{
				"/pjsk music cover",
				"/查曲绘", "/曲绘",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			query := strings.TrimSpace(ctx.GetArgs())
			if query == "" {
				return nil, fmt.Errorf("请输入要查询的歌曲名或ID")
			}
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-cover"), nil
		},
	}
}

func (sekaiHandlers) AliasSetHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/alias/add",
			Commands: []string{
				"/music alias add", "/pjsk alias add", "/pjskalias add",
				"/添加歌曲别名", "/歌曲别名添加",
			},
			Helper: MUSIC_ALIAS_ADD_HELP,
		},
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			target, aliases, err := parseMusicAliasAddArgs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, musicalias.ModeAdd, musicalias.AddCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				Target:         target,
				Aliases:        aliases,
			}), nil
		},
	}
}

func (sekaiHandlers) AliasHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/alias",
			Commands: []string{
				"/pjsk alias", "/music alias",
				"/歌曲别名", "/查歌曲别名",
			},
			Helper: MUSIC_ALIAS_QUERY_HELP,
		},
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			target := strings.TrimSpace(ctx.GetArgs())
			if target == "" {
				return nil, fmt.Errorf(MUSIC_ALIAS_QUERY_HELP)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, musicalias.ModeQuery, musicalias.QueryCommandParams{
				Target: target,
			}), nil
		},
	}
}

func (sekaiHandlers) PendingAliasHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/alias/pending",
			Commands: []string{
				"/待审核别名", "/歌曲别名待审核",
			},
			Helper: MUSIC_ALIAS_PENDING_HELP,
		},
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			if strings.TrimSpace(ctx.GetArgs()) != "" {
				return nil, fmt.Errorf(MUSIC_ALIAS_PENDING_HELP)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, musicalias.ModePendingList, musicalias.ReviewListCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
			}), nil
		},
	}
}

func (sekaiHandlers) ApproveAliasHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/alias/approve",
			Commands: []string{
				"/同意别名", "/通过别名",
			},
			Helper: MUSIC_ALIAS_APPROVE_HELP,
		},
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			reviewIDs, err := parseAliasReviewIDs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, musicalias.ModeApprove, musicalias.ApproveCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				ReviewIDs:      reviewIDs,
			}), nil
		},
	}
}

func (sekaiHandlers) RejectAliasHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/alias/reject",
			Commands: []string{
				"/拒绝别名",
			},
			Helper: MUSIC_ALIAS_REJECT_HELP,
		},
		ParseUIDArg: boolPtr(false),
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			reviewID, reason, err := parseAliasRejectArgs(strings.TrimSpace(ctx.GetArgs()))
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleAlias, musicalias.ModeReject, musicalias.RejectCommandParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				ReviewID:       reviewID,
				Reason:         reason,
			}), nil
		},
	}
}

func extractMusicDifficulty(args string) (string, string) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return "", strings.TrimSpace(args)
	}
	diffMap := map[string]string{
		"easy": "easy", "ez": "easy", "绿谱": "easy",
		"normal": "normal", "nm": "normal", "蓝谱": "normal",
		"hard": "hard", "hd": "hard", "黄谱": "hard",
		"expert": "expert", "ex": "expert", "红谱": "expert",
		"master": "master", "mas": "master", "紫谱": "master",
		"append": "append", "app": "append", "粉谱": "append",
	}
	for i, field := range fields {
		if diff, ok := diffMap[strings.ToLower(field)]; ok {
			fields = append(fields[:i], fields[i+1:]...)
			return diff, strings.TrimSpace(strings.Join(fields, " "))
		}
	}
	return "", strings.TrimSpace(args)
}

func parseMusicAliasAddArgs(args string) (string, []string, error) {
	return parseMusicAliasBulkArgs(args, MUSIC_ALIAS_ADD_HELP)
}

func parseMusicAliasDeleteArgs(args string) (string, []string, error) {
	return parseMusicAliasBulkArgs(args, MUSIC_ALIAS_DELETE_HELP)
}

func parseMusicAliasBulkArgs(args, usage string) (string, []string, error) {
	args = strings.TrimSpace(strings.ReplaceAll(args, "\r\n", "\n"))
	if args == "" {
		return "", nil, fmt.Errorf("%s", usage)
	}

	lines := strings.Split(args, "\n")
	target := strings.TrimSpace(lines[0])
	if target == "" {
		return "", nil, fmt.Errorf(MUSIC_ALIAS_QUERY_HELP)
	}

	aliases := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		aliasText := strings.TrimSpace(line)
		if aliasText == "" {
			continue
		}
		aliases = append(aliases, aliasText)
	}
	if len(aliases) == 0 {
		return "", nil, fmt.Errorf("请至少提供一个非空别名\n\n%s", usage)
	}
	return target, aliases, nil
}

func parseAliasReviewIDs(args string) ([]int64, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil, fmt.Errorf(MUSIC_ALIAS_APPROVE_HELP)
	}
	result := make([]int64, 0, len(fields))
	for _, field := range fields {
		reviewID, err := strconv.ParseInt(field, 10, 64)
		if err != nil || reviewID <= 0 {
			return nil, fmt.Errorf("待审核ID必须为正整数")
		}
		result = append(result, reviewID)
	}
	return result, nil
}

func parseAliasRejectArgs(args string) (int64, string, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return 0, "", fmt.Errorf(MUSIC_ALIAS_REJECT_HELP)
	}
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return 0, "", fmt.Errorf(MUSIC_ALIAS_REJECT_HELP)
	}
	reviewID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || reviewID <= 0 {
		return 0, "", fmt.Errorf("待审核ID必须为正整数")
	}
	reason := strings.TrimSpace(strings.TrimPrefix(args, parts[0]))
	if reason == "" {
		return 0, "", fmt.Errorf("请输入拒绝原因")
	}
	return reviewID, reason, nil
}
