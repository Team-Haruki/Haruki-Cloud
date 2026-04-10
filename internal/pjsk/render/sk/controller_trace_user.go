package sk

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/utils/drawing"
)

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
