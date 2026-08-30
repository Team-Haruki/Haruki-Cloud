package handler

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
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
	targets := deckFixedTargets{
		cards:            make([]int, 0, len(fields)),
		characters:       make([]int, 0, len(fields)),
		characterQueries: make([]string, 0, len(fields)),
	}
	for _, field := range fields {
		if err := targets.add(field); err != nil {
			return "", err
		}
	}
	if err := targets.validate(); err != nil {
		return "", err
	}
	targets.apply(params)
	return strings.TrimSpace(prefix), nil
}

type deckFixedTargets struct {
	cards            []int
	characters       []int
	characterQueries []string
}

func (t *deckFixedTargets) add(field string) error {
	token := strings.TrimLeft(strings.TrimSpace(field), "#")
	if token == "" {
		return fmt.Errorf("格式错误，#后面请填写卡牌ID或角色")
	}
	if value, err := strconv.Atoi(token); err == nil {
		if value <= 0 {
			return fmt.Errorf("固定卡牌ID必须为正整数")
		}
		t.cards = append(t.cards, value)
		return nil
	}
	charID, charQuery := resolveDeckCharacterToken(token)
	if charID > 0 {
		t.characters = append(t.characters, charID)
		return nil
	}
	if charQuery == "" {
		return fmt.Errorf("格式错误，#后面请填写卡牌ID或角色")
	}
	t.characterQueries = append(t.characterQueries, charQuery)
	return nil
}

func (t deckFixedTargets) validate() error {
	if len(t.cards)+len(t.characters)+len(t.characterQueries) > 5 {
		return fmt.Errorf("固定卡牌和固定角色总数不能超过5个")
	}
	if len(t.cards) > 0 {
		if err := validateDeckUniqueIDs(t.cards, 5, "固定卡牌"); err != nil {
			return err
		}
	}
	if len(t.characters) > 0 && len(t.characterQueries) == 0 {
		return validateDeckUniqueIDs(t.characters, 5, "固定角色")
	}
	if len(t.cards)+len(t.characters)+len(t.characterQueries) == 0 {
		return fmt.Errorf("固定卡牌或固定角色不能为空")
	}
	return nil
}

func (t deckFixedTargets) apply(params *deckAutoQueryParams) {
	if len(t.cards) > 0 {
		params.FixedCards = t.cards
	}
	if len(t.characters) > 0 {
		params.FixedCharacters = t.characters
	}
	if len(t.characterQueries) > 0 {
		params.FixedCharacterQueries = t.characterQueries
	}
}

func extractDeckEventSelection(args string, params *deckAutoQueryParams, trigger string) (string, error) {
	if remaining, handled, err := applyDeckWorldBloomFinaleSelection(args, params); handled {
		return remaining, err
	}
	if eventID, remaining := extractDeckExplicitEventID(args); eventID != nil {
		return extractDeckExplicitEventSelection(remaining, eventID, params, trigger)
	}
	if remaining, handled, err := applyDeckSimulatedEventSelection(args, params, trigger); handled {
		return remaining, err
	}
	if eventID, remaining := extractDeckEventID(args); eventID != nil {
		return extractDeckExplicitEventSelection(remaining, eventID, params, trigger)
	}
	if remaining, handled, err := applyDeckSimulatedWorldBloomSelection(args, params); handled {
		return remaining, err
	}
	if remaining, ok, err := extractDeckCurrentWorldBloomSelection(args, params, trigger); err != nil {
		return "", err
	} else if ok {
		return remaining, nil
	}

	if selector, remaining := extractDeckWorldBloomSelectorCandidate(args); selector != "" {
		params.WorldBloomCharacterQuery = selector
		return remaining, nil
	}

	return normalizeDeckSpaces(args), nil
}

func applyDeckWorldBloomFinaleSelection(args string, params *deckAutoQueryParams) (string, bool, error) {
	turn, remaining, ok := extractDeckWorldBloomFinaleTurn(args)
	if !ok {
		return "", false, nil
	}
	if turn < 2 {
		return "", true, onebot11.NewReplayError("终章从 wl2 开始，请使用 wl2 终章 或 wl3 终章")
	}
	params.WorldBloomFinaleTurn = intPtr(turn)
	remaining, err := extractDeckFinaleLeaderSelection(remaining, params)
	return remaining, true, err
}

func applyDeckSimulatedEventSelection(args string, params *deckAutoQueryParams, trigger string) (string, bool, error) {
	attr, unit, remaining, partial := extractDeckSimulatedEvent(args)
	if attr != "" && unit != "" {
		params.EventAttr = attr
		params.EventUnit = unit
		return remaining, true, nil
	}
	if partial {
		return "", true, onebot11.NewReplayError("使用方式:\n%s event123\n%s 团名 属性\n%s 角色名 wl1", trigger, trigger, trigger)
	}
	return "", false, nil
}

