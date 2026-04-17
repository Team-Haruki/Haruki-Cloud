package deck

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/utils/logger"
)

var deckPrepareDebugLogger = logger.NewLoggerFromGlobal("DeckPrepare")

func logPreparedRecommendUserData(region renderregion.Value, recType string, query AutoQuery, raw *snapshot.RawUserData, originalBytes, preparedBytes []byte) {
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
