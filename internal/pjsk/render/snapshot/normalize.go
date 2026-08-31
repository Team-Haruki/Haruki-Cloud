package snapshot

import (
	"bytes"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
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
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
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
		return normalizeExtendedJSONArray(typed, topLevel)
	case map[string]any:
		return normalizeExtendedJSONObject(typed)
	default:
		return value, nil
	}
}

func normalizeExtendedJSONArray(items []any, topLevel bool) (any, error) {
	if topLevel {
		if len(items) == 0 {
			return nil, fmt.Errorf("snapshot array is empty")
		}
		if len(items) != 1 {
			return nil, fmt.Errorf("snapshot array contains %d documents; expected 1", len(items))
		}
		return normalizeExtendedJSONValue(items[0], false)
	}

	out := make([]any, 0, len(items))
	for _, item := range items {
		normalized, err := normalizeExtendedJSONValue(item, false)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeExtendedJSONObject(object map[string]any) (any, error) {
	if len(object) == 1 {
		if normalized, matched, err := normalizeExtendedJSONWrapper(object); matched {
			return normalized, err
		}
	}

	out := make(map[string]any, len(object))
	for key, item := range object {
		normalized, err := normalizeExtendedJSONValue(item, false)
		if err != nil {
			return nil, err
		}
		out[key] = normalized
	}
	return out, nil
}

func normalizeExtendedJSONWrapper(object map[string]any) (any, bool, error) {
	for _, key := range []string{"$numberLong", "$numberInt", "$numberDouble", "$numberDecimal"} {
		if raw, ok := object[key]; ok {
			normalized, err := normalizeExtendedJSONNumber(raw, key)
			return normalized, true, err
		}
	}
	if raw, ok := object["$oid"]; ok {
		normalized, err := normalizeExtendedJSONString(raw, "$oid")
		return normalized, true, err
	}
	if raw, ok := object["$date"]; ok {
		normalized, err := normalizeExtendedJSONValue(raw, false)
		return normalized, true, err
	}
	return nil, false, nil
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
