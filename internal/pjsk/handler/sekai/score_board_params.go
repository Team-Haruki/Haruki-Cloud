package sekai

import (
	"fmt"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func buildMusicBoardParams(args string) (rendermusic.BoardQuery, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return rendermusic.BoardQuery{}, nil
	}

	params := rendermusic.BoardQuery{}

	if page, remaining, ok := extractMusicBoardPageArg(args); ok {
		params.Page = page
		args = remaining
	}

	liveType, remaining := extractMusicBoardMappedArg(args, map[string][]string{
		"solo":  {"单人", "solo", "挑战"},
		"multi": {"多人", "multi"},
		"auto":  {"自动", "auto"},
	}, "solo")
	params.LiveType = liveType
	args = remaining

	defaultTarget := "score"
	if liveType == "multi" {
		defaultTarget = "pt/time"
	}
	target, remaining := extractMusicBoardMappedArg(args, map[string][]string{
		"score":   {"live分数", "分数", "score"},
		"pt/time": {"时间效率", "pt/h", "pt时间", "时速"},
		"pt":      {"火效率", "pt/火", "pt"},
		"tps":     {"每秒点击", "tps"},
		"time":    {"时长", "时间"},
	}, defaultTarget)
	params.Target = target
	args = remaining

	order, remaining := extractMusicBoardMappedArg(args, map[string][]string{
		"asc":  {"升序", "从低到高", "从小到大"},
		"desc": {"降序", "从高到低", "从大到小"},
	}, "desc")
	params.Ascend = order == "asc"
	args = remaining

	defaultStrategy := "avg"
	if liveType == "solo" {
		defaultStrategy = "max"
	}
	strategy, remaining := extractMusicBoardMappedArg(args, map[string][]string{
		"max": {"最优", "最高", "最大", "最强", "max"},
		"min": {"最差", "最低", "最小", "最弱", "min"},
		"avg": {"平均", "期望", "随机", "均值", "avg"},
	}, defaultStrategy)
	params.SkillStrategy = strategy
	args = remaining

	skills, remaining, err := extractMusicBoardSkills(args, liveType)
	if err != nil {
		return rendermusic.BoardQuery{}, err
	}
	params.Skills = skills
	args = remaining

	if target == "pt" || target == "pt/time" {
		power, remaining, err := extractMusicBoardPower(args)
		if err != nil {
			return rendermusic.BoardQuery{}, err
		}
		if power > 0 {
			params.Power = power
		}
		args = remaining

		deckBonus, remaining, err := extractMusicBoardDeckBonus(args)
		if err != nil {
			return rendermusic.BoardQuery{}, err
		}
		if deckBonus > 0 {
			params.DeckBonus = deckBonus
		}
		args = remaining
	}

	if target == "pt/time" || target == "time" {
		interval, remaining, err := extractMusicBoardInterval(args)
		if err != nil {
			return rendermusic.BoardQuery{}, err
		}
		if interval > 0 {
			params.PlayInterval = interval
		}
		args = remaining
	}

	levelFilter, remaining := extractMusicBoardLevelFilter(args)
	params.LevelFilter = levelFilter
	args = remaining

	diffFilter, remaining := extractMusicBoardDiffFilters(args)
	params.DiffFilter = diffFilter
	args = remaining

	params.SpecQueries = splitMusicBoardSpecQueries(args)
	return params, nil
}

func splitMusicBoardSpecQueries(args string) []string {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}
	if strings.ContainsAny(args, "/|\n\r") {
		return splitMusicMetaQueries(args)
	}
	fields := strings.Fields(args)
	if len(fields) <= 1 {
		return []string{args}
	}
	if !shouldSplitMusicBoardSpecQueriesByWhitespace(fields) {
		return []string{args}
	}

	queries := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			queries = append(queries, field)
		}
	}
	return queries
}

func shouldSplitMusicBoardSpecQueriesByWhitespace(fields []string) bool {
	for _, field := range fields {
		if !looksLikeCompactMusicBoardSpecQuery(field) {
			return false
		}
	}
	return true
}

func looksLikeCompactMusicBoardSpecQuery(field string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}

	if diff, rest := rendermusic.ExtractMusicDifficulty(field); diff != "" && strings.TrimSpace(rest) != "" {
		return true
	}
	if strings.Contains(field, "*") {
		return true
	}
	if _, ok := rendermusic.ParseExplicitMusicID(field); ok {
		return true
	}
	if _, ok := rendermusic.ParseImplicitMusicID(field); ok {
		return true
	}

	hasNonASCII := false
	hasASCIIUpper := false
	hasASCII := false
	for _, r := range field {
		if r > unicode.MaxASCII {
			hasNonASCII = true
			continue
		}
		hasASCII = true
		if unicode.IsUpper(r) {
			hasASCIIUpper = true
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return false
		}
	}
	if hasNonASCII && !hasASCII {
		return true
	}
	if hasASCIIUpper {
		return false
	}
	return len(field) <= 8
}

func extractMusicBoardPageArg(args string) (int, string, bool) {
	for _, token := range strings.Fields(args) {
		page, ok := parseMusicBoardPage(strings.ToLower(strings.TrimSpace(token)))
		if !ok {
			continue
		}
		return page, removeMusicBoardToken(args, token), true
	}
	return 0, args, false
}

