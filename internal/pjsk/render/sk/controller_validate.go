package sk

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/utils/drawing"
	sekaiapi "haruki-cloud/utils/sekai"
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

func normalizeTrackerSpeedConfig(req TrackerRankQuery) (periodSeconds int64, unitText string) {
	unit := strings.ToLower(strings.TrimSpace(req.SpeedUnit))
	switch unit {
	case "d", "day", "daily", "日":
		unitText = "日"
	default:
		unitText = "时"
	}

	periodSeconds = req.SpeedPeriodSecs
	if periodSeconds <= 0 {
		if unitText == "日" {
			periodSeconds = 24 * 60 * 60
		} else {
			periodSeconds = 60 * 60
		}
	}

	return periodSeconds, unitText
}

func shouldSkipMissingTrackerRankError(skipMissing bool, err error) bool {
	return skipMissing && errors.Is(err, sekaiapi.ErrRankingNotFound)
}

func (c *Controller) buildRanksFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, error) {
	result := make([]drawing.RankInfo, 0, len(ranks))
	for _, rank := range ranks {
		info, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
			if shouldSkipMissingTrackerRankError(skipMissing, err) {
				continue
			}
			return nil, fmt.Errorf("tracker rank %d query failed: %w", rank, err)
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Rank < result[j].Rank
	})
	return result, nil
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
