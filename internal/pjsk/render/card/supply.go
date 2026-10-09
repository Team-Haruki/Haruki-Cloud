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

// formatSupplyTypeForList returns the supply label of a limited card. Drawing
// matches these exact values to choose the limited-card icon (its
// TERM_LIMITED_SUPPLY_TYPES and FES_LIMITED_SUPPLY_TYPES sets), so they are a
// cross-repo contract rather than catalog copy until Drawing accepts the raw
// supply key.
func formatSupplyTypeForList(raw string) string {
	switch normalizeSupplyType(raw) {
	case "normal", "":
		return ""
	case "term_limited":
		return "期间限定" //copylint:ignore Drawing matches this value
	case "colorful_festival_limited":
		return "CFes限定" //copylint:ignore Drawing matches this value
	case "bloom_festival_limited":
		return "BFes限定" //copylint:ignore Drawing matches this value
	case "unit_event_limited":
		return "WL限定" //copylint:ignore Drawing matches this value
	case "collaboration_limited":
		return "联动限定" //copylint:ignore Drawing matches this value
	case "birthday":
		return "生日" //copylint:ignore Drawing matches this value
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
