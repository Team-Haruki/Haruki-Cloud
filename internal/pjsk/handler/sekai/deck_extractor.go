package sekai

import (
	"fmt"
	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"strconv"
	"strings"
)

func extractDeckCommonParams(args string, params *deckAutoQueryParams, cfg deckCommonConfig) (string, error) {
	var err error
	if cfg.allowFixed {
		args, err = extractDeckFixedTargets(args, params)
		if err != nil {
			return "", err
		}
	}
	if cfg.allowAlgorithm {
		args = extractDeckAlgorithm(args, params)
	}
	if cfg.allowLiveType {
		args = extractDeckLiveType(args, params)
	}
	if cfg.allowRandom {
		args = extractDeckRandomStrategies(args, params)
	}
	if cfg.allowMultiLive && params.LiveType != "solo" && params.LiveType != "auto" {
		args, err = extractDeckMultiliveOptions(args, params)
		if err != nil {
			return "", err
		}
	}
	if cfg.allowCardConfig {
		args = extractDeckCardConfigs(args, params, cfg.defaultNoChange)
	}
	if cfg.allowTarget {
		args = extractDeckTarget(args, params)
	}
	return strings.TrimSpace(args), nil
}

func extractDeckAlgorithm(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "dfs", "sa", "ga", "all":
			if params.Algorithm == "" {
				params.Algorithm = field
				continue
			}
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}

func extractDeckLiveType(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if params.LiveType == "" {
			switch field {
			case "多人", "协力", "multi":
				params.LiveType = "multi"
				continue
			case "单人", "solo":
				params.LiveType = "solo"
				continue
			case "自动", "auto":
				params.LiveType = "auto"
				continue
			}
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}

func extractDeckRandomStrategies(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for idx := 0; idx < len(fields); idx++ {
		field := fields[idx]
		if strategy, consumed := resolveDeckStrategyField(field, idx, fields, deckSkillOrderKeywords); consumed > 0 {
			params.SkillOrderChooseStrategy = strategy
			idx += consumed - 1
			continue
		}
		if strategy, consumed := resolveDeckStrategyField(field, idx, fields, deckSkillReferenceKeywords); consumed > 0 {
			params.SkillReferenceChooseStrategy = strategy
			idx += consumed - 1
			continue
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}

func extractDeckMultiliveOptions(args string, params *deckAutoQueryParams) (string, error) {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if value, ok, err := extractDeckKeywordNumber(field, deckTeammatePowerKeywords, parseMusicBoardLargeNumber); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的队友综合力")
			}
			params.MultiLiveTeammatePower = intPtr(value)
			continue
		}
		if value, ok, err := extractDeckKeywordNumber(field, deckTeammateScoreUpKeywords, parseDeckInt); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的队友实效")
			}
			params.MultiLiveTeammateScoreUp = intPtr(value)
			continue
		}
		if value, ok, err := extractDeckKeywordNumber(field, deckSkillTargetKeywords, parseDeckInt); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的队友实效")
			}
			params.MultiLiveTeammateScoreUp = intPtr(value)
			f := float64(value)
			params.MultiLiveScoreUpLowerBound = &f
			continue
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " ")), nil
}

func extractDeckFixedTargets(args string, params *deckAutoQueryParams) (string, error) {
	args = strings.ReplaceAll(args, "＃", "#")
	if !strings.Contains(args, "#") {
		return strings.TrimSpace(args), nil
	}

	prefix, suffix, _ := strings.Cut(args, "#")
	fields := strings.Fields(strings.TrimSpace(suffix))
	if len(fields) == 0 {
		return "", fmt.Errorf("固定卡牌或固定角色不能为空")
	}

	fixedCards := make([]int, 0, len(fields))
	allInts := true
	for _, field := range fields {
		value, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || value <= 0 {
			allInts = false
			break
		}
		fixedCards = append(fixedCards, value)
	}
	if allInts {
		if err := validateDeckUniqueIDs(fixedCards, 5, "固定卡牌"); err != nil {
			return "", err
		}
		params.FixedCards = fixedCards
		return strings.TrimSpace(prefix), nil
	}

	fixedCharacters := make([]int, 0, len(fields))
	fixedCharacterQueries := make([]string, 0, len(fields))
	for _, field := range fields {
		charID, charQuery := resolveDeckCharacterToken(field)
		if charID <= 0 {
			if charQuery == "" {
				return "", fmt.Errorf("格式错误，#后面请填写卡牌ID或角色")
			}
			fixedCharacterQueries = append(fixedCharacterQueries, charQuery)
			continue
		}
		fixedCharacters = append(fixedCharacters, charID)
	}
	if len(fixedCharacters)+len(fixedCharacterQueries) > 5 {
		return "", fmt.Errorf("固定角色数量不能超过5个")
	}
	if len(fixedCharacterQueries) == 0 {
		if err := validateDeckUniqueIDs(fixedCharacters, 5, "固定角色"); err != nil {
			return "", err
		}
	}
	if len(fixedCharacters) == 0 && len(fixedCharacterQueries) == 0 {
		return "", fmt.Errorf("固定角色不能为空")
	}
	params.FixedCharacters = fixedCharacters
	params.FixedCharacterQueries = fixedCharacterQueries
	return strings.TrimSpace(prefix), nil
}

