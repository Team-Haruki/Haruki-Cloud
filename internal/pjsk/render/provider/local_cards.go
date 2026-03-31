package provider

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localCardProvider
// ===========================================================================

type localCardProvider struct {
	store      *localStore
	characters *localCharacterProvider
	skills     *localSkillProvider

	cardsOnce sync.Once
	cardAll   []*masterdata.Card
	cardByID  map[int]*masterdata.Card
	cardsErr  error

	supplyOnce sync.Once
	supplyByID map[int]string
	supplyErr  error

	gachaOnce sync.Once
	gachas    []*masterdata.Gacha
	gachaErr  error

	costumeOnce   sync.Once
	costumeByCard map[int][]*masterdata.Costume3d
	costumeErr    error

	eventCardOnce sync.Once
	cardsByEvent  map[int][]int
	eventByCard   map[int]int
	eventCardErr  error
}

func (p *localCardProvider) ensureCards() error {
	p.cardsOnce.Do(func() {
		items, err := loadJSON[masterdata.Card](p.store, "cards.json")
		if err != nil {
			p.cardsErr = err
			return
		}
		p.cardByID = make(map[int]*masterdata.Card, len(items))
		p.cardAll = make([]*masterdata.Card, 0, len(items))
		for i := range items {
			c := &items[i]
			p.cardByID[c.ID] = c
			p.cardAll = append(p.cardAll, c)
		}
		sort.Slice(p.cardAll, func(i, j int) bool {
			return p.cardAll[i].ReleaseAt < p.cardAll[j].ReleaseAt
		})
	})
	return p.cardsErr
}

func (p *localCardProvider) ensureSupplies() error {
	p.supplyOnce.Do(func() {
		items, err := loadJSON[localCardSupplyJSON](p.store, "cardSupplies.json")
		if err != nil {
			p.supplyErr = err
			return
		}
		p.supplyByID = make(map[int]string, len(items))
		for _, item := range items {
			p.supplyByID[item.ID] = cardNormalizeSupplyType(item.CardSupplyType)
		}
	})
	return p.supplyErr
}

func (p *localCardProvider) ensureGachas() error {
	p.gachaOnce.Do(func() {
		items, err := loadJSON[masterdata.Gacha](p.store, "gachas.json")
		if err != nil {
			p.gachaErr = err
			return
		}
		p.gachas = make([]*masterdata.Gacha, 0, len(items))
		for i := range items {
			p.gachas = append(p.gachas, &items[i])
		}
		sort.Slice(p.gachas, func(i, j int) bool {
			if p.gachas[i].StartAt == p.gachas[j].StartAt {
				return p.gachas[i].ID > p.gachas[j].ID
			}
			return p.gachas[i].StartAt > p.gachas[j].StartAt
		})
	})
	return p.gachaErr
}

func (p *localCardProvider) ensureCostumes() error {
	p.costumeOnce.Do(func() {
		links, err := loadJSON[localCardCostume3dJSON](p.store, "cardCostume3ds.json")
		if err != nil {
			p.costumeErr = err
			return
		}
		raw, err := loadJSON[localCostume3dJSON](p.store, "costume3ds.json")
		if err != nil {
			p.costumeErr = err
			return
		}
		costumeByID := make(map[int]*masterdata.Costume3d, len(raw))
		for i := range raw {
			costumeByID[raw[i].ID] = raw[i].toModel()
		}
		p.costumeByCard = make(map[int][]*masterdata.Costume3d)
		for _, link := range links {
			if c, ok := costumeByID[link.Costume3dID]; ok {
				p.costumeByCard[link.CardID] = append(p.costumeByCard[link.CardID], c)
			}
		}
	})
	return p.costumeErr
}

func (p *localCardProvider) ensureEventCards() error {
	p.eventCardOnce.Do(func() {
		items, err := loadJSON[localEventCardJSON](p.store, "eventCards.json")
		if err != nil {
			p.eventCardErr = err
			return
		}
		p.cardsByEvent = make(map[int][]int)
		p.eventByCard = make(map[int]int)
		for _, item := range items {
			p.cardsByEvent[item.EventID] = append(p.cardsByEvent[item.EventID], item.CardID)
			if _, ok := p.eventByCard[item.CardID]; !ok {
				p.eventByCard[item.CardID] = item.EventID
			}
		}
	})
	return p.eventCardErr
}

func (p *localCardProvider) GetByID(id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}
	card, ok := p.cardByID[id]
	if !ok {
		return nil, fmt.Errorf("card %d not found", id)
	}
	return common.CloneCard(card), nil
}

func (p *localCardProvider) GetByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	if characterID == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}

	var cards []*masterdata.Card
	for _, c := range p.cardAll {
		if c.CharacterID == characterID {
			cards = append(cards, c)
		}
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("no cards found for character %d", characterID)
	}

	var card *masterdata.Card
	if seq < 0 {
		index := len(cards) + seq
		if index < 0 || index >= len(cards) {
			return nil, fmt.Errorf("card sequence out of range: %d (total: %d)", seq, len(cards))
		}
		card = cards[index]
	} else {
		if seq < 1 || seq > len(cards) {
			return nil, fmt.Errorf("card sequence out of range: %d (total: %d)", seq, len(cards))
		}
		card = cards[seq-1]
	}
	return common.CloneCard(card), nil
}

