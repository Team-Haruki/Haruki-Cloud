package card

import (
	"fmt"
	"strconv"
	"strings"
)

type QueryType int

const (
	QueryTypeUnknown QueryType = iota
	QueryTypeID
	QueryTypeSeq
	QueryTypeFilter
)

type CardQueryInfo struct {
	Type        QueryType
	Value       int
	Sequence    int
	CharacterID int
	Unit        string
	MainUnit    string
	SupportUnit string
	Rarity      string
	Attr        string
	SkillType   string
	SupplyType  string
	Year        int
	EventID     int
	BanCharID   int
	BanSeq      int
	Original    string
}

type Parser struct {
	extractor *Extractor
}

func NewParser(nicknames map[string]int) *Parser {
	return &Parser{extractor: NewExtractor(nicknames)}
}

func (p *Parser) Parse(args string) (*CardQueryInfo, error) {
	return p.parse(args, false)
}

func (p *Parser) ParsePreferFilter(args string) (*CardQueryInfo, error) {
	return p.parse(args, true)
}

func (p *Parser) parse(args string, preferFilter bool) (*CardQueryInfo, error) {
	args = strings.TrimSpace(args)
	if info := p.tryParseNicknameSeq(args); info != nil {
		return info, nil
	}
	if preferFilter {
		if info := p.tryParseFilter(args); info != nil {
			return info, nil
		}
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
	return info.Type == QueryTypeID || info.Type == QueryTypeSeq
}

func LooksLikeSingleCardQueryPreferFilter(args string) bool {
	info, err := NewParser(defaultNicknames).ParsePreferFilter(strings.TrimSpace(args))
	if err != nil || info == nil {
		return false
	}
	return info.Type == QueryTypeID || info.Type == QueryTypeSeq
}

func (p *Parser) tryParseNicknameSeq(args string) *CardQueryInfo {
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
	return &CardQueryInfo{
		Type:        QueryTypeSeq,
		Sequence:    sequence,
		CharacterID: result.Value,
		Original:    args,
	}
}

func (p *Parser) tryParseID(args string) *CardQueryInfo {
	if !isNumeric(args) {
		return nil
	}
	value, err := strconv.Atoi(args)
	if err != nil {
		return nil
	}
	return &CardQueryInfo{
		Type:     QueryTypeID,
		Value:    value,
		Original: args,
	}
}

func (p *Parser) tryParseFilter(args string) *CardQueryInfo {
	current := args
	info := &CardQueryInfo{Type: QueryTypeFilter, Original: args}
	matched := false

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
	if result := p.extractor.ExtractAttribute(current); result.Found {
		info.Attr = result.Value
		current = result.Remaining
		matched = true
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
	if result := p.extractor.ExtractCharacter(current); result.Found {
		info.CharacterID = result.Value
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
		matched = true
	}
	if matched {
		return info
	}
	return nil
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
