package music

import (
	"encoding/json"
	"strings"
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
