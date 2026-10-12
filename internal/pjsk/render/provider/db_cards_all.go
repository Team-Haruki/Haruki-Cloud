package provider

import (
	"context"
	"fmt"
	"strconv"

	"haruki-cloud/database/sekai/card"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"

	"golang.org/x/sync/singleflight"
)

func cardFilterUsesAllCards(f *CardFilter) bool {
	return f.CharacterID == 0 && f.Unit == "" && f.MainUnit == "" &&
		f.SupportUnit == "" && f.Rarity == "" && f.Attr == "" &&
		f.SkillType == "" && len(f.SkillIDs) == 0 && f.SupplyType == "" &&
		f.Year == 0 && f.EventID == 0
}

// cardIndex serves the region's card index: every card in release order,
// loaded once per generation by one shared flight. The slice and its models
// are shared with every caller and must not be mutated; a reset drops the
// index and the next reader waits for the reload.
func (p *dbCardProvider) cardIndex(ctx context.Context) ([]*masterdata.Card, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.cardMu.RLock()
		generation := p.cardGeneration
		cards := p.allCards
		p.cardMu.RUnlock()
		if cards != nil {
			return cards, nil
		}
		caller := new(dbBulkIndexFlightToken)
		result := p.startAllCardsLoad(generation, caller)
		if err := waitDBBulkIndexFlight(ctx, result, caller, "cards.all_index_wait", "cards.all_index_shared"); err != nil {
			return nil, err
		}
		// A reset during loading discards that generation; retry the current one.
	}
}

// getAllCards returns clones of the region's cards in release order, cut to
// limit when it is positive.
func (p *dbCardProvider) getAllCards(ctx context.Context, limit int) ([]*masterdata.Card, error) {
	cards, err := p.cardIndex(ctx)
	if err != nil {
		return nil, err
	}
	if limit > 0 && limit < len(cards) {
		cards = cards[:limit]
	}
	result := make([]*masterdata.Card, len(cards))
	for i, model := range cards {
		result[i] = common.CloneCard(model)
	}
	return result, nil
}

// startAllCardsLoad joins or starts the shared load for generation. It runs
// detached from the caller so a cancelled reader does not fail the others.
func (p *dbCardProvider) startAllCardsLoad(generation uint64, caller *dbBulkIndexFlightToken) <-chan singleflight.Result {
	return p.cardLoads.DoChan(strconv.FormatUint(generation, 10), func() (any, error) {
		return runDBBulkIndexFlight(caller, func(loadCtx context.Context) error {
			p.cardMu.RLock()
			loaded := p.allCards != nil
			current := p.cardGeneration
			p.cardMu.RUnlock()
			if loaded || current != generation {
				return nil
			}
			finish := commandtrace.MeasureOperation(loadCtx, "cards.all_index")
			defer finish()
			// game_id breaks release_at ties so the order is stable across
			// reloads; the filtered SQL path this index replaced left ties
			// to the planner.
			entities, err := p.client.Card.Query().Where(card.ServerRegionEQ(p.region.String())).
				Order(card.ByReleaseAt(), card.ByGameID()).All(loadCtx)
			if err != nil {
				return fmt.Errorf("filter cards: %w", err)
			}
			models := make([]*masterdata.Card, 0, len(entities))
			for _, entity := range entities {
				model, err := common.ConvertCardEntity(entity)
				if err != nil {
					return fmt.Errorf("decode card %d for region %s: %w", entity.GameID, p.region, err)
				}
				// The index lives for the process and every read clones it, so
				// keep only the per-type maximum that card power is built from.
				model.CardParameters = masterdata.StrongestCardParameters(model.CardParameters)
				models = append(models, model)
			}
			p.cardMu.Lock()
			defer p.cardMu.Unlock()
			if p.cardGeneration == generation {
				p.allCards = models
				for _, model := range models {
					p.cardCache[model.ID] = model
				}
			}
			return nil
		}), nil
	})
}
