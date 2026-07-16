package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	sonic "github.com/bytedance/sonic"
	"os"
	"strings"
)

// extendedJSONMarkers are substrings that must appear verbatim whenever the
// payload contains a Mongo extended-JSON wrapper key ($numberLong, $numberInt,
// $numberDouble, $numberDecimal, $oid, $date). "$number" covers the four
// numeric wrappers at once.
var extendedJSONMarkers = [][]byte{
	[]byte("$number"),
	[]byte("$oid"),
	[]byte("$date"),
}

// needsSnapshotNormalization reports whether the payload may require the
// extended-JSON rewrite: either a top-level array document that needs
// unwrapping, or one of the wrapper-key markers somewhere in the bytes.
// False positives only cost the slow path; false negatives are impossible
// because any wrapper key contains a marker substring verbatim.
func needsSnapshotNormalization(trimmed []byte) bool {
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return true
	}
	for _, marker := range extendedJSONMarkers {
		if bytes.Contains(trimmed, marker) {
			return true
		}
	}
	return false
}

func normalizeSnapshotJSON(data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return data, nil
	}
	if !needsSnapshotNormalization(trimmed) {
		// Hot path: plain JSON object with no extended-JSON wrappers. The
		// decode+rewrite+encode round trip would be a no-op, so skip it;
		// malformed JSON still fails at the typed snapshot decode.
		return data, nil
	}

	var raw any
	decoder := sonic.ConfigDefault.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode snapshot JSON: %w", err)
	}

	normalized, err := normalizeExtendedJSONValue(raw, true)
	if err != nil {
		return nil, err
	}

	encoded, err := sonic.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode normalized snapshot JSON: %w", err)
	}
	return encoded, nil
}

// normalizeSnapshotDocument decodes a snapshot payload into its top-level
// document map, applying the extended-JSON rewrite only when markers are
// present. Callers that need the decoded map (e.g. the mysekai merge) use
// this instead of normalizeSnapshotJSON + Unmarshal, which would pay an
// extra encode/decode round trip. Numbers stay json.Number, so values that
// exceed float64 precision (large user IDs) survive re-encoding exactly.
func normalizeSnapshotDocument(data []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("snapshot document is empty")
	}

	var raw any
	decoder := sonic.ConfigDefault.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode snapshot JSON: %w", err)
	}

	if needsSnapshotNormalization(trimmed) {
		normalized, err := normalizeExtendedJSONValue(raw, true)
		if err != nil {
			return nil, err
		}
		raw = normalized
	}

	document, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("snapshot document must be a JSON object, got %T", raw)
	}
	return document, nil
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
