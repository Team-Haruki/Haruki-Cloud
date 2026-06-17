package sk

import (
	"strings"
	"unicode"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func hasRankInfoMetrics(info *drawing.RankInfo) bool {
	if info == nil {
		return false
	}
	return info.AveragePt != nil ||
		info.LatestPt != nil ||
		info.Speed != nil ||
		info.HourRound != nil ||
		info.Min20Time3Speed != nil
}

func (c *Controller) enrichRankInfoPreferUser(server string, eventID, rank int, userID int64, hasUserID bool, wlCharacterID *int, info *drawing.RankInfo) {
	if c == nil || info == nil {
		return
	}
	if hasUserID && userID > 0 {
		c.enrichRankInfoByUser(server, eventID, userID, wlCharacterID, info)
		if hasRankInfoMetrics(info) {
			return
		}
	}
	c.enrichRankInfoByRank(server, eventID, rank, wlCharacterID, info)
}

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
