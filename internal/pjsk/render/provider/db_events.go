package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/cardsupplie"
	"haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/eventcard"
	"haruki-cloud/database/sekai/eventdeckbonuse"
	"haruki-cloud/database/sekai/worldbloom"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type dbEventProvider struct {
	client *sekaiDB.Client
	region renderregion.Value

	eventMu    sync.RWMutex
	eventCache map[int]*masterdata.Event

	cardMu    sync.RWMutex
	cardCache map[int]*masterdata.Card

	supplyMu    sync.RWMutex
	supplyCache map[int]string
}

func (p *dbEventProvider) init() {
	if p.eventCache == nil {
		p.eventCache = make(map[int]*masterdata.Event)
	}
	if p.cardCache == nil {
		p.cardCache = make(map[int]*masterdata.Card)
	}
	if p.supplyCache == nil {
		p.supplyCache = make(map[int]string)
	}
}

func (p *dbEventProvider) GetByID(id int) (*masterdata.Event, error) {
	if id == 0 {
		return nil, fmt.Errorf("event id is required")
	}
	p.init()

	p.eventMu.RLock()
	if cached, ok := p.eventCache[id]; ok {
		p.eventMu.RUnlock()
		return common.CloneEvent(cached), nil
	}
	p.eventMu.RUnlock()

	entity, err := p.client.Event.Query().
		Where(event.ServerRegionEQ(p.region.String()), event.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query event %d: %w", id, err)
	}
	model := common.ConvertEventEntity(entity)
	p.eventMu.Lock()
	p.eventCache[id] = model
	p.eventMu.Unlock()
	return common.CloneEvent(model), nil
}

func (p *dbEventProvider) GetByCardID(cardID int) (*masterdata.Event, error) {
	p.init()
	link, err := p.client.Eventcard.Query().
		Where(eventcard.ServerRegionEQ(p.region.String()), eventcard.CardIDEQ(int64(cardID))).
		Order(eventcard.ByEventID()).
		First(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query event by card %d: %w", cardID, err)
	}
	return p.GetByID(int(link.EventID))
}

func (p *dbEventProvider) GetAll() []*masterdata.Event {
	p.init()
	entities, err := p.client.Event.Query().
		Where(event.ServerRegionEQ(p.region.String())).
		Order(event.ByStartAt()).
		All(context.Background())
	if err != nil {
		return nil
	}

	result := make([]*masterdata.Event, 0, len(entities))
	p.eventMu.Lock()
	defer p.eventMu.Unlock()
	for _, entity := range entities {
		model := common.ConvertEventEntity(entity)
		p.eventCache[model.ID] = model
		result = append(result, common.CloneEvent(model))
	}
	return result
}

func (p *dbEventProvider) GetCards(eventID int) ([]*masterdata.Card, error) {
	p.init()
	links, err := p.client.Eventcard.Query().
		Where(eventcard.ServerRegionEQ(p.region.String()), eventcard.EventIDEQ(int64(eventID))).
		Order(eventcard.ByCardID()).
		All(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query event cards for event %d: %w", eventID, err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no cards found for event %d", eventID)
	}

	cardIDs := make([]int64, 0, len(links))
	for _, link := range links {
		cardIDs = append(cardIDs, link.CardID)
	}
	return p.getCardsByIDs(cardIDs)
}

