package sk

import (
	"fmt"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) BuildPlayerTraceRequest(req drawing.PlayerTraceRequest) (*drawing.PlayerTraceRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk player-trace request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderPlayerTrace(req drawing.PlayerTraceRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	payload, err := c.BuildPlayerTraceRequest(req)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKPlayerTrace(payload)
}

// BuildPlayerTraceFromTracker builds a player-trace request by fetching trace
// data from the tracker API for a specific user or ranking line.
func (c *Controller) BuildPlayerTraceFromTracker(req TrackerRankQuery) (*drawing.PlayerTraceRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	defer finishBuild()
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}

	payload := drawing.PlayerTraceRequest{
		EventID: normalized.EventID,
		Region:  normalized.Region,
	}

	switch {
	case normalized.UserID != nil:
		ranks, err := c.buildUserTraceFromTracker(normalized.Region, normalized.EventID, *normalized.UserID, normalized.WlCharacterID)
		if err != nil {
			return nil, err
		}
		if len(ranks) == 0 {
			return nil, fmt.Errorf("no trace data available for user")
		}
		payload.Ranks = ranks

	case len(normalized.Ranks) > 0:
		if len(normalized.Ranks) > 2 {
			return nil, fmt.Errorf("player-trace 最多支持两个排名")
		}
		trace1, err := c.buildPlayerTraceByRankFromTracker(normalized.Region, normalized.EventID, normalized.Ranks[0], normalized.WlCharacterID)
		if err != nil {
			return nil, err
		}
		if len(trace1) == 0 {
			return nil, fmt.Errorf("no trace data available for rank %d", normalized.Ranks[0])
		}
		payload.Ranks = trace1
		if len(normalized.Ranks) > 1 {
			trace2, err := c.buildPlayerTraceByRankFromTracker(normalized.Region, normalized.EventID, normalized.Ranks[1], normalized.WlCharacterID)
			if err != nil {
				return nil, err
			}
			payload.Ranks2 = trace2
		}

	default:
		return nil, fmt.Errorf("player-trace requires user_id or rank")
	}
	if normalized.CompareRank > 0 {
		trace, err := c.buildRankTraceFromTracker(normalized.Region, normalized.EventID, normalized.CompareRank, normalized.WlCharacterID)
		if err != nil {
			return nil, err
		}
		latest, err := c.buildSingleRankLatestFromTracker(normalized.Region, normalized.EventID, normalized.CompareRank, normalized.WlCharacterID)
		if err != nil {
			return nil, err
		}
		payload.CompareRank = normalized.CompareRank
		payload.CompareRankTrace = trace
		payload.CompareRankLatest = &latest
		if latest.Score != nil {
			payload.CompareRankLineScore = latest.Score
		}
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		if icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region)); icon != "" {
			payload.WlCharaIconPath = &icon
		}
	}
	return c.BuildPlayerTraceRequest(payload)
}

func (c *Controller) BuildRankTraceRequest(req drawing.RankTraceRequest) (*drawing.RankTraceRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk rank-trace request has no ranks")
	}
	return &req, nil
}

func (c *Controller) BuildRankTraceRequestFromTracker(req TrackerRankQuery) (*drawing.RankTraceRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	defer finishBuild()
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID != nil {
		return nil, fmt.Errorf("rank-trace 暂不支持按用户查询，请使用排名")
	}
	targetRank := normalized.Ranks[0]
	trace, err := c.buildRankTraceFromTracker(normalized.Region, normalized.EventID, targetRank, normalized.WlCharacterID)
	if err != nil {
		return nil, err
	}
	payload := drawing.RankTraceRequest{
		EventID:    normalized.EventID,
		Region:     normalized.Region,
		TargetRank: targetRank,
		Ranks:      trace,
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		if icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region)); icon != "" {
			payload.WlCharaIconPath = &icon
		}
	}
	return c.BuildRankTraceRequest(payload)
}

func (c *Controller) RenderRankTrace(req drawing.RankTraceRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), payloadBuildStage)
	payload, err := c.BuildRankTraceRequest(req)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKRankTrace(payload)
}
