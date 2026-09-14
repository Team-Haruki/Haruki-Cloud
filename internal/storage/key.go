package storage

import (
	"fmt"
	"path"
	"strings"
)

// MaxKeyBytes is the longest accepted key.
const MaxKeyBytes = 1024

// CleanKey normalises raw into a Key. Backslashes become slashes, leading "./"
// and "/" are trimmed and "." segments are removed. Empty keys, ".." segments,
// empty segments ("a//b", a trailing "/"), control characters and keys longer
// than MaxKeyBytes are rejected with ErrInvalidKey.
func CleanKey(raw string) (Key, error) {
	trimmed := trimKeyLead(strings.ReplaceAll(raw, "\\", "/"))
	if err := checkKeyText(raw, trimmed); err != nil {
		return "", err
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" || segment == ".." {
			return "", invalidKey(raw, "empty or parent segment")
		}
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." {
		return "", invalidKey(raw, "empty key")
	}
	return Key(cleaned), nil
}

// Join joins parts with "/" and cleans the result.
func Join(parts ...string) (Key, error) {
	return CleanKey(strings.Join(parts, "/"))
}

// CleanPrefix normalises a List prefix. "" (or "/", "./") lists everything; a
// trailing "/" is preserved so "a/" does not match "ab".
func CleanPrefix(raw string) (Key, error) {
	normalized := strings.ReplaceAll(raw, "\\", "/")
	if trimKeyLead(normalized) == "" {
		return "", nil
	}
	dir := strings.HasSuffix(normalized, "/")
	key, err := CleanKey(strings.TrimSuffix(normalized, "/"))
	if err != nil {
		return "", err
	}
	if dir {
		return key + "/", nil
	}
	return key, nil
}

func trimKeyLead(value string) string {
	for {
		switch {
		case strings.HasPrefix(value, "./"):
			value = value[2:]
		case strings.HasPrefix(value, "/"):
			value = value[1:]
		default:
			return value
		}
	}
}

func checkKeyText(raw, trimmed string) error {
	if trimmed == "" {
		return invalidKey(raw, "empty key")
	}
	if len(trimmed) > MaxKeyBytes {
		return invalidKey(raw, "longer than 1024 bytes")
	}
	for _, r := range trimmed {
		if r < 0x20 {
			return invalidKey(raw, "control character")
		}
	}
	return nil
}

func invalidKey(raw, reason string) error {
	if len(raw) > 64 {
		raw = raw[:64] + "..."
	}
	return fmt.Errorf("%w %q: %s", ErrInvalidKey, raw, reason)
}
