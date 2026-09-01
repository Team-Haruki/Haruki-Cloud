package provider

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/card"
	"haruki-cloud/database/sekai/cardepisode"
	"haruki-cloud/database/sekai/eventcard"
	"haruki-cloud/database/sekai/predicate"
	"haruki-cloud/internal/observability/commandtrace"
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
	cachedAt := p.cardCachedAt[id]
	if ok && dbBulkIndexFresh(true, cachedAt) {
		p.cardMu.RUnlock()
		return common.CloneCard(cached), nil
	}
	p.cardMu.RUnlock()

	entity, err := p.client.Card.Query().
		Where(card.ServerRegionEQ(p.region.String()), card.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query card %d: %w", id, err)
	}

	model, err := common.ConvertCardEntity(entity)
	if err != nil {
		return nil, fmt.Errorf("decode card %d for region %s: %w", entity.GameID, p.region, err)
	}
	p.cardMu.Lock()
	p.cardCache[id] = model
	p.cardCachedAt[id] = time.Now()
	p.cardMu.Unlock()
	return common.CloneCard(model), nil
}

func (p *dbCardProvider) GetByCharacterAndSeq(ctx context.Context, characterID, seq int) (*masterdata.Card, error) {
	if characterID == 0 {
		return nil, fmt.Errorf("character id is required")
	}

	entities, err := p.client.Card.Query().
		Where(card.ServerRegionEQ(p.region.String()), card.CharacterIDEQ(int64(characterID))).
		Order(card.ByReleaseAt()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query cards by character: %w", err)
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("no cards found for character %d", characterID)
	}

	var entity *sekaiDB.Card
	if seq < 0 {
		index := len(entities) + seq
		if index < 0 || index >= len(entities) {
			return nil, fmt.Errorf("card sequence out of range: %d (total: %d)", seq, len(entities))
		}
		entity = entities[index]
	} else {
		if seq < 1 || seq > len(entities) {
			return nil, fmt.Errorf("card sequence out of range: %d (total: %d)", seq, len(entities))
		}
		entity = entities[seq-1]
	}

	p.init()
	model, err := common.ConvertCardEntity(entity)
	if err != nil {
		return nil, fmt.Errorf("decode card %d for region %s: %w", entity.GameID, p.region, err)
	}
	p.cardMu.Lock()
	p.cardCache[model.ID] = model
	p.cardCachedAt[model.ID] = time.Now()
	p.cardMu.Unlock()
	return common.CloneCard(model), nil
}

func (p *dbCardProvider) Filter(ctx context.Context, filter *CardFilter) ([]*masterdata.Card, error) {
	if filter == nil {
		return nil, fmt.Errorf("filter is required")
	}
	p.init()

	query, empty, err := p.cardFilterQuery(ctx, filter)
	if err != nil {
		return nil, err
	}
	if empty {
		return nil, nil
	}

	matchingSkillIDs, err := p.cardFilterSkillTypeIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	if filter.SkillType != "" && len(matchingSkillIDs) == 0 {
		return nil, nil
	}

	entities, err := query.Order(card.ByReleaseAt()).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("filter cards: %w", err)
	}

	results := make([]*masterdata.Card, 0, len(entities))
	for _, entity := range entities {
		model, err := common.ConvertCardEntity(entity)
		if err != nil {
			return nil, fmt.Errorf("decode card %d for region %s: %w", entity.GameID, p.region, err)
		}
		// The filtered entities are the same canonical card records used by
		// GetByID. Populate the shared cache before applying in-memory filters so
		// follow-up lookups (for example GetUnitByCardID while building CardBox)
		// do not turn one list query into hundreds of SELECTs.
		p.cacheFilteredCard(model)
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

func (p *dbCardProvider) cardFilterQuery(ctx context.Context, filter *CardFilter) (*sekaiDB.CardQuery, bool, error) {
	query := p.client.Card.Query().Where(card.ServerRegionEQ(p.region.String()))
	eventCardIDs, err := p.resolveFilterEventCardIDs(ctx, filter)
	if err != nil {
		return nil, false, err
	}
	if len(eventCardIDs) == 0 && filter.EventID != 0 {
		return query, true, nil
	}
	if len(eventCardIDs) > 0 {
		query = query.Where(card.GameIDIn(eventCardIDs...))
	}
	if filter.CharacterID != 0 {
		query = query.Where(card.CharacterIDEQ(int64(filter.CharacterID)))
	}
	if filter.Rarity != "" {
		query = query.Where(cardJsonFieldEQ("card_rarity_type", filter.Rarity))
	}
	if filter.Attr != "" {
		query = query.Where(cardJsonFieldEQ("attr", filter.Attr))
	}
	if filter.Year != 0 {
		start := time.Date(filter.Year, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		end := time.Date(filter.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		query = query.Where(card.ReleaseAtGTE(start), card.ReleaseAtLT(end))
	}
	return query, false, nil
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

func (p *dbCardProvider) cacheFilteredCard(model *masterdata.Card) {
	p.cardMu.Lock()
	p.cardCache[model.ID] = model
	p.cardCachedAt[model.ID] = time.Now()
	p.cardMu.Unlock()
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
	loaded := dbBulkIndexFresh(p.episodesLoaded, p.episodesLoadedAt)
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
			alreadyLoaded := dbBulkIndexFresh(p.episodesLoaded, p.episodesLoadedAt)
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
			p.episodesLoadedAt = time.Now()
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

func (p *dbCardProvider) resolveFilterEventCardIDs(ctx context.Context, filter *CardFilter) ([]int64, error) {
	if filter == nil || filter.EventID == 0 {
		return nil, nil
	}
	links, err := p.client.Eventcard.Query().
		Where(eventcard.ServerRegionEQ(p.region.String()), eventcard.EventIDEQ(int64(filter.EventID))).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query event cards for event %d: %w", filter.EventID, err)
	}
	if len(links) == 0 {
		return nil, nil
	}

	result := make([]int64, 0, len(links))
	for _, link := range links {
		result = append(result, link.CardID)
	}
	return result, nil
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

// cardJsonFieldEQ creates a predicate that matches a JSONB text field by its
// unquoted string value. Works with PostgreSQL's ->> operator.
func cardJsonFieldEQ(field, value string) predicate.Card {
	return sql.FieldEQ(field, value)
}
