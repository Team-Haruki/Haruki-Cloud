package sk

import (
	"strconv"
	"strings"
	"unicode"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) resolveTrackerNameByUserIDs(server string, eventID int, wlCharacterID *int, userIDs ...string) string {
	if c == nil {
		return ""
	}
	seen := map[string]struct{}{}
	for _, raw := range userIDs {
		uid := strings.TrimSpace(raw)
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		if name := strings.TrimSpace(c.resolveTrackerNameByUserID(server, eventID, uid, wlCharacterID)); name != "" {
			return name
		}
	}
	return ""
}

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

func (c *Controller) shouldReplaceTrackerName(server string, eventID int, current, candidate string) bool {
	next := strings.TrimSpace(candidate)
	if next == "" {
		return false
	}
	if c.shouldResolveTrackerNameByUser(server, eventID, next) {
		return false
	}
	prev := strings.TrimSpace(current)
	if prev == "" {
		return true
	}
	if c.shouldResolveTrackerNameByUser(server, eventID, prev) {
		return true
	}
	return isTrackerPlaceholderName(prev) && !isTrackerPlaceholderName(next)
}

func isTrackerPlaceholderName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "rank ") {
		return false
	}
	_, err := strconv.Atoi(strings.TrimSpace(trimmed[len("rank "):]))
	return err == nil
}

func (c *Controller) shouldResolveTrackerNameByUser(server string, eventID int, name string) bool {
	clean := strings.TrimSpace(name)
	if clean == "" || isTrackerPlaceholderName(clean) {
		return true
	}
	return c.isTrackerEventTitleName(server, eventID, clean)
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

func (c *Controller) pickTrackerResolvedName(server string, eventID int, candidates ...string) string {
	for _, raw := range candidates {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if c.shouldResolveTrackerNameByUser(server, eventID, name) {
			continue
		}
		return name
	}
	return ""
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
