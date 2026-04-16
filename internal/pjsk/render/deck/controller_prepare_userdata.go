package deck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/logger"
)

var deckPrepareDebugLogger = logger.NewLoggerFromGlobal("DeckPrepare")

func (c *Controller) prepareRecommendUserData(region renderregion.Value, recType string, query AutoQuery, option map[string]any) (*userdata.RawUserData, []byte, error) {
	original := c.snapshot.RawData()
	if original == nil {
		return nil, nil, fmt.Errorf("raw user snapshot is unavailable")
	}
	originalBytes, err := c.snapshot.RawBytes()
	if err != nil {
		return nil, nil, err
	}

	if !shouldPrepareRecommendUserData(query) {
		return original, originalBytes, nil
	}

	raw, err := userdata.CloneRawUserData(original)
	if err != nil {
		return nil, nil, err
	}
	if raw == nil {
		return nil, nil, fmt.Errorf("raw user snapshot is unavailable")
	}

	if err := c.applyProfilePreset(region, raw, query); err != nil {
		return nil, nil, err
	}
	if err := c.applyUserCardFilters(region, raw, query); err != nil {
		return nil, nil, err
	}
	if err := c.applyCurrentDeckOption(raw, original, recType, query, option); err != nil {
		return nil, nil, err
	}
	if err := c.restoreFixedCards(raw, original, option, query.UseCurrentDeck); err != nil {
		return nil, nil, err
	}
	if err := c.applyAreaItemLevel(region, raw, optionInt(option, "area_item_level")); err != nil {
		return nil, nil, err
	}

	userBytes, err := encodePreparedRecommendUserData(originalBytes, original, raw)
	if err != nil {
		return nil, nil, err
	}
	logPreparedRecommendUserData(region, recType, query, raw, originalBytes, userBytes)
	return raw, userBytes, nil
}

func encodePreparedRecommendUserData(originalBytes []byte, original, raw *userdata.RawUserData) ([]byte, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw user snapshot is unavailable")
	}
	if len(bytes.TrimSpace(originalBytes)) == 0 {
		return userdata.EncodeRawUserData(raw)
	}

	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(originalBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode original user snapshot: %w", err)
	}
	if payload == nil {
		return userdata.EncodeRawUserData(raw)
	}

	var originalCards []userdata.RawUserCard
	if original != nil {
		originalCards = original.UserCards
	}
	userCards, err := mergePreparedUserCards(payload["userCards"], originalCards, raw.UserCards)
	if err != nil {
		return nil, err
	}
	userAreas, err := mergePreparedUserAreas(payload["userAreas"], raw.UserAreas)
	if err != nil {
		return nil, err
	}

	payload["userCards"] = userCards
	payload["userAreas"] = userAreas

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode prepared user snapshot: %w", err)
	}
	return encoded, nil
}

func mergePreparedUserCards(original any, originalCards, cards []userdata.RawUserCard) ([]any, error) {
	if len(cards) == 0 {
		return []any{}, nil
	}

	originalItems := jsonArrayToObjects(original)
	originalByCardID := make(map[int]map[string]any, len(originalItems))
	for _, item := range originalItems {
		cardID, _ := jsonNumberToInt(item["cardId"])
		if cardID > 0 {
			originalByCardID[cardID] = item
		}
	}

	merged := make([]any, 0, len(cards))
	for _, card := range cards {
		if existing := originalByCardID[card.CardID]; existing != nil {
			if samePreparedUserCard(userdata.FindUserCard(originalCards, card.CardID), &card) {
				cardMap := copyJSONObject(existing)
				normalizePreparedUserCardJSON(cardMap)
				merged = append(merged, cardMap)
				continue
			}
		}

		cardMap, err := structToJSONObject(card)
		if err != nil {
			return nil, fmt.Errorf("encode prepared user card %d: %w", card.CardID, err)
		}
		if existing := originalByCardID[card.CardID]; existing != nil {
			cardMap = mergeJSONObjects(existing, cardMap)
		}
		normalizePreparedUserCardJSON(cardMap)
		merged = append(merged, cardMap)
	}
	return merged, nil
}

