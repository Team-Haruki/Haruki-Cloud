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
