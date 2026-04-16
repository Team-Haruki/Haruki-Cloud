package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/parser"
	"sort"
	"strconv"
	"strings"
)

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
		case strings.HasPrefix(token, "event") && len(token) > 5 && isDigits(token[5:]):
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
		if strings.TrimSpace(wlCharacterQuery) == "" {
			wlCharacterQuery = "wl"
		}
		return
	}
	if wlCharacterID == 0 && strings.TrimSpace(wlCharacterQuery) == "" {
		if query, ranks, ok := splitLeadingSKWorldBloomSelectorAndRanks(remaining); ok {
			wlCharacterQuery, rankArgs = query, ranks
			return
		}
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
			if strings.HasPrefix(rule.prefix, "wl") {
				return 0, "", false
			}
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

func splitLeadingSKWorldBloomSelectorAndRanks(fields []string) (string, string, bool) {
	if len(fields) == 0 {
		return "", "", false
	}
	query := strings.TrimSpace(fields[0])
	if !isSKWorldBloomSelector(query) {
		return "", "", false
	}
	rankArgs := strings.TrimSpace(strings.Join(fields[1:], " "))
	if rankArgs != "" && !isValidSKRankExpression(rankArgs) {
		return "", "", false
	}
	return query, rankArgs, true
}

func isSKWorldBloomSelector(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	return raw == "wl" || (strings.HasPrefix(raw, "wl") && len(raw) > 2)
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

var defaultSKCheckRoomLiteRanks = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100,
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
