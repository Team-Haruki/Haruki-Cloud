package sekai

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	rendermusic "haruki-cloud/internal/pjsk/render/music"
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
		args, err = extractDeckRandomStrategies(args, params)
		if err != nil {
			return "", err
		}
	}
	if cfg.allowMultiLive && params.LiveType != "solo" && params.LiveType != "auto" {
		args, err = extractDeckMultiliveOptions(args, params)
		if err != nil {
			return "", err
		}
	}
	if cfg.allowCardConfig {
		args = extractDeckCardConfigs(args, params)
	}
	if cfg.allowTarget {
		args = extractDeckTarget(args, params)
	}
	args = extractDeckBoost(args, params)
	args = extractDeckAreaItemLevel(args, params)
	args = extractDeckUnitFilter(args, params)
	args = extractDeckAttrFilter(args, params)
	args = extractDeckProfileFlags(args, params)
	args = extractDeckCurrentFlag(args, params)
	args = extractDeckExcludedCards(args, params)
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
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}

func extractDeckRandomStrategies(args string, params *deckAutoQueryParams) (string, error) {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for idx := 0; idx < len(fields); idx++ {
		field := fields[idx]
		if strategy, order, consumed, err := resolveDeckSkillOrderField(field, idx, fields); err != nil {
			return "", err
		} else if consumed > 0 {
			params.SkillOrderChooseStrategy = strategy
			params.SpecificSkillOrder = slices.Clone(order)
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
	return strings.TrimSpace(strings.Join(remaining, " ")), nil
}

func extractDeckMultiliveOptions(args string, params *deckAutoQueryParams) (string, error) {
	fields := strings.Fields(args)
	remaining := make([]string, 0, len(fields))
	for idx := 0; idx < len(fields); idx++ {
		if value, consumed, ok, err := extractDeckKeywordNumberFromFields(fields, idx, deckTeammatePowerKeywords, parseMusicBoardLargeNumber); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的队友综合力")
			}
			params.MultiLiveTeammatePower = intPtr(value)
			idx += consumed - 1
			continue
		}
		if value, consumed, ok, err := extractDeckKeywordNumberFromFields(fields, idx, deckTeammateScoreUpKeywords, parseDeckInt); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的队友实效")
			}
			params.MultiLiveTeammateScoreUp = intPtr(value)
			idx += consumed - 1
			continue
		}
		if value, consumed, ok, err := extractDeckSkillLowerBound(fields, idx); ok {
			if err != nil {
				return "", fmt.Errorf("无法解析指定的实效下限")
			}
			params.MultiLiveScoreUpLowerBound = new(float64(value))
			// Match Lunabot semantics: lower-bound syntax does not change the
			// ranking target, but it does override teammate score-up.
			params.MultiLiveTeammateScoreUp = intPtr(value)
			idx += consumed - 1
			continue
		}
		remaining = append(remaining, fields[idx])
	}
	return strings.TrimSpace(strings.Join(remaining, " ")), nil
}

func extractDeckSkillLowerBound(fields []string, index int) (int, int, bool, error) {
	if index < 0 || index >= len(fields) {
		return 0, 0, false, nil
	}
	if value, ok, err := extractDeckKeywordNumber(fields[index], deckSkillTargetKeywords, parseDeckInt); ok {
		return value, 1, true, err
	}
	if index+1 >= len(fields) {
		return 0, 0, false, nil
	}

	current := strings.TrimSpace(fields[index])
	next := strings.TrimSpace(fields[index+1])
	for _, keyword := range deckSkillTargetKeywords {
		switch {
		case current == keyword && looksLikeDeckNumericToken(next):
			value, err := parseDeckInt(strings.TrimSuffix(next, "%"))
			return value, 2, true, err
		case next == keyword && looksLikeDeckNumericToken(current):
			value, err := parseDeckInt(strings.TrimSuffix(current, "%"))
			return value, 2, true, err
		}
	}
	return 0, 0, false, nil
}

func extractDeckBoost(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return strings.TrimSpace(args)
	}

	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if boost, ok := parseDeckBoostField(field); ok {
			params.Boost = intPtr(boost)
			continue
		}
		remaining = append(remaining, field)
	}

	return strings.TrimSpace(strings.Join(remaining, " "))
}

func parseDeckBoostField(field string) (int, bool) {
	value := strings.ToLower(strings.TrimSpace(field))
	if value == "" {
		return 0, false
	}

	for _, keyword := range deckBoostKeywords {
		keywordLower := strings.ToLower(keyword)
		if !strings.HasSuffix(value, keywordLower) || len(value) <= len(keywordLower) {
			continue
		}
		raw := strings.TrimSpace(value[:len(value)-len(keywordLower)])
		boost, err := strconv.Atoi(raw)
		if err != nil || boost < 0 || boost > 10 {
			return 0, false
		}
		return boost, true
	}

	return 0, false
}

func extractDeckAreaItemLevel(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(normalizeDeckSpaces(args))
	if len(fields) == 0 {
		return strings.TrimSpace(args)
	}

	remaining := make([]string, 0, len(fields))
	for idx := 0; idx < len(fields); idx++ {
		if level, consumed, ok := parseDeckAreaItemFields(fields, idx); ok {
			params.AreaItemLevel = intPtr(level)
			idx += consumed - 1
			continue
		}
		remaining = append(remaining, fields[idx])
	}
	return normalizeDeckSpaces(strings.Join(remaining, " "))
}

