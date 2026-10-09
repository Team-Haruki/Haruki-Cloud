package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/cardsupplie"
	"haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/gamecharacterunit"
	"haruki-cloud/database/sekai/worldbloom"
	"haruki-cloud/database/sekai/worldbloomchapterrankingrewardrange"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/notfound"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/cachefill"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/utils/usererror"
)

type dbEventProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
	once   sync.Once
	local  *localEventProvider
	store  *localStore
	fill   cachefill.Group

	eventMu    sync.RWMutex
	eventCache map[int]*masterdata.Event

	cardMu         sync.RWMutex
	cardCache      map[int]*masterdata.Card
	cardGeneration uint64

	cardLinks   dbMasterIndex[map[int][]int64]
	deckBonuses dbMasterIndex[map[int][]*masterdata.EventDeckBonus]
	allEvents   dbMasterIndex[[]*masterdata.Event]

	unitMu    sync.RWMutex
	unitCache map[int]string

	supplyMu    sync.RWMutex
	supplyCache map[int]string

	wbRangeMu         sync.RWMutex
	wbRangesByChapter map[worldBloomChapterRankingRewardKey][]masterdata.WorldBloomChapterRankingRewardRange
	wbRangesLoaded    bool
}

func (p *dbEventProvider) init() {
	p.once.Do(func() {
		p.eventCache = make(map[int]*masterdata.Event)
		p.cardCache = make(map[int]*masterdata.Card)
		p.unitCache = make(map[int]string)
		p.supplyCache = make(map[int]string)
		p.wbRangesByChapter = make(map[worldBloomChapterRankingRewardKey][]masterdata.WorldBloomChapterRankingRewardRange)
	})
}

func (p *dbEventProvider) GetByID(ctx context.Context, id int) (*masterdata.Event, error) {
	if id == 0 {
		return nil, usererror.Misuse(i18n.M("event.query_required"))
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
		Only(ctx)
	if err != nil {
		if p.local != nil {
			if fallback, fallbackErr := p.local.GetByID(ctx, id); fallbackErr == nil && fallback != nil {
				return fallback, nil
			}
		}
		if sekaiDB.IsNotFound(err) {
			return nil, notfound.Event().WithCause(err)
		}
		return nil, fmt.Errorf("query event %d: %w", id, err)
	}
	model := common.ConvertEventEntity(entity)
	p.eventMu.Lock()
	p.eventCache[id] = model
	p.eventMu.Unlock()
	return common.CloneEvent(model), nil
}

// errEventCardNotFound matches the message of the per-card query this
// lookup replaced; callers only test for a non-nil error.
var errEventCardNotFound = errors.New("sekai: eventcard not found")

// GetByCardID returns the earliest event (lowest event ID) that links the
// card, answered from the cached event-card index.
func (p *dbEventProvider) GetByCardID(ctx context.Context, cardID int) (*masterdata.Event, error) {
	p.init()
	eventID, err := p.firstEventIDForCard(ctx, cardID)
	if err != nil {
		if p.local != nil {
			if fallback, fallbackErr := p.local.GetByCardID(ctx, cardID); fallbackErr == nil && fallback != nil {
				return fallback, nil
			}
		}
		return nil, fmt.Errorf("query event by card %d: %w", cardID, err)
	}
	return p.GetByID(ctx, eventID)
}

func (p *dbEventProvider) firstEventIDForCard(ctx context.Context, cardID int) (int, error) {
	links, err := p.eventCardIDs(ctx)
	if err != nil {
		return 0, err
	}
	first := 0
	for eventID, cardIDs := range links {
		if (first == 0 || eventID < first) && slices.Contains(cardIDs, int64(cardID)) {
			first = eventID
		}
	}
	if first == 0 {
		return 0, errEventCardNotFound
	}
	return first, nil
}

// GetAll lists the region's events by start time from a cached index that
// follows the masterdata reset hooks like the other event indexes.
func (p *dbEventProvider) GetAll(ctx context.Context) []*masterdata.Event {
	p.init()
	events, err := p.allEvents.get(ctx, "events.all_index", p.loadAllEvents)
	if err != nil {
		if p.local != nil {
			return p.local.GetAll(ctx)
		}
		return nil
	}

	result := make([]*masterdata.Event, 0, len(events))
	p.eventMu.Lock()
	defer p.eventMu.Unlock()
	for _, model := range events {
		p.eventCache[model.ID] = model
		result = append(result, common.CloneEvent(model))
	}
	if p.local != nil {
		for _, item := range p.local.GetAll(ctx) {
			if item == nil {
				continue
			}
			if _, ok := p.eventCache[item.ID]; ok {
				continue
			}
			p.eventCache[item.ID] = common.CloneEvent(item)
			result = append(result, common.CloneEvent(item))
		}
		sort.Slice(result, func(i, j int) bool {
			return result[i].StartAt < result[j].StartAt
		})
	}
	return result
}

// loadAllEvents reads only the columns ConvertEventEntity uses; the unused
// event_ranking_reward_ranges column alone is about 1.6 MB for JP.
func (p *dbEventProvider) loadAllEvents(ctx context.Context) ([]*masterdata.Event, error) {
	entities, err := p.client.Event.Query().
		Where(event.ServerRegionEQ(p.region.String())).
		Order(event.ByStartAt()).
		Select(
			event.FieldGameID, event.FieldEventType, event.FieldUnit, event.FieldName, event.FieldAssetbundleName,
			event.FieldStartAt, event.FieldAggregateAt, event.FieldClosedAt, event.FieldVirtualLiveID,
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]*masterdata.Event, 0, len(entities))
	for _, entity := range entities {
		models = append(models, common.ConvertEventEntity(entity))
	}
	return models, nil
}

