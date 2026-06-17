package sk

import (
	"fmt"

	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) buildPlayerTraceByRankFromTracker(server string, eventID, rank int, wlCharacterID *int) ([]drawing.RankInfo, error) {
	userID, err := c.resolveTrackerUserIDByRank(server, eventID, rank, wlCharacterID)
	if err != nil {
		return nil, err
	}
	return c.buildUserTraceFromTracker(server, eventID, userID, wlCharacterID)
}

func (c *Controller) resolveTrackerUserIDByRank(server string, eventID, rank int, wlCharacterID *int) (int64, error) {
	uid, ok, err := c.resolveTrackerUserIDByRankFromCloudV2(server, eventID, rank, wlCharacterID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("tracker cloud v2 source is not configured")
	}
	return uid, nil
}
