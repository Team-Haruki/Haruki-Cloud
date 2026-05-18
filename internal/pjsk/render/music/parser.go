package music

import (
	"fmt"
	"strconv"
	"strings"
)

func NewParser(banCharacterNicknames map[string]int) *Parser {
	return &Parser{banCharacterNicknames: cloneNicknames(banCharacterNicknames)}
}

func (p *Parser) Parse(args string) (*QueryInfo, error) {
	args = strings.TrimSpace(args)
	diff, cleanArgs := ExtractMusicDifficulty(args)

	if info := p.tryParseExplicitID(cleanArgs); info != nil {
		info.Diff = diff
		info.Difficulty = diff
		info.Original = args
		return info, nil
	}

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
	return ExtractMusicDifficulty(args)
}

func (p *Parser) tryParseExplicitID(args string) *QueryInfo {
	if id, ok := ParseExplicitMusicID(args); ok {
		return &QueryInfo{
			Type:    QueryTypeID,
			Value:   id,
			MusicID: id,
			Keyword: strings.TrimSpace(args),
		}
	}
	return nil
}

func (p *Parser) tryParseID(args string) *QueryInfo {
	if id, ok := ParseImplicitMusicID(args); ok {
		return &QueryInfo{
			Type:               QueryTypeID,
			Value:              id,
			MusicID:            id,
			Keyword:            strings.TrimSpace(args),
			AllowTitleFallback: true,
		}
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
	for nickname, characterID := range p.banCharacterNicknames {
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
			Keyword:   strings.TrimSpace(args),
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