func (p *dbEventProvider) GetCards(ctx context.Context, eventID int) ([]*masterdata.Card, error) {
	p.init()
	linksByEvent, err := p.eventCardIDs(ctx)
	if err != nil {
		if p.local != nil {
			if fallback, fallbackErr := p.local.GetCards(ctx, eventID); fallbackErr == nil && len(fallback) > 0 {
				return fallback, nil
			}
		}
		return nil, fmt.Errorf("query event cards for event %d: %w", eventID, err)
	}
	cardIDs := linksByEvent[eventID]
	if len(cardIDs) == 0 {
		if p.local != nil {
			if fallback, fallbackErr := p.local.GetCards(ctx, eventID); fallbackErr == nil && len(fallback) > 0 {
				return fallback, nil
			}
		}
		return nil, eventCardsNotFound(eventID)
	}

	return p.getCardsByIDs(ctx, cardIDs)
}

func (p *dbEventProvider) GetRankingHonorRewards(ctx context.Context, eventID int) ([]masterdata.EventRankingHonorReward, error) {
	entity, err := p.client.Event.Query().
		Where(event.ServerRegionEQ(p.region.String()), event.GameIDEQ(int64(eventID))).
		Only(ctx)
	if err != nil {
		if p.local != nil {
			if fallback, fallbackErr := p.local.GetRankingHonorRewards(ctx, eventID); fallbackErr == nil {
				return fallback, nil
			}
		}
		return nil, fmt.Errorf("query ranking honor rewards for event %d: %w", eventID, err)
	}

	rewards := parseEventRankingHonorRewards(entity.EventRankingRewardRanges)
	if len(rewards) == 0 && p.local != nil {
		if fallback, fallbackErr := p.local.GetRankingHonorRewards(ctx, eventID); fallbackErr == nil {
			return fallback, nil
		}
	}
	return rewards, nil
}

