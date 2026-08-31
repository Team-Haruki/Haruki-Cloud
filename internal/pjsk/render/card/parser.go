package card

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

func NewParser(nicknames map[string]int) *Parser {
	return &Parser{extractor: NewExtractor(nicknames)}
}

func (p *Parser) Parse(args string) (*PjskCardQueryInfo, error) {
	return p.parse(args, false)
}

func (p *Parser) ParsePreferFilter(args string) (*PjskCardQueryInfo, error) {
	return p.parse(args, true)
}

func (p *Parser) ParseStrictFilter(args string) (*PjskCardQueryInfo, error) {
	args = strings.TrimSpace(args)
	if info := p.tryParseFilter(args); info != nil {
		return info, nil
	}
	return nil, fmt.Errorf("无法解析的指令: %s", args)
}

func (p *Parser) parse(args string, preferFilter bool) (*PjskCardQueryInfo, error) {
	args = strings.TrimSpace(args)
	if info := p.tryParseNicknameSeq(args); info != nil {
		return info, nil
	}
	if info := p.tryParseLatestSeq(args); info != nil {
		return info, nil
	}
	if info := p.tryParseID(args); info != nil {
		return info, nil
	}
	if info := p.tryParseFilter(args); info != nil {
		return info, nil
	}
	return nil, fmt.Errorf("无法解析的指令: %s", args)
}

func LooksLikeSingleCardQuery(args string) bool {
	info, err := NewParser(defaultNicknames).Parse(strings.TrimSpace(args))
	if err != nil || info == nil {
		return false
	}
	return info.Type == QueryTypeID || info.Type == QueryTypeSeq || info.Type == QueryTypeLatest
}

func LooksLikeSingleCardQueryPreferFilter(args string) bool {
	info, err := NewParser(defaultNicknames).ParsePreferFilter(strings.TrimSpace(args))
	if err != nil || info == nil {
		return false
	}
	return info.Type == QueryTypeID || info.Type == QueryTypeSeq || info.Type == QueryTypeLatest
}

func (p *Parser) tryParseNicknameSeq(args string) *PjskCardQueryInfo {
	result := p.extractor.ExtractCharacter(args)
	if !result.Found {
		return nil
	}
	remaining := strings.TrimSpace(result.Remaining)
	if !strings.HasPrefix(remaining, "-") {
		return nil
	}
	numberPart := remaining[1:]
	if !isNumeric(numberPart) {
		return nil
	}
	sequence, _ := strconv.Atoi(remaining)
	return &PjskCardQueryInfo{
		Type:        QueryTypeSeq,
		Sequence:    sequence,
		CharacterID: result.Value,
		Original:    args,
	}
}

func (p *Parser) tryParseID(args string) *PjskCardQueryInfo {
	if !isNumeric(args) {
		return nil
	}
	value, err := strconv.Atoi(args)
	if err != nil {
		return nil
	}
	return &PjskCardQueryInfo{
		Type:     QueryTypeID,
		Value:    value,
		Original: args,
	}
}

func (p *Parser) tryParseLatestSeq(args string) *PjskCardQueryInfo {
	args = strings.TrimSpace(args)
	if len(args) < 2 || args[0] != '-' {
		return nil
	}
	numberPart := args[1:]
	if !isNumeric(numberPart) {
		return nil
	}
	sequence, err := strconv.Atoi(args)
	if err != nil || sequence >= 0 {
		return nil
	}
	return &PjskCardQueryInfo{
		Type:     QueryTypeLatest,
		Sequence: sequence,
		Original: args,
	}
}

func (p *Parser) tryParseFilter(args string) *PjskCardQueryInfo {
	state := cardFilterParseState{
		current: args,
		info:    &PjskCardQueryInfo{Type: QueryTypeFilter, Original: args},
	}
	state.extractEventAndBan(p.extractor)
	state.extractCharacterAndAttribute(p.extractor)
	state.extractSkills(p.extractor)
	state.extractUnit(p.extractor)
	state.extractRaritySupplyAndYear(p.extractor)
	if state.suppressSingleRuneAttr && shouldIgnoreSuppressedSingleRuneAttr(p.extractor, state.current) {
		state.current = ""
	}
	if state.matched && strings.TrimSpace(state.current) == "" {
		return state.info
	}
	return nil
}

