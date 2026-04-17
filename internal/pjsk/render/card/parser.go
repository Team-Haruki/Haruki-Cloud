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
	current := args
	info := &PjskCardQueryInfo{Type: QueryTypeFilter, Original: args}
	matched := false
	suppressSingleRuneAttr := false

	if result := p.extractor.ExtractEventID(current); result.Found {
		info.EventID = result.Value
		current = result.Remaining
		matched = true
	}
	if result := p.extractor.ExtractBanEvent(current); result.Found {
		info.BanCharID = result.Value.CharacterID
		info.BanSeq = result.Value.Sequence
		current = result.Remaining
		matched = true
	}
	if result := p.extractor.ExtractCharacter(current); result.Found {
		info.CharacterID = result.Value
		current = result.Remaining
		matched = true
		if result.PrefixTightlyJoin || result.SuffixTightlyJoin {
			suppressSingleRuneAttr = true
			if attr := p.extractor.ExtractAttributeWithoutSingleRune(current); attr.Found {
				info.Attr = attr.Value
				current = attr.Remaining
				matched = true
			}
		}
	}
	if info.Attr == "" {
		var result ExtractResult[string]
		if suppressSingleRuneAttr {
			result = p.extractor.ExtractAttributeWithoutSingleRune(current)
		} else {
			result = p.extractor.ExtractAttribute(current)
		}
		if result.Found {
			info.Attr = result.Value
			current = result.Remaining
			matched = true
		}
	}
	if result := p.extractor.ExtractSkill(current); result.Found {
		info.SkillType = result.Value
		current = result.Remaining
		matched = true
	}
	if result := p.extractor.ExtractVSUnit(current); result.Found {
		info.MainUnit = "piapro"
		if result.Value == "piapro" {
			info.SupportUnit = "none"
		} else {
			info.SupportUnit = result.Value
		}
		current = result.Remaining
		matched = true
	} else if result := p.extractor.ExtractOCUnit(current); result.Found {
		info.MainUnit = result.Value
		info.SupportUnit = "none"
		current = result.Remaining
		matched = true
	} else if result := p.extractor.ExtractUnit(current); result.Found {
		info.Unit = result.Value
		current = result.Remaining
		matched = true
	}
	if result := p.extractor.ExtractRarity(current); result.Found {
		info.Rarity = result.Value
		current = result.Remaining
		matched = true
	}
	if result := p.extractor.ExtractSupply(current); result.Found {
		info.SupplyType = result.Value
		current = result.Remaining
		matched = true
	}
	if result := p.extractor.ExtractYear(current); result.Found {
		info.Year = result.Value
		current = result.Remaining
		matched = true
	}
	if suppressSingleRuneAttr && shouldIgnoreSuppressedSingleRuneAttr(p.extractor, current) {
		current = ""
	}
	if matched && strings.TrimSpace(current) == "" {
		return info
	}
	return nil
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
