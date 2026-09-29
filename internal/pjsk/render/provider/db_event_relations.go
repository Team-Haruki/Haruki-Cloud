package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"haruki-cloud/database/sekai/eventcard"
	"haruki-cloud/database/sekai/eventdeckbonuse"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbEventProvider) eventCardIDs(ctx context.Context) (map[int][]int64, error) {
	return p.cardLinks.get(ctx, "events.card_links_index", func(loadCtx context.Context) (map[int][]int64, error) {
		links, err := p.client.Eventcard.Query().Where(eventcard.ServerRegionEQ(p.region.String())).Order(eventcard.ByCardID()).All(loadCtx)
		if err != nil {
			return nil, err
		}
		index := make(map[int][]int64)
		for _, link := range links {
			index[int(link.EventID)] = append(index[int(link.EventID)], link.CardID)
		}
		return index, nil
	})
}

func (p *dbEventProvider) eventDeckBonuses(ctx context.Context) (map[int][]*masterdata.EventDeckBonus, error) {
	return p.deckBonuses.get(ctx, "events.deck_bonus_index", func(loadCtx context.Context) (map[int][]*masterdata.EventDeckBonus, error) {
		items, err := p.client.Eventdeckbonuse.Query().Where(eventdeckbonuse.ServerRegionEQ(p.region.String())).All(loadCtx)
		if err != nil {
			return nil, err
		}
		index := make(map[int][]*masterdata.EventDeckBonus)
		for _, item := range items {
			model := &masterdata.EventDeckBonus{
				ID: item.ID, EventID: int(item.EventID), GameCharacterUnitID: int(item.GameCharacterUnitID),
				CardAttr: item.CardAttr, BonusRate: item.BonusRate,
			}
			index[model.EventID] = append(index[model.EventID], model)
		}
		return index, nil
	})
}

func cloneEventDeckBonuses(items []*masterdata.EventDeckBonus) []*masterdata.EventDeckBonus {
	result := make([]*masterdata.EventDeckBonus, 0, len(items))
	for _, item := range items {
		clone := *item
		result = append(result, &clone)
	}
	return result
}

// PreloadList batches the card entities used by both thumbnails and banner
// selection. Failures leave ordinary per-event fallback handling available.
func (p *dbEventProvider) PreloadList(ctx context.Context, eventIDs []int) error {
	if len(eventIDs) == 0 {
		return nil
	}
	p.init()
	links, linkErr := p.eventCardIDs(ctx)
	_, bonusErr := p.eventDeckBonuses(ctx)
	if linkErr != nil {
		return errors.Join(linkErr, bonusErr)
	}
	seen := make(map[int64]struct{})
	for _, id := range eventIDs {
		for _, cardID := range links[id] {
			seen[cardID] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	_, cardErr := p.getCardsByIDs(ctx, ids)
	if cardErr != nil {
		cardErr = fmt.Errorf("preload event cards: %w", cardErr)
	}
	return errors.Join(bonusErr, cardErr)
}
