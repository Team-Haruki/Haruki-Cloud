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
	if value, ok := cardSupplyTypeOverride(cardInfo.ID); ok {
		return value
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
	if p.worldLink3CardsFresh() {
		return true
	}

	callerToken := new(dbBulkIndexFlightToken)
	result := p.worldLink3Loads.DoChan(p.region.String(), func() (any, error) {
		completed := runDBBulkIndexFlight(callerToken, p.refreshWorldLink3Cards)
		return completed, nil
	})
	return waitDBBulkIndexFlight(ctx, result, callerToken, "cards.supply_index_wait", "cards.supply_index_shared") == nil
}

func (p *dbCardProvider) worldLink3CardsFresh() bool {
	p.worldLink3Mu.RLock()
	defer p.worldLink3Mu.RUnlock()
	return dbBulkIndexFresh(p.worldLink3Loaded, p.worldLink3LoadedAt)
}

func (p *dbCardProvider) refreshWorldLink3Cards(ctx context.Context) error {
	finishIndex := commandtrace.MeasureOperation(ctx, "cards.supply_index")
	defer finishIndex()
	if p.worldLink3CardsFresh() {
		return nil
	}
	eventIDs, err := p.loadWorldLinkEventIDs(ctx)
	if err != nil {
		return err
	}
	cards, err := p.loadWorldLinkCardIDs(ctx, eventIDs)
	if err != nil {
		return err
	}
	p.worldLink3Mu.Lock()
	p.worldLink3ByCard = cards
	p.worldLink3Loaded = true
	p.worldLink3LoadedAt = time.Now()
	p.worldLink3Mu.Unlock()
	return nil
}

func (p *dbCardProvider) loadWorldLinkEventIDs(ctx context.Context) ([]int64, error) {
	events, err := p.client.Event.Query().
		Where(
			event.ServerRegionEQ(p.region.String()),
			event.EventTypeEQ("world_bloom"),
			event.UnitEQ("none"),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	eventIDs := make([]int64, 0, len(events))
	for _, item := range events {
		eventIDs = append(eventIDs, item.GameID)
	}
	return eventIDs, nil
}

func (p *dbCardProvider) loadWorldLinkCardIDs(ctx context.Context, eventIDs []int64) (map[int]bool, error) {
	cards := make(map[int]bool)
	if len(eventIDs) == 0 {
		return cards, nil
	}
	links, err := p.client.Eventcard.Query().
		Where(
			eventcard.ServerRegionEQ(p.region.String()),
			eventcard.EventIDIn(eventIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		cards[int(link.CardID)] = true
	}
	return cards, nil
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

func cardSupplyTypeOverride(cardID int) (string, bool) {
	switch cardID {
	case 1345, 1346, 1347:
		return "term_limited", true
	default:
		return "", false
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
