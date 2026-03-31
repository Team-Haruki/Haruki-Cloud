package provider

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localEventProvider
// ===========================================================================

type localEventProvider struct {
	store *localStore
	cards *localCardProvider

	eventsOnce sync.Once
	eventAll   []*masterdata.Event
	eventByID  map[int]*masterdata.Event
	eventsErr  error

	eventCardOnce sync.Once
	eventByCard   map[int]int
	cardsByEvent  map[int][]int
	eventCardErr  error

	deckBonusOnce    sync.Once
	deckBonusByEvent map[int][]*masterdata.EventDeckBonus
	deckBonusErr     error

	worldBloomOnce    sync.Once
	worldBloomByEvent map[int][]*masterdata.WorldBloom
	worldBloomErr     error
}

func (p *localEventProvider) ensureEvents() error {
	p.eventsOnce.Do(func() {
		items, err := loadJSON[localEventJSON](p.store, "events.json")
		if err != nil {
			p.eventsErr = err
			return
		}
		p.eventByID = make(map[int]*masterdata.Event, len(items))
		p.eventAll = make([]*masterdata.Event, 0, len(items))
		for i := range items {
			m := items[i].toModel()
			p.eventByID[m.ID] = m
			p.eventAll = append(p.eventAll, m)
		}
		sort.Slice(p.eventAll, func(i, j int) bool {
			return p.eventAll[i].StartAt < p.eventAll[j].StartAt
		})
	})
	return p.eventsErr
}

func (p *localEventProvider) ensureEventCards() error {
	p.eventCardOnce.Do(func() {
		items, err := loadJSON[localEventCardJSON](p.store, "eventCards.json")
		if err != nil {
			p.eventCardErr = err
			return
		}
		p.eventByCard = make(map[int]int)
		p.cardsByEvent = make(map[int][]int)
		for _, item := range items {
			p.cardsByEvent[item.EventID] = append(p.cardsByEvent[item.EventID], item.CardID)
			if _, ok := p.eventByCard[item.CardID]; !ok {
				p.eventByCard[item.CardID] = item.EventID
			}
		}
	})
	return p.eventCardErr
}

func (p *localEventProvider) ensureDeckBonuses() error {
	p.deckBonusOnce.Do(func() {
		items, err := loadJSON[masterdata.EventDeckBonus](p.store, "eventDeckBonuses.json")
		if err != nil {
			p.deckBonusErr = err
			return
		}
		p.deckBonusByEvent = make(map[int][]*masterdata.EventDeckBonus)
		for i := range items {
			p.deckBonusByEvent[items[i].EventID] = append(
				p.deckBonusByEvent[items[i].EventID], &items[i])
		}
	})
	return p.deckBonusErr
}

func (p *localEventProvider) ensureWorldBlooms() error {
	p.worldBloomOnce.Do(func() {
		items, err := loadJSON[localWorldBloomJSON](p.store, "worldBlooms.json")
		if err != nil {
			p.worldBloomErr = err
			return
		}
		p.worldBloomByEvent = make(map[int][]*masterdata.WorldBloom)
		for i := range items {
			m := items[i].toModel()
			p.worldBloomByEvent[m.EventID] = append(p.worldBloomByEvent[m.EventID], m)
		}
		for _, wbs := range p.worldBloomByEvent {
			sort.Slice(wbs, func(i, j int) bool {
				return wbs[i].ChapterStartAt < wbs[j].ChapterStartAt
			})
		}
	})
	return p.worldBloomErr
}

func (p *localEventProvider) GetByID(id int) (*masterdata.Event, error) {
	if id == 0 {
		return nil, fmt.Errorf("event id is required")
	}
	if err := p.ensureEvents(); err != nil {
		return nil, err
	}
	ev, ok := p.eventByID[id]
	if !ok {
		return nil, fmt.Errorf("event %d not found", id)
	}
	return common.CloneEvent(ev), nil
}

func (p *localEventProvider) GetByCardID(cardID int) (*masterdata.Event, error) {
	if err := p.ensureEventCards(); err != nil {
		return nil, err
	}
	eventID, ok := p.eventByCard[cardID]
	if !ok {
		return nil, fmt.Errorf("no event found for card %d", cardID)
	}
	return p.GetByID(eventID)
}

func (p *localEventProvider) GetAll() []*masterdata.Event {
	if err := p.ensureEvents(); err != nil {
		return nil
	}
	result := make([]*masterdata.Event, 0, len(p.eventAll))
	for _, ev := range p.eventAll {
		result = append(result, common.CloneEvent(ev))
	}
	return result
}

func (p *localEventProvider) GetCards(eventID int) ([]*masterdata.Card, error) {
	if err := p.ensureEventCards(); err != nil {
		return nil, err
	}
	cardIDs, ok := p.cardsByEvent[eventID]
	if !ok || len(cardIDs) == 0 {
		return nil, fmt.Errorf("no cards found for event %d", eventID)
	}
	result := make([]*masterdata.Card, 0, len(cardIDs))
	for _, id := range cardIDs {
		card, err := p.cards.GetByID(id)
		if err != nil {
			return nil, err
		}
		result = append(result, card)
	}
	return result, nil
}

func (p *localEventProvider) GetBannerCharacterID(eventID int) (int, error) {
	cards, err := p.GetCards(eventID)
	if err != nil {
		return 0, err
	}
	minCardID := -1
	var selected *masterdata.Card
	for _, cardInfo := range cards {
		supplyType := p.cards.GetSupplyType(cardInfo)
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

func (p *localEventProvider) GetDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	if err := p.ensureDeckBonuses(); err != nil {
		return nil, err
	}
	bonuses, ok := p.deckBonusByEvent[eventID]
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

func (p *localEventProvider) GetBanEvents(charID int) []*masterdata.Event {
	if err := p.ensureEvents(); err != nil {
		return nil
	}
	result := make([]*masterdata.Event, 0)
	for _, ev := range p.eventAll {
		if ev.EventType != "marathon" && ev.EventType != "cheerful_carnival" {
			continue
		}
		bannerCID, err := p.GetBannerCharacterID(ev.ID)
		if err != nil || bannerCID != charID {
			continue
		}
		result = append(result, common.CloneEvent(ev))
	}
	return result
}

func (p *localEventProvider) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	if err := p.ensureWorldBlooms(); err != nil {
		return nil
	}
	wbs, ok := p.worldBloomByEvent[eventID]
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
