package deck

import (
	"fmt"
	"regexp"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
)

var deckServiceEventNotFoundRegex = regexp.MustCompile(`(?i)event\s+not\s+found\s+for\s+eventid:\s*(\d+)`)

func (c *Controller) applyWorldBloomSimulationFallbackIfMasterdataMissing(region renderregion.Value, recType string, query AutoQuery, option map[string]any) (AutoQuery, map[string]any) {
	eventID := optionInt(option, "event_id")
	if eventID <= 0 {
		return query, option
	}
	exists, checked := deckMasterdataContainsEvent(c.recommendCfg.MasterdataDir, region.String(), eventID)
	if !checked || exists {
		return query, option
	}
	fallbackQuery, fallbackOption, ok := buildWorldBloomSimulationFallback(query, option, recType)
	if !ok {
		return query, option
	}
	return fallbackQuery, fallbackOption
}

func buildWorldBloomSimulationFallbackOnError(query AutoQuery, option map[string]any, recType string, err error) (AutoQuery, map[string]any, bool) {
	if err == nil {
		return AutoQuery{}, nil, false
	}
	if eventID := optionInt(option, "event_id"); eventID <= 0 || !isDeckServiceEventNotFoundForID(err, eventID) {
		return AutoQuery{}, nil, false
	}
	return buildWorldBloomSimulationFallback(query, option, recType)
}

func buildWorldBloomSimulationFallback(query AutoQuery, option map[string]any, recType string) (AutoQuery, map[string]any, bool) {
	switch strings.ToLower(strings.TrimSpace(recType)) {
	case "event":
	default:
		return AutoQuery{}, nil, false
	}

	eventID := optionInt(option, "event_id")
	if eventID <= 0 {
		return AutoQuery{}, nil, false
	}
	turn := worldBloomSimulationTurn(query)
	if turn <= 0 {
		return AutoQuery{}, nil, false
	}
	charID := worldBloomSimulationCharacterID(query, option)
	if charID <= 0 {
		return AutoQuery{}, nil, false
	}
	unit := normalizeRecommendUnit(query.EventUnit)
	if unit == "" {
		unit = normalizeRecommendUnit(optionString(option, "event_unit"))
	}
	if turn <= 2 && unit == "" {
		return AutoQuery{}, nil, false
	}

	fallbackQuery := query
	fallbackQuery.EventID = nil
	fallbackQuery.WorldBloomEventTurn = deckIntPtr(turn)
	fallbackQuery.WorldBloomCharacterID = deckIntPtr(charID)
	if unit != "" {
		fallbackQuery.EventUnit = unit
	}

	fallbackOption := cloneRecommendOption(option)
	fallbackOption["event_id"] = nil
	fallbackOption["event_type"] = "world_bloom"
	fallbackOption["world_bloom_event_turn"] = turn
	fallbackOption["world_bloom_character_id"] = charID
	if unit != "" {
		fallbackOption["event_unit"] = unit
	}
	delete(fallbackOption, "event_attr")
	return fallbackQuery, fallbackOption, true
}

func worldBloomSimulationTurn(query AutoQuery) int {
	if query.MetadataWorldBloomEventTurn != nil && *query.MetadataWorldBloomEventTurn > 0 {
		return *query.MetadataWorldBloomEventTurn
	}
	if query.WorldBloomEventTurn != nil && *query.WorldBloomEventTurn > 0 {
		return *query.WorldBloomEventTurn
	}
	return 0
}

func worldBloomSimulationCharacterID(query AutoQuery, option map[string]any) int {
	if query.WorldBloomCharacterID != nil && *query.WorldBloomCharacterID > 0 {
		return *query.WorldBloomCharacterID
	}
	if query.MetadataWorldBloomCharacterID != nil && *query.MetadataWorldBloomCharacterID > 0 {
		return *query.MetadataWorldBloomCharacterID
	}
	return optionInt(option, "world_bloom_character_id")
}

func isDeckServiceEventNotFoundForID(err error, eventID int) bool {
	if err == nil || eventID <= 0 {
		return false
	}
	matches := deckServiceEventNotFoundRegex.FindStringSubmatch(err.Error())
	if len(matches) != 2 {
		return false
	}
	return strings.TrimSpace(matches[1]) == fmt.Sprintf("%d", eventID)
}

func deckIntPtr(value int) *int {
	return &value
}
