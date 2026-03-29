package sekai

import (
	"fmt"
	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"strconv"
	"strings"
)

type scoreControlParams struct {
	TargetPoint int    `json:"target_point"`
	Query       string `json:"query,omitempty"`
	WL          bool   `json:"wl,omitempty"`
}

type customRoomScoreParams struct {
	TargetPoint int `json:"target_point"`
}

type musicMetaQueriesParams struct {
	Queries []string `json:"queries"`
}

func (sekaiHandlers) ScoreControlHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "score",
			Commands: []string{
				"/分数", "/查分数", "/pjsk score", "/score control",
				"/控分",
			},
		},
		Regions:    []renderregion.Value{renderregion.JP},
		PrefixArgs: []string{"wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildScoreControlParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleScore, "score-control", params), nil
		},
	}
}

func buildScoreControlParams(ctx SekaiHandlerContext) (scoreControlParams, error) {
	args := strings.TrimSpace(ctx.GetArgs())
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 {
		return scoreControlParams{}, onebot11.NewReplayError("使用方式:\n%s 活动pt 歌曲名(可选)", ctx.originalTriggerCmd)
	}

	targetPT, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || targetPT <= 0 {
		return scoreControlParams{}, onebot11.NewReplayError("使用方式:\n%s 活动pt 歌曲名(可选)", ctx.originalTriggerCmd)
	}

	params := scoreControlParams{
		TargetPoint: targetPT,
		WL:          ctx.PrefixArg() == "wl",
	}
	if len(parts) > 1 {
		params.Query = strings.TrimSpace(parts[1])
	}
	return params, nil
}

func (sekaiHandlers) CustomRoomScoreControlHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "score/custom-room",
			Commands: []string{
				"/pjsk custom room score", "/custom room score",
				"/自定义房间控分", "/自定义房控分", "/自定义控分",
				"/自定义房间分数", "/自定义分数",
			},
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			targetPT, err := strconv.Atoi(args)
			if err != nil || targetPT <= 0 {
				return nil, onebot11.NewReplayError("使用方式: %s 目标PT", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleScore, "score-custom-room", customRoomScoreParams{
				TargetPoint: targetPT,
			}), nil
		},
	}
}

func (sekaiHandlers) MusicMetaHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "score/music-meta",
			Commands: []string{
				"/pjsk music meta", "/music meta",
				"/歌曲meta", "/曲目meta",
			},
			Priority: 1,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			clean := splitMusicMetaQueries(args)
			if len(clean) == 0 {
				return nil, fmt.Errorf("请至少提供一个歌曲ID或名称")
			}
			if len(clean) > 3 {
				return nil, fmt.Errorf("一次最多进行3首歌曲的比较")
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleScore, "score-music-meta", musicMetaQueriesParams{Queries: clean}), nil
		},
	}
}

func splitMusicMetaQueries(args string) []string {
	segments := strings.Split(strings.ReplaceAll(strings.TrimSpace(args), "/", "|"), "|")
	clean := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			clean = append(clean, seg)
		}
	}
	return clean
}

func (sekaiHandlers) MusicBoardHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "score/music-board",
			Commands: []string{
				"/pjsk music board", "/music board",
				"/歌曲排行", "/歌曲比较", "/歌曲排名", "/曲目榜",
			},
			Priority: 1,
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildMusicBoardParams(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleScore, "score-music-board", params), nil
		},
	}
}

func buildMusicBoardParams(args string) (rendermusic.BoardQuery, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return rendermusic.BoardQuery{}, nil
	}

	params := rendermusic.BoardQuery{}
	tokens := strings.Fields(args)
	remaining := make([]string, 0, len(tokens))

	for _, token := range tokens {
		lower := strings.ToLower(strings.TrimSpace(token))
		if lower == "" {
			continue
		}
		if page, ok := parseMusicBoardPage(lower); ok {
			params.Page = page
			continue
		}
		if liveType, ok := resolveMusicBoardLiveType(lower); ok {
			params.LiveType = liveType
			continue
		}
		if target, ok := resolveMusicBoardTarget(lower); ok {
			params.Target = target
			continue
		}
		if ascend, ok := resolveMusicBoardOrder(lower); ok {
			params.Ascend = ascend
			continue
		}
		if strategy, ok := resolveMusicBoardStrategy(lower); ok {
			params.SkillStrategy = strategy
			continue
		}
		if power, ok := parseMusicBoardPower(lower); ok {
			params.Power = power
			continue
		}
		if deckBonus, ok := parseMusicBoardDeckBonus(lower); ok {
			params.DeckBonus = deckBonus
			continue
		}
		if interval, ok := parseMusicBoardInterval(lower); ok {
			params.PlayInterval = interval
			continue
		}
		if isMusicBoardLevelFilter(lower) {
			params.LevelFilter = lower
			continue
		}
		if diff, ok := resolveMusicBoardDifficulty(lower); ok {
			params.DiffFilter = append(params.DiffFilter, diff)
			continue
		}
		remaining = append(remaining, token)
	}

	params.SpecQueries = splitMusicMetaQueries(strings.TrimSpace(strings.Join(remaining, " ")))
	return params, nil
}