func (p *dbEventProvider) GetBannerCharacterID(eventID int) (int, error) {
	cards, err := p.GetCards(eventID)
	if err != nil {
		return 0, err
	}

	minCardID := -1
	var selected *masterdata.Card
	for _, cardInfo := range cards {
		if p.isFestivalCard(cardInfo.CardSupplyID) {
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

func (p *dbEventProvider) GetDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	items, err := p.client.Eventdeckbonuse.Query().
		Where(eventdeckbonuse.ServerRegionEQ(p.region.String()), eventdeckbonuse.EventIDEQ(int64(eventID))).
		All(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query deck bonuses for event %d: %w", eventID, err)
	}

	result := make([]*masterdata.EventDeckBonus, 0, len(items))
	for _, item := range items {
		result = append(result, &masterdata.EventDeckBonus{
			ID:                  item.ID,
			EventID:             int(item.EventID),
			GameCharacterUnitID: int(item.GameCharacterUnitID),
			CardAttr:            item.CardAttr,
			BonusRate:           item.BonusRate,
		})
	}
	return result, nil
}

func (p *dbEventProvider) GetBanEvents(charID int) []*masterdata.Event {
	p.init()
	entities, err := p.client.Event.Query().
		Where(event.ServerRegionEQ(p.region.String())).
		Order(event.ByStartAt()).
		All(context.Background())
	if err != nil {
		return nil
	}

	result := make([]*masterdata.Event, 0)
	for _, entity := range entities {
		eventInfo := common.ConvertEventEntity(entity)
		if eventInfo.EventType != "marathon" && eventInfo.EventType != "cheerful_carnival" {
			continue
		}
		bannerCID, err := p.GetBannerCharacterID(eventInfo.ID)
		if err != nil || bannerCID != charID {
			continue
		}
		result = append(result, common.CloneEvent(eventInfo))
	}
	return result
}

func (p *dbEventProvider) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	items, err := p.client.Worldbloom.Query().
		Where(worldbloom.ServerRegionEQ(p.region.String()), worldbloom.EventIDEQ(int64(eventID))).
		Order(worldbloom.ByChapterStartAt()).
		All(context.Background())
	if err != nil {
		return nil
	}

	result := make([]*masterdata.WorldBloom, 0, len(items))
	for _, item := range items {
		var gameCharacterID *int
		if item.GameCharacterID != 0 {
			id := int(item.GameCharacterID)
			gameCharacterID = &id
		}
		result = append(result, &masterdata.WorldBloom{
			ID:              item.ID,
			EventID:         int(item.EventID),
			GameCharacterID: gameCharacterID,
			ChapterNo:       int(item.ChapterNo),
			ChapterStartAt:  item.ChapterStartAt,
			AggregateAt:     item.AggregateAt,
			ChapterEndAt:    item.ChapterEndAt,
			IsSupplemental:  item.IsSupplemental,
			ChapterType:     item.WorldBloomChapterType,
		})
	}
	return result
}

func (p *dbEventProvider) getCardsByIDs(ids []int64) ([]*masterdata.Card, error) {
	result := make([]*masterdata.Card, len(ids))
	var missing []int64
	missingIndex := make(map[int64]int)

	p.cardMu.RLock()
	for idx, id := range ids {
		if cached, ok := p.cardCache[int(id)]; ok {
			result[idx] = common.CloneCard(cached)
			continue
		}
		missing = append(missing, id)
		missingIndex[id] = idx
	}
	p.cardMu.RUnlock()

	if len(missing) == 0 {
		return result, nil
	}

	entities, err := p.client.Card.Query().
		Where(card.ServerRegionEQ(p.region.String()), card.GameIDIn(missing...)).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	p.cardMu.Lock()
	for _, entity := range entities {
		model, err := common.ConvertCardEntity(entity)
		if err != nil {
			p.cardMu.Unlock()
			return nil, err
		}
		p.cardCache[model.ID] = model
		if idx, ok := missingIndex[int64(model.ID)]; ok {
			result[idx] = common.CloneCard(model)
		}
	}
	p.cardMu.Unlock()

	for idx, item := range result {
		if item == nil {
			return nil, fmt.Errorf("card not found for id %d", ids[idx])
		}
	}
	return result, nil
}

func (p *dbEventProvider) isFestivalCard(supplyID int) bool {
	typ := p.getCardSupplyType(supplyID)
	return strings.Contains(typ, "festival")
}

func (p *dbEventProvider) getCardSupplyType(id int) string {
	if id == 0 {
		return ""
	}
	p.supplyMu.RLock()
	if cached, ok := p.supplyCache[id]; ok {
		p.supplyMu.RUnlock()
		return cached
	}
	p.supplyMu.RUnlock()

	supply, err := p.client.Cardsupplie.Query().
		Where(cardsupplie.ServerRegionEQ(p.region.String()), cardsupplie.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return ""
	}

	p.supplyMu.Lock()
	p.supplyCache[id] = supply.CardSupplyType
	p.supplyMu.Unlock()
	return supply.CardSupplyType
}
