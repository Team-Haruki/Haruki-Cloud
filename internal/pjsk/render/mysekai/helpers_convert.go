package mysekai

import (
"strconv"
"strings"
)

func intNumber(value interface{}, fallback int) int {
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
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

func floatNumber(value interface{}, fallback float64) float64 {
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
	case string:
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return n
		}
	}
	return fallback
}

func int64Number(value interface{}, fallback int64) int64 {
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
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func boolValue(value interface{}) bool {
	v, ok := value.(bool)
	return ok && v
}

func stringValue(value interface{}) string {
	v, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

func nestedList(root map[string]interface{}, key string) []interface{} {
	if root == nil {
		return nil
	}
	if items, ok := root[key].([]interface{}); ok {
		return items
	}
	if updated, ok := root["updatedResources"].(map[string]interface{}); ok {
		if items, ok := updated[key].([]interface{}); ok {
			return items
		}
	}
	return nil
}

func nestedInt(root map[string]interface{}, parent, child string) int {
	if root == nil {
		return 0
	}
	parentMap, ok := root[parent].(map[string]interface{})
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

