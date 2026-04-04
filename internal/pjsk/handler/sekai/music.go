package sekai

import (
	"fmt"
	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"strconv"
	"strings"
)
/*
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
*/
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
				"/打歌进度", "/歌曲进度", "/打歌信息", "/pjsk进度", "/progress", "/music-progress", "/pjsk music progress", "/pjsk progress",
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
				return nil, onebot11.NewReplayError("请输入要查询的歌曲名或ID")
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
				return nil, onebot11.NewReplayError("请输入物量数值")
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-note-count", map[string]any{
				"note_count": noteCount,
			}), nil
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
				return nil, onebot11.NewReplayError("请输入要查询的歌曲名或ID")
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

func extractMusicDifficulty(args string) (string, string) {
	return rendermusic.ExtractMusicDifficulty(args)
}
