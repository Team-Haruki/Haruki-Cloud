package parser

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/filteralias"
)

// CharacterIDByNickname resolves a character nickname to character id.
func (p *EventParser) CharacterIDByNickname(token string) (int, bool) {
	if p == nil {
		return 0, false
	}
	cid, ok := p.nicknames[normalizeEventToken(token)]
	return cid, ok
}

// NewEventParser 创建解析器
func NewEventParser(nicknames map[string]int) *EventParser {
	normalized := make(map[string]int, len(nicknames))
	ordered := make([]string, 0, len(nicknames))
	for nickname, cid := range nicknames {
		key := normalizeEventToken(nickname)
		if key == "" {
			continue
		}
		if _, exists := normalized[key]; !exists {
			ordered = append(ordered, key)
		}
		normalized[key] = cid
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return len([]rune(ordered[i])) > len([]rune(ordered[j]))
	})
	return &EventParser{
		nicknames:        normalized,
		orderedNicknames: ordered,
	}
}

// Parse 解析查询字符串
func (p *EventParser) Parse(args string) (*EventQueryInfo, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("活动查询参数不能为空")
	}

	if info := p.tryParseEventID(args); info != nil {
		return info, nil
	}

	if info := p.tryParseBanEvent(args); info != nil {
		return info, nil
	}

	if info := p.tryParseEventSeq(args); info != nil {
		return info, nil
	}

	if info := p.tryParseFilter(args); info != nil {
		return info, nil
	}

	return nil, fmt.Errorf("无法解析的活动指令: %s", args)
}

func (p *EventParser) tryParseEventID(args string) *EventQueryInfo {
	lower := normalizeEventToken(args)
	if strings.HasPrefix(lower, "event") {
		numPart := strings.TrimPrefix(lower, "event")
		id, err := strconv.Atoi(numPart)
		if err == nil {
			return &EventQueryInfo{
				Type:       QueryTypeEventID,
				EventID:    id,
				IsDetailed: true,
				Original:   args,
			}
		}
	}
	if isNumeric(args) {
		id, _ := strconv.Atoi(args)
		return &EventQueryInfo{
			Type:       QueryTypeEventID,
			EventID:    id,
			IsDetailed: true,
			Original:   args,
		}
	}
	return nil
}

func (p *EventParser) tryParseBanEvent(args string) *EventQueryInfo {
	normalized := normalizeEventToken(args)
	for _, nickname := range p.orderedNicknames {
		if strings.HasPrefix(normalized, nickname) {
			suffix := strings.TrimPrefix(normalized, nickname)
			if isNumeric(suffix) {
				seq, _ := strconv.Atoi(suffix)
				return &EventQueryInfo{
					Type:       QueryTypeEventBan,
					BanCharID:  p.nicknames[nickname],
					BanSeq:     seq,
					IsDetailed: true,
					Original:   args,
				}
			}
		}
	}
	return nil
}

func (p *EventParser) tryParseEventSeq(args string) *EventQueryInfo {
	lower := normalizeEventToken(args)
	if lower == "next" || lower == "下期" || lower == "下" {
		return &EventQueryInfo{Type: QueryTypeEventSeq, Keyword: "next", IsDetailed: true, Original: args}
	}
	if lower == "prev" || lower == "perv" || lower == "上期" || lower == "上" {
		return &EventQueryInfo{Type: QueryTypeEventSeq, Keyword: "prev", IsDetailed: true, Original: args}
	}
	if lower == "current" || lower == "curr" || lower == "当期" || lower == "今" {
		return &EventQueryInfo{Type: QueryTypeEventSeq, Keyword: "current", IsDetailed: true, Original: args}
	}

	if len(args) > 1 && (strings.HasPrefix(args, "-") || strings.HasPrefix(args, "+")) && isNumeric(args[1:]) {
		idx, _ := strconv.Atoi(strings.TrimSpace(args))
		return &EventQueryInfo{
			Type:       QueryTypeEventSeq,
			Index:      idx,
			IsDetailed: true,
			Original:   args,
		}
	}
	return nil
}

