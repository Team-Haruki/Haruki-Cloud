package sk

import (
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func (c *Controller) validateTrackerQuery(req TrackerRankQuery) (TrackerRankQuery, error) {
	if c == nil {
		return TrackerRankQuery{}, fmt.Errorf("sk controller is not initialized")
	}
	if c.tracker == nil {
		return TrackerRankQuery{}, fmt.Errorf("tracker client is not configured")
	}
	normalized := req
	normalized.Region = normalizeTrackerServer(req.Region)
	if normalized.Region == "" {
		return TrackerRankQuery{}, fmt.Errorf("region must be one of: jp/cn/tw/kr/en")
	}
	normalized.Ranks = normalizeRanks(req.Ranks)
	if normalized.UserID != nil && *normalized.UserID <= 0 {
		normalized.UserID = nil
	}
	if len(normalized.Ranks) == 0 && normalized.UserID == nil {
		return TrackerRankQuery{}, fmt.Errorf("tracker ranks/user_id are empty")
	}
	if normalized.EventID <= 0 {
		normalized.EventID = c.pickCurrentOrNextEventID(normalized.Region)
	}
	if normalized.EventID <= 0 {
		return TrackerRankQuery{}, fmt.Errorf("event_id is required when no current event can be inferred")
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID <= 0 {
		normalized.WlCharacterID = nil
	}
	if eventSource := c.eventSourceForRegion(normalized.Region); eventSource != nil {
		if eventInfo, err := eventSource.GetEventByID(normalized.EventID); err == nil && eventInfo != nil {
			if !strings.EqualFold(eventInfo.EventType, "world_bloom") && normalized.WlCharacterID != nil {
				return TrackerRankQuery{}, fmt.Errorf("wl_character_id is only valid for world bloom event")
			}
		}
	}
	return normalized, nil
}

func shouldSkipMissingTrackerRanks(req TrackerRankQuery) bool {
	return req.DefaultRanks && req.UserID == nil
}

func normalizeTrackerSpeedConfig(req TrackerRankQuery) (periodSeconds int64, unitPeriodSeconds int64, unitText string) {
	unit := strings.ToLower(strings.TrimSpace(req.SpeedUnit))
	switch unit {
	case "d", "day", "daily", "日":
		unitPeriodSeconds = 24 * 60 * 60
		unitText = "日"
	default:
		unitPeriodSeconds = 60 * 60
		unitText = "时"
	}

	periodSeconds = req.SpeedPeriodSecs
	if periodSeconds <= 0 {
		periodSeconds = unitPeriodSeconds
	}

	return periodSeconds, unitPeriodSeconds, unitText
}

func shouldSkipMissingTrackerRankError(skipMissing bool, err error) bool {
	return skipMissing && errors.Is(err, sekaiapi.ErrRankingNotFound)
}

func (c *Controller) buildRanksFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, error) {
	if len(ranks) == 0 {
		return nil, nil
	}
	if out, _, _, ok, err := c.buildRanksFromTrackerV2(server, eventID, ranks, wlCharacterID, false, skipMissing); ok {
		return out, err
	}
	return nil, fmt.Errorf("tracker cloud v2 source is not configured")
}

func (c *Controller) buildRanksOrUserFromTracker(server string, eventID int, ranks []int, userID *int64, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, error) {
	if userID != nil && *userID > 0 {
		info, err := c.buildSingleUserFromTracker(server, eventID, *userID, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker user query failed: %w", err)
		}
		return []drawing.RankInfo{info}, nil
	}
	return c.buildRanksFromTracker(server, eventID, ranks, wlCharacterID, skipMissing)
}
