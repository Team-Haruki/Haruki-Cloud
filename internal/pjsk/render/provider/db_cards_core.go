package provider

import (
	"context"
	"fmt"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/cardepisode"
	"haruki-cloud/database/sekai/eventcard"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/notfound"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbCardProvider) GetByID(ctx context.Context, id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	p.init()

	p.cardMu.RLock()
	cached, ok := p.cardCache[id]
	indexed := p.allCards != nil
	p.cardMu.RUnlock()
	if ok {
		return common.CloneCard(cached), nil
	}
	if indexed {
		// The index holds every card of the region, so a miss is a
		// negative answer and needs no query.
		return nil, notfound.CardID(id)
	}

	entity, err := p.client.Card.Query().
		Where(card.ServerRegionEQ(p.region.String()), card.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		if sekaiDB.IsNotFound(err) {
			return nil, notfound.CardID(id).WithCause(err)
		}
		return nil, fmt.Errorf("query card %d: %w", id, err)
	}

	model, err := common.ConvertCardEntity(entity)
	if err != nil {
		return nil, fmt.Errorf("decode card %d for region %s: %w", entity.GameID, p.region, err)
	}
	p.cardMu.Lock()
	p.cardCache[id] = model
	p.cardMu.Unlock()
	return common.CloneCard(model), nil
}

// GetByCharacterAndSeq returns the character's seq-th card in release order
// (1-based; a negative seq counts from the newest card).
func (p *dbCardProvider) GetByCharacterAndSeq(ctx context.Context, characterID, seq int) (*masterdata.Card, error) {
	if characterID == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	p.init()

	cards, err := p.cardIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("query cards by character: %w", err)
	}
	var owned []*masterdata.Card
	for _, model := range cards {
		if model.CharacterID == characterID {
			owned = append(owned, model)
		}
	}
	if len(owned) == 0 {
		return nil, notfound.Card("")
	}

	index := seq - 1
	if seq < 0 {
		index = len(owned) + seq
	}
	if seq == 0 || index < 0 || index >= len(owned) {
		return nil, notfound.Card("")
	}
	return common.CloneCard(owned[index]), nil
}

