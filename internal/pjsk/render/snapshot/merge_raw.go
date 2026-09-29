package snapshot

import (
	"bytes"
	"fmt"
	"strings"

	json "haruki-cloud/internal/jsonutil"
)

func mergeMySekaiData(userData, mySekaiData []byte) ([]byte, error) {
	// Exports need the recursive extended-JSON rewrite. Ordinary Toolbox
	// snapshots only need top-level fields selected; their large arrays remain
	// encoded instead of becoming millions of short-lived Go values.
	if needsSnapshotNormalization(bytes.TrimSpace(userData)) || needsSnapshotNormalization(bytes.TrimSpace(mySekaiData)) {
		return mergeMySekaiDataDecoded(userData, mySekaiData)
	}
	var base, delta map[string]json.RawMessage
	if err := json.UnmarshalUniqueNames(userData, &base); err != nil || base == nil {
		return mergeMySekaiDataDecoded(userData, mySekaiData)
	}
	if err := json.UnmarshalUniqueNames(mySekaiData, &delta); err != nil || delta == nil {
		return mergeMySekaiDataDecoded(userData, mySekaiData)
	}

	var updated map[string]json.RawMessage
	if value := bytes.TrimSpace(delta["updatedResources"]); len(value) > 0 && value[0] == '{' {
		if err := json.Unmarshal(value, &updated); err != nil {
			return nil, fmt.Errorf("decode mysekai updatedResources: %w", err)
		}
	}
	for key, value := range updated {
		if !shouldPreserveSuiteSnapshotKey(key) && !skipEmptyRawMySekaiOverride(base[key], value) {
			base[key] = value
		}
	}
	for key, value := range delta {
		if !isMergeableMySekaiTopLevelKey(key) {
			continue
		}
		// Even a skipped empty delta blocks an older top-level value.
		if _, exists := updated[key]; exists {
			continue
		}
		previous, exists := base[key]
		if skipEmptyRawMySekaiOverride(previous, value) {
			continue
		}
		if exists && (strings.HasPrefix(key, "userMysekai") || strings.HasPrefix(key, "mysekai")) && !isEmptyRawSnapshotValue(previous) {
			continue
		}
		base[key] = value
	}
	merged, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("encode merged mysekai snapshot: %w", err)
	}
	return merged, nil
}

func skipEmptyRawMySekaiOverride(previous, next json.RawMessage) bool {
	previous = bytes.TrimSpace(previous)
	next = bytes.TrimSpace(next)
	return len(previous) > 2 && previous[0] == '[' && len(bytes.TrimSpace(previous[1:len(previous)-1])) != 0 &&
		len(next) >= 2 && next[0] == '[' && len(bytes.TrimSpace(next[1:len(next)-1])) == 0
}

// Values have already passed JSON validation. Only string values need decoding
// to distinguish whitespace (including escaped Unicode whitespace) from text.
func isEmptyRawSnapshotValue(value json.RawMessage) bool {
	value = bytes.TrimSpace(value)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return true
	}
	switch value[0] {
	case '[', '{':
		return len(bytes.TrimSpace(value[1:len(value)-1])) == 0
	case '"':
		var text string
		return json.Unmarshal(value, &text) == nil && strings.TrimSpace(text) == ""
	default:
		return false
	}
}
