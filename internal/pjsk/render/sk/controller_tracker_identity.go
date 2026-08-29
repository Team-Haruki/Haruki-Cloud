package sk

import (
	"fmt"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func (c *Controller) buildSingleRankFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, error) {
	info, _, _, err := c.buildSingleRankBaseFromTracker(server, eventID, rank, wlCharacterID)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	if c.isTrackerEventTitleName(server, eventID, info.Name) {
		info.Name = fmt.Sprintf("Rank %d", info.Rank)
	}
	return info, nil
}

func (c *Controller) buildSingleRankLatestFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, error) {
	info, _, _, err := c.buildSingleRankBaseFromTracker(server, eventID, rank, wlCharacterID)
	return info, err
}

func (c *Controller) buildSingleRankBaseFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, int64, bool, error) {
	infos, _, _, ok, err := c.buildRanksFromTrackerV2(server, eventID, []int{rank}, wlCharacterID, false, false)
	if !ok {
		return drawing.RankInfo{}, 0, false, fmt.Errorf("tracker cloud v2 source is not configured")
	}
	if err != nil {
		return drawing.RankInfo{}, 0, false, err
	}
	if len(infos) == 0 {
		return drawing.RankInfo{}, 0, false, sekaiapi.ErrRankingNotFound
	}
	info := infos[0]
	uid, hasUID, uidErr := c.resolveTrackerUserIDByRankFromCloudV2(server, eventID, info.Rank, wlCharacterID)
	if uidErr != nil {
		return drawing.RankInfo{}, 0, false, uidErr
	}
	return info, uid, hasUID, nil
}

func (c *Controller) buildSingleUserFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	return c.buildSingleUserBaseFromTracker(server, eventID, userID, wlCharacterID)
}

func (c *Controller) buildSingleUserBaseFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	if info, ok, err := c.buildUserFromTrackerCloudV2(server, eventID, userID, wlCharacterID); ok {
		if err != nil {
			return drawing.RankInfo{}, err
		}
		return info, nil
	}
	return drawing.RankInfo{}, fmt.Errorf("tracker cloud v2 source is not configured")
}
