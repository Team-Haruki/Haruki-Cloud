package sekai

import (
	"fmt"
	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"regexp"
	"sort"
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
			handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := make(map[string]any)
			if diff, cleaned := extractMusicDifficulty(args); diff != "" {
				args = cleaned
				params["difficulty"] = diff
			}
			if levelParams, cleaned, ok := extractMusicListLevelArgs(args); ok {
				args = cleaned
				for key, value := range levelParams {
					params[key] = value
				}
			}
			ctx.SetArgs(args)
			if len(params) == 0 {
				return makeResolvedCmd(ctx, parser.ModuleMusic, "music-list"), nil
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-list", params), nil
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
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
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

func extractMusicListLevelArgs(args string) (map[string]any, string, bool) {
	tokens := strings.Fields(strings.TrimSpace(args))
	for i := 0; i+2 < len(tokens); i++ {
		left, okLeft := parseMusicListExactLevelToken(tokens[i])
		right, okRight := parseMusicListExactLevelToken(tokens[i+2])
		if !okLeft || !okRight || !isMusicListRangeSeparatorToken(tokens[i+1]) {
			continue
		}
		values := []int{left, right}
		sort.Ints(values)
		return map[string]any{
			"level_min": values[0],
			"level_max": values[1],
		}, joinMusicListTokensExcluding(tokens, i, i+1, i+2), true
	}

	for i := 0; i+1 < len(tokens); i++ {
		left, okLeft := parseMusicListExactLevelToken(tokens[i])
		right, okRight := parseMusicListExactLevelToken(tokens[i+1])
		if !okLeft || !okRight {
			continue
		}
		values := []int{left, right}
		sort.Ints(values)
		return map[string]any{
			"level_min": values[0],
			"level_max": values[1],
		}, joinMusicListTokensExcluding(tokens, i, i+1), true
	}

	for i, token := range tokens {
		levelParams, ok := parseMusicListLevelToken(strings.TrimSpace(token))
		if !ok {
			continue
		}
		return levelParams, joinMusicListTokensExcluding(tokens, i), true
	}
	return nil, strings.TrimSpace(args), false
}

func parseMusicListLevelToken(token string) (map[string]any, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	if exact, ok := parseMusicListExactLevelToken(token); ok {
		return map[string]any{"level": exact}, true
	}

	if left, right, ok := parseMusicListRangeToken(token); ok {
		values := []int{left, right}
		sort.Ints(values)
		return map[string]any{
			"level_min": values[0],
			"level_max": values[1],
		}, true
	}

	switch {
	case strings.HasPrefix(token, "<="):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "<="))); err == nil && value > 0 {
			return map[string]any{"level_max": value}, true
		}
	case strings.HasPrefix(token, ">="):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, ">="))); err == nil && value > 0 {
			return map[string]any{"level_min": value}, true
		}
	case strings.HasPrefix(token, "<"):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "<"))); err == nil && value > 0 {
			return map[string]any{"level_max": value - 1}, value > 1
		}
	case strings.HasPrefix(token, ">"):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, ">"))); err == nil {
			return map[string]any{"level_min": value + 1}, value >= 0
		}
	case strings.HasPrefix(token, "="):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "="))); err == nil && value > 0 {
			return map[string]any{"level": value}, true
		}
	}

	return nil, false
}

var musicListRangeSeparators = regexp.MustCompile(`^(\d+)\s*(?:-|~|～|,|，|\.\.|到|至)\s*(\d+)$`)

func parseMusicListRangeToken(token string) (int, int, bool) {
	normalized := strings.TrimSpace(token)
	for _, pair := range [][2]string{
		{"[", "]"},
		{"(", ")"},
		{"【", "】"},
		{"（", "）"},
	} {
		if strings.HasPrefix(normalized, pair[0]) && strings.HasSuffix(normalized, pair[1]) {
			normalized = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(normalized, pair[0]), pair[1]))
			break
		}
	}

	matches := musicListRangeSeparators.FindStringSubmatch(normalized)
	if len(matches) != 3 {
		return 0, 0, false
	}

	left, errLeft := strconv.Atoi(matches[1])
	right, errRight := strconv.Atoi(matches[2])
	if errLeft != nil || errRight != nil || left <= 0 || right <= 0 {
		return 0, 0, false
	}
	return left, right, true
}

func parseMusicListExactLevelToken(token string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(token))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func isMusicListRangeSeparatorToken(token string) bool {
	switch strings.TrimSpace(token) {
	case "-", "~", "～", ",", "，", "..", "到", "至":
		return true
	default:
		return false
	}
}

func joinMusicListTokensExcluding(tokens []string, skipIndexes ...int) string {
	if len(tokens) == 0 {
		return ""
	}
	skips := make(map[int]struct{}, len(skipIndexes))
	for _, idx := range skipIndexes {
		skips[idx] = struct{}{}
	}
	remaining := make([]string, 0, len(tokens))
	for idx, token := range tokens {
		if _, skip := skips[idx]; skip {
			continue
		}
		remaining = append(remaining, token)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}
