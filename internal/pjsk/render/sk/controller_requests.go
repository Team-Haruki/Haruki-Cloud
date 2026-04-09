package sk

import (
	"fmt"
	"sort"
	"strings"
	"time"

	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

func (c *Controller) BuildLineRequestFromTracker(req TrackerRankQuery) (*LineRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	skipMissing := shouldSkipMissingTrackerRanks(normalized)
	rankInfos, err := c.buildRanksOrUserFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.UserID, normalized.WlCharacterID, skipMissing)
	if err != nil {
		return nil, err
	}
	// SK line focuses on score borders; omit player names to keep output compact.
	for i := range rankInfos {
		rankInfos[i].Name = ""
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	line := LineRequest{
		SklRequest: drawing.SklRequest{
			ID:            normalized.EventID,
			Region:        normalized.Region,
			StartAt:       meta.startAt,
			AggregateAt:   meta.aggregateAt,
			Name:          meta.name,
			BannerImgPath: meta.bannerPath,
			Ranks:         rankInfos,
		},
		Full: normalized.Full,
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		wl := *normalized.WlCharacterID
		line.WlCid = &wl
		if icon := c.resolveCharacterIconPath(wl, renderregion.Normalize(normalized.Region)); icon != "" {
			line.CharaIconPath = &icon
		}
	}
	return c.BuildLineRequest(line)
}

// BuildPredictLineRequestFromTracker builds an SK line payload using external
// forecast sources (33kit / Moesekai / SekaRun) for final score prediction.
func (c *Controller) BuildPredictLineRequestFromTracker(req TrackerRankQuery) (*LineRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID != nil {
		return nil, fmt.Errorf("榜线预测暂不支持按用户查询，请使用排名")
	}
	if normalized.WlCharacterID != nil || strings.TrimSpace(normalized.WlCharacterQuery) != "" {
		return nil, fmt.Errorf("榜线预测不支持WL单榜")
	}
	if c.forecast == nil {
		return nil, fmt.Errorf("forecast provider is not configured")
	}

	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)

	currentRanks, currentErr := c.buildRanksFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, nil, shouldSkipMissingTrackerRanks(normalized))
	if currentErr == nil {
		for i := range currentRanks {
			// SK line keeps names empty to reduce visual noise.
			currentRanks[i].Name = ""
		}
		sort.Slice(currentRanks, func(i, j int) bool {
			return currentRanks[i].Rank < currentRanks[j].Rank
		})
	}

	sourceOrder := []string{"33kit", "moesekai", "sekarun"}
	sourceNames := map[string]string{
		"33kit":    "33Kit预测",
		"moesekai": "Moesekai预测",
		"sekarun":  "SekaRun预测",
		"forecast": "预测",
	}

	bySource := make(map[string]ForecastSourceData)
	var forecastErr error
	if provider, ok := c.forecast.(ForecastProviderBySource); ok {
		bySource, forecastErr = provider.FetchBySource(c.contextOrBackground(), normalized.Region, normalized.EventID, normalized.Ranks)
	} else {
		merged, err := c.forecast.Fetch(c.contextOrBackground(), normalized.Region, normalized.EventID, normalized.Ranks)
		forecastErr = err
		if len(merged) > 0 {
			bySource["forecast"] = ForecastSourceData{
				Scores: merged,
			}
			sourceOrder = append(sourceOrder, "forecast")
		}
	}

	columns := make([]drawing.SKForecastColumn, 0, len(sourceOrder))
	for _, sourceKey := range sourceOrder {
		sourceData, ok := bySource[sourceKey]
		if !ok || len(sourceData.Scores) == 0 {
			continue
		}
		rankInfos := make([]drawing.RankInfo, 0, len(normalized.Ranks))
		forecastAt := int64(0)
		for _, rank := range normalized.Ranks {
			item, ok := sourceData.Scores[rank]
			if !ok || item.Score <= 0 {
				continue
			}
			if item.Timestamp > forecastAt {
				forecastAt = item.Timestamp
			}
			score := item.Score
			rankInfos = append(rankInfos, drawing.RankInfo{
				Rank:  rank,
				Name:  "",
				Score: drawing.IntPtr(score),
				Time:  formatTrackerTimestamp(item.Timestamp),
			})
		}
		if len(rankInfos) == 0 {
			continue
		}
		sort.Slice(rankInfos, func(i, j int) bool {
			return rankInfos[i].Rank < rankInfos[j].Rank
		})

		columnName := strings.TrimSpace(sourceNames[sourceKey])
		if columnName == "" {
			columnName = sourceKey
		}
		column := drawing.SKForecastColumn{
			Key:   sourceKey,
			Name:  columnName,
			Ranks: rankInfos,
		}
		if forecastAt > 0 {
			column.ForecastTime = formatTrackerTimestamp(forecastAt)
		}
		if sourceData.FetchedAt > 0 {
			column.UpdateTime = formatTrackerTimestamp(sourceData.FetchedAt)
		}
		columns = append(columns, column)
	}

	// Fallback to real-time tracker line when all forecast sources are unavailable.
	if len(columns) == 0 {
		if currentErr != nil {
			if forecastErr != nil {
				return nil, fmt.Errorf("forecast query failed: %v; fallback tracker query failed: %w", forecastErr, currentErr)
			}
			return nil, fmt.Errorf("预测源暂无这些档位的数据，且回退实时档线失败: %w", currentErr)
		}
		line := LineRequest{
			SklRequest: drawing.SklRequest{
				ID:            normalized.EventID,
				Region:        normalized.Region,
				StartAt:       meta.startAt,
				AggregateAt:   meta.aggregateAt,
				Name:          strings.TrimSpace(meta.name + " 预测(实时)"),
				BannerImgPath: meta.bannerPath,
				Ranks:         currentRanks,
				CurrentRanks:  currentRanks,
			},
			Full: normalized.Full,
		}
		return c.BuildLineRequest(line)
	}

	// Keep rendering available forecast sources even when current tracker line fails.
	if currentErr != nil {
		currentRanks = nil
	}

	line := LineRequest{
		SklRequest: drawing.SklRequest{
			ID:              normalized.EventID,
			Region:          normalized.Region,
			StartAt:         meta.startAt,
			AggregateAt:     meta.aggregateAt,
			Name:            strings.TrimSpace(meta.name + " 预测"),
			BannerImgPath:   meta.bannerPath,
			Ranks:           currentRanks,
			CurrentRanks:    currentRanks,
			ForecastColumns: columns,
		},
		Full: normalized.Full,
	}
	return c.BuildLineRequest(line)
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
	if normalized.UserID != nil {
		return nil, fmt.Errorf("check-room 暂不支持按用户查询，请使用排名")
	}
	skipMissing := shouldSkipMissingTrackerRanks(normalized)
	rankInfos, err := c.buildRanksFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.WlCharacterID, skipMissing)
	if err != nil {
		return nil, err
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	payload := drawing.CFRequest{
		Eid:         normalized.EventID,
		EventName:   meta.name,
		Region:      normalized.Region,
		Ranks:       rankInfos,
		AggregateAt: meta.aggregateAt,
		UpdateAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		if icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region)); icon != "" {
			payload.WlCharaIconPath = &icon
		}
	}
	if len(normalized.Ranks) > 0 {
		target := normalized.Ranks[0]
		if target > 1 {
			if prev, err := c.buildSingleRankFromTracker(normalized.Region, normalized.EventID, target-1, normalized.WlCharacterID); err == nil {
				payload.PrevRank = &prev
			}
		}
		if next, err := c.buildSingleRankFromTracker(normalized.Region, normalized.EventID, target+1, normalized.WlCharacterID); err == nil {
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

func (c *Controller) BuildSpeedRequest(req drawing.SpeedRequest) (*drawing.SpeedRequest, error) {
	if len(req.Ranks) == 0 {
		return nil, fmt.Errorf("sk speed request has no ranks")
	}
	return &req, nil
}

func (c *Controller) BuildSpeedRequestFromTracker(req TrackerRankQuery) (*drawing.SpeedRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID != nil {
		return nil, fmt.Errorf("speed 暂不支持按用户查询，请使用排名")
	}
	speedPeriodSeconds, speedUnitText := normalizeTrackerSpeedConfig(normalized)
	speedInfos, err := c.buildSpeedInfosFromTracker(
		normalized.Region,
		normalized.EventID,
		normalized.Ranks,
		normalized.WlCharacterID,
		int(speedPeriodSeconds),
		speedPeriodSeconds,
		shouldSkipMissingTrackerRanks(normalized),
	)
	if err != nil {
		return nil, err
	}
	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	payload := drawing.SpeedRequest{
		EventID:          normalized.EventID,
		Region:           normalized.Region,
		EventName:        meta.name,
		EventStartAt:     meta.startAt,
		EventAggregateAt: meta.aggregateAt,
		Ranks:            speedInfos,
		IsWlEvent:        normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0,
		RequestType:      speedUnitText,
		Period:           speedPeriodSeconds,
	}
	if meta.bannerPath != "" {
		banner := meta.bannerPath
		payload.BannerImgPath = &banner
	}
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		if icon := c.resolveCharacterIconPath(*normalized.WlCharacterID, renderregion.Normalize(normalized.Region)); icon != "" {
			payload.WlCharaIconPath = &icon
		}
	}
	return c.BuildSpeedRequest(payload)
}

func (c *Controller) RenderSpeed(req drawing.SpeedRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildSpeedRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKSpeed(payload)
}

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
	payload, err := c.BuildPlayerTraceRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKPlayerTrace(payload)
}

// BuildPlayerTraceFromTracker builds a player-trace request by fetching trace
// data from the tracker API for a specific user or ranking line.
func (c *Controller) BuildPlayerTraceFromTracker(req TrackerRankQuery) (*drawing.PlayerTraceRequest, error) {
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
	payload, err := c.BuildRankTraceRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKRankTrace(payload)
}

func (c *Controller) BuildWinRateRequest(req drawing.WinRateRequest) (*drawing.WinRateRequest, error) {
	if len(req.TeamInfo) == 0 {
		return nil, fmt.Errorf("sk winrate request has no teams")
	}
	return &req, nil
}

func (c *Controller) RenderWinRate(req drawing.WinRateRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildWinRateRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKWinRate(payload)
}
