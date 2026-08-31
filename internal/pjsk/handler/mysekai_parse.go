package handler

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/filteralias"
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
		mapIDs, err := parseMysekaiMapToken(field)
		if err != nil {
			return nil, err
		}
		result = appendUniqueMysekaiMapIDs(result, seen, mapIDs)
	}
	return result, nil
}

func parseMysekaiMapToken(field string) ([]int, error) {
	token := strings.ToLower(strings.TrimSpace(field))
	if token == "" || token == "all" || !isASCIIInt(token) {
		return nil, nil
	}
	if compact, ok := parseCompactMysekaiMapToken(token); ok {
		return compact, nil
	}
	index, _ := strconv.Atoi(token)
	mapID, ok := mysekaiMapIndexToID[index]
	if !ok {
		return nil, fmt.Errorf("地图编号仅支持 1-4（对应地图ID 5-8）")
	}
	return []int{mapID}, nil
}

func parseCompactMysekaiMapToken(token string) ([]int, bool) {
	if len(token) <= 1 {
		return nil, false
	}
	for _, ch := range token {
		if ch < '1' || ch > '4' {
			return nil, false
		}
	}
	mapIDs := make([]int, 0, len(token))
	for _, ch := range token {
		mapIDs = append(mapIDs, mysekaiMapIndexToID[int(ch-'0')])
	}
	return mapIDs, true
}

func appendUniqueMysekaiMapIDs(result []int, seen map[int]struct{}, mapIDs []int) []int {
	for _, mapID := range mapIDs {
		if _, ok := seen[mapID]; ok {
			continue
		}
		seen[mapID] = struct{}{}
		result = append(result, mapID)
	}
	return result
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
	unitTokens := filteralias.UnitAliasSet()
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

func extractMysekaiFullFlag(args string) (bool, string) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return false, ""
	}

	full := false
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "", "all", "full", "全部":
			if strings.TrimSpace(field) != "" {
				full = true
			}
			continue
		default:
			remaining = append(remaining, field)
		}
	}
	return full, strings.TrimSpace(strings.Join(remaining, " "))
}

func extractMysekaiFullOnlyFlag(args string) (bool, string) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return false, ""
	}

	full := false
	remaining := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "", "full":
			if strings.TrimSpace(field) != "" {
				full = true
			}
			continue
		default:
			remaining = append(remaining, field)
		}
	}
	return full, strings.TrimSpace(strings.Join(remaining, " "))
}

var mysekaiBlueprintUnitAliases = filteralias.UnitMapWithout("piapro")

var mysekaiBlueprintDiscardedUnitTokens = filteralias.UnitAliasSetFor("piapro")

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