// Filter answers every card query from the region's in-memory index. The
// column conditions (event cards, character, rarity, attribute and release
// year) keep the semantics of the SQL predicates they replaced; the unit,
// skill and supply conditions were always applied in memory.
func (p *dbCardProvider) Filter(ctx context.Context, filter *CardFilter) ([]*masterdata.Card, error) {
	if filter == nil {
		return nil, fmt.Errorf("filter is required")
	}
	p.init()
	if cardFilterUsesAllCards(filter) {
		return p.getAllCards(ctx, filter.Limit)
	}

	eventCardIDs, err := p.resolveFilterEventCardIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	if filter.EventID != 0 && len(eventCardIDs) == 0 {
		return nil, nil
	}

	matchingSkillIDs, err := p.cardFilterSkillTypeIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	if filter.SkillType != "" && len(matchingSkillIDs) == 0 {
		return nil, nil
	}

	cards, err := p.cardIndex(ctx)
	if err != nil {
		return nil, err
	}
	columns := newCardColumnFilter(filter, eventCardIDs)
	results := make([]*masterdata.Card, 0)
	for _, model := range cards {
		if !columns.matches(model) {
			continue
		}
		if !p.cardMatchesFilter(ctx, filter, matchingSkillIDs, model) {
			continue
		}
		results = append(results, common.CloneCard(model))
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

// cardColumnFilter holds the conditions the former SQL query pushed into
// WHERE: event card membership, character, rarity, attribute and release
// year. A nil eventCardIDs set means the filter has no event condition.
type cardColumnFilter struct {
	eventCardIDs map[int]struct{}
	characterID  int
	rarity       string
	attr         string
	releaseFrom  int64
	releaseUntil int64
}

func newCardColumnFilter(filter *CardFilter, eventCardIDs []int) cardColumnFilter {
	columns := cardColumnFilter{
		characterID: filter.CharacterID,
		rarity:      filter.Rarity,
		attr:        filter.Attr,
	}
	if len(eventCardIDs) > 0 {
		columns.eventCardIDs = make(map[int]struct{}, len(eventCardIDs))
		for _, id := range eventCardIDs {
			columns.eventCardIDs[id] = struct{}{}
		}
	}
	if filter.Year != 0 {
		columns.releaseFrom = time.Date(filter.Year, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		columns.releaseUntil = time.Date(filter.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	}
	return columns
}

func (f cardColumnFilter) matches(model *masterdata.Card) bool {
	if f.eventCardIDs != nil {
		if _, ok := f.eventCardIDs[model.ID]; !ok {
			return false
		}
	}
	if f.characterID != 0 && model.CharacterID != f.characterID {
		return false
	}
	if f.rarity != "" && model.CardRarityType != f.rarity {
		return false
	}
	if f.attr != "" && model.Attr != f.attr {
		return false
	}
	if f.releaseUntil != 0 && (model.ReleaseAt < f.releaseFrom || model.ReleaseAt >= f.releaseUntil) {
		return false
	}
	return true
}

func (p *dbCardProvider) cardFilterSkillTypeIDs(ctx context.Context, filter *CardFilter) (map[int]struct{}, error) {
	if filter.SkillType == "" || p.skills == nil {
		return nil, nil
	}
	finishSkillFilter := commandtrace.MeasureOperation(ctx, "cards.skill_filter")
	ids, err := p.skills.matchingTypeIDs(ctx, filter.SkillType)
	finishSkillFilter()
	if err != nil {
		return nil, fmt.Errorf("load card skill filter: %w", err)
	}
	return ids, nil
}

func (p *dbCardProvider) cardMatchesFilter(ctx context.Context, filter *CardFilter, matchingSkillIDs map[int]struct{}, model *masterdata.Card) bool {
	if !p.matchesUnitFilter(ctx, filter, model) {
		return false
	}
	if len(filter.SkillIDs) > 0 && !containsInt(filter.SkillIDs, model.SkillID) {
		return false
	}
	if filter.SkillType != "" {
		if _, ok := matchingSkillIDs[model.SkillID]; !ok {
			return false
		}
	}
	return filter.SupplyType == "" || cardMatchesSupplyFilter(filter.SupplyType, p.GetSupplyType(ctx, model))
}

func (p *dbCardProvider) GetUnitByCardID(ctx context.Context, cardID int) (string, error) {
	cardInfo, err := p.GetByID(ctx, cardID)
	if err != nil {
		return "", err
	}
	if p.characters != nil {
		character, cErr := p.characters.GetByID(ctx, cardInfo.CharacterID)
		if cErr == nil && character != nil {
			if character.Unit != "" && character.Unit != "piapro" {
				return character.Unit, nil
			}
			if cardInfo.SupportUnit != "" && cardInfo.SupportUnit != "none" {
				return cardInfo.SupportUnit, nil
			}
			return "piapro", nil
		}
	}
	return "", fmt.Errorf("character not found for card %d", cardID)
}

func (p *dbCardProvider) GetEpisodesByCardID(ctx context.Context, cardID int) ([]*masterdata.CardEpisode, error) {
	if cardID == 0 {
		return nil, nil
	}
	if err := p.ensureAllEpisodesLoaded(ctx); err != nil {
		return nil, err
	}

	p.episodeMu.RLock()
	episodes := cloneCardEpisodes(p.episodesByCard[cardID])
	p.episodeMu.RUnlock()
	return episodes, nil
}

func (p *dbCardProvider) ensureAllEpisodesLoaded(ctx context.Context) error {
	p.init()
	p.episodeMu.RLock()
	loaded := p.episodesLoaded
	p.episodeMu.RUnlock()
	if loaded {
		return nil
	}

	callerToken := new(dbBulkIndexFlightToken)
	result := p.episodeLoads.DoChan("all", func() (any, error) {
		completed := runDBBulkIndexFlight(callerToken, func(loadCtx context.Context) error {
			finishIndex := commandtrace.MeasureOperation(loadCtx, "cards.episode_index")
			defer finishIndex()
			p.episodeMu.RLock()
			alreadyLoaded := p.episodesLoaded
			p.episodeMu.RUnlock()
			if alreadyLoaded {
				return nil
			}
			entities, err := p.client.Cardepisode.Query().
				Where(cardepisode.ServerRegionEQ(p.region.String())).
				Select(
					cardepisode.FieldGameID,
					cardepisode.FieldSeq,
					cardepisode.FieldCardID,
					cardepisode.FieldCardEpisodePartType,
				).
				Order(cardepisode.ByCardID(), cardepisode.BySeq(), cardepisode.ByID()).
				All(loadCtx)
			if err != nil {
				return fmt.Errorf("query card episodes for region %s: %w", p.region, err)
			}

			byCard := make(map[int][]*masterdata.CardEpisode)
			for _, entity := range entities {
				cardID := int(entity.CardID)
				byCard[cardID] = append(byCard[cardID], &masterdata.CardEpisode{
					ID:                  int(entity.GameID),
					Seq:                 int(entity.Seq),
					CardID:              cardID,
					CardEpisodePartType: entity.CardEpisodePartType,
				})
			}

			p.episodeMu.Lock()
			p.episodesByCard = byCard
			p.episodesLoaded = true
			p.episodeMu.Unlock()
			return nil
		})
		return completed, nil
	})

	return waitDBBulkIndexFlight(ctx, result, callerToken, "cards.episode_index_wait", "cards.episode_index_shared")
}

func cloneCardEpisodes(episodes []*masterdata.CardEpisode) []*masterdata.CardEpisode {
	if len(episodes) == 0 {
		return nil
	}
	result := make([]*masterdata.CardEpisode, 0, len(episodes))
	for _, episode := range episodes {
		if episode == nil {
			continue
		}
		clone := *episode
		result = append(result, &clone)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// resolveFilterEventCardIDs returns the ids of the event's cards, or nil
// when the filter has no event condition or the event has no cards.
func (p *dbCardProvider) resolveFilterEventCardIDs(ctx context.Context, filter *CardFilter) ([]int, error) {
	if filter == nil || filter.EventID == 0 {
		return nil, nil
	}
	byEvent, err := p.eventCards.get(ctx, "cards.event_card_index", p.loadEventCardIDs)
	if err != nil {
		return nil, fmt.Errorf("query event cards for event %d: %w", filter.EventID, err)
	}
	return byEvent[filter.EventID], nil
}

// loadEventCardIDs indexes the region's event cards by event id.
func (p *dbCardProvider) loadEventCardIDs(ctx context.Context) (map[int][]int, error) {
	links, err := p.client.Eventcard.Query().
		Where(eventcard.ServerRegionEQ(p.region.String())).
		Select(eventcard.FieldEventID, eventcard.FieldCardID).
		Order(eventcard.ByEventID(), eventcard.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	byEvent := make(map[int][]int)
	for _, link := range links {
		eventID := int(link.EventID)
		byEvent[eventID] = append(byEvent[eventID], int(link.CardID))
	}
	return byEvent, nil
}

func (p *dbCardProvider) matchesUnitFilter(ctx context.Context, filter *CardFilter, cardInfo *masterdata.Card) bool {
	if filter == nil || cardInfo == nil {
		return false
	}
	if filter.Unit == "" && filter.MainUnit == "" && filter.SupportUnit == "" {
		return true
	}
	if p.characters == nil {
		return false
	}

	character, err := p.characters.GetByID(ctx, cardInfo.CharacterID)
	if err != nil || character == nil {
		return false
	}
	return cardMatchesUnitFilter(filter, character.Unit, cardInfo.SupportUnit)
}
