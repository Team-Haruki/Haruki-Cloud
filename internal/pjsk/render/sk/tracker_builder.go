package sk

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/utils/drawing"
	sekaiapi "haruki-cloud/utils/sekai"
)

// trackerScoreSample holds a single score/timestamp pair for metrics calculation.
type trackerScoreSample struct {
	score     int
	timestamp int64
}

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
			if strings.EqualFold(eventInfo.EventType, "world_bloom") && normalized.WlCharacterID == nil {
				return TrackerRankQuery{}, fmt.Errorf("world bloom event requires wl_character_id")
			}
		}
	}
	return normalized, nil
}

func (c *Controller) buildRanksFromTracker(server string, eventID int, ranks []int, wlCharacterID *int) ([]drawing.RankInfo, error) {
	result := make([]drawing.RankInfo, 0, len(ranks))
	for _, rank := range ranks {
		info, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker rank %d query failed: %w", rank, err)
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Rank < result[j].Rank
	})
	return result, nil
}

func (c *Controller) buildRanksOrUserFromTracker(server string, eventID int, ranks []int, userID *int64, wlCharacterID *int) ([]drawing.RankInfo, error) {
	if userID != nil && *userID > 0 {
		info, err := c.buildSingleUserFromTracker(server, eventID, *userID, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker user %d query failed: %w", *userID, err)
		}
		return []drawing.RankInfo{info}, nil
	}
	return c.buildRanksFromTracker(server, eventID, ranks, wlCharacterID)
}

func (c *Controller) buildSingleRankFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil {
			return drawing.RankInfo{}, err
		}
		rankValue := rank
		if latest.RankData.Rank > 0 {
			rankValue = latest.RankData.Rank
		}
		score := latest.RankData.Score
		name := strings.TrimSpace(latest.UserData.Name)
		if name == "" {
			userID := strings.TrimSpace(latest.RankData.UserID)
			if userID == "" {
				userID = strings.TrimSpace(latest.UserData.UserID)
			}
			name = c.resolveTrackerNameByUserID(server, eventID, userID, wlCharacterID)
		}
		info := drawing.RankInfo{
			Rank:  rankValue,
			Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
			Score: drawing.IntPtr(score),
			Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
		}
		c.enrichRankInfoByRank(server, eventID, rankValue, wlCharacterID, &info)
		return info, nil
	}
	latest, err := c.tracker.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	rankValue := rank
	if latest.RankData.Rank > 0 {
		rankValue = latest.RankData.Rank
	}
	score := latest.RankData.Score
	name := strings.TrimSpace(latest.UserData.Name)
	if name == "" {
		userID := strings.TrimSpace(latest.RankData.UserID)
		if userID == "" {
			userID = strings.TrimSpace(latest.UserData.UserID)
		}
		name = c.resolveTrackerNameByUserID(server, eventID, userID, wlCharacterID)
	}
	info := drawing.RankInfo{
		Rank:  rankValue,
		Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
		Score: drawing.IntPtr(score),
		Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
	}
	c.enrichRankInfoByRank(server, eventID, rankValue, wlCharacterID, &info)
	return info, nil
}

func (c *Controller) resolveTrackerNameByUserID(server string, eventID int, userID string, wlCharacterID *int) string {
	uid, err := strconv.ParseInt(strings.TrimSpace(userID), 10, 64)
	if err != nil || uid <= 0 {
		return ""
	}
	userData, userErr := c.tracker.GetUserEventData(server, eventID, uid)
	if userErr == nil && userData != nil && strings.TrimSpace(userData.Name) != "" {
		return strings.TrimSpace(userData.Name)
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, latestErr := c.tracker.GetLatestWorldBloomRankingByUser(server, eventID, *wlCharacterID, uid)
		if latestErr == nil && latest != nil {
			return strings.TrimSpace(latest.UserData.Name)
		}
		return ""
	}
	latest, latestErr := c.tracker.GetLatestRankingByUser(server, eventID, uid)
	if latestErr == nil && latest != nil {
		return strings.TrimSpace(latest.UserData.Name)
	}
	return ""
}