func (p *EventParser) tryParseFilter(args string) *EventQueryInfo {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}
	state := newEventFilterParseState(p)
	ext := NewExtractor(nil)
	yearRes := ext.ExtractYear(args)
	if yearRes.Found {
		state.filter.Year = yearRes.Value
		args = yearRes.Remaining
		state.matched = true
	}
	for _, part := range strings.Fields(strings.ToLower(args)) {
		if !state.apply(part) {
			return nil
		}
	}
	return state.result(args)
}

type eventFilterTokenResult uint8

const (
	eventFilterUnhandled eventFilterTokenResult = iota
	eventFilterMatched
	eventFilterInvalid
)

var eventFilterTypes = map[string]string{
	"marathon": "marathon", "普活": "marathon", "马拉松": "marathon",
	"cheerful": "cheerful_carnival", "5v5": "cheerful_carnival", "carnival": "cheerful_carnival",
	"wl": "world_bloom", "worldlink": "world_bloom", "world": "world_bloom",
}

type eventFilterParseState struct {
	parser       *EventParser
	filter       EventFilter
	matched      bool
	onlyUnitNext bool
	characters   map[int]struct{}
	units        map[string]string
	attributes   map[string]string
}

func newEventFilterParseState(parser *EventParser) *eventFilterParseState {
	return &eventFilterParseState{
		parser:     parser,
		characters: map[int]struct{}{},
		units:      filteralias.UnitMap(),
		attributes: filteralias.AttributeMap(),
	}
}

func (s *eventFilterParseState) apply(part string) bool {
	token := normalizeEventToken(part)
	if token == "" {
		return true
	}
	if isEventOnlyUnitToken(token) {
		s.onlyUnitNext = true
		s.matched = true
		return true
	}
	for _, matcher := range []func(string) eventFilterTokenResult{
		s.matchUnit,
		s.matchBlend,
		s.matchEventType,
		s.matchAttribute,
		s.matchBanner,
		s.matchCharacter,
		s.matchYear,
	} {
		result := matcher(token)
		if result != eventFilterUnhandled {
			return result == eventFilterMatched
		}
	}
	return false
}

func isEventOnlyUnitToken(token string) bool {
	switch token {
	case "仅", "純", "纯", "only":
		return true
	default:
		return false
	}
}

func (s *eventFilterParseState) matchUnit(token string) eventFilterTokenResult {
	unitToken := token
	onlyUnit := s.onlyUnitNext
	if stripped, ok := stripEventOnlyUnitPrefix(token); ok {
		unitToken = stripped
		onlyUnit = true
	}
	s.onlyUnitNext = false
	unit, ok := s.units[unitToken]
	if !ok {
		if onlyUnit {
			return eventFilterInvalid
		}
		return eventFilterUnhandled
	}
	if s.filter.Blend {
		return eventFilterInvalid
	}
	s.filter.Unit = unit
	s.filter.OnlyUnit = onlyUnit
	s.matched = true
	return eventFilterMatched
}

func (s *eventFilterParseState) matchBlend(token string) eventFilterTokenResult {
	if token != "混活" && token != "混" && token != "blend" && token != "mixed" {
		return eventFilterUnhandled
	}
	if s.filter.Unit != "" {
		return eventFilterInvalid
	}
	s.filter.Blend = true
	s.matched = true
	return eventFilterMatched
}

func (s *eventFilterParseState) matchEventType(token string) eventFilterTokenResult {
	if eventType, ok := eventFilterTypes[token]; ok {
		s.filter.EventType = eventType
		s.matched = true
		return eventFilterMatched
	}
	turn, ok := parseEventWorldBloomTurn(token)
	if !ok {
		return eventFilterUnhandled
	}
	s.filter.EventType = "world_bloom"
	s.filter.WorldBloomTurn = turn
	s.matched = true
	return eventFilterMatched
}

