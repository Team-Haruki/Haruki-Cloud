package sk

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/utils/drawing"
	sekaiapi "haruki-cloud/utils/sekai"
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

func (c *Controller) buildUserTraceFromTracker(server string, eventID int, userID int64, wlCharacterID *int) ([]drawing.RankInfo, error) {
	result := make([]drawing.RankInfo, 0)
	latestName := ""
	if latest, err := c.buildSingleUserFromTracker(server, eventID, userID, wlCharacterID); err == nil {
		latestName = strings.TrimSpace(latest.Name)
	}

	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByUser(server, eventID, *wlCharacterID, userID)
		if err != nil {
			return nil, fmt.Errorf("tracker trace user query failed: %w", err)
		}
		if trace == nil {
			return nil, fmt.Errorf("tracker trace user returned empty data")
		}
		ids := make([]string, 0, len(trace.RankData)+2)
		ids = append(ids, strconv.FormatInt(userID, 10), trace.UserData.UserID)
		for _, point := range trace.RankData {
			ids = append(ids, point.UserID)
		}
		resolvedName := strings.TrimSpace(c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...))
		name := strings.TrimSpace(trace.UserData.Name)
		if c.shouldResolveTrackerNameByUser(server, eventID, name) && resolvedName != "" {
			name = resolvedName
		}
		name = strings.TrimSpace(c.censorTrackerName(name, server))
		if latestName != "" {
			name = latestName
		}
		if name == "" || c.isTrackerEventTitleName(server, eventID, name) {
			name = strconv.FormatInt(userID, 10)
		}
		for _, point := range trace.RankData {
			result = append(result, drawing.RankInfo{
				Rank:  point.Rank,
				Name:  name,
				Score: drawing.IntPtr(point.Score),
				Time:  formatTrackerTimestamp(point.Timestamp),
			})
		}
	} else {
		trace, err := c.tracker.TraceRankingByUser(server, eventID, userID)
		if err != nil {
			return nil, fmt.Errorf("tracker trace user query failed: %w", err)
		}
		if trace == nil {
			return nil, fmt.Errorf("tracker trace user returned empty data")
		}
		ids := make([]string, 0, len(trace.RankData)+2)
		ids = append(ids, strconv.FormatInt(userID, 10), trace.UserData.UserID)
		for _, point := range trace.RankData {
			ids = append(ids, point.UserID)
		}
		resolvedName := strings.TrimSpace(c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...))
		name := strings.TrimSpace(trace.UserData.Name)
		if c.shouldResolveTrackerNameByUser(server, eventID, name) && resolvedName != "" {
			name = resolvedName
		}
		name = strings.TrimSpace(c.censorTrackerName(name, server))
		if latestName != "" {
			name = latestName
		}
		if name == "" || c.isTrackerEventTitleName(server, eventID, name) {
			name = strconv.FormatInt(userID, 10)
		}
		for _, point := range trace.RankData {
			result = append(result, drawing.RankInfo{
				Rank:  point.Rank,
				Name:  name,
				Score: drawing.IntPtr(point.Score),
				Time:  formatTrackerTimestamp(point.Timestamp),
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return fmt.Sprintf("%v", result[i].Time) < fmt.Sprintf("%v", result[j].Time)
	})
	if len(result) == 0 {
		latest, err := c.buildSingleUserFromTracker(server, eventID, userID, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker trace fallback user query failed: %w", err)
		}
		return []drawing.RankInfo{latest}, nil
	}
	return result, nil
}

