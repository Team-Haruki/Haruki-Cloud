package assets

func UnitIconFilename(unit string) string {
	switch unit {
	case "light_sound", "light_sound_club":
		return "icon_light_sound"
	case "idol", "more_more_jump":
		return "icon_idol"
	case "street", "vivid_bad_squad":
		return "icon_street"
	case "theme_park", "wonderlands_x_showtime":
		return "icon_theme_park"
	case "school_refusal", "25_ji_night_cord_de":
		return "icon_school_refusal"
	case "piapro":
		return "icon_piapro"
	default:
		return ""
	}
}