func (s *eventFilterParseState) matchAttribute(token string) eventFilterTokenResult {
	attribute, ok := s.attributes[token]
	if !ok {
		return eventFilterUnhandled
	}
	s.filter.Attr = attribute
	s.matched = true
	return eventFilterMatched
}

func (s *eventFilterParseState) matchBanner(token string) eventFilterTokenResult {
	if !strings.Contains(token, "箱") && !strings.Contains(token, "ban") {
		return eventFilterUnhandled
	}
	bannerToken := strings.ReplaceAll(strings.ReplaceAll(token, "箱", ""), "ban", "")
	characterID, ok := s.parser.CharacterIDByNickname(bannerToken)
	if !ok {
		return eventFilterUnhandled
	}
	s.filter.BannerCharID = characterID
	s.matched = true
	return eventFilterMatched
}

func (s *eventFilterParseState) matchCharacter(token string) eventFilterTokenResult {
	characterID, ok := s.parser.CharacterIDByNickname(token)
	if !ok {
		return eventFilterUnhandled
	}
	s.characters[characterID] = struct{}{}
	s.matched = true
	return eventFilterMatched
}

func (s *eventFilterParseState) matchYear(token string) eventFilterTokenResult {
	year, ok := eventFilterYear(token, time.Now().Year())
	if !ok {
		return eventFilterUnhandled
	}
	s.filter.Year = year
	s.matched = true
	return eventFilterMatched
}

func eventFilterYear(token string, currentYear int) (int, bool) {
	switch token {
	case "去年":
		return currentYear - 1, true
	case "今年":
		return currentYear, true
	}
	if strings.HasSuffix(token, "年") {
		return normalizedEventFilterYear(strings.TrimSuffix(token, "年"))
	}
	if !isNumeric(token) {
		return 0, false
	}
	year, _ := strconv.Atoi(token)
	ok := year > 2019 && year < 2030
	return year, ok
}

func normalizedEventFilterYear(token string) (int, bool) {
	if !isNumeric(token) {
		return 0, false
	}
	year, _ := strconv.Atoi(token)
	if year < 100 {
		year += 2000
	}
	return year, true
}

func (s *eventFilterParseState) result(original string) *EventQueryInfo {
	if s.onlyUnitNext || !s.matched || (s.filter.OnlyUnit && s.filter.Unit == "") {
		return nil
	}
	s.filter.CharacterIDs = sortedEventFilterCharacterIDs(s.characters)
	if len(s.filter.CharacterIDs) == 1 {
		s.filter.CharacterID = s.filter.CharacterIDs[0]
	}
	return &EventQueryInfo{
		Type:     QueryTypeEventFilter,
		Filter:   s.filter,
		Original: strings.TrimSpace(original),
	}
}

func sortedEventFilterCharacterIDs(characters map[int]struct{}) []int {
	if len(characters) == 0 {
		return nil
	}
	ids := make([]int, 0, len(characters))
	for characterID := range characters {
		ids = append(ids, characterID)
	}
	sort.Ints(ids)
	return ids
}

func parseEventWorldBloomTurn(token string) (int, bool) {
	token = normalizeEventToken(token)
	for _, prefix := range []string{"wl", "worldlink", "world"} {
		if !strings.HasPrefix(token, prefix) || len(token) <= len(prefix) {
			continue
		}
		raw := strings.TrimPrefix(token, prefix)
		if !isNumeric(raw) {
			continue
		}
		turn, err := strconv.Atoi(raw)
		if err == nil && turn > 0 {
			return turn, true
		}
	}
	return 0, false
}

func stripEventOnlyUnitPrefix(token string) (string, bool) {
	for _, prefix := range []string{"仅", "純", "纯", "only"} {
		if strings.HasPrefix(token, prefix) && len(token) > len(prefix) {
			return strings.TrimPrefix(token, prefix), true
		}
	}
	return token, false
}

func normalizeEventToken(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), ""))
}
