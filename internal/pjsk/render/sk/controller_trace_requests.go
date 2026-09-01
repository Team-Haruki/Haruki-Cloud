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
	payload.Ranks, payload.Ranks2, err = c.buildPrimaryPlayerTrace(normalized)
	if err != nil {
		return nil, err
	}
	if err := c.applyPlayerTraceComparison(&payload, normalized); err != nil {
		return nil, err
	}
	payload.WlCharaIconPath = c.playerTraceCharacterIcon(normalized)
	return c.BuildPlayerTraceRequest(payload)
}

func (c *Controller) buildPrimaryPlayerTrace(query TrackerRankQuery) ([]drawing.RankInfo, []drawing.RankInfo, error) {
	if query.UserID != nil {
		ranks, err := c.buildUserTraceFromTracker(query.Region, query.EventID, *query.UserID, query.WlCharacterID)
		if err == nil && len(ranks) == 0 {
			err = fmt.Errorf("no trace data available for user")
		}
		return ranks, nil, err
	}
	if len(query.Ranks) == 0 {
		return nil, nil, fmt.Errorf("player-trace requires user_id or rank")
	}
	if len(query.Ranks) > 2 {
		return nil, nil, fmt.Errorf("player-trace 最多支持两个排名")
	}
	first, err := c.buildPlayerTraceByRankFromTracker(query.Region, query.EventID, query.Ranks[0], query.WlCharacterID)
	if err != nil {
		return nil, nil, err
	}
	if len(first) == 0 {
		return nil, nil, fmt.Errorf("no trace data available for rank %d", query.Ranks[0])
	}
	if len(query.Ranks) == 1 {
		return first, nil, nil
	}
	second, err := c.buildPlayerTraceByRankFromTracker(query.Region, query.EventID, query.Ranks[1], query.WlCharacterID)
	return first, second, err
}

func (c *Controller) applyPlayerTraceComparison(payload *drawing.PlayerTraceRequest, query TrackerRankQuery) error {
	if query.CompareRank <= 0 {
		return nil
	}
	trace, err := c.buildRankTraceFromTracker(query.Region, query.EventID, query.CompareRank, query.WlCharacterID)
	if err != nil {
		return err
	}
	latest, err := c.buildSingleRankLatestFromTracker(query.Region, query.EventID, query.CompareRank, query.WlCharacterID)
	if err != nil {
		return err
	}
	payload.CompareRank = query.CompareRank
	payload.CompareRankTrace = trace
	payload.CompareRankLatest = &latest
	payload.CompareRankLineScore = latest.Score
	return nil
}

func (c *Controller) playerTraceCharacterIcon(query TrackerRankQuery) *string {
	if query.WlCharacterID == nil || *query.WlCharacterID <= 0 {
		return nil
	}
	icon := c.resolveCharacterIconPath(*query.WlCharacterID, renderregion.Normalize(query.Region))
	if icon == "" {
		return nil
	}
	return &icon
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
