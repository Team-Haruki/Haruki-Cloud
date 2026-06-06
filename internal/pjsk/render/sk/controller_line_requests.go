package sk

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

const (
	skPredictionNotice      = "预测数据仅供参考，请以实际为准规划好冲榜计划"
	skPredictionStopMessage = "结活前最后一个小时停止提供预测"
	skPredictionNoActiveMsg = "当前无进行中的活动"
)

func (c *Controller) BuildLineRequestFromTracker(req TrackerRankQuery) (*LineRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	skipMissing := shouldSkipMissingTrackerRanks(normalized)
	rankInfos, err := c.buildLineRanksOrUserFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.UserID, normalized.WlCharacterID, skipMissing)
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
// forecast sources (33kit / Moesekai / SekaRun / local) for final score prediction.
func (c *Controller) BuildPredictLineRequestFromTracker(req TrackerRankQuery) (*LineRequest, error) {
	normalized, err := c.validateTrackerQuery(req)
	if err != nil {
		return nil, err
	}
	if normalized.UserID != nil {
		return nil, fmt.Errorf("榜线预测暂不支持按用户查询，请使用排名")
	}
	if c.forecastCache == nil {
		return nil, fmt.Errorf("forecast cache is not configured")
	}

	meta := c.resolveEventMeta(normalized.EventID, renderregion.Normalize(normalized.Region))
	meta.applyOverrides(req)
	meta = c.applyWorldBloomChapterMeta(normalized, meta)
	if err := ensureSKPredictionAllowed(meta); err != nil {
		return nil, err
	}
	forecastQuery := buildForecastQuery(normalized)
	bySource, forecastErr := c.forecastCache.CachedBySourceQuery(forecastQuery)
	if forecastErr != nil {
		c.forecastCache.StartRefreshQuery(forecastQuery)
		return nil, fmt.Errorf("预测数据尚未就绪，请稍后再试: %w", forecastErr)
	}

	sourceOrder := forecastSourceDisplayOrder(normalized.Region, bySource)
	forecastRanks := forecastProvidedRanks(bySource)
	if len(forecastRanks) == 0 {
		c.forecastCache.StartRefreshQuery(forecastQuery)
		return nil, fmt.Errorf("预测缓存暂无这些档位的数据")
	}
	sourceNames := map[string]string{
		"33kit":    "33Kit预测",
		"moesekai": "Moesekai预测",
		"sekarun":  "SekaRun预测",
		"local":    "本地预测",
		"forecast": "预测",
	}

	columns := make([]drawing.SKForecastColumn, 0, len(sourceOrder))
	for _, sourceKey := range sourceOrder {
		sourceData, ok := bySource[sourceKey]
		if !ok || len(sourceData.Scores) == 0 {
			continue
		}
		rankInfos := make([]drawing.RankInfo, 0, len(forecastRanks))
		forecastAt := int64(0)
		for _, rank := range forecastRanks {
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

	if len(columns) == 0 {
		c.forecastCache.StartRefreshQuery(forecastQuery)
		return nil, fmt.Errorf("预测缓存暂无这些档位的数据")
	}

	currentRanks, currentErr := c.buildRanksFromTracker(
		normalized.Region,
		normalized.EventID,
		forecastRanks,
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

	// Keep rendering available forecast sources even when current tracker line fails.
	if currentErr != nil {
		currentRanks = nil
	}

	line := LineRequest{
		SklRequest: drawing.SklRequest{
			ID:               normalized.EventID,
			Region:           normalized.Region,
			StartAt:          meta.startAt,
			AggregateAt:      meta.aggregateAt,
			Name:             strings.TrimSpace(meta.name + " 预测"),
			BannerImgPath:    meta.bannerPath,
			Ranks:            currentRanks,
			CurrentRanks:     currentRanks,
			ForecastColumns:  columns,
			PredictionNotice: skPredictionNotice,
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

func (c *Controller) RenderPredictLineFromTracker(req TrackerRankQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	return c.renderPredictLineFromTracker(req)
}

func (c *Controller) renderPredictLineFromTracker(req TrackerRankQuery) ([]byte, error) {
	payload, err := c.BuildPredictLineRequestFromTracker(req)
	if err != nil {
		return nil, err
	}
	return c.RenderLine(*payload)
}

func buildForecastQuery(req TrackerRankQuery) ForecastQuery {
	query := ForecastQuery{
		Region:  req.Region,
		EventID: req.EventID,
		Ranks:   req.Ranks,
		Scope:   ForecastScopeTotal,
	}
	if req.WlCharacterID != nil && *req.WlCharacterID > 0 {
		query.Scope = ForecastScopeChapter
		query.WlCharacterID = req.WlCharacterID
	}
	return query
}

func forecastProvidedRanks(bySource map[string]ForecastSourceData) []int {
	if len(bySource) == 0 {
		return nil
	}
	seen := make(map[int]struct{})
	for _, sourceData := range bySource {
		for rank, score := range sourceData.Scores {
			if rank <= 0 || score.Score <= 0 {
				continue
			}
			seen[rank] = struct{}{}
		}
	}
	ranks := make([]int, 0, len(seen))
	for rank := range seen {
		ranks = append(ranks, rank)
	}
	sort.Ints(ranks)
	return ranks
}

func ensureSKPredictionAllowed(meta eventMeta) error {
	if meta.aggregateAt <= 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	if now >= meta.aggregateAt {
		return errors.New(skPredictionNoActiveMsg)
	}
	stopAt := meta.aggregateAt - int64(time.Hour/time.Millisecond)
	if now >= stopAt {
		return errors.New(skPredictionStopMessage)
	}
	return nil
}
