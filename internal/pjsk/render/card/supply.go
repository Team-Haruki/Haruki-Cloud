package card

import "strings"

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

func formatSupplyTypeForList(raw string) string {
	switch normalizeSupplyType(raw) {
	case "normal", "":
		return ""
	case "term_limited":
		return "期间限定"
	case "colorful_festival_limited":
		return "CFes限定"
	case "bloom_festival_limited":
		return "BFes限定"
	case "unit_event_limited":
		return "WL限定"
	case "collaboration_limited":
		return "联动限定"
	case "birthday":
		return "生日"
	default:
		return strings.TrimSpace(raw)
	}
}

func formatSupplyTypeForDetail(raw string) string {
	switch normalizeSupplyType(raw) {
	case "normal", "":
		return "常驻"
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
