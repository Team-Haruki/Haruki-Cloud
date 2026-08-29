package sk

import (
	"strings"
	"unicode"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) eventTitleForNameCheck(server string, eventID int) string {
	if c == nil || eventID <= 0 {
		return ""
	}
	region := renderregion.Normalize(server)
	if region.IsZero() {
		region = renderregion.JP
	}
	eventSource := c.eventSourceForRegion(region.String())
	if eventSource == nil {
		return ""
	}
	eventInfo, err := eventSource.GetEventByID(eventID)
	if err != nil || eventInfo == nil {
		return ""
	}
	return strings.TrimSpace(eventInfo.Name)
}

func (c *Controller) isTrackerEventTitleName(server string, eventID int, name string) bool {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return false
	}
	meta := strings.TrimSpace(c.eventTitleForNameCheck(server, eventID))
	if meta == "" {
		return false
	}
	return isTrackerEventTitleFuzzyMatch(clean, meta)
}

func isTrackerEventTitleFuzzyMatch(name, eventTitle string) bool {
	left := normalizeTrackerNameForCompare(name)
	right := normalizeTrackerNameForCompare(eventTitle)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	if len([]rune(left)) >= 6 && strings.Contains(left, right) {
		return true
	}
	if len([]rune(right)) >= 6 && strings.Contains(right, left) {
		return true
	}
	return false
}

func normalizeTrackerNameForCompare(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range strings.ToLower(raw) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
