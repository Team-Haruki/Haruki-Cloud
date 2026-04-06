package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"sort"
	"strconv"
	"strings"
)

func (sekaiHandlers) SKLineHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/line",
			Commands: []string{
				"/sk-line", "/sk线", "/榜线", "/pjsk sk line", "/pjsk board line", "/skl",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, true, true, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-line", params), nil
		},
	}
}

func (sekaiHandlers) SKQueryHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{Path: "sk/query", Commands: []string{
			"/sk-query", "/sk查询", "/sk查分", "/pjsk sk board", "/pjsk board",
		},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-query", params), nil
		},
	}
}

func (sekaiHandlers) SKSpeedHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/speed",
			Commands: []string{
				"/pjsk sk speed", "/pjsk board speed", "/时速", "/sks", "/skv", "/sk时速",
				"/sk-speed", "/sk时速", "/时速线", "/pjsk sk speed", "/pjsk board speed", "/sks", "/skv", "/sktime",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-speed", params), nil
		},
	}
}
func (sekaiHandlers) SKCheckRoomHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/check-room",
			Commands: []string{
				"/sk-check-room", "/sk查房", "/查房", "/cf", "/pjsk查房", "/csb", "/冲水板", "/pjsk冲水板",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-check-room", params), nil
		},
	}
}

func (sekaiHandlers) SKPlayerTraceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/player-trace",
			Commands: []string{
				"/sk-player-trace", "/sk玩家轨迹", "/玩家轨迹", "/ptr", "/pjsk玩家追踪", "/pjsk ptr",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKPlayerTraceParams(ctx)
			if err != nil {
				return nil, err
			}
			if len(params) == 0 {
				return makeResolvedCmd(ctx, parser.ModuleSK, "sk-player-trace"), nil
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-player-trace", params), nil
		},
	}
}

func (sekaiHandlers) SKRankTraceHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/rank-trace",
			Commands: []string{
				"/sk-rank-trace", "/sk档线轨迹", "/档线轨迹", "/rtr", "/skt", "/sklt", "/sktl", "/pjsk追踪", "/pjsk sk追踪",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-rank-trace", params), nil
		},
	}
}

func (sekaiHandlers) WinratePredictHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{Path: "sk/winrate", Commands: []string{
			"/pjsk winrate predict", "/胜率预测", "/5v5预测", "/胜率", "/5v5胜率", "/预测胜率", "/预测5v5",
		}},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleSK, "sk-winrate"), nil
		},
	}
}

// TODO
func (sekaiHandlers) SKDailySpeedHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/speed",
			Commands: []string{
				"/pjsk sk daily speed", "/pjsk board daily speed", "/日速", "/skds", "/skdv", "/sk日速",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-speed", params), nil
		},
	}
}

func (sekaiHandlers) SKPredictHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/predict",
			Commands: []string{
				"/pjsk sk predict", "/pjsk board predict", "/sk预测", "/榜线预测", "/skp",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-predict", params), nil
		},
	}
}

func (sekaiHandlers) SKBoardHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/query",
			Commands: []string{
				"/pjsk sk board", "/pjsk board", "/sk",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, false, true, true)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-query", params), nil
		},
	}
}

func (sekaiHandlers) CSBHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "sk/check-room",
			Commands: []string{
				"/csb", "/查水表", "/pjsk查水表", "/停车时间",
			},
		},
		PrefixArgs: []string{"", "wl"},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			params, err := buildSKTrackerParams(ctx, false, false, false)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleSK, "sk-check-room", params), nil
		},
	}
}

