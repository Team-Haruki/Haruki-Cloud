package card

import (
	"fmt"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

func NewSearchService(source DataSource, parser *Parser) *SearchService {
	return &SearchService{
		source: source,
		parser: parser,
	}
}

func (s *SearchService) WithAllowUnreleased(allow bool) *SearchService {
	if s == nil {
		return nil
	}
	s.allowUnreleased = allow
	return s
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
		if !s.allowUnreleased && !isCardVisibleAt(card, now) {
			return nil, releasecheck.New(releasecheck.KindCard, "", info.Value)
		}
		return card, nil
	case QueryTypeSeq:
		card, err := s.cardByCharacterAndSeq(info.CharacterID, info.Sequence, now)
		if err != nil {
			return nil, err
		}
		return card, nil
	case QueryTypeLatest:
		card, err := s.latestCard(info.Sequence, now)
		if err != nil {
			return nil, err
		}
		return card, nil
	case QueryTypeFilter:
		items, err := s.source.FilterCards(info)
		if err != nil {
			return nil, err
		}
		visibleItems := items
		if !s.allowUnreleased {
			visibleItems = filterVisibleCards(items, now)
		}
		if len(visibleItems) == 0 {
			if len(items) > 0 {
				return nil, releasecheck.New(releasecheck.KindCard, query, 0)
			}
			return nil, fmt.Errorf("card not found (filter): %s", query)
		}
		sortCardsByReleaseAndID(visibleItems)
		return visibleItems[len(visibleItems)-1], nil
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
		if !s.allowUnreleased {
			items = filterVisibleCards(items, now)
		}
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
		if !s.allowUnreleased && !isCardVisibleAt(card, now) {
			return nil, releasecheck.New(releasecheck.KindCard, "", info.Value)
		}
		return []*masterdata.Card{card}, nil
	case QueryTypeSeq:
		card, err := s.cardByCharacterAndSeq(info.CharacterID, info.Sequence, now)
		if err != nil {
			return nil, err
		}
		return []*masterdata.Card{card}, nil
	case QueryTypeLatest:
		card, err := s.latestCard(info.Sequence, now)
		if err != nil {
			return nil, err
		}
		return []*masterdata.Card{card}, nil
	default:
		return nil, fmt.Errorf("无法解析的列表查询指令: %s", query)
	}
}

func (s *SearchService) cardByCharacterAndSeq(characterID, sequence int, now int64) (*masterdata.Card, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("card data source is not configured")
	}
	if characterID == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	if sequence == 0 {
		return nil, fmt.Errorf("card sequence must not be zero")
	}

	items, err := s.source.FilterCards(&PjskCardQueryInfo{CharacterID: characterID})
	if err != nil {
		return nil, err
	}
	visibleItems := items
	if !s.allowUnreleased {
		visibleItems = filterVisibleCards(items, now)
	}
	if len(visibleItems) == 0 {
		if len(items) > 0 {
			return nil, releasecheck.New(releasecheck.KindCard, "", 0)
		}
		return nil, fmt.Errorf("card not found: %d/%d", characterID, sequence)
	}

	sortCardsByReleaseAndID(visibleItems)

	index := 0
	if sequence < 0 {
		index = len(visibleItems) + sequence
	} else {
		index = sequence - 1
	}
	if index < 0 || index >= len(visibleItems) {
		return nil, fmt.Errorf("card not found: %d/%d", characterID, sequence)
	}
	return visibleItems[index], nil
}

func (s *SearchService) latestCard(sequence int, now int64) (*masterdata.Card, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("card data source is not configured")
	}
	if sequence >= 0 {
		return nil, fmt.Errorf("card sequence must be negative: %d", sequence)
	}

	items, err := s.source.FilterCards(&PjskCardQueryInfo{})
	if err != nil {
		return nil, err
	}
	visibleItems := items
	if !s.allowUnreleased {
		visibleItems = filterVisibleCards(items, now)
	}
	if len(visibleItems) == 0 {
		if len(items) > 0 {
			return nil, releasecheck.New(releasecheck.KindCard, "", 0)
		}
		return nil, fmt.Errorf("no released cards found")
	}

	sortCardsByReleaseAndID(visibleItems)
	index := len(visibleItems) + sequence
	if index < 0 || index >= len(visibleItems) {
		return nil, fmt.Errorf("card not found: latest/%d", sequence)
	}
	return visibleItems[index], nil
}
