package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localEventProvider
// ===========================================================================

type eventIndex struct {
	all  []*masterdata.Event
	byID map[int]*masterdata.Event
}

type eventCardIndex struct {
	eventByCard  map[int]int
	cardsByEvent map[int][]int
}

type localEventProvider struct {
	store *localStore
	cards *localCardProvider

	events     lazyValue[eventIndex]
	eventCards lazyValue[eventCardIndex]
	deckBonus  lazyValue[map[int][]*masterdata.EventDeckBonus]
	worldBloom lazyValue[map[int][]*masterdata.WorldBloom]
}

func (p *localEventProvider) ensureEvents() error {
	return p.events.init(func() (eventIndex, error) {
		items, err := loadJSON[localEventJSON](p.store, "events.json")
		if err != nil {
			return eventIndex{}, err
		}
		idx := eventIndex{
			byID: make(map[int]*masterdata.Event, len(items)),
			all:  make([]*masterdata.Event, 0, len(items)),
		}
		for i := range items {
			m := items[i].toModel()
			idx.byID[m.ID] = m
			idx.all = append(idx.all, m)
		}
		sort.Slice(idx.all, func(i, j int) bool {
			return idx.all[i].StartAt < idx.all[j].StartAt
		})
		return idx, nil
	})
}

func (p *localEventProvider) ensureEventCards() error {
	return p.eventCards.init(func() (eventCardIndex, error) {
		items, err := loadJSON[localEventCardJSON](p.store, "eventCards.json")
		if err != nil {
			return eventCardIndex{}, err
		}
		idx := eventCardIndex{
			eventByCard:  make(map[int]int),
			cardsByEvent: make(map[int][]int),
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

func (p *localEventProvider) ensureDeckBonuses() error {
	return p.deckBonus.init(func() (map[int][]*masterdata.EventDeckBonus, error) {
		items, err := loadJSON[masterdata.EventDeckBonus](p.store, "eventDeckBonuses.json")
		if err != nil {
			return nil, err
		}
		byEvent := make(map[int][]*masterdata.EventDeckBonus)
		for i := range items {
			byEvent[items[i].EventID] = append(byEvent[items[i].EventID], &items[i])
		}
		return byEvent, nil
	})
}

func (p *localEventProvider) ensureWorldBlooms() error {
	return p.worldBloom.init(func() (map[int][]*masterdata.WorldBloom, error) {
		items, err := loadJSON[localWorldBloomJSON](p.store, "worldBlooms.json")
		if err != nil {
			return nil, err
		}
		byEvent := make(map[int][]*masterdata.WorldBloom)
		for i := range items {
			m := items[i].toModel()
			byEvent[m.EventID] = append(byEvent[m.EventID], m)
		}
		for _, wbs := range byEvent {
			sort.Slice(wbs, func(i, j int) bool {
				return wbs[i].ChapterStartAt < wbs[j].ChapterStartAt
			})
		}
		return byEvent, nil
	})
}

func (p *localEventProvider) GetByID(_ context.Context, id int) (*masterdata.Event, error) {
	if id == 0 {
		return nil, fmt.Errorf("event id is required")
	}
	if err := p.ensureEvents(); err != nil {
		return nil, err
	}
	ev, ok := p.events.v().byID[id]
	if !ok {
		return nil, fmt.Errorf("event %d not found", id)
	}
	return common.CloneEvent(ev), nil
}

func (p *localEventProvider) GetByCardID(ctx context.Context, cardID int) (*masterdata.Event, error) {
	if err := p.ensureEventCards(); err != nil {
		return nil, err
	}
	eventID, ok := p.eventCards.v().eventByCard[cardID]
	if !ok {
		return nil, fmt.Errorf("no event found for card %d", cardID)
	}
	return p.GetByID(ctx, eventID)
}

func (p *localEventProvider) GetAll(_ context.Context) []*masterdata.Event {
	if err := p.ensureEvents(); err != nil {
		return nil
	}
	result := make([]*masterdata.Event, 0, len(p.events.v().all))
	for _, ev := range p.events.v().all {
		result = append(result, common.CloneEvent(ev))
	}
	return result
}

func (p *localEventProvider) GetCards(ctx context.Context, eventID int) ([]*masterdata.Card, error) {
	if err := p.ensureEventCards(); err != nil {
		return nil, err
	}
	cardIDs, ok := p.eventCards.v().cardsByEvent[eventID]
	if !ok || len(cardIDs) == 0 {
		return nil, fmt.Errorf("no cards found for event %d", eventID)
	}
	result := make([]*masterdata.Card, 0, len(cardIDs))
	for _, id := range cardIDs {
		card, err := p.cards.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, card)
	}
	return result, nil
}

func (p *localEventProvider) GetBannerCharacterID(ctx context.Context, eventID int) (int, error) {
	cards, err := p.GetCards(ctx, eventID)
	if err != nil {
		return 0, err
	}
	minCardID := -1
	var selected *masterdata.Card
	for _, cardInfo := range cards {
		supplyType := p.cards.GetSupplyType(ctx, cardInfo)
		if strings.Contains(supplyType, "festival") {
			continue
		}
		if minCardID == -1 || cardInfo.ID < minCardID {
			minCardID = cardInfo.ID
			selected = cardInfo
		}
	}
	if selected == nil {
		return 0, fmt.Errorf("no valid banner card found for event %d", eventID)
	}
	return selected.CharacterID, nil
}

func (p *localEventProvider) GetDeckBonuses(_ context.Context, eventID int) ([]*masterdata.EventDeckBonus, error) {
	if err := p.ensureDeckBonuses(); err != nil {
		return nil, err
	}
	bonuses, ok := p.deckBonus.v()[eventID]
	if !ok {
		return nil, nil
	}
	result := make([]*masterdata.EventDeckBonus, 0, len(bonuses))
	for _, b := range bonuses {
		c := *b
		result = append(result, &c)
	}
	return result, nil
}

func (p *localEventProvider) GetBanEvents(ctx context.Context, charID int) []*masterdata.Event {
	if err := p.ensureEvents(); err != nil {
		return nil
	}
	result := make([]*masterdata.Event, 0)
	for _, ev := range p.events.v().all {
		if ev.EventType != "marathon" && ev.EventType != "cheerful_carnival" {
			continue
		}
		bannerCID, err := p.GetBannerCharacterID(ctx, ev.ID)
		if err != nil || bannerCID != charID {
			continue
		}
		result = append(result, common.CloneEvent(ev))
	}
	return result
}

func (p *localEventProvider) GetWorldBloomChapters(_ context.Context, eventID int) []*masterdata.WorldBloom {
	if err := p.ensureWorldBlooms(); err != nil {
		return nil
	}
	wbs, ok := p.worldBloom.v()[eventID]
	if !ok {
		return nil
	}
	result := make([]*masterdata.WorldBloom, 0, len(wbs))
	for _, wb := range wbs {
		c := *wb
		result = append(result, &c)
	}
	return result
}
