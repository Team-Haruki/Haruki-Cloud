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
	source, ok := c.trackerCloudV2()
	if !ok || userID <= 0 {
		return drawing.RankInfo{}, false, nil
	}
	resp, err := source.GetCloudSKQuery(server, eventID, wlCharacterID, nil, &userID, false, false, 3600)
	if err != nil {
		return drawing.RankInfo{}, true, err
	}
	if resp == nil || len(resp.Ranks) == 0 {
		return drawing.RankInfo{}, true, sekaiapi.ErrRankingNotFound
	}
	return rankInfoFromCloudV2(resp.Ranks[0]), true, nil
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
	_ = current
	currentInfo := rankInfoFromCloudV2(current)
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
	resp, err := source.GetCloudSKTrace(server, eventID, wlCharacterID, subjectType, subject, 5000)
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
	if item.Speed != nil {
		speed := *item.Speed
		info.Speed = &speed
	}
	return info
}