func extractDeckTarget(args string, params *deckAutoQueryParams) string {
	switch {
	case containsDeckKeyword(args, deckPowerTargetKeywords):
		params.Target = "power"
		args = removeDeckKeywordOnce(args, deckPowerTargetKeywords)
	case containsDeckKeyword(args, deckSkillTargetKeywords):
		params.Target = "skill"
		args = removeDeckKeywordOnce(args, deckSkillTargetKeywords)
	}
	return normalizeDeckSpaces(args)
}

func extractDeckMusicQuery(args string, params *deckAutoQueryParams) (string, error) {
	args = normalizeDeckSpaces(args)
	if args == "" {
		return "", nil
	}

	if diff, cleaned := rendermusic.ExtractMusicDifficulty(args); diff != "" {
		params.MusicDiff = diff
		args = cleaned
	}
	args = normalizeDeckSpaces(args)
	if args == "" {
		return "", nil
	}
	if musicID, ok := rendermusic.ParseExplicitMusicID(args); ok {
		params.MusicID = intPtr(musicID)
		params.MusicQuery = ""
		return "", nil
	}
	params.MusicQuery = args
	return "", nil
}

func extractDeckEventSelection(args string, params *deckAutoQueryParams, trigger string) (string, error) {
	if turn, _, charQuery, remaining := extractDeckSimulatedWorldBloom(args); turn > 0 && charQuery != "" {
		params.WorldBloomEventTurn = intPtr(turn)
		params.WorldBloomCharacterQuery = charQuery
		return remaining, nil
	}

	if eventID, remaining := extractDeckEventID(args); eventID != nil {
		params.EventID = eventID
		if _, charQuery, next := extractDeckCharacterCandidate(remaining, true); charQuery != "" {
			params.WorldBloomCharacterQuery = charQuery
			return next, nil
		}
		return remaining, nil
	}

	attr, unit, remaining := extractDeckSimulatedEvent(args)
	switch {
	case attr != "" && unit != "":
		params.EventAttr = attr
		params.EventUnit = unit
		return remaining, nil
	case attr != "" || unit != "":
		return "", onebot11.NewReplayError("使用方式:\n%s event123\n%s 团名 属性\n%s 角色名 wl1", trigger, trigger, trigger)
	default:
		return normalizeDeckSpaces(args), nil
	}
}

func extractDeckSimulatedWorldBloom(args string) (turn int, charID int, charQuery string, remaining string) {
	matches := deckWlTurnRegex.FindStringSubmatch(args)
	if len(matches) < 2 {
		return 0, 0, "", strings.TrimSpace(args)
	}
	turnValue, err := strconv.Atoi(matches[1])
	if err != nil || turnValue <= 0 {
		return 0, 0, "", strings.TrimSpace(args)
	}

	args = deckWlTurnRegex.ReplaceAllString(args, " ")
	charID, charQuery, args = extractDeckCharacterCandidate(args, true)
	if charID <= 0 && charQuery == "" {
		return 0, 0, "", strings.TrimSpace(args)
	}
	return turnValue, charID, charQuery, normalizeDeckSpaces(args)
}

func extractDeckEventID(args string) (*int, string) {
	if strings.Contains(args, "终章") {
		eventID := 180
		return &eventID, normalizeDeckSpaces(strings.Replace(args, "终章", "", 1))
	}
	matches := deckEventIDRegex.FindStringSubmatch(args)
	if len(matches) < 3 {
		return nil, normalizeDeckSpaces(args)
	}
	eventID, err := strconv.Atoi(matches[2])
	if err != nil || eventID <= 0 {
		return nil, normalizeDeckSpaces(args)
	}
	return &eventID, normalizeDeckSpaces(deckEventIDRegex.ReplaceAllString(args, " "))
}

func extractDeckSimulatedEvent(args string) (attr string, unit string, remaining string) {
	attr, args = extractDeckAttribute(args)
	unit, args = extractDeckUnit(args)
	return attr, unit, normalizeDeckSpaces(args)
}

func extractDeckAttribute(args string) (string, string) {
	ext := parser.NewExtractor(nil)
	result := ext.ExtractAttribute(args)
	if !result.Found {
		return "", strings.TrimSpace(args)
	}
	return result.Value, normalizeDeckSpaces(result.Remaining)
}

func extractDeckUnit(args string) (string, string) {
	for _, rule := range deckUnitAliasRules {
		if !rule.re.MatchString(args) {
			continue
		}
		remaining := rule.re.ReplaceAllString(args, " ")
		return rule.unit, normalizeDeckSpaces(remaining)
	}
	return "", strings.TrimSpace(args)
}

func extractDeckCharacter(args string) (int, string) {
	return 0, normalizeDeckSpaces(args)
}

func extractDeckCharacterCandidate(args string, allowSingleFieldFallback bool) (int, string, string) {
	charID, remaining := extractDeckCharacter(args)
	if charID > 0 {
		return charID, "", remaining
	}
	args = normalizeDeckSpaces(args)
	if !allowSingleFieldFallback || args == "" {
		return 0, "", args
	}
	if len(strings.Fields(args)) != 1 {
		return 0, "", args
	}
	return 0, args, ""
}