type cardFilterParseState struct {
	current                string
	info                   *PjskCardQueryInfo
	matched                bool
	suppressSingleRuneAttr bool
}

func (s *cardFilterParseState) extractEventAndBan(extractor *Extractor) {
	if result := extractor.ExtractEventID(s.current); result.Found {
		s.info.EventID = result.Value
		s.accept(result.Remaining)
	}
	if result := extractor.ExtractBanEvent(s.current); result.Found {
		s.info.BanCharID = result.Value.CharacterID
		s.info.BanSeq = result.Value.Sequence
		s.accept(result.Remaining)
	}
}

func (s *cardFilterParseState) extractCharacterAndAttribute(extractor *Extractor) {
	if result := extractor.ExtractCharacter(s.current); result.Found {
		s.info.CharacterID = result.Value
		s.accept(result.Remaining)
		s.suppressSingleRuneAttr = result.PrefixTightlyJoin || result.SuffixTightlyJoin
	}
	if s.info.Attr != "" {
		return
	}
	var result ExtractResult[string]
	if s.suppressSingleRuneAttr {
		result = extractor.ExtractAttributeWithoutSingleRune(s.current)
	} else {
		result = extractor.ExtractAttribute(s.current)
	}
	if result.Found {
		s.info.Attr = result.Value
		s.accept(result.Remaining)
	}
}

func (s *cardFilterParseState) extractSkills(extractor *Extractor) {
	if result := extractor.ExtractDetailedSkillIDs(s.current); result.Found {
		s.info.SkillIDs = result.Value
		s.accept(result.Remaining)
	}
	if result := extractor.ExtractSkill(s.current); result.Found {
		s.info.SkillType = result.Value
		s.accept(result.Remaining)
	}
}

func (s *cardFilterParseState) extractUnit(extractor *Extractor) {
	if result := extractor.ExtractVSUnit(s.current); result.Found {
		s.info.MainUnit = "piapro"
		s.info.SupportUnit = result.Value
		if result.Value == "piapro" {
			s.info.SupportUnit = "none"
		}
		s.accept(result.Remaining)
		return
	}
	if result := extractor.ExtractOCUnit(s.current); result.Found {
		s.info.MainUnit = result.Value
		s.info.SupportUnit = "none"
		s.accept(result.Remaining)
		return
	}
	if result := extractor.ExtractUnit(s.current); result.Found {
		s.info.Unit = result.Value
		s.accept(result.Remaining)
	}
}

func (s *cardFilterParseState) extractRaritySupplyAndYear(extractor *Extractor) {
	if result := extractor.ExtractRarity(s.current); result.Found {
		s.info.Rarity = result.Value
		s.accept(result.Remaining)
	}
	if result := extractor.ExtractSupply(s.current); result.Found {
		s.info.SupplyType = result.Value
		s.accept(result.Remaining)
	}
	if result := extractor.ExtractYear(s.current); result.Found {
		s.info.Year = result.Value
		s.accept(result.Remaining)
	}
}

func (s *cardFilterParseState) accept(remaining string) {
	s.current = remaining
	s.matched = true
}

func shouldIgnoreSuppressedSingleRuneAttr(extractor *Extractor, text string) bool {
	if extractor == nil {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if utf8.RuneCountInString(text) == 1 {
		if r, _ := utf8.DecodeRuneInString(text); r != utf8.RuneError && r > 127 {
			return true
		}
	}
	if !extractor.ExtractAttribute(text).Found {
		return false
	}
	return !extractor.ExtractAttributeWithoutSingleRune(text).Found
}

func isNumeric(text string) bool {
	if text == "" {
		return false
	}
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
