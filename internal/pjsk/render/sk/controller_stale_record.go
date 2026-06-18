package sk

import (
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

const StaleSelfRecordWarning = "你已不在可记录范围内，仅展示最后被记录到的信息"

const staleSelfRecordThreshold = 5 * time.Minute

func (c *Controller) StaleSelfRecordWarning(req TrackerRankQuery, ranks []drawing.RankInfo) string {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return ""
	}
	if !c.shouldWarnStaleSelfRecord(normalized, ranks, time.Now().UTC()) {
		return ""
	}
	return StaleSelfRecordWarning
}

func (c *Controller) StaleSelfLatestRecordWarning(req TrackerRankQuery) string {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return ""
	}

	var (
		info drawing.RankInfo
	)
	switch {
	case normalized.UserID != nil && *normalized.UserID > 0:
		info, err = c.buildSingleUserFromTracker(normalized.Region, normalized.EventID, *normalized.UserID, normalized.WlCharacterID)
	case len(normalized.Ranks) == 1:
		info, err = c.buildSingleRankFromTracker(normalized.Region, normalized.EventID, normalized.Ranks[0], normalized.WlCharacterID)
	default:
		return ""
	}
	if err != nil {
		return ""
	}
	return c.StaleSelfRecordWarning(normalized, []drawing.RankInfo{info})
}

func (c *Controller) shouldWarnStaleSelfRecord(req TrackerRankQuery, ranks []drawing.RankInfo, now time.Time) bool {
	if c == nil || c.tracker == nil || len(ranks) == 0 {
		return false
	}
	if req.EventID <= 0 || normalizeTrackerServer(req.Region) == "" {
		return false
	}
	if ranks[0].Rank <= 100 {
		return false
	}
	if !isTrackerRankInfoStale(ranks[0], now, staleSelfRecordThreshold) {
		return false
	}

	statusSource, ok := c.tracker.(trackerEventStatusSource)
	if !ok || statusSource == nil {
		return false
	}
	status, err := statusSource.GetEventStatus(normalizeTrackerServer(req.Region), req.EventID)
	if err != nil {
		return false
	}
	return trackerEventStatusIsHealthy(status)
}

func isTrackerRankInfoStale(info drawing.RankInfo, now time.Time, threshold time.Duration) bool {
	if info.Time <= 0 || threshold <= 0 {
		return false
	}
	lastSec := normalizeTrackerUnixSeconds(info.Time)
	if lastSec <= 0 {
		return false
	}
	lastAt := time.Unix(lastSec, 0).UTC()
	return now.UTC().Sub(lastAt) > threshold
}

func trackerEventStatusIsHealthy(status *sekaiapi.EventStatusResponse) bool {
	if status == nil {
		return false
	}
	desc := strings.ToLower(strings.TrimSpace(status.StatusDesc))
	if desc != "" {
		switch {
		case strings.Contains(desc, "error"),
			strings.Contains(desc, "fail"),
			strings.Contains(desc, "down"),
			strings.Contains(desc, "maintenance"),
			strings.Contains(desc, "timeout"),
			strings.Contains(desc, "异常"),
			strings.Contains(desc, "错误"),
			strings.Contains(desc, "失败"),
			strings.Contains(desc, "维护"),
			strings.Contains(desc, "超时"),
			strings.Contains(desc, "不可用"):
			return false
		case strings.Contains(desc, "ok"),
			strings.Contains(desc, "normal"),
			strings.Contains(desc, "healthy"),
			strings.Contains(desc, "running"),
			strings.Contains(desc, "success"),
			strings.Contains(desc, "available"),
			strings.Contains(desc, "active"),
			strings.Contains(desc, "正常"),
			strings.Contains(desc, "可用"),
			strings.Contains(desc, "运行"),
			strings.Contains(desc, "成功"):
			return true
		}
	}
	return status.Status == 0 || status.Status == 1
}