func (c *Controller) buildSpeedInfosFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, interval int, unitPeriodSeconds int64, skipMissing bool) ([]drawing.SpeedInfo, error) {
	var (
		points []sekaiapi.ScoreGrowthPoint
		err    error
	)
	if wlCharacterID != nil && *wlCharacterID > 0 {
		points, err = c.tracker.GetWorldBloomRankingScoreGrowth(server, eventID, *wlCharacterID, interval)
	} else {
		points, err = c.tracker.GetRankingScoreGrowth(server, eventID, interval)
	}
	if err != nil {
		points = nil
	}
	pointByRank := make(map[int]sekaiapi.ScoreGrowthPoint, len(points))
	for _, point := range points {
		if point.Rank <= 0 {
			continue
		}
		existing, ok := pointByRank[point.Rank]
		if !ok || point.TimestampLatest > existing.TimestampLatest {
			pointByRank[point.Rank] = point
		}
	}

	result := make([]drawing.SpeedInfo, 0, len(ranks))
	for _, rank := range ranks {
		if point, ok := pointByRank[rank]; ok {
			info := speedInfoFromGrowthPoint(point, unitPeriodSeconds)
			if info.Speed == nil {
				if traceInfo, traceOK := c.buildSpeedInfoFromTrace(server, eventID, rank, wlCharacterID, interval, unitPeriodSeconds); traceOK {
					// Prefer score/time from growth endpoint when present because it
					// reflects the tracker speed aggregation point used by drawing.
					if info.Score > 0 {
						traceInfo.Score = info.Score
					}
					if point.TimestampLatest > 0 {
						traceInfo.RecordTime = formatTrackerTimestamp(point.TimestampLatest)
					}
					result = append(result, traceInfo)
					continue
				}
			}
			result = append(result, info)
			continue
		}
		if traceInfo, traceOK := c.buildSpeedInfoFromTrace(server, eventID, rank, wlCharacterID, interval, unitPeriodSeconds); traceOK {
			result = append(result, traceInfo)
			continue
		}
		info, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
			if shouldSkipMissingTrackerRankError(skipMissing, err) {
				continue
			}
			return nil, fmt.Errorf("tracker speed rank %d query failed: %w", rank, err)
		}
		score := 0
		if info.Score != nil {
			score = *info.Score
		}
		result = append(result, drawing.SpeedInfo{
			Rank:       info.Rank,
			Score:      score,
			RecordTime: info.Time,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Rank < result[j].Rank
	})
	return result, nil
}

func (c *Controller) buildRankTraceFromTracker(server string, eventID, rank int, wlCharacterID *int) ([]drawing.RankInfo, error) {
	result := make([]drawing.RankInfo, 0)
	latestName := ""
	if latest, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID); err == nil {
		latestName = strings.TrimSpace(latest.Name)
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil {
			return nil, fmt.Errorf("tracker trace rank %d query failed: %w", rank, err)
		}
		ids := make([]string, 0, len(trace.RankData)+1)
		ids = append(ids, trace.UserData.UserID)
		for _, point := range trace.RankData {
			ids = append(ids, point.UserID)
		}
		resolvedName := strings.TrimSpace(c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...))
		name := strings.TrimSpace(trace.UserData.Name)
		if c.shouldResolveTrackerNameByUser(server, eventID, name) && resolvedName != "" {
			name = resolvedName
		}
		name = pickTrackerDisplayName(c.censorTrackerName(name, server), rank)
		if c.isTrackerEventTitleName(server, eventID, name) && resolvedName != "" {
			name = strings.TrimSpace(c.censorTrackerName(resolvedName, server))
		}
		if c.isTrackerEventTitleName(server, eventID, name) {
			name = fmt.Sprintf("Rank %d", rank)
		}
		if latestName != "" {
			name = latestName
		}
		for _, point := range trace.RankData {
			rankValue := rank
			if point.Rank > 0 {
				rankValue = point.Rank
			}
			score := point.Score
			result = append(result, drawing.RankInfo{
				Rank:  rankValue,
				Name:  name,
				Score: drawing.IntPtr(score),
				Time:  formatTrackerTimestamp(point.Timestamp),
			})
		}
	} else {
		trace, err := c.tracker.TraceRankingByRank(server, eventID, rank)
		if err != nil {
			return nil, fmt.Errorf("tracker trace rank %d query failed: %w", rank, err)
		}
		ids := make([]string, 0, len(trace.RankData)+1)
		ids = append(ids, trace.UserData.UserID)
		for _, point := range trace.RankData {
			ids = append(ids, point.UserID)
		}
		resolvedName := strings.TrimSpace(c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...))
		name := strings.TrimSpace(trace.UserData.Name)
		if c.shouldResolveTrackerNameByUser(server, eventID, name) && resolvedName != "" {
			name = resolvedName
		}
		name = pickTrackerDisplayName(c.censorTrackerName(name, server), rank)
		if c.isTrackerEventTitleName(server, eventID, name) && resolvedName != "" {
			name = strings.TrimSpace(c.censorTrackerName(resolvedName, server))
		}
		if c.isTrackerEventTitleName(server, eventID, name) {
			name = fmt.Sprintf("Rank %d", rank)
		}
		if latestName != "" {
			name = latestName
		}
		for _, point := range trace.RankData {
			rankValue := rank
			if point.Rank > 0 {
				rankValue = point.Rank
			}
			score := point.Score
			result = append(result, drawing.RankInfo{
				Rank:  rankValue,
				Name:  name,
				Score: drawing.IntPtr(score),
				Time:  formatTrackerTimestamp(point.Timestamp),
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		ti := fmt.Sprintf("%v", result[i].Time)
		tj := fmt.Sprintf("%v", result[j].Time)
		return ti < tj
	})
	if len(result) == 0 {
		latest, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker trace fallback rank %d query failed: %w", rank, err)
		}
		return []drawing.RankInfo{latest}, nil
	}
	return result, nil
}