func (p *localCardProvider) Filter(filter *CardFilter) ([]*masterdata.Card, error) {
	if filter == nil {
		return nil, fmt.Errorf("filter is required")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}

	var allowedIDs map[int]struct{}
	if filter.EventID != 0 {
		if err := p.ensureEventCards(); err != nil {
			return nil, err
		}
		cardIDs, ok := p.cardsByEvent[filter.EventID]
		if !ok || len(cardIDs) == 0 {
			return nil, nil
		}
		allowedIDs = make(map[int]struct{}, len(cardIDs))
		for _, id := range cardIDs {
			allowedIDs[id] = struct{}{}
		}
	}

	results := make([]*masterdata.Card, 0)
	for _, card := range p.cardAll {
		if allowedIDs != nil {
			if _, ok := allowedIDs[card.ID]; !ok {
				continue
			}
		}
		if filter.CharacterID != 0 && card.CharacterID != filter.CharacterID {
			continue
		}
		if filter.Rarity != "" && card.CardRarityType != filter.Rarity {
			continue
		}
		if filter.Attr != "" && card.Attr != filter.Attr {
			continue
		}
		if filter.Year != 0 {
			start := time.Date(filter.Year, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			end := time.Date(filter.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			if card.ReleaseAt < start || card.ReleaseAt >= end {
				continue
			}
		}
		if filter.Unit != "" || filter.SupportUnit != "" {
			if !p.matchesUnitFilter(filter, card) {
				continue
			}
		}
		if filter.SkillType != "" {
			if p.skills != nil {
				skill, sErr := p.skills.GetByID(card.SkillID)
				if sErr != nil || skill == nil || skill.DescriptionSpriteName != filter.SkillType {
					continue
				}
			} else {
				continue
			}
		}
		if filter.SupplyType != "" && !cardMatchesSupplyFilter(filter.SupplyType, p.GetSupplyType(card)) {
			continue
		}
		results = append(results, common.CloneCard(card))
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

func (p *localCardProvider) matchesUnitFilter(filter *CardFilter, card *masterdata.Card) bool {
	if filter == nil || card == nil {
		return false
	}
	if filter.Unit == "" && filter.SupportUnit == "" {
		return true
	}
	if p.characters == nil {
		return false
	}
	character, err := p.characters.GetByID(card.CharacterID)
	if err != nil || character == nil {
		return false
	}
	mainUnit := cardNormalizeUnit(character.Unit)
	supportUnit := cardNormalizeSupportUnit(card.SupportUnit)

	if filter.Unit != "" && filter.Unit != mainUnit && filter.Unit != supportUnit {
		return false
	}
	if filter.SupportUnit != "" && filter.SupportUnit != supportUnit {
		return false
	}
	return true
}

func (p *localCardProvider) GetSupplyType(cardInfo *masterdata.Card) string {
	if cardInfo == nil {
		return cardNormalizeSupplyType("")
	}
	if cardInfo.CardRarityType == "rarity_birthday" {
		return cardNormalizeSupplyType("birthday")
	}
	if cardInfo.CardSupplyID == 0 {
		return cardNormalizeSupplyType("")
	}
	if err := p.ensureSupplies(); err != nil {
		return cardNormalizeSupplyType("")
	}
	if v, ok := p.supplyByID[cardInfo.CardSupplyID]; ok {
		return v
	}
	return cardNormalizeSupplyType("")
}

func (p *localCardProvider) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	if cardID == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureGachas(); err != nil {
		return nil, err
	}
	for _, g := range p.gachas {
		if cardContainsPickup(g, cardID) {
			return common.CloneGacha(g), nil
		}
	}
	return nil, fmt.Errorf("gacha not found for card: %d", cardID)
}

func (p *localCardProvider) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	if cardID == 0 {
		return nil, nil
	}
	if err := p.ensureCostumes(); err != nil {
		return nil, err
	}
	costumes, ok := p.costumeByCard[cardID]
	if !ok || len(costumes) == 0 {
		return nil, nil
	}
	return common.CloneCostumes(costumes), nil
}

func (p *localCardProvider) GetUnitByCardID(cardID int) (string, error) {
	card, err := p.GetByID(cardID)
	if err != nil {
		return "", err
	}
	if p.characters != nil {
		character, cErr := p.characters.GetByID(card.CharacterID)
		if cErr == nil && character != nil {
			if character.Unit != "" && character.Unit != "piapro" {
				return character.Unit, nil
			}
			if card.SupportUnit != "" && card.SupportUnit != "none" {
				return card.SupportUnit, nil
			}
			return "piapro", nil
		}
	}
	return "", fmt.Errorf("character not found for card %d", cardID)
}
