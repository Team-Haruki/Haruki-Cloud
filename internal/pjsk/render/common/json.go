package common

import (
	"bytes"
	"encoding/json"
	sonic "github.com/bytedance/sonic"
	"strings"
)

// DecodeJSONUseNumber unmarshals data into target using a decoder with
// UseNumber() enabled, so numeric fields land as json.Number instead of
// float64. This is required when passing JSON through without losing
// precision on large integers.
func DecodeJSONUseNumber(data []byte, target any) error {
	decoder := sonic.ConfigDefault.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
}

// JSONString extracts a plain string from a json.RawMessage value.
func JSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := sonic.Unmarshal(raw, &s); err != nil {
		return string(raw)
	}
	return s
}

// DecodeSlice unmarshals a json.RawMessage into a typed slice.
func DecodeSlice[T any](raw json.RawMessage) ([]T, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var items []T
	if err := sonic.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// DecodeMap unmarshals a json.RawMessage into a typed value.
func DecodeMap[T any](raw json.RawMessage) (T, error) {
	var result T
	if len(raw) == 0 {
		return result, nil
	}
	if err := sonic.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, nil
}

// ToStringSliceFromRaw unmarshals a JSON array of strings, filtering blanks.
func ToStringSliceFromRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var items []string
	if err := sonic.Unmarshal(raw, &items); err != nil {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			result = append(result, item)
		}
	}
	return result
}
