package sekai

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
)

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

func extractDeckEventSelection(args string, params *deckAutoQueryParams, trigger string) (string, error) {
	if turn, charID, charQuery, remaining := extractDeckSimulatedWorldBloom(args); turn > 0 && (charID > 0 || charQuery != "") {
		params.WorldBloomEventTurn = intPtr(turn)
		if charID > 0 {
			params.WorldBloomCharacterID = intPtr(charID)
			if params.EventUnit == "" {
				params.EventUnit = deckCharacterUnit(charID)
			}
		} else {
			params.WorldBloomCharacterQuery = charQuery
		}
		return remaining, nil
	}

	if eventID, remaining := extractDeckEventID(args); eventID != nil {
		params.EventID = eventID
		if charID, charQuery, next := extractDeckCharacterCandidate(remaining, true); charID > 0 {
			params.WorldBloomCharacterID = intPtr(charID)
			if params.EventUnit == "" {
				params.EventUnit = deckCharacterUnit(charID)
			}
			return next, nil
		} else if charQuery != "" {
			if next == "" && looksLikeInlineMusicQuery(charQuery) {
				return remaining, nil
			}
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
	normalized := normalizeDeckSpaces(args)
	if strings.Contains(args, "终章") {
		eventID := 180
		return &eventID, normalizeDeckSpaces(strings.Replace(args, "终章", "", 1))
	}
	matches := deckEventIDRegex.FindStringSubmatch(normalized)
	if len(matches) < 3 {
		fields := strings.Fields(normalized)
		if len(fields) < 2 || len(fields[0]) > 3 {
			return nil, normalized
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			return nil, normalized
		}

		// Keep the legacy behavior for inputs like "123 ex" which are commonly
		// used as bare music queries with difficulty suffixes.
		if len(fields) == 2 {
			if diff, cleaned := rendermusic.ExtractMusicDifficulty(normalized); diff != "" && normalizeDeckSpaces(cleaned) == fields[0] {
				return nil, normalized
			}
		}

		eventID, err := strconv.Atoi(fields[0])
		if err != nil || eventID <= 0 {
			return nil, normalized
		}
		return &eventID, normalizeDeckSpaces(strings.Join(fields[1:], " "))
	}
	eventID, err := strconv.Atoi(matches[2])
	if err != nil || eventID <= 0 {
		return nil, normalized
	}
	return &eventID, normalizeDeckSpaces(deckEventIDRegex.ReplaceAllString(normalized, " "))
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
	normalized := normalizeDeckSpaces(args)
	if normalized == "" {
		return 0, ""
	}
	ext := rendercard.NewExtractor(rendercard.DefaultCharacterNicknames())
	result := ext.ExtractCharacter(normalized)
	if result.Found {
		return result.Value, normalizeDeckSpaces(result.Remaining)
	}
	if charID, ok := rendercard.ResolveDefaultCharacterNickname(normalized); ok {
		return charID, ""
	}
	return 0, normalized
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
	if _, ok := rendercard.ResolveDefaultCharacterNickname(args); ok {
		return 0, args, ""
	}
	if len(strings.Fields(args)) != 1 {
		return 0, "", args
	}
	return 0, args, ""
}

func looksLikeInlineMusicQuery(raw string) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return false
	}
	for _, suffix := range deckInlineDifficultySuffixes {
		if !strings.HasSuffix(normalized, strings.ToLower(suffix)) {
			continue
		}
		diff, cleaned := rendermusic.ExtractMusicDifficulty(raw)
		if diff == "" {
			return false
		}
		return strings.TrimSpace(cleaned) != ""
	}
	return false
}
