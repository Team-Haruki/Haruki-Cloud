package music

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
	QueryTypeEvent
	QueryTypeBan
	QueryTypeTitle
	QueryTypeChart
)

type QueryInfo struct {
	Type       QueryType
	Value      int
	Diff       string
	Difficulty string
	MusicID    int
	Keyword    string
	BanCharID  int
	BanSeq     int
	Original   string
}

type Parser struct {
	nicknames map[string]int
}

func NewParser(nicknames map[string]int) *Parser {
	return &Parser{nicknames: cloneNicknames(nicknames)}
}

func (p *Parser) Parse(args string) (*QueryInfo, error) {
	args = strings.TrimSpace(args)
	diff, cleanArgs := p.extractDiff(args)

	if info := p.tryParseID(cleanArgs); info != nil {
		info.Diff = diff
		info.Difficulty = diff
		info.Original = args
		return info, nil
	}
	if info := p.tryParseSeq(cleanArgs); info != nil {
		info.Diff = diff
		info.Difficulty = diff
		info.Original = args
		return info, nil
	}
	if info := p.tryParseEvent(cleanArgs); info != nil {
		info.Diff = diff
		info.Difficulty = diff
		info.Original = args
		return info, nil
	}
	if info := p.tryParseBan(cleanArgs); info != nil {
		info.Diff = diff
		info.Difficulty = diff
		info.Original = args
		return info, nil
	}
	if cleanArgs != "" {
		return &QueryInfo{
			Type:       QueryTypeTitle,
			Keyword:    cleanArgs,
			Diff:       diff,
			Difficulty: diff,
			Original:   args,
		}, nil
	}

	return nil, fmt.Errorf("unable to parse music query: %s", args)
}

func (p *Parser) ParseChart(args string) (*QueryInfo, error) {
	info, err := p.Parse(args)
	if err != nil {
		return nil, err
	}
	if info.Diff == "" {
		info.Diff = "master"
		info.Difficulty = "master"
	}
	info.Type = QueryTypeChart
	return info, nil
}

func (p *Parser) extractDiff(args string) (string, string) {
	aliases := map[string]string{
		"easy":   "easy",
		"ez":     "easy",
		"normal": "normal",
		"nm":     "normal",
		"hard":   "hard",
		"hd":     "hard",
		"expert": "expert",
		"ex":     "expert",
		"exp":    "expert",
		"爷":      "expert",
		"master": "master",
		"ma":     "master",
		"mas":    "master",
		"红":      "master",
		"紫":      "master",
		"append": "append",
		"apd":    "append",
	}

	parts := strings.Fields(args)
	remaining := make([]string, 0, len(parts))
	var diff string
	for _, part := range parts {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if mapped, ok := aliases[normalized]; ok {
			diff = mapped
			continue
		}
		remaining = append(remaining, part)
	}
	return diff, strings.TrimSpace(strings.Join(remaining, " "))
}

func (p *Parser) tryParseID(args string) *QueryInfo {
	normalized := strings.ToLower(strings.TrimSpace(args))
	if strings.HasPrefix(normalized, "id") {
		raw := strings.TrimPrefix(normalized, "id")
		if isNumeric(raw) {
			id, _ := strconv.Atoi(raw)
			return &QueryInfo{Type: QueryTypeID, Value: id, MusicID: id}
		}
	}
	if isNumeric(normalized) {
		id, _ := strconv.Atoi(normalized)
		return &QueryInfo{Type: QueryTypeID, Value: id, MusicID: id}
	}
	return nil
}

func (p *Parser) tryParseSeq(args string) *QueryInfo {
	normalized := strings.TrimSpace(args)
	if strings.HasPrefix(normalized, "-") && isNumeric(normalized[1:]) {
		index, _ := strconv.Atoi(normalized)
		return &QueryInfo{Type: QueryTypeSeq, Value: index}
	}
	return nil
}

func (p *Parser) tryParseEvent(args string) *QueryInfo {
	normalized := strings.ToLower(strings.TrimSpace(args))
	if strings.HasPrefix(normalized, "event") {
		raw := strings.TrimPrefix(normalized, "event")
		if isNumeric(raw) {
			eventID, _ := strconv.Atoi(raw)
			return &QueryInfo{Type: QueryTypeEvent, Value: eventID}
		}
	}
	return nil
}

func (p *Parser) tryParseBan(args string) *QueryInfo {
	normalized := strings.ToLower(strings.TrimSpace(args))
	for nickname, characterID := range p.nicknames {
		if !strings.HasPrefix(normalized, nickname) {
			continue
		}
		raw := strings.TrimPrefix(normalized, nickname)
		if !isNumeric(raw) {
			continue
		}
		seq, _ := strconv.Atoi(raw)
		return &QueryInfo{
			Type:      QueryTypeBan,
			BanCharID: characterID,
			BanSeq:    seq,
		}
	}
	return nil
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
