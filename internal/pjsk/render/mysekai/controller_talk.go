package mysekai

import (
	"fmt"
	"strconv"
	"strings"
)

var mysekaiTalkUnitAliases = map[string]string{
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
	"vs":                     "piapro",
	"piapro":                 "piapro",
	"virtualsinger":          "piapro",
}

var mysekaiFixedVirtualSingerUnits = map[int]string{
	22: "idol",
	23: "street",
	24: "light_sound",
	25: "street",
	26: "theme_park",
}

func (c *Controller) resolveTalkCharacter(query string) (int, int, error) {
	unit, cleanedQuery := extractMysekaiTalkUnit(query)
	cleanedQuery = strings.TrimSpace(cleanedQuery)
	if cleanedQuery == "" {
		return 0, 0, nil
	}

	gameCharacterUnits := c.masterdata.loadList("gameCharacterUnits.json")
	if len(strings.Fields(cleanedQuery)) == 1 {
		if target, err := strconv.Atoi(cleanedQuery); err == nil && target > 0 {
			for _, item := range gameCharacterUnits {
				if intNumber(item["id"], 0) == target {
					return intNumber(item["gameCharacterId"], 0), target, nil
				}
			}
			return c.resolveTalkCharacterUnit(cleanedQuery, unit, target, gameCharacterUnits)
		}
	}

	characterID := c.lookupTalkCharacterID(cleanedQuery)
	if characterID == 0 {
		return 0, 0, fmt.Errorf("找不到要查询的角色")
	}
	return c.resolveTalkCharacterUnit(cleanedQuery, unit, characterID, gameCharacterUnits)
}

func extractMysekaiTalkUnit(query string) (string, string) {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return "", ""
	}

	unit := ""
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		if resolved, ok := mysekaiTalkUnitAliases[strings.ToLower(strings.TrimSpace(field))]; ok && unit == "" {
			unit = resolved
			continue
		}
		remaining = append(remaining, field)
	}
	return unit, strings.TrimSpace(strings.Join(remaining, " "))
}

func (c *Controller) lookupTalkCharacterID(query string) int {
	normalized := normalizeMysekaiTalkCharacterQuery(query)
	if normalized == "" {
		return 0
	}
	if characterID, ok := c.nicknames[normalized]; ok {
		return characterID
	}

	characters := c.masterdata.loadMapByID("gameCharacters.json")
	for characterID, item := range characters {
		candidates := []string{
			stringValue(item["firstName"]),
			stringValue(item["givenName"]),
			strings.TrimSpace(stringValue(item["firstName"]) + stringValue(item["givenName"])),
			strings.TrimSpace(stringValue(item["firstName"]) + " " + stringValue(item["givenName"])),
			stringValue(item["firstNameEnglish"]),
			stringValue(item["givenNameEnglish"]),
			strings.TrimSpace(stringValue(item["firstNameEnglish"]) + stringValue(item["givenNameEnglish"])),
			strings.TrimSpace(stringValue(item["firstNameEnglish"]) + " " + stringValue(item["givenNameEnglish"])),
		}
		for _, candidate := range candidates {
			if normalizeMysekaiTalkCharacterQuery(candidate) == normalized {
				return characterID
			}
		}
	}
	return 0
}

func (c *Controller) resolveTalkCharacterUnit(query, unit string, characterID int, gameCharacterUnits []map[string]any) (int, int, error) {
	candidates := make([]map[string]any, 0, 6)
	for _, item := range gameCharacterUnits {
		if intNumber(item["gameCharacterId"], 0) != characterID {
			continue
		}
		candidates = append(candidates, item)
	}
	if len(candidates) == 0 {
		return 0, 0, fmt.Errorf("找不到要查询的角色")
	}

	candidates = c.filterMysekaiVirtualSingerCandidates(characterID, candidates)
	if fixedUnit, ok := mysekaiFixedVirtualSingerUnits[characterID]; ok {
		if unit != "" && normalizeMysekaiTalkUnit(unit) != fixedUnit {
			return 0, 0, fmt.Errorf("找不到要查询的角色")
		}
		unit = fixedUnit
	}

	if unit != "" {
		normalizedUnit := normalizeMysekaiTalkUnit(unit)
		for _, item := range candidates {
			if normalizeMysekaiTalkUnit(stringValue(item["unit"])) == normalizedUnit {
				return characterID, intNumber(item["id"], 0), nil
			}
		}
		return 0, 0, fmt.Errorf("找不到要查询的角色")
	}

	if len(candidates) == 1 {
		return characterID, intNumber(candidates[0]["id"], 0), nil
	}
	if characterID == 21 {
		return 0, 0, fmt.Errorf("查询存在多个组合的V家角色时需要同时指定组合，例如\"%s ln\"", strings.TrimSpace(query))
	}
	return characterID, intNumber(candidates[0]["id"], 0), nil
}

func (c *Controller) filterMysekaiVirtualSingerCandidates(characterID int, candidates []map[string]any) []map[string]any {
	if characterID < 21 || characterID > 26 || len(candidates) == 0 {
		return candidates
	}
	available := c.availableMysekaiCharacterUnitIDs()
	if len(available) == 0 {
		return candidates
	}
	filtered := make([]map[string]any, 0, len(candidates))
	for _, item := range candidates {
		if _, ok := available[intNumber(item["id"], 0)]; ok {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return candidates
	}
	return filtered
}

func (c *Controller) availableMysekaiCharacterUnitIDs() map[int]struct{} {
	items := c.masterdata.loadList("mysekaiGateCharacterLotteries.json")
	if len(items) == 0 {
		return nil
	}
	result := make(map[int]struct{}, len(items))
	for _, item := range items {
		unitID := intNumber(item["gameCharacterUnitId"], 0)
		if unitID == 0 {
			unitID = intNumber(item["game_character_unit_id"], 0)
		}
		if unitID == 0 {
			continue
		}
		result[unitID] = struct{}{}
	}
	return result
}

func normalizeMysekaiTalkUnit(unit string) string {
	if resolved, ok := mysekaiTalkUnitAliases[strings.ToLower(strings.TrimSpace(unit))]; ok {
		return resolved
	}
	return strings.ToLower(strings.TrimSpace(unit))
}

func normalizeMysekaiTalkCharacterQuery(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), ""))
}
