package card

import (
	"strings"

	"haruki-cloud/internal/i18n"
)

func normalizeSupplyType(raw string) string {
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

// supplyTypeKey is the raw supply key sent to Drawing as supply_type_key.
// Drawing chooses the limited icon and background from it; the label from
// formatSupplyTypeForList / formatSupplyTypeForDetail is display text only.
func supplyTypeKey(raw string) string {
	return normalizeSupplyType(raw)
}

// formatSupplyTypeForList returns the supply label of a limited card, or ""
// for a permanent one.
func formatSupplyTypeForList(raw string) string {
	switch normalizeSupplyType(raw) {
	case "normal", "":
		return ""
	case "term_limited":
		return i18n.T("render_card.supply.term_limited")
	case "colorful_festival_limited":
		return i18n.T("render_card.supply.colorful_festival_limited")
	case "bloom_festival_limited":
		return i18n.T("render_card.supply.bloom_festival_limited")
	case "unit_event_limited":
		return i18n.T("render_card.supply.unit_event_limited")
	case "collaboration_limited":
		return i18n.T("render_card.supply.collaboration_limited")
	case "birthday":
		return i18n.T("render_card.supply.birthday")
	default:
		return strings.TrimSpace(raw)
	}
}

func formatSupplyTypeForDetail(raw string) string {
	switch normalizeSupplyType(raw) {
	case "normal", "":
		return i18n.T("render_card.supply.permanent")
	default:
		return formatSupplyTypeForList(raw)
	}
}

func matchesRawSupplyFilter(filter, raw string) bool {
	switch normalizeSupplyType(raw) {
	case "colorful_festival_limited", "bloom_festival_limited":
		if filter == SupplyFes || filter == SupplyLimited {
			return true
		}
		return normalizeSupplyType(filter) == normalizeSupplyType(raw)
	case "term_limited", "unit_event_limited", "collaboration_limited":
		if filter == SupplyLimited {
			return true
		}
		return normalizeSupplyType(filter) == normalizeSupplyType(raw)
	case "birthday":
		return filter == SupplyBirthday
	case "normal":
		return filter == SupplyNormal
	default:
		return false
	}
}
