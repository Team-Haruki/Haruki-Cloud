package sekai

import (
	"fmt"
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
	args = extractDeckBoost(args)
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
			params.Target = "skill"
			f := float64(value)
			params.MultiLiveScoreUpLowerBound = &f
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

func extractDeckBoost(args string) string {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return strings.TrimSpace(args)
	}

	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, ok := parseDeckBoostField(field); ok {
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
