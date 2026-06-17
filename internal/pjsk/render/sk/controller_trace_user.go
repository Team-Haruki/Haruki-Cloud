package sk

import (
	"fmt"

	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) buildUserTraceFromTracker(server string, eventID int, userID int64, wlCharacterID *int) ([]drawing.RankInfo, error) {
	if out, ok, err := c.buildSubjectTraceFromTrackerV2(server, eventID, "user", v2SubjectUserID(userID), wlCharacterID); ok {
		return out, err
	}
	return nil, fmt.Errorf("tracker cloud v2 source is not configured")
}