func (p *dbEventProvider) GetBannerCharacterID(ctx context.Context, eventID int) (int, error) {
	cards, err := p.GetCards(ctx, eventID)
	if err != nil {
		return 0, err
	}

	minCardID := -1
	var selected *masterdata.Card
	for _, cardInfo := range cards {
		if p.isFestivalCard(ctx, cardInfo.CardSupplyID) {
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

func (p *dbEventProvider) GetDeckBonuses(ctx context.Context, eventID int) ([]*masterdata.EventDeckBonus, error) {
	byEvent, err := p.eventDeckBonuses(ctx)
	if err != nil {
		if p.local != nil {
			if fallback, fallbackErr := p.local.GetDeckBonuses(ctx, eventID); fallbackErr == nil && fallback != nil {
				return fallback, nil
			}
		}
		return nil, fmt.Errorf("query deck bonuses for event %d: %w", eventID, err)
	}

	result := cloneEventDeckBonuses(byEvent[eventID])
	if len(result) == 0 && p.local != nil {
		if fallback, fallbackErr := p.local.GetDeckBonuses(ctx, eventID); fallbackErr == nil && fallback != nil {
			return fallback, nil
		}
	}
	return result, nil
}

func (p *dbEventProvider) GetBanEvents(ctx context.Context, charID int) []*masterdata.Event {
	p.init()
	entities, err := p.client.Event.Query().
		Where(event.ServerRegionEQ(p.region.String())).
		Order(event.ByStartAt()).
		All(ctx)
	if err != nil {
		return nil
	}

	result := make([]*masterdata.Event, 0)
	for _, entity := range entities {
		eventInfo := common.ConvertEventEntity(entity)
		if eventInfo.EventType != "marathon" && eventInfo.EventType != "cheerful_carnival" {
			continue
		}
		bannerCID, err := p.GetBannerCharacterID(ctx, eventInfo.ID)
		if err != nil || bannerCID != charID {
			continue
		}
		if !p.isBoxEvent(ctx, eventInfo.ID) {
			continue
		}
		result = append(result, common.CloneEvent(eventInfo))
	}
	return result
}

func (p *dbEventProvider) isBoxEvent(ctx context.Context, eventID int) bool {
	bonuses, err := p.GetDeckBonuses(ctx, eventID)
	if err != nil || len(bonuses) == 0 {
		return false
	}
	return eventUsesSingleBonusUnit(ctx, bonuses, p.getBonusUnit)
}

func (p *dbEventProvider) getBonusUnit(ctx context.Context, gameCharacterUnitID int) (string, bool) {
	if gameCharacterUnitID == 0 {
		return "", false
	}
	p.init()

	p.unitMu.RLock()
	if cached, ok := p.unitCache[gameCharacterUnitID]; ok {
		p.unitMu.RUnlock()
		return cached, cached != ""
	}
	p.unitMu.RUnlock()

	entity, err := p.client.Gamecharacterunit.Query().
		Where(gamecharacterunit.ServerRegionEQ(p.region.String()), gamecharacterunit.GameIDEQ(int64(gameCharacterUnitID))).
		Only(ctx)
	if err != nil {
		return "", false
	}

	unit, ok := normalizeEventBonusUnit(entity.Unit)
	p.unitMu.Lock()
	p.unitCache[gameCharacterUnitID] = unit
	p.unitMu.Unlock()
	return unit, ok
}

func (p *dbEventProvider) GetWorldBloomChapters(ctx context.Context, eventID int) []*masterdata.WorldBloom {
	items, err := p.client.Worldbloom.Query().
		Where(worldbloom.ServerRegionEQ(p.region.String()), worldbloom.EventIDEQ(int64(eventID))).
		Order(worldbloom.ByChapterStartAt()).
		All(ctx)
	if err != nil {
		if p.local != nil {
			return p.local.GetWorldBloomChapters(ctx, eventID)
		}
		return nil
	}

	result := make([]*masterdata.WorldBloom, 0, len(items))
	for _, item := range items {
		var gameCharacterID *int
		if item.GameCharacterID != 0 {
			gameCharacterID = new(int(item.GameCharacterID))
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
	if len(result) == 0 && p.local != nil {
		return p.local.GetWorldBloomChapters(ctx, eventID)
	}
	return result
}

// GetWorldBloomChapterRankingRewardRanges serves a chapter's reward ranges
// from the database, else from the local files when the database has none
// for the chapter. A query error is served from the local files when they
// are configured and returned otherwise.
func (p *dbEventProvider) GetWorldBloomChapterRankingRewardRanges(ctx context.Context, eventID, gameCharacterID int) ([]masterdata.WorldBloomChapterRankingRewardRange, error) {
	if eventID <= 0 || gameCharacterID <= 0 {
		return nil, nil
	}
	ranges, err := p.worldBloomChapterRankingRewardRangesFromDB(ctx, eventID, gameCharacterID)
	if err == nil && len(ranges) > 0 {
		return ranges, nil
	}
	if p.local != nil {
		ranges, localErr := p.local.GetWorldBloomChapterRankingRewardRanges(ctx, eventID, gameCharacterID)
		if localErr == nil && len(ranges) > 0 {
			return ranges, nil
		}
	}
	if p.store == nil || !p.store.Configured() {
		if err != nil {
			return nil, fmt.Errorf("load world bloom chapter ranking reward ranges: %w", err)
		}
		return nil, nil
	}
	local := &localEventProvider{store: p.store}
	return local.GetWorldBloomChapterRankingRewardRanges(ctx, eventID, gameCharacterID)
}

// worldBloomChapterRankingRewardRangesFromDB serves a chapter's reward ranges
// from worldbloomchapterrankingrewardranges. The region is loaded once,
// including an empty result, and kept until the masterdata cache is reset
// (the registry poll resets it after an ingest); a query error is returned
// and leaves the cache unloaded.
func (p *dbEventProvider) worldBloomChapterRankingRewardRangesFromDB(ctx context.Context, eventID, gameCharacterID int) ([]masterdata.WorldBloomChapterRankingRewardRange, error) {
	if err := p.ensureWorldBloomChapterRankingRewardRangesLoaded(ctx); err != nil {
		return nil, err
	}
	key := worldBloomChapterRankingRewardKey{eventID: eventID, gameCharacterID: gameCharacterID}
	p.wbRangeMu.RLock()
	defer p.wbRangeMu.RUnlock()
	ranges := p.wbRangesByChapter[key]
	if len(ranges) == 0 {
		return nil, nil
	}
	return slices.Clone(ranges), nil
}

func (p *dbEventProvider) ensureWorldBloomChapterRankingRewardRangesLoaded(ctx context.Context) error {
	p.init()
	p.wbRangeMu.RLock()
	loaded := p.wbRangesLoaded
	p.wbRangeMu.RUnlock()
	if loaded {
		return nil
	}
	return p.fill.Do(ctx, "worldBloomChapterRankingRewardRanges", p.loadWorldBloomChapterRankingRewardRanges)
}

func (p *dbEventProvider) loadWorldBloomChapterRankingRewardRanges(ctx context.Context) error {
	p.wbRangeMu.Lock()
	defer p.wbRangeMu.Unlock()
	if p.wbRangesLoaded {
		return nil
	}
	items, err := p.client.Worldbloomchapterrankingrewardrange.Query().
		Where(worldbloomchapterrankingrewardrange.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return err
	}
	byChapter := make(map[worldBloomChapterRankingRewardKey][]masterdata.WorldBloomChapterRankingRewardRange)
	for _, item := range items {
		model := masterdata.WorldBloomChapterRankingRewardRange{
			ID:              int(item.GameID),
			EventID:         int(item.EventID),
			GameCharacterID: int(item.GameCharacterID),
			FromRank:        int(item.FromRank),
			ToRank:          int(item.ToRank),
			ResourceBoxID:   int(item.ResourceBoxID),
		}
		if model.EventID <= 0 || model.GameCharacterID <= 0 {
			continue
		}
		key := worldBloomChapterRankingRewardKey{eventID: model.EventID, gameCharacterID: model.GameCharacterID}
		byChapter[key] = append(byChapter[key], model)
	}
	for _, ranges := range byChapter {
		sort.Slice(ranges, func(i, j int) bool {
			if ranges[i].ToRank != ranges[j].ToRank {
				return ranges[i].ToRank < ranges[j].ToRank
			}
			return ranges[i].FromRank < ranges[j].FromRank
		})
	}
	p.wbRangesByChapter = byChapter
	p.wbRangesLoaded = true
	return nil
}

func (p *dbEventProvider) getCardsByIDs(ctx context.Context, ids []int64) ([]*masterdata.Card, error) {
	result := make([]*masterdata.Card, len(ids))
	var missing []int64
	missingIndex := make(map[int64][]int)

	p.cardMu.RLock()
	generation := p.cardGeneration
	for idx, id := range ids {
		if cached, ok := p.cardCache[int(id)]; ok {
			result[idx] = common.CloneCard(cached)
			continue
		}
		if len(missingIndex[id]) == 0 {
			missing = append(missing, id)
		}
		missingIndex[id] = append(missingIndex[id], idx)
	}
	p.cardMu.RUnlock()

	if len(missing) == 0 {
		return result, nil
	}

	entities, err := p.client.Card.Query().
		Where(card.ServerRegionEQ(p.region.String()), card.GameIDIn(missing...)).
		All(ctx)
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
		if p.cardGeneration == generation {
			p.cardCache[model.ID] = model
		}
		for _, idx := range missingIndex[int64(model.ID)] {
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

func (p *dbEventProvider) isFestivalCard(ctx context.Context, supplyID int) bool {
	typ := p.getCardSupplyType(ctx, supplyID)
	return strings.Contains(typ, "festival")
}

func (p *dbEventProvider) getCardSupplyType(ctx context.Context, id int) string {
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
		Only(ctx)
	if err != nil {
		return ""
	}

	p.supplyMu.Lock()
	p.supplyCache[id] = supply.CardSupplyType
	p.supplyMu.Unlock()
	return supply.CardSupplyType
}
