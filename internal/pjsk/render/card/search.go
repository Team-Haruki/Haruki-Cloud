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
	now := currentCardVisibilityTime()
	info, err := s.parser.Parse(query)
	if err != nil || info == nil {
		return nil, fmt.Errorf("无法解析的指令: %s", query)
	}
	switch info.Type {
	case QueryTypeID:
		card, err := s.source.GetCardByID(info.Value)
		if err != nil {
			return nil, err
		}
		if !isCardVisibleAt(card, now) {
			return nil, fmt.Errorf("card %d not found", info.Value)
		}
		return card, nil
	case QueryTypeSeq:
		card, err := s.source.GetCardByCharacterAndSeq(info.CharacterID, info.Sequence)
		if err != nil {
			return nil, err
		}
		if !isCardVisibleAt(card, now) {
			return nil, fmt.Errorf("card not found: %d/%d", info.CharacterID, info.Sequence)
		}
		return card, nil
	case QueryTypeLatest:
		card, err := s.latestVisibleCard(info.Sequence, now)
		if err != nil {
			return nil, err
		}
		return card, nil
	case QueryTypeFilter:
		items, err := s.source.FilterCards(info)
		if err != nil {
			return nil, err
		}
		items = filterVisibleCards(items, now)
		if len(items) == 0 {
			return nil, fmt.Errorf("card not found (filter): %s", query)
		}
		sortCardsByReleaseAndID(items)
		return items[len(items)-1], nil
	default:
		return nil, fmt.Errorf("无法解析的指令: %s", query)
	}
}

func (s *SearchService) SearchList(query string) ([]*masterdata.Card, error) {
	now := currentCardVisibilityTime()
	info, err := s.parser.ParsePreferFilter(query)
	if err != nil || info == nil {
		return nil, fmt.Errorf("无法解析的列表查询指令: %s", query)
	}
	switch info.Type {
	case QueryTypeFilter:
		items, err := s.source.FilterCards(info)
		if err != nil {
			return nil, err
		}
		items = filterVisibleCards(items, now)
		if len(items) == 0 {
			return nil, fmt.Errorf("no cards found for filter: %s", query)
		}
		sortCardsByReleaseAndID(items)
		return items, nil
	case QueryTypeID:
		card, err := s.source.GetCardByID(info.Value)
		if err != nil {
			return nil, err
		}
		if !isCardVisibleAt(card, now) {
			return nil, fmt.Errorf("card %d not found", info.Value)
		}
		return []*masterdata.Card{card}, nil
	case QueryTypeSeq:
		card, err := s.source.GetCardByCharacterAndSeq(info.CharacterID, info.Sequence)
		if err != nil {
			return nil, err
		}
		if !isCardVisibleAt(card, now) {
			return nil, fmt.Errorf("card not found: %d/%d", info.CharacterID, info.Sequence)
		}
		return []*masterdata.Card{card}, nil
	case QueryTypeLatest:
		card, err := s.latestVisibleCard(info.Sequence, now)
		if err != nil {
			return nil, err
		}
		return []*masterdata.Card{card}, nil
	default:
		return nil, fmt.Errorf("无法解析的列表查询指令: %s", query)
	}
}

func (s *SearchService) latestVisibleCard(sequence int, now int64) (*masterdata.Card, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("card data source is not configured")
	}
	if sequence >= 0 {
		return nil, fmt.Errorf("card sequence must be negative: %d", sequence)
	}

	items, err := s.source.FilterCards(&CardQueryInfo{})
	if err != nil {
		return nil, err
	}
	items = filterVisibleCards(items, now)
	if len(items) == 0 {
		return nil, fmt.Errorf("no released cards found")
	}

	sortCardsByReleaseAndID(items)
	index := len(items) + sequence
	if index < 0 || index >= len(items) {
		return nil, fmt.Errorf("card not found: latest/%d", sequence)
	}
	return items[index], nil
}
