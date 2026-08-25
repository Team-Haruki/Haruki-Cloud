package snapshot

import (
	"bytes"
	json "haruki-cloud/internal/jsonutil"
	"strconv"
	"strings"
)

func resolveMusicResultMap(raw RawUserData) map[string]map[int]string {
	result := buildMusicResultMap(raw.UserMusicStats)
	mergeMusicResultMaps(result, buildMusicResultMapFromUserMusics(raw.UserMusics))
	mergeMusicResultMaps(result, buildMusicResultMapFromCompact(raw.CompactUserMusicResults))
	return result
}

func buildMusicResultMap(rawResults []RawMusicResult) map[string]map[int]string {
	result := make(map[string]map[int]string)
	for _, item := range rawResults {
		diff := strings.ToLower(strings.TrimSpace(item.MusicDifficultyType))
		if diff == "" {
			diff = strings.ToLower(strings.TrimSpace(item.MusicDifficulty))
		}
		if diff == "" {
			continue
		}
		status := normalizePlayResult(item)
		if _, ok := result[diff]; !ok {
			result[diff] = make(map[int]string)
		}
		if prioritizePlayResult(status) >= prioritizePlayResult(result[diff][item.MusicID]) {
			result[diff][item.MusicID] = status
		}
	}
	return result
}

func mergeMusicResultMaps(dst map[string]map[int]string, src map[string]map[int]string) {
	for diff, items := range src {
		if len(items) == 0 {
			continue
		}
		if _, ok := dst[diff]; !ok {
			dst[diff] = make(map[int]string, len(items))
		}
		for musicID, status := range items {
			if prioritizePlayResult(status) >= prioritizePlayResult(dst[diff][musicID]) {
				dst[diff][musicID] = status
			}
		}
	}
}

func buildMusicResultMapFromUserMusics(userMusics []RawUserMusic) map[string]map[int]string {
	if len(userMusics) == 0 {
		return nil
	}

	results := make([]RawMusicResult, 0, len(userMusics))
	for _, music := range userMusics {
		for _, status := range music.UserMusicDifficultyStatuses {
			for _, item := range status.UserMusicResults {
				normalized := item
				if normalized.MusicID == 0 {
					normalized.MusicID = music.MusicID
				}
				if strings.TrimSpace(normalized.MusicDifficultyType) == "" {
					normalized.MusicDifficultyType = strings.TrimSpace(status.MusicDifficultyType)
				}
				if strings.TrimSpace(normalized.MusicDifficulty) == "" {
					normalized.MusicDifficulty = strings.TrimSpace(status.MusicDifficulty)
				}
				if strings.TrimSpace(normalized.MusicDifficultyType) == "" {
					normalized.MusicDifficultyType = strings.TrimSpace(normalized.MusicDifficulty)
				}
				if strings.TrimSpace(normalized.MusicDifficulty) == "" {
					normalized.MusicDifficulty = strings.TrimSpace(normalized.MusicDifficultyType)
				}
				results = append(results, normalized)
			}
		}
	}
	return buildMusicResultMap(results)
}

func buildMusicResultMapFromCompact(raw json.RawMessage) map[string]map[int]string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	var payload map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}

	enumValues := decodeCompactEnumValues(payload["__ENUM__"])
	columns := make(map[string][]any, len(payload))
	rowCount := 0
	for key, fieldRaw := range payload {
		if key == "__ENUM__" {
			continue
		}

		var values []any
		decoder := json.NewDecoder(bytes.NewReader(fieldRaw))
		decoder.UseNumber()
		if err := decoder.Decode(&values); err != nil {
			continue
		}
		if len(values) == 0 {
			continue
		}
		columns[key] = values
		if len(values) > rowCount {
			rowCount = len(values)
		}
	}

	if rowCount == 0 {
		return nil
	}

	results := make([]RawMusicResult, 0, rowCount)
	for index := 0; index < rowCount; index++ {
		musicID, ok := compactIntFromColumns(columns, "musicId", index)
		if !ok || musicID <= 0 {
			continue
		}

		diff := compactStringFromColumns(columns, enumValues, []string{"musicDifficultyType", "musicDifficulty"}, index)
		if diff == "" {
			continue
		}

		playResult := compactStringFromColumns(columns, enumValues, []string{"playResult"}, index)
		fullCombo, _ := compactBoolFromColumns(columns, "fullComboFlg", index)
		fullPerfect, _ := compactBoolFromColumns(columns, "fullPerfectFlg", index)

		results = append(results, RawMusicResult{
			MusicID:             musicID,
			MusicDifficulty:     diff,
			MusicDifficultyType: diff,
			PlayResult:          playResult,
			FullComboFlg:        fullCombo,
			FullPerfectFlg:      fullPerfect,
		})
	}

	return buildMusicResultMap(results)
}

func decodeCompactEnumValues(raw json.RawMessage) map[string][]string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	var values map[string][]string
	if err := json.Unmarshal(trimmed, &values); err != nil {
		return nil
	}
	return values
}

func compactStringFromColumns(columns map[string][]any, enumValues map[string][]string, keys []string, index int) string {
	for _, key := range keys {
		values, ok := columns[key]
		if !ok || index >= len(values) {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(compactEnumString(values[index], enumValues[key])))
		if value != "" {
			return value
		}
	}
	return ""
}

func compactIntFromColumns(columns map[string][]any, key string, index int) (int, bool) {
	values, ok := columns[key]
	if !ok || index >= len(values) {
		return 0, false
	}
	return compactIntValue(values[index])
}

func compactBoolFromColumns(columns map[string][]any, key string, index int) (bool, bool) {
	values, ok := columns[key]
	if !ok || index >= len(values) {
		return false, false
	}
	return compactBoolValue(values[index])
}

func compactEnumString(value any, enumValues []string) string {
	if idx, ok := compactIntValue(value); ok {
		if idx >= 0 && idx < len(enumValues) {
			return enumValues[idx]
		}
	}

	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func compactIntValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func compactBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err != nil {
			return false, false
		}
		return parsed, true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return false, false
		}
		return parsed != 0, true
	case float64:
		return typed != 0, true
	default:
		return false, false
	}
}

func normalizePlayResult(item RawMusicResult) string {
	switch {
	case item.FullPerfectFlg:
		return "ap"
	case item.FullComboFlg:
		return "fc"
	case strings.EqualFold(item.PlayResult, "not_clear") || item.PlayResult == "":
		return "not_clear"
	default:
		return "clear"
	}
}

func prioritizePlayResult(result string) int {
	switch result {
	case "ap":
		return 3
	case "fc":
		return 2
	case "clear":
		return 1
	default:
		return 0
	}
}
