package sk

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func (c *Controller) trackerCloudV2() (trackerCloudV2Source, bool) {
	if c == nil || c.tracker == nil {
		return nil, false
	}
	source, ok := c.tracker.(trackerCloudV2Source)
	return source, ok
}

func (c *Controller) buildRanksFromTrackerV2(server string, eventID int, ranks []int, wlCharacterID *int, includeAdjacent bool, skipMissing bool) ([]drawing.RankInfo, *drawing.RankInfo, *drawing.RankInfo, bool, error) {
	source, ok := c.trackerCloudV2()
	if !ok || len(ranks) == 0 {
		return nil, nil, nil, false, nil
	}
	resp, err := source.GetCloudSKQuery(server, eventID, wlCharacterID, ranks, nil, includeAdjacent, skipMissing, 3600)
	if err != nil {
		return nil, nil, nil, true, err
	}
	out := make([]drawing.RankInfo, 0, len(resp.Ranks))
	for _, item := range resp.Ranks {
		out = append(out, rankInfoFromCloudV2(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Rank < out[j].Rank
	})
	var previous *drawing.RankInfo
	if resp.Previous != nil {
		info := rankInfoFromCloudV2(*resp.Previous)
		previous = &info
	}
	var next *drawing.RankInfo
	if resp.Next != nil {
		info := rankInfoFromCloudV2(*resp.Next)
		next = &info
	}
	return out, previous, next, true, nil
}

func (c *Controller) buildUserFromTrackerCloudV2(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, bool, error) {
	infos, _, _, ok, err := c.buildUserQueryFromTrackerV2(server, eventID, userID, wlCharacterID, false)
	if !ok || err != nil {
		return drawing.RankInfo{}, ok, err
	}
	if len(infos) == 0 {
		return drawing.RankInfo{}, true, sekaiapi.ErrRankingNotFound
	}
	return infos[0], true, nil
}

func (c *Controller) buildUserQueryFromTrackerV2(server string, eventID int, userID int64, wlCharacterID *int, includeAdjacent bool) ([]drawing.RankInfo, *drawing.RankInfo, *drawing.RankInfo, bool, error) {
	source, ok := c.trackerCloudV2()
	if !ok || userID <= 0 {
		return nil, nil, nil, false, nil
	}
	resp, err := source.GetCloudSKQuery(server, eventID, wlCharacterID, nil, &userID, includeAdjacent, false, 3600)
	if err != nil {
		return nil, nil, nil, true, err
	}
	if resp == nil || len(resp.Ranks) == 0 {
		return nil, nil, nil, true, sekaiapi.ErrRankingNotFound
	}
	out := make([]drawing.RankInfo, 0, len(resp.Ranks))
	for _, item := range resp.Ranks {
		out = append(out, rankInfoFromCloudV2(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Rank < out[j].Rank
	})
	var previous *drawing.RankInfo
	if resp.Previous != nil {
		info := rankInfoFromCloudV2(*resp.Previous)
		previous = &info
	}
	var next *drawing.RankInfo
	if resp.Next != nil {
		info := rankInfoFromCloudV2(*resp.Next)
		next = &info
	}
	return out, previous, next, true, nil
}

func (c *Controller) buildCheckRoomFromTrackerCloudV2(server string, eventID int, ranks []int, userID *int64, wlCharacterID *int, skipMissing bool) (drawing.RankInfo, *drawing.RankInfo, *drawing.RankInfo, bool, error) {
	source, ok := c.trackerCloudV2()
	if !ok {
		return drawing.RankInfo{}, nil, nil, false, nil
	}
	resp, err := source.GetCloudSKCheckRoom(server, eventID, wlCharacterID, ranks, userID, skipMissing, 3600)
	if err != nil {
		return drawing.RankInfo{}, nil, nil, true, err
	}
	current := resp.Rank
	if current.Rank <= 0 && len(resp.Ranks) > 0 {
		current = resp.Ranks[0]
	}
	if current.Rank <= 0 {
		return drawing.RankInfo{}, nil, nil, true, sekaiapi.ErrRankingNotFound
	}
	currentInfo := rankInfoFromCloudV2(current)
	c.enrichRankInfoFromCloudV2Trace(server, eventID, wlCharacterID, current, &currentInfo)
	var previous *drawing.RankInfo
	if resp.Previous != nil {
		info := rankInfoFromCloudV2(*resp.Previous)
		previous = &info
	}
	var next *drawing.RankInfo
	if resp.Next != nil {
		info := rankInfoFromCloudV2(*resp.Next)
		next = &info
	}
	return currentInfo, previous, next, true, nil
}

func (c *Controller) buildCheckRoomRanksFromTrackerCloudV2(server string, eventID int, ranks []int, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, *drawing.RankInfo, *drawing.RankInfo, bool, error) {
	source, ok := c.trackerCloudV2()
	if !ok || len(ranks) == 0 {
		return nil, nil, nil, false, nil
	}
	resp, err := source.GetCloudSKCheckRoom(server, eventID, wlCharacterID, ranks, nil, skipMissing, 3600)
	if err != nil {
		return nil, nil, nil, true, err
	}
	items := resp.Ranks
	if len(items) == 0 && resp.Rank.Rank > 0 {
		items = []sekaiapi.CloudRankInfo{resp.Rank}
	}
	out := make([]drawing.RankInfo, 0, len(items))
	var previous *drawing.RankInfo
	var next *drawing.RankInfo
	for _, current := range items {
		if current.Rank <= 0 {
			if skipMissing {
				continue
			}
			return nil, nil, nil, true, sekaiapi.ErrRankingNotFound
		}
		info := rankInfoFromCloudV2(current)
		c.enrichRankInfoFromCloudV2Trace(server, eventID, wlCharacterID, current, &info)
		out = append(out, info)
	}
	if resp.Previous != nil {
		info := rankInfoFromCloudV2(*resp.Previous)
		previous = &info
	}
	if resp.Next != nil {
		info := rankInfoFromCloudV2(*resp.Next)
		next = &info
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Rank < out[j].Rank
	})
	if len(out) == 0 {
		return nil, nil, nil, true, sekaiapi.ErrRankingNotFound
	}
	return out, previous, next, true, nil
}

func (c *Controller) buildLineRanksFromTrackerV2(server string, eventID int, ranks []int, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, bool, error) {
	source, ok := c.trackerCloudV2()
	if !ok || len(ranks) == 0 {
		return nil, false, nil
	}
	resp, err := source.GetCloudSKLine(server, eventID, wlCharacterID, ranks, nil, skipMissing, 3600)
	if err != nil {
		return nil, true, err
	}
	out := make([]drawing.RankInfo, 0, len(resp.Ranks))
	for _, item := range resp.Ranks {
		info := rankInfoFromCloudV2(item)
		info.Name = ""
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Rank < out[j].Rank
	})
	return out, true, nil
}

