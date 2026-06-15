package sk

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"golang.org/x/sync/errgroup"
)

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
			if !strings.EqualFold(eventInfo.EventType, "world_bloom") && normalized.WlCharacterID != nil {
				return TrackerRankQuery{}, fmt.Errorf("wl_character_id is only valid for world bloom event")
			}
		}
	}
	return normalized, nil
}

func shouldSkipMissingTrackerRanks(req TrackerRankQuery) bool {
	return req.DefaultRanks && req.UserID == nil
}

func normalizeTrackerSpeedConfig(req TrackerRankQuery) (periodSeconds int64, unitPeriodSeconds int64, unitText string) {
	unit := strings.ToLower(strings.TrimSpace(req.SpeedUnit))
	switch unit {
	case "d", "day", "daily", "日":
		unitPeriodSeconds = 24 * 60 * 60
		unitText = "日"
	default:
		unitPeriodSeconds = 60 * 60
		unitText = "时"
	}

	periodSeconds = req.SpeedPeriodSecs
	if periodSeconds <= 0 {
		periodSeconds = unitPeriodSeconds
	}

	return periodSeconds, unitPeriodSeconds, unitText
}

func shouldSkipMissingTrackerRankError(skipMissing bool, err error) bool {
	return skipMissing && errors.Is(err, sekaiapi.ErrRankingNotFound)
}

func (c *Controller) buildRanksFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, error) {
	if len(ranks) == 0 {
		return nil, nil
	}
	if len(ranks) == 1 {
		info, err := c.buildSingleRankFromTracker(server, eventID, ranks[0], wlCharacterID)
		if err != nil {
			if shouldSkipMissingTrackerRankError(skipMissing, err) {
				return nil, nil
			}
			return nil, fmt.Errorf("tracker rank %d query failed: %w", ranks[0], err)
		}
		return []drawing.RankInfo{info}, nil
	}

	if result, ok, err := c.buildRanksFromTrackerLatestWithMetrics(server, eventID, ranks, wlCharacterID, skipMissing); ok {
		return result, err
	}

	result := make([]drawing.RankInfo, 0, len(ranks))
	for _, rank := range ranks {
		info, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
			if shouldSkipMissingTrackerRankError(skipMissing, err) {
				continue
			}
			return nil, fmt.Errorf("tracker rank %d query failed: %w", rank, err)
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Rank < result[j].Rank
	})
	return result, nil
}

func (c *Controller) buildRanksOrUserFromTracker(server string, eventID int, ranks []int, userID *int64, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, error) {
	if userID != nil && *userID > 0 {
		info, err := c.buildSingleUserFromTracker(server, eventID, *userID, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker user query failed: %w", err)
		}
		return []drawing.RankInfo{info}, nil
	}
	return c.buildRanksFromTracker(server, eventID, ranks, wlCharacterID, skipMissing)
}

func (c *Controller) buildRanksFromTrackerLatestWithMetrics(server string, eventID int, ranks []int, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, bool, error) {
	if c == nil || c.tracker == nil || len(ranks) == 0 {
		return nil, false, nil
	}
	var rankBatch trackerRankTraceBatchSource
	var worldBloomBatch trackerWorldBloomRankTraceBatchSource
	if wlCharacterID != nil && *wlCharacterID > 0 {
		var ok bool
		worldBloomBatch, ok = c.tracker.(trackerWorldBloomRankTraceBatchSource)
		if !ok {
			return nil, false, nil
		}
	} else {
		var ok bool
		rankBatch, ok = c.tracker.(trackerRankTraceBatchSource)
		if !ok {
			return nil, false, nil
		}
	}

	base := make([]lineRankResult, len(ranks))
	var group errgroup.Group
	group.SetLimit(skLineTrackerConcurrency)
	for idx, rank := range ranks {
		idx, rank := idx, rank
		group.Go(func() error {
			info, err := c.buildSingleRankLatestFromTracker(server, eventID, rank, wlCharacterID)
			if err != nil {
				if shouldSkipMissingTrackerRankError(skipMissing, err) {
					return nil
				}
				return fmt.Errorf("tracker rank %d query failed: %w", rank, err)
			}
			base[idx] = lineRankResult{info: info, ok: true}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, true, err
	}

	out := make([]drawing.RankInfo, 0, len(base))
	for _, result := range base {
		if !result.ok {
			continue
		}
		out = append(out, result.info)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Rank < out[j].Rank
	})

	if len(out) == 0 {
		return out, true, nil
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		resp, err := worldBloomBatch.TraceWorldBloomRankingsByRanks(server, eventID, *wlCharacterID, ranks)
		if err == nil {
			overlayWorldBloomBatchTraceMetrics(out, resp)
		}
		return out, true, nil
	}
	resp, err := rankBatch.TraceRankingsByRanks(server, eventID, ranks)
	if err == nil {
		overlayBatchTraceMetrics(out, resp)
	}
	return out, true, nil
}

func overlayBatchTraceMetrics(ranks []drawing.RankInfo, resp *sekaiapi.BatchTraceRankingResponse) {
	if len(ranks) == 0 || resp == nil {
		return
	}
	pointsByRank := make(map[int][]sekaiapi.RankDataPoint, len(resp.Items))
	for _, item := range resp.Items {
		if len(item.RankData) > 0 {
			pointsByRank[item.Rank] = item.RankData
		}
	}
	for i := range ranks {
		if points, ok := pointsByRank[ranks[i].Rank]; ok {
			applyRankInfoMetrics(&ranks[i], rankTraceSamples(points))
		}
	}
}

func overlayWorldBloomBatchTraceMetrics(ranks []drawing.RankInfo, resp *sekaiapi.BatchWorldBloomTraceRankingResponse) {
	if len(ranks) == 0 || resp == nil {
		return
	}
	pointsByRank := make(map[int][]sekaiapi.WorldBloomRankDataPoint, len(resp.Items))
	for _, item := range resp.Items {
		if len(item.RankData) > 0 {
			pointsByRank[item.Rank] = item.RankData
		}
	}
	for i := range ranks {
		if points, ok := pointsByRank[ranks[i].Rank]; ok {
			applyRankInfoMetrics(&ranks[i], worldBloomTraceSamples(points))
		}
	}
}
