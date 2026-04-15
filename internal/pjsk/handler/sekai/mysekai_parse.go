package sekai

import (
	"fmt"
	"strconv"
	"strings"
)

var mysekaiMapIndexToID = map[int]int{
	1: 5,
	2: 7,
	3: 6,
	4: 8,
}

func parseMysekaiFixtureIDs(args string) []int {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil
	}
	ids := make([]int, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil || value <= 0 {
			return nil
		}
		ids = append(ids, value)
	}
	return ids
}

func parseMysekaiMapIDs(args string) ([]int, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil, nil
	}
	result := make([]int, 0, len(fields))
	seen := make(map[int]struct{}, len(fields))
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || lower == "all" {
			continue
		}
		if !isASCIIInt(lower) {
			continue
		}

		// Support compact forms like "13" -> [1, 3].
		if len(lower) > 1 {
			splittable := true
			for _, ch := range lower {
				if ch < '1' || ch > '4' {
					splittable = false
					break
				}
			}
			if splittable {
				for _, ch := range lower {
					index := int(ch - '0')
					mapID := mysekaiMapIndexToID[index]
					if _, ok := seen[mapID]; ok {
						continue
					}
					seen[mapID] = struct{}{}
					result = append(result, mapID)
				}
				continue
			}
		}

		index, _ := strconv.Atoi(lower)
		mapID, ok := mysekaiMapIndexToID[index]
		if !ok {
			return nil, fmt.Errorf("地图编号仅支持 1-4（对应地图ID 5-8）")
		}
		if _, ok := seen[mapID]; ok {
			continue
		}
		seen[mapID] = struct{}{}
		result = append(result, mapID)
	}
	return result, nil
}

func isASCIIInt(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func cleanMysekaiArgs(args string) string {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return ""
	}
	unitTokens := map[string]struct{}{
		"ln": {}, "mmj": {}, "vbs": {}, "ws": {}, "wxs": {}, "25": {}, "25h": {}, "25ji": {}, "niigo": {}, "vs": {}, "piapro": {},
	}
	var kept []string
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || lower == "all" || lower == "id" {
			continue
		}
		if _, ok := unitTokens[lower]; ok {
			continue
		}
		kept = append(kept, field)
	}
	return strings.TrimSpace(strings.Join(kept, " "))
}

var mysekaiBlueprintUnitAliases = map[string]string{
	"l/n":                    "light_sound",
	"ln":                     "light_sound",
	"leoneed":                "light_sound",
	"light_sound":            "light_sound",
	"lightsound":             "light_sound",
	"light_sound_club":       "light_sound",
	"leo/need":               "light_sound",
	"mmj":                    "idol",
	"moremorejump":           "idol",
	"more_more_jump":         "idol",
	"idol":                   "idol",
	"vbs":                    "street",
	"vividbadsquad":          "street",
	"vivid_bad_squad":        "street",
	"street":                 "street",
	"ws":                     "theme_park",
	"wxs":                    "theme_park",
	"wonderlands":            "theme_park",
	"wonderlandsxshowtime":   "theme_park",
	"wonderlands_x_showtime": "theme_park",
	"theme_park":             "theme_park",
	"themepark":              "theme_park",
	"25":                     "school_refusal",
	"25h":                    "school_refusal",
	"25ji":                   "school_refusal",
	"niigo":                  "school_refusal",
	"nightcord":              "school_refusal",
	"school_refusal":         "school_refusal",
	"schoolrefusal":          "school_refusal",
	"25_ji_night_cord_de":    "school_refusal",
}

var mysekaiBlueprintDiscardedUnitTokens = map[string]struct{}{
	"vs":            {},
	"piapro":        {},
	"virtualsinger": {},
}

func extractMysekaiGateID(args string) (int, string) {
	lower := strings.ToLower(strings.TrimSpace(args))
	unitMap := map[string]int{
		"light_sound":    1,
		"ln":             1,
		"idol":           2,
		"mmj":            2,
		"street":         3,
		"vbs":            3,
		"theme_park":     4,
		"ws":             4,
		"wxs":            4,
		"school_refusal": 5,
		"25":             5,
		"25h":            5,
		"25ji":           5,
		"niigo":          5,
	}
	for token, gateID := range unitMap {
		if strings.Contains(lower, token) {
			cleaned := strings.TrimSpace(strings.ReplaceAll(lower, token, ""))
			return gateID, cleaned
		}
	}
	return 0, strings.TrimSpace(args)
}

func extractMysekaiAllFlag(args string) (bool, string) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return false, ""
	}

	showAll := false
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "", "all", "全部":
			if strings.TrimSpace(field) != "" {
				showAll = true
			}
			continue
		default:
			remaining = append(remaining, field)
		}
	}
	return showAll, strings.TrimSpace(strings.Join(remaining, " "))
}

func parseMysekaiBlueprintArgs(args string) (string, string, bool) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return "", "", false
	}

	showAllTalks := false
	unit := ""
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		switch lower {
		case "", "id":
			continue
		case "all":
			showAllTalks = true
			continue
		}
		if resolved, ok := mysekaiBlueprintUnitAliases[lower]; ok && unit == "" {
			unit = resolved
			continue
		}
		if _, ok := mysekaiBlueprintDiscardedUnitTokens[lower]; ok {
			continue
		}
		remaining = append(remaining, field)
	}
	return strings.TrimSpace(strings.Join(remaining, " ")), unit, showAllTalks
}

func buildMysekaiTalkQuery(unit, query string) string {
	query = strings.TrimSpace(query)
	unit = strings.TrimSpace(unit)
	if query == "" {
		return ""
	}
	if unit == "" {
		return query
	}
	return unit + " " + query
}
