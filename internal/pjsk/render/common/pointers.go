package common

import "strings"

// CloneStringPtr returns a copy of a *string, or nil if the input is nil.
func CloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	return new(*value)
}

// CloneIntPtr returns a copy of a *int, or nil if the input is nil.
func CloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	return new(*value)
}

// PositiveIntPtr returns a pointer to value when it is positive, or nil.
func PositiveIntPtr(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

// BoolPtr returns a pointer to the given bool value.
func BoolPtr(value bool) *bool {
	return &value
}

// OptionalString returns a pointer to s (trimmed) if non-empty, or nil.
func OptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
