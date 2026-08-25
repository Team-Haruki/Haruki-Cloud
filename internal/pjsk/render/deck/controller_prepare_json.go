package deck

import (
	"bytes"
	json "haruki-cloud/internal/jsonutil"
)

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
