package sk

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
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
	if c.forecast == nil {
		return nil, fmt.Errorf("forecast provider is not configured")
	}

	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)

	currentRanks, currentErr := c.buildRanksFromTracker(
		normalized.Region,
		normalized.EventID,
		normalized.Ranks,
		normalized.WlCharacterID,
		shouldSkipMissingTrackerRanks(normalized),
	)
	if currentErr == nil {
		for i := range currentRanks {
			// SK line keeps names empty to reduce visual noise.
			currentRanks[i].Name = ""
		}
		sort.Slice(currentRanks, func(i, j int) bool {
			return currentRanks[i].Rank < currentRanks[j].Rank
		})
	}

	// WL chapter requests currently have no chapter-level forecast source, so they
	// always fall back to the real-time line for the selected chapter.
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		return c.buildPredictRealtimeFallbackLine(normalized, meta, currentRanks, currentErr)
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
			column.ForecastTime = drawing.Int64Ptr(formatTrackerTimestamp(forecastAt))
		}
		if sourceData.FetchedAt > 0 {
			column.UpdateTime = drawing.Int64Ptr(formatTrackerTimestamp(sourceData.FetchedAt))
		}
		columns = append(columns, column)
	}

	// Fallback to real-time tracker line when all forecast sources are unavailable.
	if len(columns) == 0 {
		if currentErr != nil {
			if forecastErr != nil {
				return nil, fmt.Errorf("forecast query failed: %w; fallback tracker query failed: %w", forecastErr, currentErr)
			}
			return nil, fmt.Errorf("预测源暂无这些档位的数据，且回退实时档线失败: %w", currentErr)
		}
		return c.buildPredictRealtimeFallbackLine(normalized, meta, currentRanks, nil)
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

func (c *Controller) buildPredictRealtimeFallbackLine(
	normalized TrackerRankQuery,
	meta eventMeta,
	currentRanks []drawing.RankInfo,
	currentErr error,
) (*LineRequest, error) {
	if currentErr != nil {
		return nil, currentErr
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
	if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
		wl := *normalized.WlCharacterID
		line.WlCid = &wl
		if icon := c.resolveCharacterIconPath(wl, renderregion.Normalize(normalized.Region)); icon != "" {
			line.CharaIconPath = &icon
		}
	}
	return c.BuildLineRequest(line)
}
