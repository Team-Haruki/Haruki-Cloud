package sk

import (
	"fmt"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

const skCheckRoomRankLimit = 100

var queryAdjacentRanksNormal = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 1500, 2000, 2500, 3000, 4000, 5000,
	10000, 20000, 30000, 40000, 50000,
	100000, 200000, 300000,
}

var queryAdjacentRanksWorldLink = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	20, 30, 40, 50, 100, 200, 300, 400, 500,
	1000, 2000, 3000, 4000, 5000, 7000,
	10000, 20000, 30000, 40000, 50000, 70000, 100000,
}

func (c *Controller) BuildQueryRequest(req drawing.SKRequest) (*drawing.SKRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk query request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderQuery(req drawing.SKRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildQueryRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKQuery(payload)
}

func (c *Controller) BuildQueryRequestFromTracker(req TrackerRankQuery) (*drawing.SKRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	skipMissing := shouldSkipMissingTrackerRanks(normalized)
	rankInfos, err := c.buildRanksOrUserFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.UserID, normalized.WlCharacterID, skipMissing)
	if err != nil {
		return nil, err
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	payload := drawing.SKRequest{
		ID:          normalized.EventID,
		Region:      normalized.Region,
		Name:        meta.name,
		AggregateAt: meta.aggregateAt,
		Ranks:       rankInfos,
	}
	if len(rankInfos) == 1 {
		targetRank := rankInfos[0].Rank
		prevRank, nextRank, hasPrev, hasNext := queryAdjacentSKLineRanks(targetRank, normalized.WlCharacterID != nil)
		if hasPrev {
			if prev, err := c.buildSingleRankLatestFromTracker(normalized.Region, normalized.EventID, prevRank, normalized.WlCharacterID); err == nil {
				payload.PrevRanks = &prev
			}
		}
		if hasNext {
			if next, err := c.buildSingleRankLatestFromTracker(normalized.Region, normalized.EventID, nextRank, normalized.WlCharacterID); err == nil {
				payload.NextRanks = &next
			}
		}
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region))
		if icon != "" {
			payload.WlCharaIconPath = &icon
			payload.CharaIconPath = &icon
		}
	}
	return c.BuildQueryRequest(payload)
}

func queryAdjacentSKLineRanks(rank int, wlMode bool) (prev int, next int, hasPrev bool, hasNext bool) {
	if rank <= 0 {
		return 0, 0, false, false
	}
	nodes := queryAdjacentRanksNormal
	if wlMode {
		nodes = queryAdjacentRanksWorldLink
	}
	for i, node := range nodes {
		if node >= rank {
			if node == rank {
				if i > 0 {
					prev = nodes[i-1]
					hasPrev = true
				}
				if i+1 < len(nodes) {
					next = nodes[i+1]
					hasNext = true
				}
				return prev, next, hasPrev, hasNext
			}
			if i > 0 {
				prev = nodes[i-1]
				hasPrev = true
			}
			next = node
			hasNext = true
			return prev, next, hasPrev, hasNext
		}
	}
	if len(nodes) > 0 {
		prev = nodes[len(nodes)-1]
		hasPrev = true
	}
	return prev, 0, hasPrev, false
}

func (c *Controller) BuildCheckRoomRequest(req drawing.CFRequest) (*drawing.CFRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk check-room request has no ranks")
	}
	if err := validateSKCheckRoomSupportedRanks(req.Ranks); err != nil {
		return nil, err
	}
	return &req, nil
}

