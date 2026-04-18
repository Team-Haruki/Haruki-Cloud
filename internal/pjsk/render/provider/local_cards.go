package provider

import (
	"context"
	"fmt"
	"sort"
	"time"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localCardProvider
// ===========================================================================

type cardIndex struct {
	all  []*masterdata.Card
	byID map[int]*masterdata.Card
}

type cardEventIndex struct {
	cardsByEvent map[int][]int
	eventByCard  map[int]int
}

type localCardProvider struct {
	store      *localStore
	characters *localCharacterProvider
	skills     *localSkillProvider

	cards      lazyValue[cardIndex]
	episodes   lazyValue[map[int][]*masterdata.CardEpisode]
	supplies   lazyValue[map[int]string]
	gachas     lazyValue[[]*masterdata.Gacha]
	costumes   lazyValue[map[int][]*masterdata.Costume3d]
	eventCards lazyValue[cardEventIndex]
}

func (p *localCardProvider) ensureCards() error {
	return p.cards.init(func() (cardIndex, error) {
		items, err := loadJSON[masterdata.Card](p.store, "cards.json")
		if err != nil {
			return cardIndex{}, err
		}
		idx := cardIndex{
			byID: make(map[int]*masterdata.Card, len(items)),
			all:  make([]*masterdata.Card, 0, len(items)),
		}
		for i := range items {
			c := &items[i]
			idx.byID[c.ID] = c
			idx.all = append(idx.all, c)
		}
		sort.Slice(idx.all, func(i, j int) bool {
			return idx.all[i].ReleaseAt < idx.all[j].ReleaseAt
		})
		return idx, nil
	})
}

func (p *localCardProvider) ensureSupplies() error {
	return p.supplies.init(func() (map[int]string, error) {
		items, err := loadJSON[localCardSupplyJSON](p.store, "cardSupplies.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]string, len(items))
		for _, item := range items {
			byID[item.ID] = cardNormalizeSupplyType(item.CardSupplyType)
		}
		return byID, nil
	})
}

func (p *localCardProvider) ensureEpisodes() error {
	return p.episodes.init(func() (map[int][]*masterdata.CardEpisode, error) {
		items, err := loadJSON[localCardEpisodeJSON](p.store, "cardEpisodes.json")
		if err != nil {
			return nil, err
		}
		byCard := make(map[int][]*masterdata.CardEpisode)
		for i := range items {
			episode := items[i].toModel()
			byCard[episode.CardID] = append(byCard[episode.CardID], episode)
		}
		for cardID := range byCard {
			sort.Slice(byCard[cardID], func(i, j int) bool {
				if byCard[cardID][i].Seq == byCard[cardID][j].Seq {
					return byCard[cardID][i].ID < byCard[cardID][j].ID
				}
				return byCard[cardID][i].Seq < byCard[cardID][j].Seq
			})
		}
		return byCard, nil
	})
}

func (p *localCardProvider) ensureGachas() error {
	return p.gachas.init(func() ([]*masterdata.Gacha, error) {
		items, err := loadJSON[masterdata.Gacha](p.store, "gachas.json")
		if err != nil {
			return nil, err
		}
		gachas := make([]*masterdata.Gacha, 0, len(items))
		for i := range items {
			gachas = append(gachas, &items[i])
		}
		sort.Slice(gachas, func(i, j int) bool {
			if gachas[i].StartAt == gachas[j].StartAt {
				return gachas[i].ID > gachas[j].ID
			}
			return gachas[i].StartAt > gachas[j].StartAt
		})
		return gachas, nil
	})
}

func (p *localCardProvider) ensureCostumes() error {
	return p.costumes.init(func() (map[int][]*masterdata.Costume3d, error) {
		links, err := loadJSON[localCardCostume3dJSON](p.store, "cardCostume3ds.json")
		if err != nil {
			return nil, err
		}
		raw, err := loadJSON[localCostume3dJSON](p.store, "costume3ds.json")
		if err != nil {
			return nil, err
		}
		costumeByID := make(map[int]*masterdata.Costume3d, len(raw))
		for i := range raw {
			costumeByID[raw[i].ID] = raw[i].toModel()
		}
		byCard := make(map[int][]*masterdata.Costume3d)
		for _, link := range links {
			if c, ok := costumeByID[link.Costume3dID]; ok {
				byCard[link.CardID] = append(byCard[link.CardID], c)
			}
		}
		return byCard, nil
	})
}

func (p *localCardProvider) ensureEventCards() error {
	return p.eventCards.init(func() (cardEventIndex, error) {
		items, err := loadJSON[localEventCardJSON](p.store, "eventCards.json")
		if err != nil {
			return cardEventIndex{}, err
		}
		idx := cardEventIndex{
			cardsByEvent: make(map[int][]int),
			eventByCard:  make(map[int]int),
		}
		for _, item := range items {
			idx.cardsByEvent[item.EventID] = append(idx.cardsByEvent[item.EventID], item.CardID)
			if _, ok := idx.eventByCard[item.CardID]; !ok {
				idx.eventByCard[item.CardID] = item.EventID
			}
		}
		return idx, nil
	})
}

func (p *localCardProvider) GetByID(_ context.Context, id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}
	card, ok := p.cards.v().byID[id]
	if !ok {
		return nil, fmt.Errorf("card %d not found", id)
	}
	return common.CloneCard(card), nil
}

func (p *localCardProvider) GetByCharacterAndSeq(_ context.Context, characterID, seq int) (*masterdata.Card, error) {
	if characterID == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}

	var cards []*masterdata.Card
	for _, c := range p.cards.v().all {
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

func (p *localCardProvider) Filter(ctx context.Context, filter *CardFilter) ([]*masterdata.Card, error) {
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
		cardIDs, ok := p.eventCards.v().cardsByEvent[filter.EventID]
		if !ok || len(cardIDs) == 0 {
			return nil, nil
		}
		allowedIDs = make(map[int]struct{}, len(cardIDs))
		for _, id := range cardIDs {
			allowedIDs[id] = struct{}{}
		}
	}

	results := make([]*masterdata.Card, 0)
	for _, card := range p.cards.v().all {
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
		if filter.Unit != "" || filter.MainUnit != "" || filter.SupportUnit != "" {
			if !p.matchesUnitFilter(ctx, filter, card) {
				continue
			}
		}
		if filter.SkillType != "" {
			if p.skills != nil {
				skill, sErr := p.skills.GetByID(ctx, card.SkillID)
				if sErr != nil || skill == nil || !cardSkillTypesMatch(filter.SkillType, skill.DescriptionSpriteName) {
					continue
				}
			} else {
				continue
			}
		}
		if filter.SupplyType != "" && !cardMatchesSupplyFilter(filter.SupplyType, p.GetSupplyType(ctx, card)) {
			continue
		}
		results = append(results, common.CloneCard(card))
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

func (p *localCardProvider) matchesUnitFilter(ctx context.Context, filter *CardFilter, card *masterdata.Card) bool {
	if filter == nil || card == nil {
		return false
	}
	if filter.Unit == "" && filter.MainUnit == "" && filter.SupportUnit == "" {
		return true
	}
	if p.characters == nil {
		return false
	}
	character, err := p.characters.GetByID(ctx, card.CharacterID)
	if err != nil || character == nil {
		return false
	}
	return cardMatchesUnitFilter(filter, character.Unit, card.SupportUnit)
}

func (p *localCardProvider) GetSupplyType(_ context.Context, cardInfo *masterdata.Card) string {
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
	if v, ok := p.supplies.v()[cardInfo.CardSupplyID]; ok {
		return v
	}
	return cardNormalizeSupplyType("")
}

func (p *localCardProvider) GetGachaByCardID(_ context.Context, cardID int) (*masterdata.Gacha, error) {
	if cardID == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureGachas(); err != nil {
		return nil, err
	}
	for _, g := range p.gachas.v() {
		if cardContainsPickup(g, cardID) {
			return common.CloneGacha(g), nil
		}
	}
	return nil, fmt.Errorf("gacha not found for card: %d", cardID)
}

func (p *localCardProvider) GetCostume3dsByCardID(_ context.Context, cardID int) ([]*masterdata.Costume3d, error) {
	if cardID == 0 {
		return nil, nil
	}
	if err := p.ensureCostumes(); err != nil {
		return nil, err
	}
	costumes, ok := p.costumes.v()[cardID]
	if !ok || len(costumes) == 0 {
		return nil, nil
	}
	return common.CloneCostumes(costumes), nil
}

func (p *localCardProvider) GetUnitByCardID(ctx context.Context, cardID int) (string, error) {
	card, err := p.GetByID(ctx, cardID)
	if err != nil {
		return "", err
	}
	if p.characters != nil {
		character, cErr := p.characters.GetByID(ctx, card.CharacterID)
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

func (p *localCardProvider) GetEpisodesByCardID(_ context.Context, cardID int) ([]*masterdata.CardEpisode, error) {
	if cardID == 0 {
		return nil, nil
	}
	if err := p.ensureEpisodes(); err != nil {
		return nil, err
	}
	episodes := p.episodes.v()[cardID]
	if len(episodes) == 0 {
		return nil, nil
	}
	result := make([]*masterdata.CardEpisode, 0, len(episodes))
	for _, episode := range episodes {
		if episode == nil {
			continue
		}
		cloned := *episode
		result = append(result, &cloned)
	}
	return result, nil
}
