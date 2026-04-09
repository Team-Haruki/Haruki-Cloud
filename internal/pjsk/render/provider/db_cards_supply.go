package provider

import (
	"context"
	"strings"

	"haruki-cloud/database/sekai/cardsupplie"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (p *dbCardProvider) GetSupplyType(cardInfo *masterdata.Card) string {
	return p.getSupplyType(nil, cardInfo)
}

func (p *dbCardProvider) getSupplyType(ctx context.Context, cardInfo *masterdata.Card) string {
	if cardInfo == nil {
		return cardNormalizeSupplyType("")
	}
	if cardInfo.CardRarityType == "rarity_birthday" {
		return cardNormalizeSupplyType("birthday")
	}
	if cardInfo.CardSupplyID == 0 {
		return cardNormalizeSupplyType("")
	}
	ctx = cardContextOrBackground(ctx)

	p.supplyMu.RLock()
	if cached, ok := p.supplyByID[cardInfo.CardSupplyID]; ok {
		p.supplyMu.RUnlock()
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
	return value
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
