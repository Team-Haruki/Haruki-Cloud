package card

import (
	"fmt"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

type SearchService struct {
	source DataSource
	parser *Parser
}

func NewSearchService(source DataSource, parser *Parser) *SearchService {
	return &SearchService{
		source: source,
		parser: parser,
	}
}

func (s *SearchService) Search(query string) (*masterdata.Card, error) {
	info, err := s.parser.Parse(query)
	if err != nil || info == nil {
		return nil, fmt.Errorf("无法解析的指令: %s", query)
	}
	switch info.Type {
	case QueryTypeID:
		return s.source.GetCardByID(info.Value)
	case QueryTypeSeq:
		return s.source.GetCardByCharacterAndSeq(info.CharacterID, info.Sequence)
	case QueryTypeFilter:
		items, err := s.source.FilterCards(info)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("card not found (filter): %s", query)
		}
		return items[len(items)-1], nil
	default:
		return nil, fmt.Errorf("无法解析的指令: %s", query)
	}
}

func (s *SearchService) SearchList(query string) ([]*masterdata.Card, error) {
	info, err := s.parser.Parse(query)
	if err != nil || info == nil {
		return nil, fmt.Errorf("无法解析的列表查询指令: %s", query)
	}
	switch info.Type {
	case QueryTypeFilter:
		items, err := s.source.FilterCards(info)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("no cards found for filter: %s", query)
		}
		return items, nil
	case QueryTypeID:
		card, err := s.source.GetCardByID(info.Value)
		if err != nil {
			return nil, err
		}
		return []*masterdata.Card{card}, nil
	case QueryTypeSeq:
		card, err := s.source.GetCardByCharacterAndSeq(info.CharacterID, info.Sequence)
		if err != nil {
			return nil, err
		}
		return []*masterdata.Card{card}, nil
	default:
		return nil, fmt.Errorf("无法解析的列表查询指令: %s", query)
	}
}