func parseDeckAreaItemFields(fields []string, index int) (int, int, bool) {
	if index < 0 || index >= len(fields) {
		return 0, 0, false
	}
	field := strings.ToLower(strings.TrimSpace(fields[index]))
	if field == "" {
		return 0, 0, false
	}

	for _, keyword := range deckAreaItemKeywords {
		keywordLower := strings.ToLower(keyword)
		switch {
		case strings.Contains(field, keywordLower):
			raw := strings.TrimSpace(strings.Replace(field, keywordLower, "", 1))
			raw = strings.TrimSuffix(raw, "级")
			if level, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && level > 0 {
				return level, 1, true
			}
		case index+1 < len(fields) && strings.TrimSpace(fields[index]) == keyword:
			raw := strings.TrimSuffix(strings.TrimSpace(fields[index+1]), "级")
			if level, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && level > 0 {
				return level, 2, true
			}
		case index+1 < len(fields) && strings.TrimSpace(fields[index+1]) == keyword:
			raw := strings.TrimSuffix(strings.TrimSpace(fields[index]), "级")
			if level, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && level > 0 {
				return level, 2, true
			}
		}
	}
	return 0, 0, false
}

func extractDeckUnitFilter(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(normalizeDeckSpaces(args))
	if len(fields) == 0 {
		return strings.TrimSpace(args)
	}

	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		matched := false
		for unit, keywords := range deckUnitFilterKeywords {
			for _, keyword := range keywords {
				if field != keyword {
					continue
				}
				params.UnitFilter = unit
				matched = true
				break
			}
			if matched {
				break
			}
		}
		if matched {
			continue
		}
		remaining = append(remaining, field)
	}
	return normalizeDeckSpaces(strings.Join(remaining, " "))
}

func extractDeckAttrFilter(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(normalizeDeckSpaces(args))
	if len(fields) == 0 {
		return strings.TrimSpace(args)
	}

	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		matched := false
		for attr, aliases := range deckAttrFilterAliases {
			for _, alias := range aliases {
				for _, prefix := range []string{"纯", "仅"} {
					if field != prefix+alias {
						continue
					}
					params.AttrFilter = attr
					matched = true
					break
				}
				if matched {
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			continue
		}
		remaining = append(remaining, field)
	}
	return normalizeDeckSpaces(strings.Join(remaining, " "))
}

func extractDeckProfileFlags(args string, params *deckAutoQueryParams) string {
	for _, keyword := range deckSubMaxProfileKeywords {
		if strings.Contains(args, keyword) {
			params.SubMaxProfile = true
			args = strings.Replace(args, keyword, "", 1)
			break
		}
	}
	for _, keyword := range deckMaxProfileKeywords {
		if strings.Contains(args, keyword) {
			params.MaxProfile = true
			args = strings.Replace(args, keyword, "", 1)
			break
		}
	}
	return normalizeDeckSpaces(args)
}

func extractDeckCurrentFlag(args string, params *deckAutoQueryParams) string {
	remaining, found := extractDeckCurrentKeyword(args)
	if found {
		params.UseCurrentDeck = true
	}
	return remaining
}

func extractDeckExcludedCards(args string, params *deckAutoQueryParams) string {
	fields := strings.Fields(normalizeDeckSpaces(args))
	if len(fields) == 0 {
		return strings.TrimSpace(args)
	}

	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if cardID, ok := parseDeckExcludedCardField(field); ok {
			params.ExcludedCards = append(params.ExcludedCards, cardID)
			continue
		}
		remaining = append(remaining, field)
	}
	return normalizeDeckSpaces(strings.Join(remaining, " "))
}

func parseDeckExcludedCardField(field string) (int, bool) {
	value := strings.TrimSpace(field)
	if !strings.HasPrefix(value, "-") || len(value) <= 1 {
		return 0, false
	}
	cardID, err := strconv.Atoi(strings.TrimSpace(value[1:]))
	if err != nil || cardID <= 0 || cardID >= 5000 {
		return 0, false
	}
	return cardID, true
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

func extractDeckCurrentKeyword(args string) (string, bool) {
	fields := strings.Fields(normalizeDeckSpaces(args))
	if len(fields) == 0 {
		return "", false
	}

	remaining := make([]string, 0, len(fields))
	found := false
	for _, field := range fields {
		if containsDeckKeyword(field, deckCurrentDeckKeywords) {
			found = true
			continue
		}
		remaining = append(remaining, field)
	}
	return normalizeDeckSpaces(strings.Join(remaining, " ")), found
}

func extractDeckMusicCompare(args string) (string, string, bool) {
	normalized := normalizeDeckSpaces(args)
	for _, keyword := range deckMusicCompareKeywords {
		index := strings.Index(normalized, keyword)
		if index < 0 {
			continue
		}
		prefix := normalizeDeckSpaces(normalized[:index])
		suffix := normalizeDeckSpaces(normalized[index+len(keyword):])
		return prefix, suffix, true
	}
	return normalized, "", false
}
