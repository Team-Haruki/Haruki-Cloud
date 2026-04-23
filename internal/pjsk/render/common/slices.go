package common

import "strings"

// ContainsString reports whether values contains target (case-insensitive).
func ContainsString(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}
