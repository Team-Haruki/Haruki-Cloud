package masterdata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DecodeCardParameters accepts both the legacy array form and the newer
// object form used by some migrated card records.
func DecodeCardParameters(raw json.RawMessage) ([]CardParameter, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	var items []CardParameter
	if err := json.Unmarshal(raw, &items); err == nil {
		normalizeCardParameters(items, 0)
		return items, nil
	}

	var single CardParameter
	if err := json.Unmarshal(raw, &single); err == nil && cardParameterMeaningful(single) {
		single.CardParameterType = normalizeCardParameterType(single.CardParameterType)
		return []CardParameter{single}, nil
	}

	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return compareCardParameterKeys(keys[i], keys[j]) < 0
	})

	result := make([]CardParameter, 0, len(keys))
	for _, key := range keys {
		item, ok, err := decodeCardParameterEntry(key, values[key])
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		result = append(result, item)
	}

	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func (c *Card) UnmarshalJSON(data []byte) error {
	type alias Card
	aux := struct {
		*alias
		CardParametersCamel json.RawMessage `json:"cardParameters"`
		CardParametersSnake json.RawMessage `json:"card_parameters"`
	}{
		alias: (*alias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	raw := aux.CardParametersCamel
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = aux.CardParametersSnake
	}

	parameters, err := DecodeCardParameters(raw)
	if err != nil {
		return fmt.Errorf("decode card parameters: %w", err)
	}
	normalizeCardParameters(parameters, c.ID)
	c.CardParameters = parameters
	return nil
}

func decodeCardParameterEntry(key string, raw json.RawMessage) (CardParameter, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return CardParameter{}, false, nil
	}

	var item CardParameter
	if err := json.Unmarshal(raw, &item); err == nil && cardParameterMeaningful(item) {
		if strings.TrimSpace(item.CardParameterType) == "" {
			item.CardParameterType = normalizeCardParameterType(key)
		} else {
			item.CardParameterType = normalizeCardParameterType(item.CardParameterType)
		}
		return item, true, nil
	}

	var intValue int
	if err := json.Unmarshal(raw, &intValue); err == nil {
		return CardParameter{
			CardParameterType: normalizeCardParameterType(key),
			Power:             intValue,
		}, true, nil
	}

	var floatValue float64
	if err := json.Unmarshal(raw, &floatValue); err == nil {
		return CardParameter{
			CardParameterType: normalizeCardParameterType(key),
			Power:             int(floatValue),
		}, true, nil
	}

	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		stringValue = strings.TrimSpace(stringValue)
		if stringValue == "" {
			return CardParameter{}, false, nil
		}
		power, convErr := strconv.Atoi(stringValue)
		if convErr != nil {
			return CardParameter{}, false, fmt.Errorf("unsupported card parameter value %q for %s", stringValue, key)
		}
		return CardParameter{
			CardParameterType: normalizeCardParameterType(key),
			Power:             power,
		}, true, nil
	}

	return CardParameter{}, false, fmt.Errorf("unsupported card parameter shape for %s", key)
}

func compareCardParameterKeys(left, right string) int {
	leftGroup, leftOrder, leftNormalized := cardParameterKeyOrder(left)
	rightGroup, rightOrder, rightNormalized := cardParameterKeyOrder(right)

	if leftGroup != rightGroup {
		if leftGroup < rightGroup {
			return -1
		}
		return 1
	}
	if leftOrder != rightOrder {
		if leftOrder < rightOrder {
			return -1
		}
		return 1
	}
	switch {
	case leftNormalized < rightNormalized:
		return -1
	case leftNormalized > rightNormalized:
		return 1
	default:
		return 0
	}
}

func cardParameterKeyOrder(value string) (group int, order int, normalized string) {
	normalized = normalizeCardParameterType(value)
	if strings.HasPrefix(normalized, "param") {
		if index, err := strconv.Atoi(strings.TrimPrefix(normalized, "param")); err == nil {
			return 0, index, normalized
		}
	}
	return 1, 0, normalized
}

func normalizeCardParameters(items []CardParameter, cardID int) {
	for idx := range items {
		if strings.TrimSpace(items[idx].CardParameterType) != "" {
			items[idx].CardParameterType = normalizeCardParameterType(items[idx].CardParameterType)
		}
		if items[idx].CardID == 0 && cardID > 0 {
			items[idx].CardID = cardID
		}
	}
}

func normalizeCardParameterType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cardParameterMeaningful(item CardParameter) bool {
	return item.ID != 0 || item.CardID != 0 || item.Power != 0 || strings.TrimSpace(item.CardParameterType) != ""
}