func buildSKTrackerParams(ctx SekaiHandlerContext, defaultFull bool, allowUID bool, selfWhenEmpty bool) (map[string]any, error) {
	eventID, wlCharacterID, wlCharacterQuery, full, rankArgs := extractSKMetaArgs(
		strings.TrimSpace(ctx.GetArgs()),
		defaultFull,
		ctx.PrefixArg() == "wl",
	)
	wlMode := ctx.PrefixArg() == "wl"

	effectiveRankArgs := rankArgs
	rankArgsProvided := strings.TrimSpace(effectiveRankArgs) != ""
	targetUserID := ""
	targetSelector := ""
	if allowUID {
		if uidArg := strings.TrimSpace(ctx.UIDArg()); uidArg != "" && strings.TrimSpace(effectiveRankArgs) == "" {
			switch {
			case strings.HasPrefix(uidArg, "@"):
				candidate := strings.TrimSpace(strings.TrimPrefix(uidArg, "@"))
				if isDigits(candidate) {
					targetUserID = candidate
				}
			case isBindingSelector(uidArg):
				targetUserID = strings.TrimSpace(ctx.GetUserId())
				targetSelector = strings.ToLower(uidArg)
			case isDigits(uidArg):
				effectiveRankArgs = uidArg
			}
		}
		if selfWhenEmpty && strings.TrimSpace(effectiveRankArgs) == "" && targetUserID == "" {
			targetUserID = strings.TrimSpace(ctx.GetUserId())
		}
	}

	var (
		ranks  []int
		userID *int64
	)
	if strings.TrimSpace(effectiveRankArgs) != "" || targetUserID == "" {
		var err error
		ranks, userID, err = parseSKRanks(effectiveRankArgs, allowUID)
		if err != nil {
			return nil, err
		}
	}
	if len(ranks) == 0 && userID == nil && targetUserID == "" {
		return nil, fmt.Errorf("请至少提供一个排名或UID")
	}
	// Empty rank query should use mode-specific default lines.
	if !rankArgsProvided && userID == nil && targetUserID == "" {
		ranks = defaultSKRanksByMode(wlMode)
	}
	if ctx.PrefixArg() == "wl" && wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "" {
		return nil, fmt.Errorf("wl 模式需要角色ID或角色名，例如: /wlsk 初音未来 100 500")
	}

	params := map[string]any{
		"region":          strings.ToLower(strings.TrimSpace(ctx.Region().String())),
		"region_explicit": ctx.HasExplicitRegion(),
	}
	if len(ranks) > 0 {
		params["ranks"] = ranks
	}
	if !rankArgsProvided && userID == nil && targetUserID == "" && len(ranks) > 0 {
		params["default_ranks"] = true
	}
	if eventID > 0 {
		params["event_id"] = eventID
	}
	if wlCharacterID > 0 {
		params["wl_character_id"] = wlCharacterID
	}
	if strings.TrimSpace(wlCharacterQuery) != "" {
		params["wl_character_query"] = strings.TrimSpace(wlCharacterQuery)
	}
	if userID != nil && *userID > 0 {
		params["user_id"] = *userID
	}
	if targetUserID != "" {
		params["target_platform"] = strings.ToLower(strings.TrimSpace(ctx.GetPlatform()))
		params["target_user_id"] = targetUserID
		if targetSelector != "" {
			params["target_selector"] = targetSelector
		}
	}
	if full {
		params["full"] = true
	}
	return params, nil
}

