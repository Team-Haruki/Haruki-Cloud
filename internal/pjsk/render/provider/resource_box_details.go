package provider

import (
	"slices"
	"encoding/json"
	"strings"
)

type resourceBoxDetailRecord struct {
	ResourceBoxID      int    `json:"resourceBoxId"`
	ResourceBoxPurpose string `json:"resourceBoxPurpose"`
	ResourceID         *int   `json:"resourceId"`
	ResourceLevel      *int   `json:"resourceLevel"`
	ResourceQuantity   int    `json:"resourceQuantity"`
	ResourceType       string `json:"resourceType"`
}

func supplementResourceBoxDetailsFromStore(store *localStore, byPurpose map[string]map[int]*ResourceBox) {
	if store == nil || !store.Configured() || len(byPurpose) == 0 {
		return
	}

	detailIndex := loadResourceBoxDetailsIndex(store)
	if len(detailIndex) == 0 {
		return
	}

	for purpose, purposeBoxes := range byPurpose {
		purposeDetails, ok := detailIndex[purpose]
		if !ok {
			continue
		}
		for id, box := range purposeBoxes {
			if box == nil || len(box.Details) > 0 {
				continue
			}
			if details := purposeDetails[id]; len(details) > 0 {
				box.Details = slices.Clone(details)
			}
		}
	}
}

func loadResourceBoxDetailsIndex(store *localStore) map[string]map[int][]ResourceBoxDetail {
	if rows, ok := loadResourceBoxDetailsRows(store); ok {
		return indexResourceBoxDetails(rows)
	}
	if rows, ok := loadCompactResourceBoxDetailsRows(store); ok {
		return indexResourceBoxDetails(rows)
	}
	return nil
}

func loadResourceBoxDetailsRows(store *localStore) ([]resourceBoxDetailRecord, bool) {
	data, err := store.readFile("resourceBoxDetails.json")
	if err != nil {
		return nil, false
	}

	var rows []resourceBoxDetailRecord
	if err := decodeJSONUseNumber(data, &rows); err != nil {
		return nil, false
	}
	if len(rows) == 0 {
		return nil, false
	}
	return rows, true
}

func loadCompactResourceBoxDetailsRows(store *localStore) ([]resourceBoxDetailRecord, bool) {
	data, err := store.readFile("compactResourceBoxDetails.json")
	if err != nil {
		return nil, false
	}

	var payload map[string]json.RawMessage
	if err := decodeJSONUseNumber(data, &payload); err != nil {
		return nil, false
	}

	enumValues := decodeCompactEnumValues(payload["__ENUM__"])
	columns := make(map[string][]any, len(payload))
	rowCount := 0
	for key, fieldRaw := range payload {
		if key == "__ENUM__" {
			continue
		}

		var values []any
		if err := decodeJSONUseNumber(fieldRaw, &values); err != nil {
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

	if rowCount == 0 {
		return nil, false
	}

	rows := make([]resourceBoxDetailRecord, 0, rowCount)
	for index := 0; index < rowCount; index++ {
		boxID, ok := compactIntFromColumns(columns, "resourceBoxId", index)
		if !ok || boxID <= 0 {
			continue
		}

		purpose := compactStringFromColumns(columns, enumValues, []string{"resourceBoxPurpose"}, index)
		resourceType := compactStringFromColumns(columns, enumValues, []string{"resourceType"}, index)
		if purpose == "" || resourceType == "" {
			continue
		}

		row := resourceBoxDetailRecord{
			ResourceBoxID:      boxID,
			ResourceBoxPurpose: purpose,
			ResourceQuantity:   compactIntFromColumnsDefault(columns, "resourceQuantity", index),
			ResourceType:       resourceType,
		}
		if resourceID, ok := compactIntFromColumns(columns, "resourceId", index); ok {
			row.ResourceID = &resourceID
		}
		if resourceLevel, ok := compactIntFromColumns(columns, "resourceLevel", index); ok {
			row.ResourceLevel = &resourceLevel
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil, false
	}
	return rows, true
}

func indexResourceBoxDetails(rows []resourceBoxDetailRecord) map[string]map[int][]ResourceBoxDetail {
	index := make(map[string]map[int][]ResourceBoxDetail)
	for _, row := range rows {
		purpose := strings.TrimSpace(row.ResourceBoxPurpose)
		resourceType := strings.TrimSpace(row.ResourceType)
		if purpose == "" || resourceType == "" || row.ResourceBoxID <= 0 {
			continue
		}

		detail := ResourceBoxDetail{
			ResourceType:     resourceType,
			ResourceQuantity: row.ResourceQuantity,
		}
		if row.ResourceID != nil {
			detail.ResourceID = *row.ResourceID
		}
		if row.ResourceLevel != nil {
			detail.ResourceLevel = *row.ResourceLevel
		}

		if _, ok := index[purpose]; !ok {
			index[purpose] = make(map[int][]ResourceBoxDetail)
		}
		index[purpose][row.ResourceBoxID] = append(index[purpose][row.ResourceBoxID], detail)
	}
	return index
}

func decodeCompactEnumValues(raw json.RawMessage) map[string][]string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	var values map[string][]string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil
	}
	return values
}

func compactStringFromColumns(columns map[string][]any, enumValues map[string][]string, keys []string, index int) string {
	for _, key := range keys {
		values, ok := columns[key]
		if !ok || index >= len(values) {
			continue
		}
		value := strings.TrimSpace(compactEnumString(values[index], enumValues[key]))
		if value != "" {
			return value
		}
	}
	return ""
}

func compactIntFromColumns(columns map[string][]any, key string, index int) (int, bool) {
	values, ok := columns[key]
	if !ok || index >= len(values) {
		return 0, false
	}
	return interfaceToInt(values[index])
}

func compactIntFromColumnsDefault(columns map[string][]any, key string, index int) int {
	value, _ := compactIntFromColumns(columns, key, index)
	return value
}

func compactEnumString(value any, enumValues []string) string {
	if idx, ok := interfaceToInt(value); ok {
		if idx >= 0 && idx < len(enumValues) {
			return enumValues[idx]
		}
	}

	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}
