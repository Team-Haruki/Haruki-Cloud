package sk

import (
	"fmt"
	"strconv"

	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) buildRankTraceFromTracker(server string, eventID, rank int, wlCharacterID *int) ([]drawing.RankInfo, error) {
	if out, ok, err := c.buildSubjectTraceFromTrackerV2(server, eventID, "rank", strconv.Itoa(rank), wlCharacterID); ok {
		return out, err
	}
	return nil, fmt.Errorf("tracker cloud v2 source is not configured")
}