func buildSKPlayerTraceParams(ctx SekaiHandlerContext) (map[string]any, error) {
	eventID, wlCharacterID, wlCharacterQuery, _, rankArgs := extractSKMetaArgs(
		strings.TrimSpace(ctx.GetArgs()),
		false,
		ctx.PrefixArg() == "wl",
	)

	params := map[string]any{
		"region":          strings.ToLower(strings.TrimSpace(ctx.Region().String())),
		"region_explicit": ctx.HasExplicitRegion(),
	}
	if eventID > 0 {
		params["event_id"] = eventID
	}
	if wlCharacterID > 0 {
		params["wl_character_id"] = wlCharacterID
	}
	if strings.TrimSpace(wlCharacterQuery) != "" {
		params["wl_character_query"] = strings.TrimSpace(wlCharacterQuery)
	}

	if ctx.PrefixArg() == "wl" && wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "" {
		return nil, fmt.Errorf("wl 模式需要角色ID或角色名，例如: /wlptr 初音未来 100 500")
	}

	targetUserID := ""
	targetSelector := ""
	if uidArg := strings.TrimSpace(ctx.UIDArg()); uidArg != "" && strings.TrimSpace(rankArgs) == "" {
		switch {
		case strings.HasPrefix(uidArg, "@"):
			candidate := strings.TrimSpace(strings.TrimPrefix(uidArg, "@"))
			if isDigits(candidate) {
				targetUserID = candidate
			}
		case isBindingSelector(uidArg):
			targetUserID = strings.TrimSpace(ctx.GetUserId())
			targetSelector = strings.ToLower(uidArg)
		case isDigits(uidArg):
			rankArgs = uidArg
		}
	}

	if strings.TrimSpace(rankArgs) != "" {
		ranks, userID, err := parseSKRanks(rankArgs, true)
		if err != nil {
			return nil, err
		}
		if len(ranks) > 2 {
			return nil, fmt.Errorf("ptr 最多支持两个排名，例如: /ptr 1 2")
		}
		if len(ranks) > 0 {
			params["ranks"] = ranks
		}
		if userID != nil && *userID > 0 {
			params["user_id"] = *userID
		}
	}

	if targetUserID != "" {
		params["target_platform"] = strings.ToLower(strings.TrimSpace(ctx.GetPlatform()))
		params["target_user_id"] = targetUserID
		if targetSelector != "" {
			params["target_selector"] = targetSelector
		}
	}

	return params, nil
}

func extractSKMetaArgs(args string, defaultFull bool, wlMode bool) (eventID int, wlCharacterID int, wlCharacterQuery string, full bool, rankArgs string) {
	full = defaultFull
	fields := strings.Fields(strings.TrimSpace(args))
	remaining := make([]string, 0, len(fields))
	for _, raw := range fields {
		token := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case token == "full" || token == "-f" || token == "--full":
			full = true
			continue
		case strings.HasPrefix(token, "event") && isDigits(token[5:]):
			eventID, _ = strconv.Atoi(token[5:])
			continue
		case strings.HasPrefix(token, "e") && len(token) > 1 && isDigits(token[1:]):
			eventID, _ = strconv.Atoi(token[1:])
			continue
		case wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "":
			if id, query, ok := parseSKWorldBloomCharacterToken(raw); ok {
				wlCharacterID = id
				wlCharacterQuery = query
				continue
			}
		}
		remaining = append(remaining, raw)
	}
	if wlMode && wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "" {
		wlCharacterQuery, rankArgs = splitSKWorldBloomCharacterAndRanks(remaining)
		return
	}
	rankArgs = strings.TrimSpace(strings.Join(remaining, " "))
	return
}

func parseSKWorldBloomCharacterToken(raw string) (int, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, "", false
	}

	type prefixRule struct {
		prefix     string
		allowQuery bool
	}

	rules := []prefixRule{
		{prefix: "wl:", allowQuery: true},
		{prefix: "wl", allowQuery: true},
		{prefix: "cid:", allowQuery: false},
		{prefix: "cid", allowQuery: false},
		{prefix: "chara:", allowQuery: true},
		{prefix: "chara", allowQuery: true},
		{prefix: "char:", allowQuery: true},
		{prefix: "char", allowQuery: true},
	}

	lower := strings.ToLower(raw)
	for _, rule := range rules {
		if !strings.HasPrefix(lower, rule.prefix) || len(raw) <= len(rule.prefix) {
			continue
		}
		value := strings.TrimSpace(raw[len(rule.prefix):])
		if value == "" {
			return 0, "", false
		}
		if isDigits(value) {
			id, _ := strconv.Atoi(value)
			return id, "", true
		}
		if rule.allowQuery {
			return 0, value, true
		}
		return 0, "", false
	}
	return 0, "", false
}