func mergePreparedUserAreas(original any, areas []userdata.RawUserArea) ([]any, error) {
	if len(areas) == 0 {
		return []any{}, nil
	}

	originalAreas := jsonArrayToObjects(original)
	merged := make([]any, 0, len(areas))
	for i := range areas {
		areaMap, err := structToJSONObject(areas[i])
		if err != nil {
			return nil, fmt.Errorf("encode prepared user area %d: %w", i, err)
		}
		if existing := objectAt(originalAreas, i); existing != nil {
			areaMap = mergeJSONObjects(existing, areaMap)
			areaItems, err := mergePreparedAreaItems(existing["areaItems"], areas[i].AreaItems)
			if err != nil {
				return nil, err
			}
			areaMap["areaItems"] = areaItems
		} else {
			areaItems, err := mergePreparedAreaItems(nil, areas[i].AreaItems)
			if err != nil {
				return nil, err
			}
			areaMap["areaItems"] = areaItems
		}
		normalizePreparedUserAreaJSON(areaMap)
		merged = append(merged, areaMap)
	}
	return merged, nil
}

func mergePreparedAreaItems(original any, items []userdata.RawUserAreaItem) ([]any, error) {
	if len(items) == 0 {
		return []any{}, nil
	}

	originalItems := jsonArrayToObjects(original)
	originalByItemID := make(map[int]map[string]any, len(originalItems))
	for _, item := range originalItems {
		itemID, _ := jsonNumberToInt(item["areaItemId"])
		if itemID > 0 {
			originalByItemID[itemID] = item
		}
	}

	merged := make([]any, 0, len(items))
	for _, item := range items {
		itemMap, err := structToJSONObject(item)
		if err != nil {
			return nil, fmt.Errorf("encode prepared area item %d: %w", item.AreaItemID, err)
		}
		if existing := originalByItemID[item.AreaItemID]; existing != nil {
			itemMap = mergeJSONObjects(existing, itemMap)
		}
		merged = append(merged, itemMap)
	}
	return merged, nil
}

func structToJSONObject(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return map[string]any{}, nil
	}
	return object, nil
}

func jsonArrayToObjects(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, _ := item.(map[string]any)
		if object != nil {
			result = append(result, object)
		}
	}
	return result
}

func objectAt(items []map[string]any, index int) map[string]any {
	if index < 0 || index >= len(items) {
		return nil
	}
	return items[index]
}

