package event

import (
	"context"
	"fmt"
	"sort"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/cardsupplie"
	"haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/eventcard"
	"haruki-cloud/database/sekai/eventdeckbonuse"
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/database/sekai/gamecharacterunit"
	"haruki-cloud/database/sekai/worldbloom"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type CloudSource struct {
	client      *sekaiDB.Client
	region      renderregion.Value
	queryRegion renderregion.Value

	eventMu    sync.RWMutex
	eventCache map[int]*masterdata.Event

	cardMu    sync.RWMutex
	cardCache map[int]*masterdata.Card

	supplyMu    sync.RWMutex
	supplyCache map[int]string

	gcuMu    sync.RWMutex
	gcuCache map[int]*masterdata.GameCharacterUnit

	charMu    sync.RWMutex
	charCache map[int]*masterdata.Character
}

func NewCloudSource(client *sekaiDB.Client, defaultRegion renderregion.Value) *CloudSource {
	if client == nil {
		return nil
	}
	region := renderregion.WithDefault(defaultRegion)
	return &CloudSource{
		client:      client,
		region:      region,
		queryRegion: region,
		eventCache:  make(map[int]*masterdata.Event),
		cardCache:   make(map[int]*masterdata.Card),
		supplyCache: make(map[int]string),
		gcuCache:    make(map[int]*masterdata.GameCharacterUnit),
		charCache:   make(map[int]*masterdata.Character),
	}
}

func (c *CloudSource) DefaultRegion() renderregion.Value {
	return c.region
}

func (c *CloudSource) GetEventByID(id int) (*masterdata.Event, error) {
	c.eventMu.RLock()
	if cached, ok := c.eventCache[id]; ok {
		c.eventMu.RUnlock()
		return cloneEvent(cached), nil
	}
	c.eventMu.RUnlock()

	entity, err := c.client.Event.Query().
		Where(event.ServerRegionEQ(c.queryRegion.String()), event.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, err
	}

	model := convertEventEntity(entity)
	c.eventMu.Lock()
	c.eventCache[id] = model
	c.eventMu.Unlock()
	return cloneEvent(model), nil
}

func (c *CloudSource) GetEventByCardID(cardID int) (*masterdata.Event, error) {
	link, err := c.client.Eventcard.Query().
		Where(eventcard.ServerRegionEQ(c.queryRegion.String()), eventcard.CardIDEQ(int64(cardID))).
		Order(eventcard.ByEventID()).
		First(context.Background())
	if err != nil {
		return nil, err
	}
	return c.GetEventByID(int(link.EventID))
}

func (c *CloudSource) GetEvents() []*masterdata.Event {
	entities, err := c.client.Event.Query().
		Where(event.ServerRegionEQ(c.queryRegion.String())).
		Order(event.ByStartAt()).
		All(context.Background())
	if err != nil {
		return nil
	}

	result := make([]*masterdata.Event, 0, len(entities))
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	for _, entity := range entities {
		model := convertEventEntity(entity)
		c.eventCache[model.ID] = model
		result = append(result, cloneEvent(model))
	}
	return result
}

func (c *CloudSource) GetEventCards(eventID int) ([]*masterdata.Card, error) {
	links, err := c.client.Eventcard.Query().
		Where(eventcard.ServerRegionEQ(c.queryRegion.String()), eventcard.EventIDEQ(int64(eventID))).
		Order(eventcard.ByCardID()).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no cards found for event %d", eventID)
	}

	cardIDs := make([]int, 0, len(links))
	for _, link := range links {
		cardIDs = append(cardIDs, int(link.CardID))
	}
	return c.getCardsByIDs(cardIDs)
}

