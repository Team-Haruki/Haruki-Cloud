package sk

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
)

func (c *Controller) buildRankTraceFromTracker(server string, eventID, rank int, wlCharacterID *int) ([]drawing.RankInfo, error) {
	result := make([]drawing.RankInfo, 0)
	latestName := ""
	if latest, err := c.buildSingleRankLatestFromTracker(server, eventID, rank, wlCharacterID); err == nil {
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
		return result[i].Time < result[j].Time
	})
	if len(result) == 0 {
		latest, err := c.buildSingleRankLatestFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker trace fallback rank %d query failed: %w", rank, err)
		}
		return []drawing.RankInfo{latest}, nil
	}
	return result, nil
}
