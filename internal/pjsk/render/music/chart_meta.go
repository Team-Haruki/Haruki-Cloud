package music

import (
	"encoding/json"
	"sort"
	"strings"

	"haruki-cloud/utils/drawing"
)

func findMusicMeta(payload []byte, musicID int, difficulty string) map[string]interface{} {
	if len(payload) == 0 || musicID <= 0 {
		return nil
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil
	}

	diff := normalizeDifficulty(difficulty)
	for _, item := range items {
		if musicMetaID(item) != musicID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringValue(item["difficulty"])), diff) {
			continue
		}
		return cloneMetaMap(item)
	}
	return nil
}

func findAllMusicMetas(payload []byte, musicID int) []map[string]interface{} {
	if len(payload) == 0 || musicID <= 0 {
		return nil
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil
	}

	result := make([]map[string]interface{}, 0)
	for _, item := range items {
		if musicMetaID(item) != musicID {
			continue
		}
		result = append(result, cloneMetaMap(item))
	}
	sort.SliceStable(result, func(i, j int) bool {
		return difficultyOrder(stringValue(result[i]["difficulty"])) < difficultyOrder(stringValue(result[j]["difficulty"]))
	})
	return result
}

func musicMetaInfosFromPayload(payload []byte, musicID int) []drawing.MusicMetaInfo {
	items := findAllMusicMetas(payload, musicID)
	if len(items) == 0 {
		return nil
	}

	result := make([]drawing.MusicMetaInfo, 0, len(items))
	for _, item := range items {
		result = append(result, drawing.MusicMetaInfo{
			Difficulty:      normalizedDifficultyValue(item["difficulty"]),
			MusicTime:       floatValue(item["music_time"]),
			TapCount:        intValue(item["tap_count"]),
			EventRate:       floatValue(item["event_rate"]),
			BaseScore:       floatValue(item["base_score"]),
			BaseScoreAuto:   floatValue(item["base_score_auto"]),
			SkillScoreSolo:  floatSliceValue(item["skill_score_solo"]),
			SkillScoreAuto:  floatSliceValue(item["skill_score_auto"]),
			SkillScoreMulti: floatSliceValue(item["skill_score_multi"]),
			FeverScore:      floatValue(item["fever_score"]),
		})
	}
	return result
}

func musicMetaID(item map[string]interface{}) int {
	if item == nil {
		return 0
	}
	switch value := item["music_id"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return 0
	}
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return text
}

func normalizedDifficultyValue(value interface{}) string {
	raw := strings.TrimSpace(stringValue(value))
	if normalized := normalizeDifficulty(raw); normalized != "" {
		return normalized
	}
	return raw
}

func floatValue(value interface{}) float64 {
	switch item := value.(type) {
	case float64:
		return item
	case float32:
		return float64(item)
	case int:
		return float64(item)
	case int64:
		return float64(item)
	default:
		return 0
	}
}

func intValue(value interface{}) int {
	return int(floatValue(value))
}

func floatSliceValue(value interface{}) []float64 {
	switch items := value.(type) {
	case []float64:
		return append([]float64(nil), items...)
	case []interface{}:
		result := make([]float64, 0, len(items))
		for _, item := range items {
			result = append(result, floatValue(item))
		}
		return result
	default:
		return nil
	}
}

func cloneMetaMap(item map[string]interface{}) map[string]interface{} {
	data, err := json.Marshal(item)
	if err != nil {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