func (c *Controller) buildSingleUserFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByUser(server, eventID, *wlCharacterID, userID)
		if err != nil {
			return drawing.RankInfo{}, err
		}
		rankValue := latest.RankData.Rank
		if rankValue <= 0 {
			rankValue = 1
		}
		score := latest.RankData.Score
		name := strings.TrimSpace(latest.UserData.Name)
		if name == "" {
			lookupUserID := strings.TrimSpace(latest.RankData.UserID)
			if lookupUserID == "" {
				lookupUserID = strconv.FormatInt(userID, 10)
			}
			name = c.resolveTrackerNameByUserID(server, eventID, lookupUserID, wlCharacterID)
		}
		info := drawing.RankInfo{
			Rank:  rankValue,
			Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
			Score: drawing.IntPtr(score),
			Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
		}
		c.enrichRankInfoByUser(server, eventID, userID, wlCharacterID, &info)
		return info, nil
	}
	latest, err := c.tracker.GetLatestRankingByUser(server, eventID, userID)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	rankValue := latest.RankData.Rank
	if rankValue <= 0 {
		rankValue = 1
	}
	score := latest.RankData.Score
	name := strings.TrimSpace(latest.UserData.Name)
	if name == "" {
		lookupUserID := strings.TrimSpace(latest.RankData.UserID)
		if lookupUserID == "" {
			lookupUserID = strconv.FormatInt(userID, 10)
		}
		name = c.resolveTrackerNameByUserID(server, eventID, lookupUserID, wlCharacterID)
	}
	info := drawing.RankInfo{
		Rank:  rankValue,
		Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
		Score: drawing.IntPtr(score),
		Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
	}
	c.enrichRankInfoByUser(server, eventID, userID, wlCharacterID, &info)
	return info, nil
}

func (c *Controller) enrichRankInfoByRank(server string, eventID, rank int, wlCharacterID *int, info *drawing.RankInfo) {
	if c == nil || c.tracker == nil || info == nil || rank <= 0 {
		return
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil || trace == nil {
			return
		}
		name := strings.TrimSpace(c.censorTrackerName(trace.UserData.Name, server))
		if name == "" {
			ids := make([]string, 0, len(trace.RankData)+1)
			ids = append(ids, trace.UserData.UserID)
			for _, point := range trace.RankData {
				ids = append(ids, point.UserID)
			}
			resolved := c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...)
			name = strings.TrimSpace(c.censorTrackerName(resolved, server))
		}
		if name != "" {
			info.Name = name
		}
		samples := make([]trackerScoreSample, 0, len(trace.RankData))
		for _, point := range trace.RankData {
			samples = append(samples, trackerScoreSample{
				score:     point.Score,
				timestamp: point.Timestamp,
			})
		}
		applyRankInfoMetrics(info, samples)
		return
	}

	trace, err := c.tracker.TraceRankingByRank(server, eventID, rank)
	if err != nil || trace == nil {
		return
	}
	name := strings.TrimSpace(c.censorTrackerName(trace.UserData.Name, server))
	if name == "" {
		ids := make([]string, 0, len(trace.RankData)+1)
		ids = append(ids, trace.UserData.UserID)
		for _, point := range trace.RankData {
			ids = append(ids, point.UserID)
		}
		resolved := c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...)
		name = strings.TrimSpace(c.censorTrackerName(resolved, server))
	}
	if name != "" {
		info.Name = name
	}
	samples := make([]trackerScoreSample, 0, len(trace.RankData))
	for _, point := range trace.RankData {
		samples = append(samples, trackerScoreSample{
			score:     point.Score,
			timestamp: point.Timestamp,
		})
	}
	applyRankInfoMetrics(info, samples)
}

func (c *Controller) enrichRankInfoByUser(server string, eventID int, userID int64, wlCharacterID *int, info *drawing.RankInfo) {
	if c == nil || c.tracker == nil || info == nil || userID <= 0 {
		return
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByUser(server, eventID, *wlCharacterID, userID)
		if err != nil || trace == nil {
			return
		}
		name := strings.TrimSpace(c.censorTrackerName(trace.UserData.Name, server))
		if name == "" {
			ids := make([]string, 0, len(trace.RankData)+2)
			ids = append(ids, strconv.FormatInt(userID, 10), trace.UserData.UserID)
			for _, point := range trace.RankData {
				ids = append(ids, point.UserID)
			}
			resolved := c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...)
			name = strings.TrimSpace(c.censorTrackerName(resolved, server))
		}
		if name != "" {
			info.Name = name
		}
		samples := make([]trackerScoreSample, 0, len(trace.RankData))
		for _, point := range trace.RankData {
			samples = append(samples, trackerScoreSample{
				score:     point.Score,
				timestamp: point.Timestamp,
			})
		}
		applyRankInfoMetrics(info, samples)
		return
	}

	trace, err := c.tracker.TraceRankingByUser(server, eventID, userID)
	if err != nil || trace == nil {
		return
	}
	name := strings.TrimSpace(c.censorTrackerName(trace.UserData.Name, server))
	if name == "" {
		ids := make([]string, 0, len(trace.RankData)+2)
		ids = append(ids, strconv.FormatInt(userID, 10), trace.UserData.UserID)
		for _, point := range trace.RankData {
			ids = append(ids, point.UserID)
		}
		resolved := c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...)
		name = strings.TrimSpace(c.censorTrackerName(resolved, server))
	}
	if name != "" {
		info.Name = name
	}
	samples := make([]trackerScoreSample, 0, len(trace.RankData))
	for _, point := range trace.RankData {
		samples = append(samples, trackerScoreSample{
			score:     point.Score,
			timestamp: point.Timestamp,
		})
	}
	applyRankInfoMetrics(info, samples)
}

