package mysekai

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/filteralias"
)

var mysekaiTalkUnitAliases = filteralias.UnitMap()

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
				if intNumberFrom(item, 0, "id", "game_id") == target {
					return intNumberFrom(item, 0, "gameCharacterId", "game_character_id"), target, nil
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
			stringValueFrom(item, "firstName", "first_name"),
			stringValueFrom(item, "givenName", "given_name"),
			strings.TrimSpace(stringValueFrom(item, "firstName", "first_name") + stringValueFrom(item, "givenName", "given_name")),
			strings.TrimSpace(stringValueFrom(item, "firstName", "first_name") + " " + stringValueFrom(item, "givenName", "given_name")),
			stringValueFrom(item, "firstNameEnglish", "first_name_english"),
			stringValueFrom(item, "givenNameEnglish", "given_name_english"),
			strings.TrimSpace(stringValueFrom(item, "firstNameEnglish", "first_name_english") + stringValueFrom(item, "givenNameEnglish", "given_name_english")),
			strings.TrimSpace(stringValueFrom(item, "firstNameEnglish", "first_name_english") + " " + stringValueFrom(item, "givenNameEnglish", "given_name_english")),
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
		if intNumberFrom(item, 0, "gameCharacterId", "game_character_id") != characterID {
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
			if normalizeMysekaiTalkUnit(stringValueFrom(item, "unit")) == normalizedUnit {
				return characterID, intNumberFrom(item, 0, "id", "game_id"), nil
			}
		}
		return 0, 0, fmt.Errorf("找不到要查询的角色")
	}

	if len(candidates) == 1 {
		return characterID, intNumberFrom(candidates[0], 0, "id", "game_id"), nil
	}
	if characterID == 21 {
		return 0, 0, fmt.Errorf("查询存在多个组合的V家角色时需要同时指定组合，例如\"%s ln\"", strings.TrimSpace(query))
	}
	return characterID, intNumberFrom(candidates[0], 0, "id", "game_id"), nil
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
		if _, ok := available[intNumberFrom(item, 0, "id", "game_id")]; ok {
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
		unitID := intNumberFrom(item, 0, "gameCharacterUnitId", "game_character_unit_id")
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
