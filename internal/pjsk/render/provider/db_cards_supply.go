package provider

import (
	"context"
	"strings"
	"time"

	"haruki-cloud/database/sekai/cardsupplie"
	"haruki-cloud/database/sekai/event"
	"haruki-cloud/database/sekai/eventcard"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbCardProvider) GetSupplyType(ctx context.Context, cardInfo *masterdata.Card) string {
	p.init()
	if cardInfo == nil {
		return cardNormalizeSupplyType("")
	}
	if cardInfo.CardRarityType == "rarity_birthday" {
		return cardNormalizeSupplyType("birthday")
	}
	if cardInfo.CardSupplyID == 0 {
		return cardNormalizeSupplyType("")
	}

	p.supplyMu.RLock()
	if cached, ok := p.supplyByID[cardInfo.CardSupplyID]; ok {
		p.supplyMu.RUnlock()
		if cached == "term_limited" && p.isWorldLink3Card(ctx, cardInfo.ID) {
			return cardNormalizeSupplyType("unit_event_limited")
		}
		return cached
	}
	p.supplyMu.RUnlock()

	entity, err := p.client.Cardsupplie.Query().
		Where(cardsupplie.ServerRegionEQ(p.region.String()), cardsupplie.GameIDEQ(int64(cardInfo.CardSupplyID))).
		Only(ctx)
	if err != nil {
		return cardNormalizeSupplyType("")
	}

	value := cardNormalizeSupplyType(entity.CardSupplyType)
	p.supplyMu.Lock()
	p.supplyByID[cardInfo.CardSupplyID] = value
	p.supplyMu.Unlock()
	if value == "term_limited" && p.isWorldLink3Card(ctx, cardInfo.ID) {
		return cardNormalizeSupplyType("unit_event_limited")
	}
	return value
}

func (p *dbCardProvider) isWorldLink3Card(ctx context.Context, cardID int) bool {
	if cardID == 0 {
		return false
	}
	p.init()

	if !p.loadWorldLink3Cards(ctx) {
		return false
	}
	p.worldLink3Mu.RLock()
	isWorldLink3 := p.worldLink3ByCard[cardID]
	p.worldLink3Mu.RUnlock()
	return isWorldLink3
}

func (p *dbCardProvider) loadWorldLink3Cards(ctx context.Context) bool {
	p.init()
	p.worldLink3Mu.RLock()
	loaded := dbBulkIndexFresh(p.worldLink3Loaded, p.worldLink3LoadedAt)
	p.worldLink3Mu.RUnlock()
	if loaded {
		return true
	}

	callerToken := new(dbBulkIndexFlightToken)
	result := p.worldLink3Loads.DoChan(p.region.String(), func() (any, error) {
		completed := runDBBulkIndexFlight(callerToken, func(loadCtx context.Context) error {
			finishIndex := commandtrace.MeasureOperation(loadCtx, "cards.supply_index")
			defer finishIndex()
			p.worldLink3Mu.RLock()
			loaded := dbBulkIndexFresh(p.worldLink3Loaded, p.worldLink3LoadedAt)
			p.worldLink3Mu.RUnlock()
			if loaded {
				return nil
			}

			events, err := p.client.Event.Query().
				Where(
					event.ServerRegionEQ(p.region.String()),
					event.EventTypeEQ("world_bloom"),
					event.UnitEQ("none"),
				).
				All(loadCtx)
			if err != nil {
				return err
			}
			eventIDs := make([]int64, 0, len(events))
			for _, item := range events {
				eventIDs = append(eventIDs, item.GameID)
			}

			cards := make(map[int]bool)
			if len(eventIDs) > 0 {
				links, err := p.client.Eventcard.Query().
					Where(
						eventcard.ServerRegionEQ(p.region.String()),
						eventcard.EventIDIn(eventIDs...),
					).
					All(loadCtx)
				if err != nil {
					return err
				}
				for _, link := range links {
					cards[int(link.CardID)] = true
				}
			}

			p.worldLink3Mu.Lock()
			p.worldLink3ByCard = cards
			p.worldLink3Loaded = true
			p.worldLink3LoadedAt = time.Now()
			p.worldLink3Mu.Unlock()
			return nil
		})
		return completed, nil
	})
	return waitDBBulkIndexFlight(ctx, result, callerToken, "cards.supply_index_wait", "cards.supply_index_shared") == nil
}

func cardNormalizeSupportUnit(raw string) string {
	value := cardNormalizeUnit(raw)
	if value == "" {
		return "none"
	}
	return value
}

func cardNormalizeUnit(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "light_sound", "light_sound_club":
		return "light_sound"
	case "idol", "more_more_jump":
		return "idol"
	case "street", "vivid_bad_squad":
		return "street"
	case "theme_park", "wonderlands_x_showtime":
		return "theme_park"
	case "school_refusal", "25_ji_night_cord_de":
		return "school_refusal"
	case "piapro":
		return "piapro"
	case "", "none":
		return ""
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func cardNormalizeSupplyType(raw string) string {
	switch strings.TrimSpace(raw) {
	case "", "normal", "not_limited":
		return "normal"
	case "term_limited":
		return "term_limited"
	case "festival_limited", "colorful_festival_limited":
		return "colorful_festival_limited"
	case "bloom_festival_limited":
		return "bloom_festival_limited"
	case "unit_event_limited":
		return "unit_event_limited"
	case "collaboration_limited":
		return "collaboration_limited"
	case "birthday", "rarity_birthday":
		return "birthday"
	default:
		return strings.TrimSpace(raw)
	}
}

func isWorldLink3Event(eventInfo *masterdata.Event) bool {
	if eventInfo == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(eventInfo.EventType), "world_bloom") &&
		strings.EqualFold(strings.TrimSpace(eventInfo.Unit), "none")
}

func cardMatchesSupplyFilter(filter, raw string) bool {
	switch cardNormalizeSupplyType(raw) {
	case "colorful_festival_limited", "bloom_festival_limited":
		if filter == "festival" || filter == "limited" {
			return true
		}
		return cardNormalizeSupplyType(filter) == cardNormalizeSupplyType(raw)
	case "term_limited", "unit_event_limited", "collaboration_limited":
		if filter == "limited" {
			return true
		}
		return cardNormalizeSupplyType(filter) == cardNormalizeSupplyType(raw)
	case "birthday":
		return filter == "birthday"
	case "normal":
		return filter == "normal"
	default:
		return false
	}
}
