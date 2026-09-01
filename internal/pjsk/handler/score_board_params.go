package handler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	rendermusic "haruki-cloud/internal/pjsk/render/music"
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
		defaultTarget = pointsPerTimeMetric
	}
	target, remaining := extractMusicBoardMappedArg(args, map[string][]string{
		"score":             {"live分数", "分数", "score"},
		pointsPerTimeMetric: {"时间效率", "pt/h", "pt时间", "时速"},
		"pt":                {"火效率", "pt/火", "pt"},
		"tps":               {"每秒点击", "tps"},
		"time":              {"时长", "时间"},
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

	if target == "pt" || target == pointsPerTimeMetric {
		args, err = applyMusicBoardPowerAndBonus(&params, args)
		if err != nil {
			return rendermusic.BoardQuery{}, err
		}
	}

	if target == pointsPerTimeMetric || target == "time" {
		args, err = applyMusicBoardInterval(&params, args)
		if err != nil {
			return rendermusic.BoardQuery{}, err
		}
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

func applyMusicBoardPowerAndBonus(params *rendermusic.BoardQuery, args string) (string, error) {
	power, remaining, err := extractMusicBoardPower(args)
	if err != nil {
		return "", err
	}
	if power > 0 {
		params.Power = power
	}
	deckBonus, remaining, err := extractMusicBoardDeckBonus(remaining)
	if err != nil {
		return "", err
	}
	if deckBonus > 0 {
		params.DeckBonus = deckBonus
	}
	return remaining, nil
}

func applyMusicBoardInterval(params *rendermusic.BoardQuery, args string) (string, error) {
	interval, remaining, err := extractMusicBoardInterval(args)
	if err != nil {
		return "", err
	}
	if interval > 0 {
		params.PlayInterval = interval
	}
	return remaining, nil
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

	if diff, rest := extractCompactMusicDifficulty(field); diff != "" && strings.TrimSpace(rest) != "" {
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

	hasNonASCII, hasASCII, hasASCIIUpper, valid := compactMusicBoardFieldShape(field)
	if !valid {
		return false
	}
	if hasNonASCII && !hasASCII {
		return true
	}
	if hasASCIIUpper {
		return false
	}
	return len(field) <= 8
}

func compactMusicBoardFieldShape(field string) (hasNonASCII, hasASCII, hasASCIIUpper, valid bool) {
	for _, r := range field {
		if r > unicode.MaxASCII {
			hasNonASCII = true
			continue
		}
		hasASCII = true
		hasASCIIUpper = hasASCIIUpper || unicode.IsUpper(r)
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return hasNonASCII, hasASCII, hasASCIIUpper, false
		}
	}
	return hasNonASCII, hasASCII, hasASCIIUpper, true
}

func extractCompactMusicDifficulty(query string) (string, string) {
	diff, cleaned := rendermusic.ExtractMusicDifficulty(query)
	if diff != "" || strings.TrimSpace(cleaned) == "" {
		return diff, cleaned
	}

	lower := strings.ToLower(strings.TrimSpace(query))
	for _, suffix := range []struct {
		alias     string
		canonical string
	}{
		{alias: "append", canonical: "append"},
		{alias: "expert", canonical: "expert"},
		{alias: "master", canonical: "master"},
		{alias: "normal", canonical: "normal"},
		{alias: "easy", canonical: "easy"},
		{alias: "hard", canonical: "hard"},
		{alias: "apd", canonical: "append"},
		{alias: "app", canonical: "append"},
		{alias: "exp", canonical: "expert"},
		{alias: "mas", canonical: "master"},
		{alias: "nm", canonical: "normal"},
		{alias: "ez", canonical: "easy"},
		{alias: "hd", canonical: "hard"},
		{alias: "ex", canonical: "expert"},
		{alias: "ma", canonical: "master"},
	} {
		if !strings.HasSuffix(lower, suffix.alias) || len(lower) <= len(suffix.alias) {
			continue
		}
		return suffix.canonical, strings.TrimSpace(query[:len(query)-len(suffix.alias)])
	}
	return "", cleaned
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
