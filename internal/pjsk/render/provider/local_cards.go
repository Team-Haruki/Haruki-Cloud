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
	cardsByEvent     map[int][]int
	eventByCard      map[int]int
	worldLink3ByCard map[int]bool
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
		items, err := p.store.loadJSON[masterdata.Card]("cards.json")
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
		items, err := p.store.loadJSON[localCardSupplyJSON]("cardSupplies.json")
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
		items, err := p.store.loadJSON[localCardEpisodeJSON]("cardEpisodes.json")
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
		items, err := p.store.loadJSON[masterdata.Gacha]("gachas.json")
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
		links, err := p.store.loadJSON[localCardCostume3dJSON]("cardCostume3ds.json")
		if err != nil {
			return nil, err
		}
		raw, err := p.store.loadJSON[localCostume3dJSON]("costume3ds.json")
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
		items, err := p.store.loadJSON[localEventCardJSON]("eventCards.json")
		if err != nil {
			return cardEventIndex{}, err
		}
		idx := cardEventIndex{
			cardsByEvent:     make(map[int][]int),
			eventByCard:      make(map[int]int),
			worldLink3ByCard: make(map[int]bool),
		}
		events, err := p.store.loadJSON[localEventJSON]("events.json")
		if err != nil {
			return cardEventIndex{}, err
		}
		worldLink3Events := make(map[int]struct{})
		for _, item := range events {
			if isWorldLink3Event(item.toModel()) {
				worldLink3Events[item.ID] = struct{}{}
			}
		}
		for _, item := range items {
			idx.cardsByEvent[item.EventID] = append(idx.cardsByEvent[item.EventID], item.CardID)
			if _, ok := idx.eventByCard[item.CardID]; !ok {
				idx.eventByCard[item.CardID] = item.EventID
			}
			if _, ok := worldLink3Events[item.EventID]; ok {
				idx.worldLink3ByCard[item.CardID] = true
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

	allowedIDs, err := p.allowedCardIDs(filter.EventID)
	if err != nil {
		return nil, err
	}
	if filter.EventID != 0 && len(allowedIDs) == 0 {
		return nil, nil
	}

	results := make([]*masterdata.Card, 0)
	for _, card := range p.cards.v().all {
		if !p.matchesCardFilter(ctx, filter, card, allowedIDs) {
			continue
		}
		results = append(results, common.CloneCard(card))
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

func (p *localCardProvider) allowedCardIDs(eventID int) (map[int]struct{}, error) {
	if eventID == 0 {
		return nil, nil
	}
	if err := p.ensureEventCards(); err != nil {
		return nil, err
	}
	cardIDs := p.eventCards.v().cardsByEvent[eventID]
	allowedIDs := make(map[int]struct{}, len(cardIDs))
	for _, id := range cardIDs {
		allowedIDs[id] = struct{}{}
	}
	return allowedIDs, nil
}

func (p *localCardProvider) matchesCardFilter(ctx context.Context, filter *CardFilter, card *masterdata.Card, allowedIDs map[int]struct{}) bool {
	if allowedIDs != nil && !containsCardID(allowedIDs, card.ID) {
		return false
	}
	if filter.CharacterID != 0 && card.CharacterID != filter.CharacterID {
		return false
	}
	if filter.Rarity != "" && card.CardRarityType != filter.Rarity {
		return false
	}
	if filter.Attr != "" && card.Attr != filter.Attr {
		return false
	}
	if len(filter.SkillIDs) > 0 && !containsInt(filter.SkillIDs, card.SkillID) {
		return false
	}
	if !cardMatchesReleaseYear(card, filter.Year) {
		return false
	}
	if !p.matchesUnitFilter(ctx, filter, card) {
		return false
	}
	if !p.matchesCardSkillType(ctx, filter.SkillType, card.SkillID) {
		return false
	}
	return filter.SupplyType == "" || cardMatchesSupplyFilter(filter.SupplyType, p.GetSupplyType(ctx, card))
}

func containsCardID(allowedIDs map[int]struct{}, cardID int) bool {
	_, ok := allowedIDs[cardID]
	return ok
}

func cardMatchesReleaseYear(card *masterdata.Card, year int) bool {
	if year == 0 {
		return true
	}
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	return card.ReleaseAt >= start && card.ReleaseAt < end
}

func (p *localCardProvider) matchesCardSkillType(ctx context.Context, skillType string, skillID int) bool {
	if skillType == "" {
		return true
	}
	if p.skills == nil {
		return false
	}
	skill, err := p.skills.GetByID(ctx, skillID)
	return err == nil && skill != nil && cardSkillTypesMatch(skillType, skill.DescriptionSpriteName)
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
	if value, ok := cardSupplyTypeOverride(cardInfo.ID); ok {
		return value
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
		if v == "term_limited" && p.isWorldLink3Card(cardInfo.ID) {
			return cardNormalizeSupplyType("unit_event_limited")
		}
		return v
	}
	return cardNormalizeSupplyType("")
}

func (p *localCardProvider) isWorldLink3Card(cardID int) bool {
	if cardID == 0 {
		return false
	}
	if err := p.ensureEventCards(); err != nil {
		return false
	}
	return p.eventCards.v().worldLink3ByCard[cardID]
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
