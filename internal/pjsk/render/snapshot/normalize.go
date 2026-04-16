package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func normalizeSnapshotJSON(data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return data, nil
	}

	var raw any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode snapshot JSON: %w", err)
	}

	normalized, err := normalizeExtendedJSONValue(raw, true)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode normalized snapshot JSON: %w", err)
	}
	return encoded, nil
}

func normalizeExtendedJSONValue(value any, topLevel bool) (any, error) {
	switch typed := value.(type) {
	case []any:
		if topLevel {
			if len(typed) == 0 {
				return nil, fmt.Errorf("snapshot array is empty")
			}
			if len(typed) != 1 {
				return nil, fmt.Errorf("snapshot array contains %d documents; expected 1", len(typed))
			}
			return normalizeExtendedJSONValue(typed[0], false)
		}

		out := make([]any, 0, len(typed))
		for _, item := range typed {
			normalized, err := normalizeExtendedJSONValue(item, false)
			if err != nil {
				return nil, err
			}
			out = append(out, normalized)
		}
		return out, nil

	case map[string]any:
		if len(typed) == 1 {
			if raw, ok := typed["$numberLong"]; ok {
				return normalizeExtendedJSONNumber(raw, "$numberLong")
			}
			if raw, ok := typed["$numberInt"]; ok {
				return normalizeExtendedJSONNumber(raw, "$numberInt")
			}
			if raw, ok := typed["$numberDouble"]; ok {
				return normalizeExtendedJSONNumber(raw, "$numberDouble")
			}
			if raw, ok := typed["$numberDecimal"]; ok {
				return normalizeExtendedJSONNumber(raw, "$numberDecimal")
			}
			if raw, ok := typed["$oid"]; ok {
				return normalizeExtendedJSONString(raw, "$oid")
			}
			if raw, ok := typed["$date"]; ok {
				return normalizeExtendedJSONValue(raw, false)
			}
		}

		out := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized, err := normalizeExtendedJSONValue(item, false)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		}
		return out, nil

	default:
		return value, nil
	}
}

func normalizeExtendedJSONNumber(raw any, key string) (json.Number, error) {
	switch value := raw.(type) {
	case json.Number:
		return value, nil
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return "", fmt.Errorf("%s value is empty", key)
		}
		return json.Number(value), nil
	default:
		return "", fmt.Errorf("%s value must be a string or number", key)
	}
}

func normalizeExtendedJSONString(raw any, key string) (string, error) {
	switch value := raw.(type) {
	case string:
		return value, nil
	default:
		return "", fmt.Errorf("%s value must be a string", key)
	}
}

func writeNormalizedSnapshotFile(pattern string, data []byte) (string, error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create normalized snapshot file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("write normalized snapshot file: %w", err)
	}
	return file.Name(), nil
}
