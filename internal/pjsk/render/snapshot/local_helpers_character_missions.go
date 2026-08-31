package snapshot

import (
	"bytes"
	json "haruki-cloud/internal/jsonutil"
	"slices"
	"strings"
)

func ResolveCharacterMissionV2Statuses(raw *RawUserData) []RawUserCharacterMissionV2Status {
	if raw == nil {
		return nil
	}
	standard := slices.Clone(raw.UserCharacterMissionV2Statuses)
	compact := decodeCompactCharacterMissionV2Statuses(raw.CompactUserCharacterMissionV2Statuses)
	if len(standard) == 0 {
		return compact
	}
	if len(compact) == 0 {
		return standard
	}
	return mergeCharacterMissionV2Statuses(standard, compact)
}

func DecodeCompactCharacterMissionV2Statuses(raw json.RawMessage) []RawUserCharacterMissionV2Status {
	return decodeCompactCharacterMissionV2Statuses(raw)
}

func decodeCompactCharacterMissionV2Statuses(raw json.RawMessage) []RawUserCharacterMissionV2Status {
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

	columns, rowCount := decodeCompactMissionColumns(payload)
	if rowCount == 0 {
		return nil
	}

	enumValues := decodeCompactEnumValues(payload["__ENUM__"])
	statuses := make([]RawUserCharacterMissionV2Status, 0, rowCount)
	for index := 0; index < rowCount; index++ {
		status, ok := compactCharacterMissionStatusAt(columns, enumValues, index)
		if ok {
			statuses = append(statuses, status)
		}
	}
	return statuses
}

func decodeCompactMissionColumns(payload map[string]json.RawMessage) (map[string][]any, int) {
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
	return columns, rowCount
}

func compactCharacterMissionStatusAt(columns map[string][]any, enumValues map[string][]string, index int) (RawUserCharacterMissionV2Status, bool) {
	characterID, characterOK := compactPositiveIntFromColumns(columns, "characterId", index)
	parameterGroupID, parameterGroupOK := compactPositiveIntFromColumns(columns, "parameterGroupId", index)
	seq, seqOK := compactPositiveIntFromColumns(columns, "seq", index)
	if !characterOK || !parameterGroupOK || !seqOK {
		return RawUserCharacterMissionV2Status{}, false
	}

	status := compactMissionStatusString(columns, enumValues, index)
	if strings.EqualFold(status, "reset") {
		return RawUserCharacterMissionV2Status{}, false
	}
	return RawUserCharacterMissionV2Status{
		ParameterGroupID: parameterGroupID,
		Seq:              seq,
		CharacterID:      characterID,
		MissionID:        compactIntFromColumnsOrZero(columns, "missionId", index),
		MissionStatus:    status,
	}, true
}

func compactPositiveIntFromColumns(columns map[string][]any, key string, index int) (int, bool) {
	value, ok := compactIntFromColumns(columns, key, index)
	return value, ok && value > 0
}

func compactIntFromColumnsOrZero(columns map[string][]any, key string, index int) int {
	value, ok := compactIntFromColumns(columns, key, index)
	if !ok {
		return 0
	}
	return value
}

func compactMissionStatusString(columns map[string][]any, enumValues map[string][]string, index int) string {
	values, ok := columns["missionStatus"]
	if !ok || index >= len(values) {
		return ""
	}

	if idx, ok := compactIntValue(values[index]); ok {
		enums := enumValues["missionStatus"]
		if idx >= 0 && idx < len(enums) {
			return strings.ToLower(strings.TrimSpace(enums[idx]))
		}
		if idx > 0 && idx <= len(enums) {
			return strings.ToLower(strings.TrimSpace(enums[idx-1]))
		}
	}

	switch typed := values[index].(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(typed))
	case json.Number:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func mergeCharacterMissionV2Statuses(groups ...[]RawUserCharacterMissionV2Status) []RawUserCharacterMissionV2Status {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	if total == 0 {
		return nil
	}

	result := make([]RawUserCharacterMissionV2Status, 0, total)
	seen := make(map[[3]int]struct{}, total)
	for _, group := range groups {
		for _, item := range group {
			key := [3]int{item.CharacterID, item.ParameterGroupID, item.Seq}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}
