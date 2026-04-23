package snapshot

import (
	"encoding/json"
	sonic "github.com/bytedance/sonic"
	"fmt"
	"slices"
	"strings"
)

func (s *Service) RawValue(key string) ([]byte, error) {
	if s == nil || len(s.rawJSON) == 0 {
		return nil, fmt.Errorf("raw user snapshot is unavailable")
	}

	name := strings.TrimSpace(key)
	if name == "" {
		return nil, fmt.Errorf("raw user snapshot key is empty")
	}

	var payload map[string]json.RawMessage
	if err := sonic.Unmarshal(s.rawJSON, &payload); err != nil {
		return nil, fmt.Errorf("decode raw user snapshot: %w", err)
	}

	value, ok := payload[name]
	if !ok || len(value) == 0 {
		return nil, fmt.Errorf("raw user snapshot key %q is unavailable", name)
	}
	return slices.Clone(value), nil
}
