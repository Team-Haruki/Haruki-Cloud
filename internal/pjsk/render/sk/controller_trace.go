package sk

import (
	"fmt"

	"haruki-cloud/utils/drawing"
)

func (c *Controller) buildPlayerTraceByRankFromTracker(server string, eventID, rank int, wlCharacterID *int) ([]drawing.RankInfo, error) {
	userID, err := c.resolveTrackerUserIDByRank(server, eventID, rank, wlCharacterID)
	if err != nil {
		return nil, err
	}
	return c.buildUserTraceFromTracker(server, eventID, userID, wlCharacterID)
}

func (c *Controller) resolveTrackerUserIDByRank(server string, eventID, rank int, wlCharacterID *int) (int64, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil {
			return 0, fmt.Errorf("tracker rank %d query failed: %w", rank, err)
		}
		uid, ok := parseTrackerUserID(latest.RankData.UserID, latest.UserData.UserID)
		if !ok {
			return 0, fmt.Errorf("tracker rank %d latest user id is empty", rank)
		}
		return uid, nil
	}
	latest, err := c.tracker.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return 0, fmt.Errorf("tracker rank %d query failed: %w", rank, err)
	}
	uid, ok := parseTrackerUserID(latest.RankData.UserID, latest.UserData.UserID)
	if !ok {
		return 0, fmt.Errorf("tracker rank %d latest user id is empty", rank)
	}
	return uid, nil
}
