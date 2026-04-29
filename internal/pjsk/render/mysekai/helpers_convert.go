package mysekai

import (
	"encoding/json"
	"strconv"
	"strings"
)

func intNumber(value any, fallback int) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
		if n, err := v.Float64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

func intNumberFrom(item map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		if item == nil {
			break
		}
		if value, ok := item[key]; ok {
			if n := intNumber(value, fallback); n != fallback {
				return n
			}
		}
	}
	return fallback
}

func boolValueFrom(item map[string]any, keys ...string) bool {
	for _, key := range keys {
		if item == nil {
			break
		}
		if value, ok := item[key]; ok {
			return boolValue(value)
		}
	}
	return false
}

func stringValueFrom(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if item == nil {
			break
		}
		if value, ok := item[key]; ok {
			if text := stringValue(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func floatNumber(value any, fallback float64) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if n, err := v.Float64(); err == nil {
			return n
		}
	case string:
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return n
		}
	}
	return fallback
}

func int64Number(value any, fallback int64) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n
		}
		if n, err := v.Float64(); err == nil {
			return int64(n)
		}
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func boolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case float32:
		return v != 0
	case int:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case json.Number:
		n, err := v.Int64()
		return err == nil && n != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "y":
			return true
		}
	}
	return false
}

func stringValue(value any) string {
	v, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

func nestedList(root map[string]any, key string) []any {
	if root == nil {
		return nil
	}
	if items, ok := root[key].([]any); ok {
		return items
	}
	if updated, ok := root["updatedResources"].(map[string]any); ok {
		if items, ok := updated[key].([]any); ok {
			return items
		}
	}
	return nil
}

func nestedInt(root map[string]any, parent, child string) int {
	if root == nil {
		return 0
	}
	parentMap, ok := root[parent].(map[string]any)
	if !ok {
		return 0
	}
	return intNumber(parentMap[child], 0)
}

func parseIntTokens(query string) []int {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return r == ',' || r == ' ' || r == '，' || r == '\t' || r == '\n'
	})
	result := make([]int, 0, len(fields))
	seen := make(map[int]struct{}, len(fields))
	for _, field := range fields {
		id, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