func (c *Controller) buildSpeedInfosFromTrackerV2(server string, eventID int, ranks []int, wlCharacterID *int, interval int, unitPeriodSeconds int64, skipMissing bool) ([]drawing.SpeedInfo, bool, error) {
	source, ok := c.trackerCloudV2()
	if !ok || len(ranks) == 0 {
		return nil, false, nil
	}
	resp, err := source.GetCloudSKSpeed(server, eventID, wlCharacterID, ranks, int64(interval), unitPeriodSeconds, skipMissing)
	if err != nil {
		return nil, true, err
	}
	result := make([]drawing.SpeedInfo, 0, len(resp.Speeds))
	for _, item := range resp.Speeds {
		info := rankInfoFromCloudV2(item)
		speed := 0
		if item.Speed != nil {
			speed = *item.Speed
		}
		score := 0
		if info.Score != nil {
			score = *info.Score
		}
		result = append(result, drawing.SpeedInfo{
			Rank:       info.Rank,
			Score:      score,
			Speed:      &speed,
			RecordTime: info.Time,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Rank < result[j].Rank
	})
	return result, true, nil
}

func (c *Controller) buildSubjectTraceFromTrackerV2(server string, eventID int, subjectType string, subject string, wlCharacterID *int) ([]drawing.RankInfo, bool, error) {
	source, ok := c.trackerCloudV2()
	if !ok {
		return nil, false, nil
	}
	resp, err := source.GetCloudSKTrace(server, eventID, wlCharacterID, subjectType, subject, 0)
	if err != nil {
		return nil, true, err
	}
	if resp == nil || len(resp.RankData) == 0 {
		return nil, true, sekaiapi.ErrRankingNotFound
	}
	out := make([]drawing.RankInfo, 0, len(resp.RankData))
	for _, item := range resp.RankData {
		out = append(out, rankInfoFromCloudV2(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Time < out[j].Time
	})
	return out, true, nil
}

func (c *Controller) resolveTrackerUserIDByRankFromCloudV2(server string, eventID, rank int, wlCharacterID *int) (int64, bool, error) {
	source, ok := c.trackerCloudV2()
	if !ok {
		return 0, false, nil
	}
	resp, err := source.GetCloudSKQuery(server, eventID, wlCharacterID, []int{rank}, nil, false, false, 3600)
	if err != nil {
		return 0, true, fmt.Errorf("tracker rank %d query failed: %w", rank, err)
	}
	if resp == nil || len(resp.Ranks) == 0 || resp.Ranks[0].UserID == nil {
		return 0, true, fmt.Errorf("tracker rank %d latest user id is empty", rank)
	}
	uid, err := strconv.ParseInt(strings.TrimSpace(*resp.Ranks[0].UserID), 10, 64)
	if err != nil || uid <= 0 {
		return 0, true, fmt.Errorf("tracker rank %d latest user id is invalid", rank)
	}
	return uid, true, nil
}

func v2SubjectUserID(userID int64) string {
	return strconv.FormatInt(userID, 10)
}

func rankInfoFromCloudV2(item sekaiapi.CloudRankInfo) drawing.RankInfo {
	info := drawing.RankInfo{
		Rank:  item.Rank,
		Name:  strings.TrimSpace(item.Name),
		Score: drawing.IntPtr(item.Score),
		Time:  formatTrackerTimestamp(item.Timestamp),
	}
	if item.AverageRound != nil {
		value := *item.AverageRound
		info.AverageRound = &value
	}
	if item.AveragePt != nil {
		value := *item.AveragePt
		info.AveragePt = &value
	}
	if item.LatestPt != nil {
		value := *item.LatestPt
		info.LatestPt = &value
	}
	if item.Speed != nil {
		speed := *item.Speed
		info.Speed = &speed
	}
	if item.Min20Time3Speed != nil {
		value := *item.Min20Time3Speed
		info.Min20Time3Speed = &value
	}
	if item.HourRound != nil {
		value := *item.HourRound
		info.HourRound = &value
	}
	if item.RecordStartAt != nil {
		value := formatTrackerTimestamp(*item.RecordStartAt)
		info.RecordStartAt = &value
	}
	return info
}

func (c *Controller) enrichRankInfoFromCloudV2Trace(server string, eventID int, wlCharacterID *int, item sekaiapi.CloudRankInfo, info *drawing.RankInfo) {
	if c == nil || info == nil || hasRankInfoRoundMetrics(info) {
		return
	}
	source, ok := c.trackerCloudV2()
	if !ok {
		return
	}
	if item.UserID != nil {
		userID := strings.TrimSpace(*item.UserID)
		if userID != "" {
			if c.applyCloudV2TraceMetrics(source, server, eventID, wlCharacterID, "user", userID, info) {
				return
			}
		}
	}
	if item.Rank > 0 {
		c.applyCloudV2TraceMetrics(source, server, eventID, wlCharacterID, "rank", strconv.Itoa(item.Rank), info)
	}
}

func (c *Controller) applyCloudV2TraceMetrics(source trackerCloudV2Source, server string, eventID int, wlCharacterID *int, subjectType string, subject string, info *drawing.RankInfo) bool {
	resp, err := source.GetCloudSKTrace(server, eventID, wlCharacterID, subjectType, subject, 0)
	if err != nil || resp == nil || len(resp.RankData) == 0 {
		return false
	}
	samples := make([]trackerScoreSample, 0, len(resp.RankData))
	for _, point := range resp.RankData {
		samples = append(samples, trackerScoreSample{
			score:     point.Score,
			timestamp: point.Timestamp,
		})
	}
	applyRankInfoMetrics(info, samples)
	return hasRankInfoRoundMetrics(info)
}

func hasRankInfoRoundMetrics(info *drawing.RankInfo) bool {
	if info == nil {
		return false
	}
	return info.AverageRound != nil &&
		info.AveragePt != nil &&
		info.LatestPt != nil &&
		info.HourRound != nil &&
		info.Min20Time3Speed != nil &&
		info.Speed != nil &&
		info.RecordStartAt != nil
}
