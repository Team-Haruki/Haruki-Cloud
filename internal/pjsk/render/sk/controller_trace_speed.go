package sk

import (
	"fmt"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func (c *Controller) buildSpeedInfosFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, interval int, unitPeriodSeconds int64, skipMissing bool) ([]drawing.SpeedInfo, error) {
	if out, ok, err := c.buildSpeedInfosFromTrackerV2(server, eventID, ranks, wlCharacterID, interval, unitPeriodSeconds, skipMissing); ok {
		return out, err
	}
	return nil, fmt.Errorf("tracker cloud v2 source is not configured")
}

func speedInfoFromGrowthPoint(point sekaiapi.ScoreGrowthPoint, unitPeriodSeconds int64) drawing.SpeedInfo {
	var speed *int
	if unitPeriodSeconds <= 0 {
		unitPeriodSeconds = 60 * 60
	}

	growth := point.Growth
	if growth == nil && point.ScoreEarlier != nil {
		val := point.ScoreLatest - *point.ScoreEarlier
		if val >= 0 {
			growth = &val
		}
	}

	timeDiff := point.TimeDiff
	if (timeDiff == nil || *timeDiff <= 0) && point.TimestampEarlier != nil {
		latest := normalizeTrackerUnixSeconds(point.TimestampLatest)
		earlier := normalizeTrackerUnixSeconds(*point.TimestampEarlier)
		diff := latest - earlier
		if diff > 0 {
			timeDiff = &diff
		}
	}

	if growth != nil && *growth >= 0 && timeDiff != nil && *timeDiff > 0 {
		speed = new(int((int64(*growth) * unitPeriodSeconds) / *timeDiff))
	}
	score := point.ScoreLatest
	if score <= 0 && point.ScoreEarlier != nil {
		score = *point.ScoreEarlier
	}
	recordTs := point.TimestampLatest
	if recordTs <= 0 && point.TimestampEarlier != nil {
		recordTs = *point.TimestampEarlier
	}
	return drawing.SpeedInfo{
		Rank:       point.Rank,
		Score:      score,
		Speed:      speed,
		RecordTime: formatTrackerTimestamp(recordTs),
	}
}