func (c *Controller) resolveTrackerNameByUserIDs(server string, eventID int, wlCharacterID *int, userIDs ...string) string {
	if c == nil {
		return ""
	}
	seen := map[string]struct{}{}
	for _, raw := range userIDs {
		uid := strings.TrimSpace(raw)
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		if name := strings.TrimSpace(c.resolveTrackerNameByUserID(server, eventID, uid, wlCharacterID)); name != "" {
			return name
		}
	}
	return ""
}

func applyRankInfoMetrics(info *drawing.RankInfo, samples []trackerScoreSample) {
	if info == nil || len(samples) == 0 {
		return
	}

	normalized := make([]trackerScoreSample, 0, len(samples))
	for _, sample := range samples {
		if sample.timestamp <= 0 {
			continue
		}
		normalized = append(normalized, sample)
	}
	if len(normalized) == 0 {
		return
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalizeTrackerUnixSeconds(normalized[i].timestamp) < normalizeTrackerUnixSeconds(normalized[j].timestamp)
	})

	info.RecordStartAt = formatTrackerTimestamp(normalized[0].timestamp)
	if len(normalized) < 2 {
		return
	}

	deltas := make([]int, 0, len(normalized)-1)
	for i := 1; i < len(normalized); i++ {
		diff := normalized[i].score - normalized[i-1].score
		if diff > 0 {
			deltas = append(deltas, diff)
		}
	}
	if len(deltas) > 0 {
		latest := deltas[len(deltas)-1]
		info.LatestPt = drawing.IntPtr(latest)

		avgWindow := deltas
		if len(avgWindow) > 10 {
			avgWindow = avgWindow[len(avgWindow)-10:]
		}
		sum := 0
		for _, value := range avgWindow {
			sum += value
		}
		roundCount := len(avgWindow)
		if roundCount > 0 {
			avg := sum / roundCount
			info.AverageRound = drawing.IntPtr(roundCount)
			info.AveragePt = drawing.IntPtr(avg)
		}
	}

	first := normalized[0]
	last := normalized[len(normalized)-1]
	startSec := normalizeTrackerUnixSeconds(first.timestamp)
	endSec := normalizeTrackerUnixSeconds(last.timestamp)
	if endSec <= startSec {
		return
	}

	scoreGain := last.score - first.score
	elapsed := endSec - startSec
	if scoreGain > 0 {
		speed := int((int64(scoreGain) * 3600) / elapsed)
		if speed > 0 {
			info.Speed = drawing.IntPtr(speed)
		}
	}

	if len(deltas) > 0 {
		hourRound := int((int64(len(deltas)) * 3600) / elapsed)
		if hourRound > 0 {
			info.HourRound = drawing.IntPtr(hourRound)
		}
	}

	windowStart := endSec - 20*60
	baseIdx := 0
	for i := 0; i < len(normalized)-1; i++ {
		if normalizeTrackerUnixSeconds(normalized[i].timestamp) >= windowStart {
			baseIdx = i
			break
		}
		baseIdx = i + 1
	}
	base := normalized[baseIdx]
	baseSec := normalizeTrackerUnixSeconds(base.timestamp)
	if endSec > baseSec && last.score > base.score {
		windowSpeed := int((int64(last.score-base.score) * 3600) / (endSec - baseSec))
		if windowSpeed > 0 {
			info.Min20Time3Speed = drawing.IntPtr(windowSpeed)
		}
	}
}

func normalizeTrackerUnixSeconds(ts int64) int64 {
	if ts > 1_000_000_000_000 {
		return ts / 1000
	}
	return ts
}

func (c *Controller) buildSpeedInfosFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, interval int) ([]drawing.SpeedInfo, error) {
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
			result = append(result, speedInfoFromGrowthPoint(point))
			continue
		}
		info, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
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
	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil {
			return nil, fmt.Errorf("tracker trace rank %d query failed: %w", rank, err)
		}
		name := pickTrackerDisplayName(c.censorTrackerName(trace.UserData.Name, server), rank)
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
		name := pickTrackerDisplayName(c.censorTrackerName(trace.UserData.Name, server), rank)
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

func speedInfoFromGrowthPoint(point sekaiapi.ScoreGrowthPoint) drawing.SpeedInfo {
	var speed *int
	if point.Growth != nil && point.TimeDiff != nil && *point.TimeDiff > 0 {
		val := int((int64(*point.Growth) * 3600) / *point.TimeDiff)
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