func (c *CloudSource) GetEventBannerCharacterID(eventID int) (int, error) {
	cards, err := c.GetEventCards(eventID)
	if err != nil {
		return 0, err
	}

	minCardID := -1
	var selected *masterdata.Card
	for _, cardInfo := range cards {
		if c.isFestivalCard(cardInfo.CardSupplyID) {
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

func (c *CloudSource) GetEventDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	items, err := c.client.Eventdeckbonuse.Query().
		Where(eventdeckbonuse.ServerRegionEQ(c.queryRegion.String()), eventdeckbonuse.EventIDEQ(int64(eventID))).
		All(context.Background())
	if err != nil {
		return nil, err
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

func (c *CloudSource) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	c.gcuMu.RLock()
	if cached, ok := c.gcuCache[id]; ok {
		c.gcuMu.RUnlock()
		return cloneGameCharacterUnit(cached), nil
	}
	c.gcuMu.RUnlock()

	entity, err := c.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(c.queryRegion.String()), gamecharacterunit.IDEQ(id)).
		Only(context.Background())
	if err != nil {
		return nil, err
	}

	model := &masterdata.GameCharacterUnit{
		ID:              entity.ID,
		GameCharacterID: int(entity.GameCharacterID),
		Unit:            entity.Unit,
		ColorCode:       entity.ColorCode,
	}
	c.gcuMu.Lock()
	c.gcuCache[id] = model
	c.gcuMu.Unlock()
	return cloneGameCharacterUnit(model), nil
}

func (c *CloudSource) GetBanEvents(charID int) []*masterdata.Event {
	entities, err := c.client.Event.Query().
		Where(
			event.ServerRegionEQ(c.queryRegion.String()),
			event.EventTypeIn("marathon", "cheerful_carnival"),
		).
		Order(event.ByStartAt()).
		All(context.Background())
	if err != nil {
		return nil
	}

	result := make([]*masterdata.Event, 0, len(entities))
	for _, entity := range entities {
		eventInfo := convertEventEntity(entity)
		bannerCID, err := c.GetEventBannerCharacterID(eventInfo.ID)
		if err != nil || bannerCID != charID {
			continue
		}
		result = append(result, cloneEvent(eventInfo))
	}
	return result
}

func (c *CloudSource) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	items, err := c.client.Worldbloom.Query().
		Where(worldbloom.ServerRegionEQ(c.queryRegion.String()), worldbloom.EventIDEQ(int64(eventID))).
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

func (c *CloudSource) GetCharacterByID(id int) (*masterdata.Character, error) {
	c.charMu.RLock()
	if cached, ok := c.charCache[id]; ok {
		c.charMu.RUnlock()
		return cloneCharacter(cached), nil
	}
	c.charMu.RUnlock()

	entity, err := c.client.Gamecharacter.Query().
		Where(gamecharacter.ServerRegionEQ(c.queryRegion.String()), gamecharacter.IDEQ(id)).
		Only(context.Background())
	if err != nil {
		return nil, err
	}

	model := &masterdata.Character{
		ID:        entity.ID,
		FirstName: entity.FirstName,
		GivenName: entity.GivenName,
		Unit:      entity.Unit,
	}
	c.charMu.Lock()
	c.charCache[id] = model
	c.charMu.Unlock()
	return cloneCharacter(model), nil
}

func (c *CloudSource) getCardsByIDs(ids []int) ([]*masterdata.Card, error) {
	result := make([]*masterdata.Card, len(ids))
	var missing []int64
	missingIndex := make(map[int]int)

	c.cardMu.RLock()
	for idx, id := range ids {
		if cached, ok := c.cardCache[id]; ok {
			result[idx] = cloneCard(cached)
			continue
		}
		missing = append(missing, int64(id))
		missingIndex[id] = idx
	}
	c.cardMu.RUnlock()

	if len(missing) == 0 {
		return result, nil
	}

	entities, err := c.client.Card.Query().
		Where(card.ServerRegionEQ(c.queryRegion.String()), card.GameIDIn(missing...)).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	c.cardMu.Lock()
	for _, entity := range entities {
		model := convertCardEntity(entity)
		c.cardCache[model.ID] = model
		if idx, ok := missingIndex[model.ID]; ok {
			result[idx] = cloneCard(model)
		}
	}
	c.cardMu.Unlock()

	for idx, item := range result {
		if item == nil {
			return nil, fmt.Errorf("card not found for id %d", ids[idx])
		}
	}
	return result, nil
}

func (c *CloudSource) isFestivalCard(supplyID int) bool {
	typ := c.getCardSupplyType(supplyID)
	return typ == "colorful_festival_limited" || typ == "bloom_festival_limited"
}

func (c *CloudSource) getCardSupplyType(id int) string {
	if id == 0 {
		return ""
	}
	c.supplyMu.RLock()
	if cached, ok := c.supplyCache[id]; ok {
		c.supplyMu.RUnlock()
		return cached
	}
	c.supplyMu.RUnlock()

	supply, err := c.client.Cardsupplie.Query().
		Where(cardsupplie.ServerRegionEQ(c.queryRegion.String()), cardsupplie.IDEQ(id)).
		Only(context.Background())
	if err != nil {
		return ""
	}

	c.supplyMu.Lock()
	c.supplyCache[id] = supply.CardSupplyType
	c.supplyMu.Unlock()
	return supply.CardSupplyType
}

func convertEventEntity(entity *sekaiDB.Event) *masterdata.Event {
	return &masterdata.Event{
		ID:              int(entity.GameID),
		EventType:       entity.EventType,
		Name:            entity.Name,
		AssetBundleName: entity.AssetbundleName,
		StartAt:         entity.StartAt,
		AggregateAt:     entity.AggregateAt,
		ClosedAt:        entity.ClosedAt,
	}
}

func convertCardEntity(entity *sekaiDB.Card) *masterdata.Card {
	return &masterdata.Card{
		ID:                              int(entity.GameID),
		CharacterID:                     int(entity.CharacterID),
		CardRarityType:                  entity.CardRarityType,
		Attr:                            entity.Attr,
		Prefix:                          entity.Prefix,
		AssetBundleName:                 entity.AssetbundleName,
		ReleaseAt:                       entity.ReleaseAt,
		SkillID:                         int(entity.SkillID),
		CardSkillName:                   entity.CardSkillName,
		SupportUnit:                     entity.SupportUnit,
		SpecialTrainingPower1BonusFixed: int(entity.SpecialTrainingPower1BonusFixed),
		SpecialTrainingPower2BonusFixed: int(entity.SpecialTrainingPower2BonusFixed),
		SpecialTrainingPower3BonusFixed: int(entity.SpecialTrainingPower3BonusFixed),
		SpecialTrainingSkillID:          int(entity.SpecialTrainingSkillID),
		SpecialTrainingSkillName:        entity.SpecialTrainingSkillName,
		CardSupplyID:                    int(entity.CardSupplyID),
	}
}

func cloneEvent(item *masterdata.Event) *masterdata.Event {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func cloneCard(item *masterdata.Card) *masterdata.Card {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func cloneGameCharacterUnit(item *masterdata.GameCharacterUnit) *masterdata.GameCharacterUnit {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func cloneCharacter(item *masterdata.Character) *masterdata.Character {
	if item == nil {
		return nil
	}
	copy := *item
	return &copy
}

func sortUniqueInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}