func (c *Controller) BuildCheckRoomRequestFromTracker(req TrackerRankQuery) (*drawing.CFRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	payload := drawing.CFRequest{
		Eid:         normalized.EventID,
		EventName:   meta.name,
		Region:      normalized.Region,
		AggregateAt: meta.aggregateAt,
		UpdateAt:    time.Now().UTC().UnixMilli(),
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		if icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region)); icon != "" {
			payload.WlCharaIconPath = &icon
		}
	}

	targetRank := 0
	if normalized.UserID != nil && *normalized.UserID > 0 {
		info, err := c.buildSingleUserFromTracker(normalized.Region, normalized.EventID, *normalized.UserID, normalized.WlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker user query failed: %w", err)
		}
		if err := validateSKCheckRoomSupportedRank(info.Rank); err != nil {
			return nil, err
		}
		payload.Ranks = []drawing.RankInfo{info}
		targetRank = info.Rank
	} else {
		skipMissing := shouldSkipMissingTrackerRanks(normalized)
		rankInfos, err := c.buildRanksFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.WlCharacterID, skipMissing)
		if err != nil {
			return nil, err
		}
		if err := validateSKCheckRoomSupportedRanks(rankInfos); err != nil {
			return nil, err
		}
		payload.Ranks = rankInfos
		if len(rankInfos) > 0 {
			targetRank = rankInfos[0].Rank
		}
	}

	if targetRank > 0 {
		if targetRank > 1 {
			if prev, err := c.buildSingleRankLatestFromTracker(normalized.Region, normalized.EventID, targetRank-1, normalized.WlCharacterID); err == nil {
				payload.PrevRank = &prev
			}
		}
		if next, err := c.buildSingleRankLatestFromTracker(normalized.Region, normalized.EventID, targetRank+1, normalized.WlCharacterID); err == nil {
			payload.NextRank = &next
		}
	}
	return c.BuildCheckRoomRequest(payload)
}

func (c *Controller) RenderCheckRoom(req drawing.CFRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildCheckRoomRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKCheckRoom(payload)
}

func (c *Controller) BuildCSBRequest(req drawing.CSBRequest) (*drawing.CSBRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk csb request has no ranks")
	}
	if err := validateSKCheckRoomSupportedRank(req.Ranks[len(req.Ranks)-1].Rank); err != nil {
		return nil, err
	}
	return &req, nil
}

func (c *Controller) BuildCSBRequestFromTracker(req TrackerRankQuery) (*drawing.CSBRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID == nil && len(normalized.Ranks) > 1 {
		return nil, fmt.Errorf("查水表目前仅支持单人查询")
	}

	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	now := time.Now().UTC()
	payload := drawing.CSBRequest{
		Eid:         normalized.EventID,
		EventName:   meta.name,
		Region:      normalized.Region,
		AggregateAt: meta.aggregateAt,
		UpdateAt:    now.UnixMilli(),
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		if icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region)); icon != "" {
			payload.WlCharaIconPath = &icon
		}
	}

	if normalized.UserID != nil && *normalized.UserID > 0 {
		info, err := c.buildSingleUserFromTracker(normalized.Region, normalized.EventID, *normalized.UserID, normalized.WlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker user query failed: %w", err)
		}
		if err := validateSKCheckRoomSupportedRank(info.Rank); err != nil {
			return nil, err
		}
		trace, err := c.buildUserTraceFromTracker(normalized.Region, normalized.EventID, *normalized.UserID, normalized.WlCharacterID)
		if err != nil {
			return nil, err
		}
		payload.Ranks = appendIdleTrackerRankTraceAt(trace, now)
		return c.BuildCSBRequest(payload)
	}

	if len(normalized.Ranks) == 0 {
		return nil, fmt.Errorf("查水表目前仅支持单人查询")
	}
	info, err := c.buildSingleRankFromTracker(normalized.Region, normalized.EventID, normalized.Ranks[0], normalized.WlCharacterID)
	if err != nil {
		return nil, err
	}
	if err := validateSKCheckRoomSupportedRank(info.Rank); err != nil {
		return nil, err
	}
	userID, err := c.resolveTrackerUserIDByRank(normalized.Region, normalized.EventID, normalized.Ranks[0], normalized.WlCharacterID)
	if err != nil {
		return nil, err
	}
	trace, err := c.buildUserTraceFromTracker(normalized.Region, normalized.EventID, userID, normalized.WlCharacterID)
	if err != nil {
		return nil, err
	}
	payload.Ranks = appendIdleTrackerRankTraceAt(trace, now)
	return c.BuildCSBRequest(payload)
}

func (c *Controller) RenderCSB(req drawing.CSBRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildCSBRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKCSB(payload)
}

func validateSKCheckRoomSupportedRanks(ranks []drawing.RankInfo) error {
	for _, rank := range ranks {
		if err := validateSKCheckRoomSupportedRank(rank.Rank); err != nil {
			return err
		}
	}
	return nil
}

func validateSKCheckRoomSupportedRank(rank int) error {
	if rank > skCheckRoomRankLimit {
		return fmt.Errorf("查房/查水表目前仅支持前100名查询")
	}
	return nil
}
