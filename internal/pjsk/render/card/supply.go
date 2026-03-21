package card

func formatSupplyType(raw string) string {
	switch raw {
	case "", "normal":
		return "常驻"
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
	case "birthday", "rarity_birthday":
		return "生日"
	default:
		return raw
	}
}

func matchesSupplyFilter(filter, localized string) bool {
	switch filter {
	case SupplyFes:
		return localized == formatSupplyType("colorful_festival_limited") || localized == formatSupplyType("bloom_festival_limited")
	case SupplyLimited:
		return localized != formatSupplyType("") && localized != formatSupplyType("birthday")
	case SupplyNormal:
		return localized == formatSupplyType("")
	case SupplyBirthday:
		return localized == formatSupplyType("birthday")
	default:
		return false
	}
}