func mergeJSONObjects(base, overlay map[string]any) map[string]any {
	if len(base) == 0 {
		return copyJSONObject(overlay)
	}
	merged := copyJSONObject(base)
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func copyJSONObject(source map[string]any) map[string]any {
	if len(source) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func jsonNumberToInt(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	default:
		return 0, false
	}
}

func samePreparedUserCard(original, current *userdata.RawUserCard) bool {
	if original == nil || current == nil {
		return false
	}
	if original.CardID != current.CardID {
		return false
	}
	if original.Level != current.Level ||
		original.SkillLevel != current.SkillLevel ||
		original.MasterRank != current.MasterRank ||
		original.SpecialTrainingStatus != current.SpecialTrainingStatus ||
		original.DefaultImage != current.DefaultImage {
		return false
	}
	if len(original.Episodes) != len(current.Episodes) {
		return false
	}
	for i := range original.Episodes {
		if original.Episodes[i] != current.Episodes[i] {
			return false
		}
	}
	return true
}

func normalizePreparedUserCardJSON(card map[string]any) {
	if card == nil {
		return
	}
	if value, ok := card["episodes"]; ok && value == nil {
		delete(card, "episodes")
	}
}

func normalizePreparedUserAreaJSON(area map[string]any) {
	if area == nil {
		return
	}
	if value, ok := area["userAreaStatus"]; !ok || value == nil {
		area["userAreaStatus"] = map[string]any{}
	}
}

func logPreparedRecommendUserData(region renderregion.Value, recType string, query AutoQuery, raw *userdata.RawUserData, originalBytes, preparedBytes []byte) {
	if raw == nil {
		return
	}

	removedKeys, nullPaths, nestedCardRemoved, nestedAreaRemoved, summaryErr := summarizePreparedRecommendUserData(originalBytes, preparedBytes)
	areaItemLevel := 0
	if query.AreaItemLevel != nil {
		areaItemLevel = *query.AreaItemLevel
	}

	deckPrepareDebugLogger.Debugf(
		"prepared userdata summary: user_id=%d nickname=%q region=%s recommend_type=%s unit_filter=%q attr_filter=%q excluded_cards=%d use_current_deck=%t max_profile=%t sub_max_profile=%t area_item_level=%d original_bytes=%d prepared_bytes=%d user_cards=%d user_areas=%d removed_keys=%v null_paths=%v removed_userCard_keys=%v removed_areaItem_keys=%v summary_err=%v",
		raw.UserGamedata.UserID,
		strings.TrimSpace(raw.UserGamedata.Name),
		region.String(),
		strings.TrimSpace(recType),
		strings.TrimSpace(query.UnitFilter),
		strings.TrimSpace(query.AttrFilter),
		len(query.ExcludedCards),
		query.UseCurrentDeck,
		query.MaxProfile,
		query.SubMaxProfile,
		areaItemLevel,
		len(originalBytes),
		len(preparedBytes),
		len(raw.UserCards),
		len(raw.UserAreas),
		removedKeys,
		nullPaths,
		nestedCardRemoved,
		nestedAreaRemoved,
		summaryErr,
	)
}

func summarizePreparedRecommendUserData(originalBytes, preparedBytes []byte) ([]string, []string, []string, []string, error) {
	var original map[string]any
	var prepared map[string]any
	if err := json.Unmarshal(originalBytes, &original); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("decode original prepared-userdata summary: %w", err)
	}
	if err := json.Unmarshal(preparedBytes, &prepared); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("decode prepared-userdata summary: %w", err)
	}

	removed := make([]string, 0)
	for key := range original {
		if _, ok := prepared[key]; !ok {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)

	nullPaths := make([]string, 0)
	collectNullJSONPaths(prepared, "", &nullPaths, 24)
	cardRemoved := removedNestedKeys(firstObjectFromArray(original, "userCards"), firstObjectFromArray(prepared, "userCards"))
	areaRemoved := removedNestedKeys(firstAreaItemObject(original), firstAreaItemObject(prepared))
	return removed, nullPaths, cardRemoved, areaRemoved, nil
}

func collectNullJSONPaths(value any, prefix string, out *[]string, limit int) {
	if len(*out) >= limit {
		return
	}
	switch typed := value.(type) {
	case nil:
		if prefix == "" {
			prefix = "$"
		}
		*out = append(*out, prefix)
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			collectNullJSONPaths(typed[key], next, out, limit)
			if len(*out) >= limit {
				return
			}
		}
	case []any:
		for idx, item := range typed {
			next := fmt.Sprintf("[%d]", idx)
			if prefix != "" {
				next = fmt.Sprintf("%s[%d]", prefix, idx)
			}
			collectNullJSONPaths(item, next, out, limit)
			if len(*out) >= limit {
				return
			}
		}
	}
}

func firstObjectFromArray(payload map[string]any, key string) map[string]any {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	first, _ := items[0].(map[string]any)
	return first
}

func firstAreaItemObject(payload map[string]any) map[string]any {
	area := firstObjectFromArray(payload, "userAreas")
	if area == nil {
		return nil
	}
	value, ok := area["areaItems"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	first, _ := items[0].(map[string]any)
	return first
}

func removedNestedKeys(original, prepared map[string]any) []string {
	if len(original) == 0 {
		return nil
	}
	removed := make([]string, 0)
	for key := range original {
		if _, ok := prepared[key]; !ok {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)
	return removed
}

func shouldPrepareRecommendUserData(query AutoQuery) bool {
	if query.MaxProfile || query.SubMaxProfile {
		return true
	}
	if query.UnitFilter != "" || query.AttrFilter != "" || len(query.ExcludedCards) > 0 {
		return true
	}
	if query.AreaItemLevel != nil && *query.AreaItemLevel > 0 {
		return true
	}
	if query.UseCurrentDeck {
		return true
	}
	return false
}