func extractMusicBoardMappedArg(args string, aliasMap map[string][]string, defaultValue string) (string, string) {
	type candidate struct {
		value string
		alias string
	}

	candidates := make([]candidate, 0, len(aliasMap))
	for value, aliases := range aliasMap {
		for _, alias := range aliases {
			candidates = append(candidates, candidate{value: value, alias: alias})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].alias) > len(candidates[j].alias)
	})

	lowerArgs := strings.ToLower(args)
	for _, item := range candidates {
		lowerAlias := strings.ToLower(item.alias)
		index := strings.Index(lowerArgs, lowerAlias)
		if index < 0 {
			continue
		}
		return item.value, removeMusicBoardSpan(args, index, len(lowerAlias))
	}

	return defaultValue, strings.TrimSpace(args)
}

func extractMusicBoardSkills(args, liveType string) ([]float64, string, error) {
	hadKeyword := strings.Contains(args, "技能") || strings.Contains(args, "实效")
	cleaned := strings.ReplaceAll(args, "技能", "")
	cleaned = strings.ReplaceAll(cleaned, "实效", "")
	cleaned = strings.TrimSpace(cleaned)

	required := 5
	if liveType == "multi" {
		required = 1
	}

	fields := strings.Fields(cleaned)
	numbers := make([]float64, 0, required)
	numberTokens := make([]string, 0, required)
	for _, field := range fields {
		value, ok := parseMusicBoardSkillNumber(field)
		if !ok {
			break
		}
		numbers = append(numbers, value/100.0)
		numberTokens = append(numberTokens, field)
		if len(numbers) >= required {
			break
		}
	}

	shouldTreatAsSkills := hadKeyword || (required > 1 && len(numbers) == required)
	if !shouldTreatAsSkills || len(numbers) == 0 {
		return nil, cleaned, nil
	}
	if len(numbers) != required {
		return nil, "", fmt.Errorf("解析技能加分失败")
	}

	remaining := cleaned
	for _, token := range numberTokens {
		remaining = removeMusicBoardToken(remaining, token)
	}

	if liveType == "multi" {
		return []float64{numbers[0], numbers[0], numbers[0], numbers[0], numbers[0]}, remaining, nil
	}
	return numbers, remaining, nil
}

func parseMusicBoardSkillNumber(token string) (float64, bool) {
	raw := strings.TrimSpace(strings.TrimSuffix(token, "%"))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && value > 0
}

func extractMusicBoardPower(args string) (int, string, error) {
	for _, token := range strings.Fields(args) {
		if !strings.Contains(token, "综合") {
			continue
		}
		value, err := parseMusicBoardLargeNumber(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(token)), "综合", ""))
		if err != nil || value <= 0 {
			return 0, "", fmt.Errorf("解析综合力失败: %q", token)
		}
		return value, removeMusicBoardToken(args, token), nil
	}
	return 0, args, nil
}

func extractMusicBoardDeckBonus(args string) (float64, string, error) {
	for _, token := range strings.Fields(args) {
		if !strings.Contains(token, "加成") {
			continue
		}
		raw := strings.TrimRight(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(token)), "加成", ""), "%")
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value <= 0 {
			return 0, "", fmt.Errorf("解析活动加成失败: %q", token)
		}
		return value, removeMusicBoardToken(args, token), nil
	}
	return 0, args, nil
}

func extractMusicBoardInterval(args string) (float64, string, error) {
	for _, token := range strings.Fields(args) {
		if !strings.Contains(token, "间隔") {
			continue
		}
		raw := strings.TrimRight(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(token)), "间隔", ""), "秒s")
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value <= 0 {
			return 0, "", fmt.Errorf("解析游玩间隔失败: %q", token)
		}
		return value, removeMusicBoardToken(args, token), nil
	}
	return 0, args, nil
}

func extractMusicBoardLevelFilter(args string) (string, string) {
	remaining := strings.TrimSpace(args)
	for _, token := range strings.Fields(args) {
		lower := strings.ToLower(strings.TrimSpace(token))
		if !isMusicBoardLevelFilter(lower) {
			continue
		}
		return lower, removeMusicBoardToken(remaining, token)
	}
	return "", remaining
}

func extractMusicBoardDiffFilters(args string) ([]string, string) {
	remaining := strings.TrimSpace(args)
	diffFilter := make([]string, 0, 2)
	for _, token := range strings.Fields(args) {
		diff, rest := rendermusic.ExtractMusicDifficulty(token)
		if diff == "" || strings.TrimSpace(rest) != "" {
			continue
		}
		if !containsMusicBoardString(diffFilter, diff) {
			diffFilter = append(diffFilter, diff)
		}
		remaining = removeMusicBoardToken(remaining, token)
	}
	return diffFilter, strings.TrimSpace(remaining)
}

func removeMusicBoardToken(args, token string) string {
	index := strings.Index(args, token)
	if index < 0 {
		return strings.TrimSpace(args)
	}
	return removeMusicBoardSpan(args, index, len(token))
}

func removeMusicBoardSpan(args string, start, length int) string {
	if start < 0 || length <= 0 || start+length > len(args) {
		return strings.TrimSpace(args)
	}
	return strings.TrimSpace(args[:start] + args[start+length:])
}

func containsMusicBoardString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func parseMusicBoardPage(token string) (int, bool) {
	if strings.Contains(token, "页") || strings.Contains(token, "p") {
		value := strings.Replace(token, "页", "", 1)
		value = strings.Replace(value, "p", "", 1)
		page, err := strconv.Atoi(value)
		return page, err == nil && page > 0
	}
	return 0, false
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
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return 0, fmt.Errorf("empty power")
	}
	multiplier := 1.0
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
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return int(value * multiplier), nil
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