func splitSKWorldBloomCharacterAndRanks(fields []string) (string, string) {
	remaining := strings.TrimSpace(strings.Join(fields, " "))
	if remaining == "" {
		return "", ""
	}
	if isValidSKRankExpression(remaining) {
		return "", remaining
	}
	for i := 1; i < len(fields); i++ {
		charQuery := strings.TrimSpace(strings.Join(fields[:i], " "))
		rankArgs := strings.TrimSpace(strings.Join(fields[i:], " "))
		if charQuery == "" || rankArgs == "" {
			continue
		}
		if isValidSKRankExpression(rankArgs) {
			return charQuery, rankArgs
		}
	}
	return remaining, ""
}

func isValidSKRankExpression(args string) bool {
	_, err := parser.NewCommandParser().Parse(strings.TrimSpace(args))
	return err == nil
}

func parseSKRanks(args string, allowUID bool) ([]int, *int64, error) {
	cmd, err := parser.NewCommandParser().Parse(strings.TrimSpace(args))
	if err != nil {
		return nil, nil, fmt.Errorf("无法解析排名参数: %w", err)
	}

	switch cmd.Type {
	case parser.CmdTypeEventQuerySelf:
		return append([]int(nil), defaultSKRanks...), nil, nil
	case parser.CmdTypeEventQueryRank:
		return normalizeRanks([]int{cmd.Param1}), nil, nil
	case parser.CmdTypeEventQueryMultiRank:
		ranks := normalizeRanks(cmd.MultiArgs)
		if len(ranks) > 20 {
			return nil, nil, fmt.Errorf("一次最多查询20个排名")
		}
		return ranks, nil, nil
	case parser.CmdTypeEventQueryRankRange:
		if cmd.Param1 <= 0 || cmd.Param2 <= 0 {
			return nil, nil, fmt.Errorf("排名必须大于 0")
		}
		count := cmd.Param2 - cmd.Param1 + 1
		if count > 20 {
			return nil, nil, fmt.Errorf("排名区间最多20个排名")
		}
		ranks := make([]int, 0, count)
		for rank := cmd.Param1; rank <= cmd.Param2; rank++ {
			ranks = append(ranks, rank)
		}
		return ranks, nil, nil
	case parser.CmdTypeEventQueryUID:
		if !allowUID {
			return nil, nil, fmt.Errorf("该命令暂不支持按用户查询，请改用排名")
		}
		uid, parseErr := strconv.ParseInt(cmd.TargetID, 10, 64)
		if parseErr != nil || uid <= 0 {
			return nil, nil, fmt.Errorf("无效的UID: %s", cmd.TargetID)
		}
		return nil, &uid, nil
	case parser.CmdTypeEventQueryAt:
		return nil, nil, fmt.Errorf("暂不支持@用户查询，请直接输入游戏UID")
	default:
		return nil, nil, fmt.Errorf("暂不支持该查询格式")
	}
}

func normalizeRanks(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, rank := range values {
		if rank <= 0 {
			continue
		}
		if _, ok := seen[rank]; ok {
			continue
		}
		seen[rank] = struct{}{}
		out = append(out, rank)
	}
	sort.Ints(out)
	return out
}

var defaultSKRanksNormal = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 1500, 2000, 2500, 3000, 4000, 5000,
	10000, 20000, 30000, 40000, 50000,
	100000, 200000, 300000,
}

var defaultSKRanksWorldLink = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 2000, 3000, 4000, 5000, 7000,
	10000, 20000, 30000, 40000, 50000, 70000, 100000,
}

var defaultSKRanks = defaultSKRanksNormal

func defaultSKRanksByMode(wlMode bool) []int {
	if wlMode {
		return append([]int(nil), defaultSKRanksWorldLink...)
	}
	return append([]int(nil), defaultSKRanksNormal...)
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
