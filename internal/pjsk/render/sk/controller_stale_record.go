package sk

import (
	"errors"
	"time"

	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

const staleSelfRecordThreshold = 5 * time.Minute

func (c *Controller) StaleSelfRecordWarning(req TrackerRankQuery, ranks []drawing.RankInfo) string {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return ""
	}
	if !c.shouldWarnStaleSelfRecord(normalized, ranks, time.Now().UTC()) {
		return ""
	}
	return i18n.T("sk.stale_self_record")
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
	if !isTrackerRankInfoStale(ranks[0], now, staleSelfRecordThreshold) {
		return false
	}
	if !c.isCurrentSelfRecordOutsideRecordRange(req) {
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

func (c *Controller) isCurrentSelfRecordOutsideRecordRange(req TrackerRankQuery) bool {
	if c == nil || req.UserID == nil || *req.UserID <= 0 {
		return false
	}
	source, ok := c.trackerCloudV2()
	if !ok || source == nil {
		return false
	}
	resp, err := source.GetCloudSKQuery(
		normalizeTrackerServer(req.Region),
		req.EventID,
		req.WlCharacterID,
		nil,
		req.UserID,
		false,
		false,
		3600,
	)
	if err != nil {
		return errors.Is(err, sekaiapi.ErrRankingNotFound)
	}
	if resp == nil || len(resp.Ranks) == 0 {
		return true
	}
	for _, item := range resp.Ranks {
		if item.Rank > 0 && cloudRankInfoMatchesUser(*req.UserID, item) {
			return false
		}
	}
	return true
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
	return upstreamerr.RankingStatusHealthy(int(status.Status), status.StatusDesc)
}