func applyDeckSimulatedWorldBloomSelection(args string, params *deckAutoQueryParams) (string, bool, error) {
	turn, charID, charQuery, remaining, err := extractDeckSimulatedWorldBloom(args)
	if err != nil {
		return "", true, err
	}
	if turn <= 0 || charID <= 0 && charQuery == "" {
		return "", false, nil
	}
	params.WorldBloomEventTurn = intPtr(turn)
	if charID > 0 {
		applyDeckWorldBloomCharacterID(params, charID)
	} else {
		params.WorldBloomCharacterQuery = charQuery
	}
	return remaining, true, nil
}

func applyDeckWorldBloomCharacterID(params *deckAutoQueryParams, charID int) {
	params.WorldBloomCharacterID = intPtr(charID)
	if params.EventUnit == "" {
		params.EventUnit = deckCharacterUnit(charID)
	}
}

func extractDeckExplicitEventSelection(args string, eventID *int, params *deckAutoQueryParams, trigger string) (string, error) {
	params.EventID = eventID
	if selector, remaining := extractDeckWorldBloomSelectorCandidate(args); selector != "" {
		if _, ok := parseDeckWorldBloomTurn(selector); ok {
			return "", invalidDeckWorldBloomTurnUsageError(trigger)
		}
		if charID, _, _ := extractDeckCharacterCandidate(remaining, false); charID > 0 {
			return "", invalidDeckWorldBloomMixedSelectorError()
		}
		params.WorldBloomCharacterQuery = selector
		return remaining, nil
	}
	if eventID != nil && *eventID == 180 {
		return extractDeckFinaleLeaderSelection(args, params)
	}
	if charID, charQuery, remaining := extractDeckCharacterCandidate(args, true); charID > 0 {
		params.WorldBloomCharacterID = intPtr(charID)
		if params.EventUnit == "" {
			params.EventUnit = deckCharacterUnit(charID)
		}
		return remaining, nil
	} else if charQuery != "" {
		if remaining == "" && looksLikeInlineMusicQuery(charQuery) {
			return args, nil
		}
		params.WorldBloomCharacterQuery = charQuery
		return remaining, nil
	}
	return normalizeDeckSpaces(args), nil
}

func extractDeckFinaleLeaderSelection(args string, params *deckAutoQueryParams) (string, error) {
	charID, charQuery, remaining := extractDeckCharacterCandidate(args, true)
	if charID > 0 {
		params.ForcedLeaderCharacterID = intPtr(charID)
		return remaining, nil
	}
	if charQuery != "" {
		if remaining == "" && looksLikeInlineMusicQuery(charQuery) {
			return normalizeDeckSpaces(args), nil
		}
		params.ForcedLeaderCharacterQuery = charQuery
		return remaining, nil
	}
	return normalizeDeckSpaces(args), nil
}

func extractDeckWorldBloomFinaleTurn(args string) (int, string, bool) {
	if !strings.Contains(args, "终章") {
		return 0, normalizeDeckSpaces(args), false
	}

	remaining := normalizeDeckSpaces(strings.Replace(args, "终章", " ", 1))
	matches := deckWlTurnRegex.FindStringSubmatch(remaining)
	if len(matches) < 2 {
		return 0, remaining, false
	}
	turn, err := strconv.Atoi(matches[1])
	if err != nil || turn <= 0 {
		return 0, remaining, false
	}
	return turn, normalizeDeckSpaces(deckWlTurnRegex.ReplaceAllString(remaining, " ")), true
}

func extractDeckCurrentWorldBloomSelection(args string, params *deckAutoQueryParams, trigger string) (string, bool, error) {
	selector, remaining := extractDeckWorldBloomSelectorCandidate(args)
	if selector == "" {
		return normalizeDeckSpaces(args), false, nil
	}

	if _, ok := parseDeckWorldBloomTurn(selector); ok {
		return "", false, invalidDeckWorldBloomTurnUsageError(trigger)
	}

	if charID, _, leftover := extractDeckCharacterCandidate(remaining, false); charID > 0 {
		if turn, ok := parseDeckWorldBloomTurn(selector); ok {
			_ = turn
			return "", false, invalidDeckWorldBloomMixedSelectorError()
		}
		_ = leftover
	}

	params.WorldBloomCharacterQuery = selector
	return remaining, true, nil
}

func extractDeckWorldBloomSelectorCandidate(args string) (string, string) {
	normalized := normalizeDeckSpaces(args)
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return "", normalized
	}

	first := strings.TrimSpace(fields[0])
	if isDeckWorldBloomSelectorToken(first) {
		return strings.ToLower(first), normalizeDeckSpaces(strings.Join(fields[1:], " "))
	}

	last := strings.TrimSpace(fields[len(fields)-1])
	if isDeckWorldBloomSelectorToken(last) {
		return strings.ToLower(last), normalizeDeckSpaces(strings.Join(fields[:len(fields)-1], " "))
	}
	return "", normalized
}

func isDeckWorldBloomSelectorToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	return strings.EqualFold(lower, "wl") || strings.HasPrefix(lower, "wl")
}

func parseDeckWorldBloomTurn(value string) (int, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || !strings.HasPrefix(value, "wl") {
		return 0, false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(value, "wl"))
	if raw == "" {
		return 0, false
	}
	turn, err := strconv.Atoi(raw)
	if err != nil || turn <= 0 {
		return 0, false
	}
	return turn, true
}

func extractDeckSimulatedWorldBloom(args string) (turn int, charID int, charQuery string, remaining string, err error) {
	matches := deckWlTurnRegex.FindStringSubmatch(args)
	if len(matches) < 2 {
		return 0, 0, "", strings.TrimSpace(args), nil
	}
	turnValue, err := strconv.Atoi(matches[1])
	if err != nil || turnValue <= 0 {
		return 0, 0, "", strings.TrimSpace(args), nil
	}

	args = deckWlTurnRegex.ReplaceAllString(args, " ")
	charID, charQuery, args = extractDeckCharacterCandidate(args, false)
	if charID <= 0 && charQuery == "" {
		return 0, 0, "", strings.TrimSpace(args), nil
	}
	return turnValue, charID, charQuery, normalizeDeckSpaces(args), nil
}

func invalidDeckWorldBloomMixedSelectorError() error {
	return onebot11.NewReplayError("不能同时指定 WL 章节和角色，请只保留其中一种写法")
}

func invalidDeckWorldBloomTurnUsageError(trigger string) error {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		return onebot11.NewReplayError("不再支持 wl2 这种 WL 章节写法，请改用 wl1 miku 或 event123 miku")
	}
	return onebot11.NewReplayError("不再支持 wl2 这种 WL 章节写法，请改用:\n%s wl1 miku\n%s event123 miku", trigger, trigger)
}

func extractDeckExplicitEventID(args string) (*int, string) {
	normalized := normalizeDeckSpaces(args)
	if strings.Contains(args, "终章") {
		return intPtr(180), normalizeDeckSpaces(strings.Replace(args, "终章", "", 1))
	}
	matches := deckEventIDRegex.FindStringSubmatch(normalized)
	if len(matches) < 3 {
		return nil, normalized
	}
	eventID, err := strconv.Atoi(matches[2])
	if err != nil || eventID <= 0 {
		return nil, normalized
	}
	return &eventID, normalizeDeckSpaces(deckEventIDRegex.ReplaceAllString(normalized, " "))
}

func extractDeckEventID(args string) (*int, string) {
	normalized := normalizeDeckSpaces(args)
	if eventID, remaining := extractDeckExplicitEventID(normalized); eventID != nil {
		return eventID, remaining
	}

	fields := strings.Fields(normalized)
	if len(fields) == 0 || len(fields[0]) > 3 {
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

func extractDeckSimulatedEvent(args string) (attr string, unit string, remaining string, partial bool) {
	attr, args = extractDeckAttribute(args)
	unit, args = extractDeckSimulatedEventUnit(args)
	return attr, unit, normalizeDeckSpaces(args), attr != "" || unit != ""
}

func extractDeckSimulatedEventUnit(args string) (string, string) {
	unit, remaining := extractDeckUnit(args)
	switch unit {
	case "piapro":
		if !strings.Contains(args, "vs") {
			return "", strings.TrimSpace(args)
		}
	case "school_refusal":
		if deckSchoolRefusalAliasIsAmbiguous(args) {
			return "", strings.TrimSpace(args)
		}
	}
	return unit, remaining
}

func deckSchoolRefusalAliasIsAmbiguous(args string) bool {
	index := strings.Index(args, "25")
	if index < 0 {
		return false
	}
	left := deckByteRuneAt(args, index-1)
	right := deckByteRuneAt(args, index+2)
	return deckAliasAdjacentDigit(left) || deckAliasAdjacentDigit(right) || left == 't' || left == '活'
}

func deckByteRuneAt(value string, index int) rune {
	if index < 0 || index >= len(value) {
		return ' '
	}
	return rune(value[index])
}

func deckAliasAdjacentDigit(value rune) bool {
	return value >= '0' && value <= '9'
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

func validateNoEventDeckArgs(args, trigger string) error {
	attr, unit, remaining, _ := extractDeckSimulatedEvent(args)
	if attr == "" && unit == "" {
		return nil
	}
	if normalizeDeckSpaces(remaining) != "" {
		return nil
	}
	return onebot11.NewReplayError(
		"使用方式:\n%s\n%s 歌曲名 难度\n%s 团名 属性",
		trigger,
		trigger,
		normalizeNoEventDeckHintTrigger(trigger),
	)
}

func normalizeNoEventDeckHintTrigger(trigger string) string {
	trigger = strings.Replace(trigger, "最强", "", 1)
	trigger = strings.Replace(trigger, "长草", "", 1)
	return trigger
}
