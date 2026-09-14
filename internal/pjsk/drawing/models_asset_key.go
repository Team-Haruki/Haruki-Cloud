package drawing

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// AssetKey is a Drawing asset field that forks on existence (C1): one path, or
// an ordered candidate list from which Drawing takes the first existing
// candidate. A single candidate marshals as a plain JSON string, so a field
// with one path keeps today's wire shape (and render-cache key) byte for byte;
// two or more marshal as a JSON array. An empty key marshals as "".
type AssetKey []string

// AssetCandidates builds an AssetKey from paths in preference order, dropping
// blank entries and later duplicates. It never returns a key with blanks.
func AssetCandidates(paths ...string) AssetKey {
	var key AssetKey
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || slices.Contains(key, path) {
			continue
		}
		key = append(key, path)
	}
	return key
}

// AssetPath is the single-path AssetKey (nil for a blank path).
func AssetPath(path string) AssetKey {
	return AssetCandidates(path)
}

// First returns the preferred candidate, or "" for an empty key.
func (k AssetKey) First() string {
	if len(k) == 0 {
		return ""
	}
	return k[0]
}

// Last returns the final candidate (Drawing's pick when none exists), or "".
func (k AssetKey) Last() string {
	if len(k) == 0 {
		return ""
	}
	return k[len(k)-1]
}

// MarshalJSON emits a string for zero or one candidate and an array otherwise.
func (k AssetKey) MarshalJSON() ([]byte, error) {
	if len(k) <= 1 {
		return json.Marshal(k.First())
	}
	return json.Marshal([]string(k))
}

// UnmarshalJSON accepts a JSON string, an array of strings, or null.
func (k *AssetKey) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case trimmed == "null":
		*k = nil
		return nil
	case strings.HasPrefix(trimmed, "["):
		var paths []string
		if err := json.Unmarshal(data, &paths); err != nil {
			return fmt.Errorf("decode asset key candidates: %w", err)
		}
		*k = AssetKey(paths)
		return nil
	default:
		var path string
		if err := json.Unmarshal(data, &path); err != nil {
			return fmt.Errorf("decode asset key: %w", err)
		}
		if path == "" {
			*k = nil
			return nil
		}
		*k = AssetKey{path}
		return nil
	}
}
