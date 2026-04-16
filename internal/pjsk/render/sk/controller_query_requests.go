package sk

import (
	"fmt"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

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
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region))
		if icon != "" {
			payload.WlCharaIconPath = &icon
			payload.CharaIconPath = &icon
		}
	}
	return c.BuildQueryRequest(payload)
}

func (c *Controller) BuildCheckRoomRequest(req drawing.CFRequest) (*drawing.CFRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk check-room request has no ranks")
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
		UpdateAt:    time.Now().UTC().Format(time.RFC3339),
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
		payload.Ranks = []drawing.RankInfo{info}
		targetRank = info.Rank
	} else {
		skipMissing := shouldSkipMissingTrackerRanks(normalized)
		rankInfos, err := c.buildRanksFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.WlCharacterID, skipMissing)
		if err != nil {
			return nil, err
		}
		payload.Ranks = rankInfos
		if len(normalized.Ranks) > 0 {
			targetRank = normalized.Ranks[0]
		}
	}

	if targetRank > 0 {
		if targetRank > 1 {
			if prev, err := c.buildSingleRankFromTracker(normalized.Region, normalized.EventID, targetRank-1, normalized.WlCharacterID); err == nil {
				payload.PrevRank = &prev
			}
		}
		if next, err := c.buildSingleRankFromTracker(normalized.Region, normalized.EventID, targetRank+1, normalized.WlCharacterID); err == nil {
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