func parseMusicBoardPage(token string) (int, bool) {
	switch {
	case strings.HasSuffix(token, "页"):
		value := strings.TrimSuffix(token, "页")
		page, err := strconv.Atoi(value)
		return page, err == nil && page > 0
	case strings.HasPrefix(token, "p"):
		value := strings.TrimPrefix(token, "p")
		page, err := strconv.Atoi(value)
		return page, err == nil && page > 0
	default:
		return 0, false
	}
}

func resolveMusicBoardLiveType(token string) (string, bool) {
	switch token {
	case "solo", "单人", "挑战":
		return "solo", true
	case "multi", "多人":
		return "multi", true
	case "auto", "自动":
		return "auto", true
	default:
		return "", false
	}
}

func resolveMusicBoardTarget(token string) (string, bool) {
	switch token {
	case "live分数", "分数", "score":
		return "score", true
	case "时间效率", "pt/h", "pt时间", "时速":
		return "pt/time", true
	case "火效率", "pt/火", "pt":
		return "pt", true
	case "每秒点击", "tps":
		return "tps", true
	case "时长", "时间":
		return "time", true
	default:
		return "", false
	}
}

func resolveMusicBoardOrder(token string) (bool, bool) {
	switch token {
	case "升序", "从低到高", "从小到大":
		return true, true
	case "降序", "从高到低", "从大到小":
		return false, true
	default:
		return false, false
	}
}

func resolveMusicBoardStrategy(token string) (string, bool) {
	switch token {
	case "最优", "最高", "最大", "最强", "max":
		return "max", true
	case "最差", "最低", "最小", "最弱", "min":
		return "min", true
	case "平均", "期望", "随机", "均值", "avg":
		return "avg", true
	default:
		return "", false
	}
}

func parseMusicBoardPower(token string) (int, bool) {
	if !strings.HasPrefix(token, "综合") {
		return 0, false
	}
	value, err := parseMusicBoardLargeNumber(strings.TrimPrefix(token, "综合"))
	return value, err == nil && value > 0
}

func parseMusicBoardDeckBonus(token string) (float64, bool) {
	if !strings.Contains(token, "加成") {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.ReplaceAll(token, "加成", ""), "%")
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && value > 0
}

func parseMusicBoardInterval(token string) (float64, bool) {
	if !strings.Contains(token, "间隔") {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimSuffix(strings.ReplaceAll(token, "间隔", ""), "秒"), "s")
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && value > 0
}

func parseMusicBoardLargeNumber(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty power")
	}
	multiplier := 1
	switch {
	case strings.HasSuffix(raw, "万"):
		raw = strings.TrimSuffix(raw, "万")
		multiplier = 10000
	case strings.HasSuffix(raw, "w"):
		raw = strings.TrimSuffix(raw, "w")
		multiplier = 10000
	case strings.HasSuffix(raw, "k"):
		raw = strings.TrimSuffix(raw, "k")
		multiplier = 1000
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value * multiplier, nil
}

func isMusicBoardLevelFilter(token string) bool {
	if token == "" {
		return false
	}
	switch {
	case strings.HasPrefix(token, "<="), strings.HasPrefix(token, ">="), strings.HasPrefix(token, "=="):
		token = token[2:]
	case strings.HasPrefix(token, "<"), strings.HasPrefix(token, ">"), strings.HasPrefix(token, "="):
		token = token[1:]
	default:
		return false
	}
	for _, ch := range token {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return token != ""
}

func resolveMusicBoardDifficulty(token string) (string, bool) {
	switch token {
	case "easy", "ez":
		return "easy", true
	case "normal", "nm":
		return "normal", true
	case "hard", "hd":
		return "hard", true
	case "expert", "ex", "exp":
		return "expert", true
	case "master", "ma", "mas":
		return "master", true
	case "append", "apd":
		return "append", true
	default:
		return "", false
	}
}
