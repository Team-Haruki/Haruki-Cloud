package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"strconv"
	"strings"
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
			Commands: []string{
				"/pjsk alias del", "/pjskalias del",
				"/删除歌曲别名", "/歌曲别名删除",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			return nil, fmt.Errorf("TODO: 删除歌曲别名未实现，args=%q", args)
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
			Commands: []string{
				"/pjsk note num", "/pjsk note count",
				"/物量", "/查物量",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			noteCount, err := strconv.Atoi(args)
			if err != nil {
				return nil, fmt.Errorf("请输入物量数值")
			}
			return nil, fmt.Errorf("TODO: 物量查询未实现，note_count=%d", noteCount)
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
			Commands: []string{
				"/pjsk bpm", "/查bpm", "/查BPM",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			query := strings.TrimSpace(ctx.GetArgs())
			return nil, fmt.Errorf("TODO: BPM查询未实现，query=%q", query)
		},
	}
}

func (sekaiHandlers) MusicCoverHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk music cover",
				"/查曲绘", "/曲绘",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			query := strings.TrimSpace(ctx.GetArgs())
			return nil, fmt.Errorf("TODO: 曲绘查询未实现，query=%q", query)
		},
	}
}

func (sekaiHandlers) AliasSetHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk alias add", "/pjskalias add",
				"/添加歌曲别名", "/歌曲别名添加",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			return nil, fmt.Errorf("TODO: 添加歌曲别名未实现，args=%q", args)
		},
	}
}

func (sekaiHandlers) AliasHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk alias", "/music alias",
				"/歌曲别名", "/查歌曲别名",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			return nil, fmt.Errorf("TODO: 查看歌曲别名未实现，args=%q", args)
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
		"normal": "normal", "nm": "normal", "黄谱": "normal",
		"hard": "hard", "hd": "hard", "红谱": "hard",
		"expert": "expert", "ex": "expert", "紫谱": "expert",
		"master": "master", "mas": "master", "粉谱": "master",
		"append": "append", "app": "append", "黑谱": "append",
	}
	for i, field := range fields {
		if diff, ok := diffMap[strings.ToLower(field)]; ok {
			fields = append(fields[:i], fields[i+1:]...)
			return diff, strings.TrimSpace(strings.Join(fields, " "))
		}
	}
	return "", strings.TrimSpace(args)
}