func (c *Controller) buildSpeedInfoFromTrace(server string, eventID, rank int, wlCharacterID *int, interval int, unitPeriodSeconds int64) (drawing.SpeedInfo, bool) {
	if c == nil || c.tracker == nil || rank <= 0 {
		return drawing.SpeedInfo{}, false
	}
	if interval <= 0 {
		interval = 60 * 60
	}
	if unitPeriodSeconds <= 0 {
		unitPeriodSeconds = 60 * 60
	}
	samples := make([]trackerRankScoreSample, 0)
	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil || trace == nil {
			return drawing.SpeedInfo{}, false
		}
		samples = make([]trackerRankScoreSample, 0, len(trace.RankData))
		for _, point := range trace.RankData {
			if point.Timestamp <= 0 {
				continue
			}
			rankValue := rank
			if point.Rank > 0 {
				rankValue = point.Rank
			}
			samples = append(samples, trackerRankScoreSample{
				rank:      rankValue,
				score:     point.Score,
				timestamp: point.Timestamp,
			})
		}
	} else {
		trace, err := c.tracker.TraceRankingByRank(server, eventID, rank)
		if err != nil || trace == nil {
			return drawing.SpeedInfo{}, false
		}
		samples = make([]trackerRankScoreSample, 0, len(trace.RankData))
		for _, point := range trace.RankData {
			if point.Timestamp <= 0 {
				continue
			}
			rankValue := rank
			if point.Rank > 0 {
				rankValue = point.Rank
			}
			samples = append(samples, trackerRankScoreSample{
				rank:      rankValue,
				score:     point.Score,
				timestamp: point.Timestamp,
			})
		}
	}
	if len(samples) == 0 {
		return drawing.SpeedInfo{}, false
	}

	sort.Slice(samples, func(i, j int) bool {
		return normalizeTrackerUnixSeconds(samples[i].timestamp) < normalizeTrackerUnixSeconds(samples[j].timestamp)
	})
	last := samples[len(samples)-1]
	info := drawing.SpeedInfo{
		Rank:       last.rank,
		Score:      last.score,
		RecordTime: formatTrackerTimestamp(last.timestamp),
	}
	if len(samples) < 2 {
		return info, true
	}

	endSec := normalizeTrackerUnixSeconds(last.timestamp)
	windowStart := endSec - int64(interval)
	baseIdx := 0
	for i := range samples {
		sec := normalizeTrackerUnixSeconds(samples[i].timestamp)
		if sec <= windowStart {
			baseIdx = i
			continue
		}
		break
	}
	base := samples[baseIdx]
	baseSec := normalizeTrackerUnixSeconds(base.timestamp)
	if endSec > baseSec && last.score > base.score {
		speed := int((int64(last.score-base.score) * unitPeriodSeconds) / (endSec - baseSec))
		if speed > 0 {
			info.Speed = drawing.IntPtr(speed)
		}
	}
	return info, true
}

func speedInfoFromGrowthPoint(point sekaiapi.ScoreGrowthPoint, unitPeriodSeconds int64) drawing.SpeedInfo {
	var speed *int
	if unitPeriodSeconds <= 0 {
		unitPeriodSeconds = 60 * 60
	}

	growth := point.Growth
	if (growth == nil || *growth <= 0) && point.ScoreEarlier != nil {
		val := point.ScoreLatest - *point.ScoreEarlier
		if val > 0 {
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

	if growth != nil && *growth > 0 && timeDiff != nil && *timeDiff > 0 {
		val := int((int64(*growth) * unitPeriodSeconds) / *timeDiff)
		speed = &val
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
