package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

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

// getAllCards serves the region's card index. After the TTL the previous
// index keeps being served while one shared flight reloads it (a load is
// 0.5-2 s for JP); only the first load and a reset make readers wait.
func (p *dbCardProvider) getAllCards(ctx context.Context, limit int) ([]*masterdata.Card, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.cardMu.RLock()
		generation := p.cardGeneration
		cards := p.allCards
		fresh := dbBulkIndexFresh(cards != nil, p.allCardsLoadedAt)
		p.cardMu.RUnlock()
		if !fresh {
			caller := new(dbBulkIndexFlightToken)
			result := p.startAllCardsLoad(generation, caller)
			if cards == nil {
				if err := waitDBBulkIndexFlight(ctx, result, caller, "cards.all_index_wait", "cards.all_index_shared"); err != nil {
					return nil, err
				}
				// A reset during loading discards that generation; retry the current one.
				continue
			}
			commandtrace.RecordOperation(ctx, "cards.all_index_stale", 0)
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
}

// startAllCardsLoad joins or starts the shared load for generation. It runs
// detached from the caller, so a reader holding a stale index need not wait.
func (p *dbCardProvider) startAllCardsLoad(generation uint64, caller *dbBulkIndexFlightToken) <-chan singleflight.Result {
	return p.cardLoads.DoChan(strconv.FormatUint(generation, 10), func() (any, error) {
		return runDBBulkIndexFlight(caller, func(loadCtx context.Context) error {
			p.cardMu.RLock()
			fresh := dbBulkIndexFresh(p.allCards != nil, p.allCardsLoadedAt)
			current := p.cardGeneration
			p.cardMu.RUnlock()
			if fresh || current != generation {
				return nil
			}
			finish := commandtrace.MeasureOperation(loadCtx, "cards.all_index")
			defer finish()
			entities, err := p.client.Card.Query().Where(card.ServerRegionEQ(p.region.String())).
				Order(card.ByReleaseAt()).All(loadCtx)
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
				now := time.Now()
				p.allCards, p.allCardsLoadedAt = models, now
				for _, model := range models {
					p.cardCache[model.ID] = model
					p.cardCachedAt[model.ID] = now
				}
			}
			return nil
		}), nil
	})
}
