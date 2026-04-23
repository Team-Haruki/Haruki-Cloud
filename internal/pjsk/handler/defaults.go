package handler

import (
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
)

// DefaultRegionStr is the default region string used when no region is specified.
const DefaultRegionStr = string(renderregion.JP)

// regionWithDefault returns the region string, defaulting to "jp" if empty.
func regionWithDefault(region string) string {
	s := strings.ToLower(strings.TrimSpace(region))
	if s == "" {
		return DefaultRegionStr
	}
	return s
}

// maskPJSKUID masks the middle digits of a PJSK user ID when visible is false.
// Shows first 3 and last 3 digits with asterisks in between.
func maskPJSKUID(uid string, visible bool) string {
	if visible || len(uid) <= 6 {
		return uid
	}
	return uid[:3] + strings.Repeat("*", len(uid)-6) + uid[len(uid)-3:]
}
