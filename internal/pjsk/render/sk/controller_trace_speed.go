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
	if unitPeriodSeconds <= 0 {
		unitPeriodSeconds = 60 * 60
	}
	growth := trackerPointGrowth(point)
	timeDiff := trackerPointTimeDiff(point)
	var speed *int
	if growth != nil && *growth >= 0 && timeDiff != nil && *timeDiff > 0 {
		speed = new(int((int64(*growth) * unitPeriodSeconds) / *timeDiff))
	}
	return drawing.SpeedInfo{
		Rank:       point.Rank,
		Score:      latestTrackerPointScore(point),
		Speed:      speed,
		RecordTime: formatTrackerTimestamp(latestTrackerPointTimestamp(point)),
	}
}

func trackerPointGrowth(point sekaiapi.ScoreGrowthPoint) *int {
	if point.Growth != nil || point.ScoreEarlier == nil {
		return point.Growth
	}
	growth := point.ScoreLatest - *point.ScoreEarlier
	if growth < 0 {
		return nil
	}
	return &growth
}

func trackerPointTimeDiff(point sekaiapi.ScoreGrowthPoint) *int64 {
	if point.TimeDiff != nil && *point.TimeDiff > 0 || point.TimestampEarlier == nil {
		return point.TimeDiff
	}
	diff := normalizeTrackerUnixSeconds(point.TimestampLatest) - normalizeTrackerUnixSeconds(*point.TimestampEarlier)
	if diff <= 0 {
		return point.TimeDiff
	}
	return &diff
}

func latestTrackerPointScore(point sekaiapi.ScoreGrowthPoint) int {
	if point.ScoreLatest > 0 || point.ScoreEarlier == nil {
		return point.ScoreLatest
	}
	return *point.ScoreEarlier
}

func latestTrackerPointTimestamp(point sekaiapi.ScoreGrowthPoint) int64 {
	if point.TimestampLatest > 0 || point.TimestampEarlier == nil {
		return point.TimestampLatest
	}
	return *point.TimestampEarlier
}
