package sk

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/drawing"
	sekaiapi "haruki-cloud/utils/sekai"
)

type LineRequest struct {
	drawing.SklRequest
	Full bool `json:"full,omitempty"`
}

type TrackerSource interface {
	GetLatestRankingByRank(server string, eventID, rank int) (*sekaiapi.LatestRankingResponse, error)
	GetLatestRankingByUser(server string, eventID int, userID int64) (*sekaiapi.LatestRankingResponse, error)
	GetLatestWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomLatestRankingResponse, error)
	GetLatestWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomLatestRankingResponse, error)
	GetUserEventData(server string, eventID int, userID int64) (*sekaiapi.UserEventData, error)
	GetRankingScoreGrowth(server string, eventID, interval int) ([]sekaiapi.ScoreGrowthPoint, error)
	GetWorldBloomRankingScoreGrowth(server string, eventID, characterID, interval int) ([]sekaiapi.ScoreGrowthPoint, error)
	TraceRankingByRank(server string, eventID, rank int) (*sekaiapi.TraceRankingResponse, error)
	TraceRankingByUser(server string, eventID int, userID int64) (*sekaiapi.TraceRankingResponse, error)
	TraceWorldBloomRankingByRank(server string, eventID, characterID, rank int) (*sekaiapi.WorldBloomTraceRankingResponse, error)
	TraceWorldBloomRankingByUser(server string, eventID, characterID int, userID int64) (*sekaiapi.WorldBloomTraceRankingResponse, error)
}

type EventSource interface {
	DefaultRegion() renderregion.Value
	GetEventByID(id int) (*masterdata.Event, error)
	GetEvents() []*masterdata.Event
}

type TrackerRankQuery struct {
	EventID          int     `json:"event_id"`
	Region           string  `json:"region"`
	RegionExplicit   bool    `json:"region_explicit,omitempty"`
	Ranks            []int   `json:"ranks"`
	DefaultRanks     bool    `json:"default_ranks,omitempty"`
	SpeedUnit        string  `json:"speed_unit,omitempty"`
	SpeedPeriodSecs  int64   `json:"speed_period_seconds,omitempty"`
	UserID           *int64  `json:"user_id,omitempty"`
	TargetPlatform   string  `json:"target_platform,omitempty"`
	TargetUserID     string  `json:"target_user_id,omitempty"`
	TargetSelector   string  `json:"target_selector,omitempty"`
	WlCharacterID    *int    `json:"wl_character_id,omitempty"`
	WlCharacterQuery string  `json:"wl_character_query,omitempty"`
	Full             bool    `json:"full,omitempty"`
	EventName        *string `json:"event_name,omitempty"`
	EventStartAt     *int64  `json:"event_start_at,omitempty"`
	EventAggregateAt *int64  `json:"event_aggregate_at,omitempty"`
	BannerImgPath    *string `json:"banner_img_path,omitempty"`
}

type Controller struct {
	drawing    *drawing.HarukiDrawingClient
	tracker    TrackerSource
	forecast   ForecastProvider
	events     *regionsource.Registry[EventSource]
	assets     *renderassets.AssetHelper
	censor     CensorService
	requestCtx context.Context
}

// CensorService is a minimal interface for name censoring, satisfied by *censor.Service.
type CensorService interface {
	CensorName(ctx context.Context, harukiUserID int, userID string, name string, server string) bool
}

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return &Controller{
		drawing:  drawingClient,
		forecast: NewRemoteForecastProvider(),
		events:   regionsource.NewRegistry[EventSource](renderregion.JP),
		assets:   renderassets.NewAssetHelper("", nil),
	}
}

func (c *Controller) SetTrackerIntegration(tracker TrackerSource, events EventSource, assetHelper *renderassets.AssetHelper) {
	if c == nil {
		return
	}
	c.tracker = tracker
	c.RegisterEventSource(events)
	if assetHelper != nil {
		c.assets = assetHelper
	}
}

func (c *Controller) RegisterEventSource(events EventSource) {
	if c == nil || events == nil {
		return
	}
	if c.events == nil {
		c.events = regionsource.NewRegistry[EventSource](renderregion.JP)
	}
	c.events.RegisterSource(events)
}

func (c *Controller) SetCensor(svc CensorService) {
	if c == nil {
		return
	}
	c.censor = svc
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	return &clone
}

func (c *Controller) contextOrBackground() context.Context {
	if c != nil && c.requestCtx != nil {
		return c.requestCtx
	}
	return context.Background()
}

func (c *Controller) SetForecastProvider(provider ForecastProvider) {
	if c == nil || provider == nil {
		return
	}
	c.forecast = provider
}

// censorTrackerName runs the name through CensorService for logging only and
// always returns the original tracker name so ranking outputs keep in-game names.
func (c *Controller) censorTrackerName(name, server string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return ""
	}
	if c.censor != nil {
		_ = c.censor.CensorName(c.contextOrBackground(), 0, "", clean, server)
	}
	return clean
}

func (c *Controller) BuildLineRequest(req LineRequest) (*LineRequest, error) {
	if len(req.Ranks) == 0 && len(req.ForecastColumns) == 0 {
		return nil, fmt.Errorf("sk line request has no ranks")
	}
	return &req, nil
}

func (c *Controller) RenderLine(req LineRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildLineRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateSKLine(&payload.SklRequest, payload.Full)
}

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

func (c *Controller) buildPlayerTraceByRankFromTracker(server string, eventID, rank int, wlCharacterID *int) ([]drawing.RankInfo, error) {
	userID, err := c.resolveTrackerUserIDByRank(server, eventID, rank, wlCharacterID)
	if err != nil {
		return nil, err
	}
	return c.buildUserTraceFromTracker(server, eventID, userID, wlCharacterID)
}

func (c *Controller) resolveTrackerUserIDByRank(server string, eventID, rank int, wlCharacterID *int) (int64, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil {
			return 0, fmt.Errorf("tracker rank %d query failed: %w", rank, err)
		}
		uid, ok := parseTrackerUserID(latest.RankData.UserID, latest.UserData.UserID)
		if !ok {
			return 0, fmt.Errorf("tracker rank %d latest user id is empty", rank)
		}
		return uid, nil
	}
	latest, err := c.tracker.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return 0, fmt.Errorf("tracker rank %d query failed: %w", rank, err)
	}
	uid, ok := parseTrackerUserID(latest.RankData.UserID, latest.UserData.UserID)
	if !ok {
		return 0, fmt.Errorf("tracker rank %d latest user id is empty", rank)
	}
	return uid, nil
}

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

type eventMeta struct {
	name        string
	startAt     int64
	aggregateAt int64
	bannerPath  string
}

func (m *eventMeta) applyOverrides(req TrackerRankQuery) {
	if req.EventName != nil && strings.TrimSpace(*req.EventName) != "" {
		m.name = strings.TrimSpace(*req.EventName)
	}
	if req.EventStartAt != nil && *req.EventStartAt > 0 {
		m.startAt = *req.EventStartAt
	}
	if req.EventAggregateAt != nil && *req.EventAggregateAt > 0 {
		m.aggregateAt = *req.EventAggregateAt
	}
	if req.BannerImgPath != nil && strings.TrimSpace(*req.BannerImgPath) != "" {
		m.bannerPath = strings.TrimSpace(*req.BannerImgPath)
	}
}

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
			if strings.EqualFold(eventInfo.EventType, "world_bloom") && normalized.WlCharacterID == nil {
				return TrackerRankQuery{}, fmt.Errorf("world bloom event requires wl_character_id")
			}
		}
	}
	return normalized, nil
}

func shouldSkipMissingTrackerRanks(req TrackerRankQuery) bool {
	return req.DefaultRanks && req.UserID == nil
}

func normalizeTrackerSpeedConfig(req TrackerRankQuery) (periodSeconds int64, unitText string) {
	unit := strings.ToLower(strings.TrimSpace(req.SpeedUnit))
	switch unit {
	case "d", "day", "daily", "日":
		unitText = "日"
	default:
		unitText = "时"
	}

	periodSeconds = req.SpeedPeriodSecs
	if periodSeconds <= 0 {
		if unitText == "日" {
			periodSeconds = 24 * 60 * 60
		} else {
			periodSeconds = 60 * 60
		}
	}

	return periodSeconds, unitText
}

func shouldSkipMissingTrackerRankError(skipMissing bool, err error) bool {
	return skipMissing && errors.Is(err, sekaiapi.ErrRankingNotFound)
}

func (c *Controller) buildRanksFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, skipMissing bool) ([]drawing.RankInfo, error) {
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

func (c *Controller) buildSingleRankFromTracker(server string, eventID, rank int, wlCharacterID *int) (drawing.RankInfo, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil {
			return drawing.RankInfo{}, err
		}
		rankValue := rank
		if latest.RankData.Rank > 0 {
			rankValue = latest.RankData.Rank
		}
		score := latest.RankData.Score
		name := strings.TrimSpace(latest.UserData.Name)
		userID := strings.TrimSpace(latest.RankData.UserID)
		if userID == "" {
			userID = strings.TrimSpace(latest.UserData.UserID)
		}
		if rankValue <= 100 || c.shouldResolveTrackerNameByUser(server, eventID, name) {
			if resolved := c.resolveTrackerNameByUserID(server, eventID, userID, wlCharacterID); strings.TrimSpace(resolved) != "" {
				name = resolved
			}
		}
		info := drawing.RankInfo{
			Rank:  rankValue,
			Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
			Score: drawing.IntPtr(score),
			Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
		}
		uid, ok := parseTrackerUserID(latest.RankData.UserID, latest.UserData.UserID)
		c.enrichRankInfoPreferUser(server, eventID, rankValue, uid, ok, wlCharacterID, &info)
		if c.isTrackerEventTitleName(server, eventID, info.Name) {
			info.Name = fmt.Sprintf("Rank %d", info.Rank)
		}
		return info, nil
	}
	latest, err := c.tracker.GetLatestRankingByRank(server, eventID, rank)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	rankValue := rank
	if latest.RankData.Rank > 0 {
		rankValue = latest.RankData.Rank
	}
	score := latest.RankData.Score
	name := strings.TrimSpace(latest.UserData.Name)
	userID := strings.TrimSpace(latest.RankData.UserID)
	if userID == "" {
		userID = strings.TrimSpace(latest.UserData.UserID)
	}
	if rankValue <= 100 || c.shouldResolveTrackerNameByUser(server, eventID, name) {
		if resolved := c.resolveTrackerNameByUserID(server, eventID, userID, wlCharacterID); strings.TrimSpace(resolved) != "" {
			name = resolved
		}
	}
	info := drawing.RankInfo{
		Rank:  rankValue,
		Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
		Score: drawing.IntPtr(score),
		Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
	}
	uid, ok := parseTrackerUserID(latest.RankData.UserID, latest.UserData.UserID)
	c.enrichRankInfoPreferUser(server, eventID, rankValue, uid, ok, wlCharacterID, &info)
	if c.isTrackerEventTitleName(server, eventID, info.Name) {
		info.Name = fmt.Sprintf("Rank %d", info.Rank)
	}
	return info, nil
}

func (c *Controller) resolveTrackerNameByUserID(server string, eventID int, userID string, wlCharacterID *int) string {
	uid, err := strconv.ParseInt(strings.TrimSpace(userID), 10, 64)
	if err != nil || uid <= 0 {
		return ""
	}
	candidates := make([]string, 0, 2)
	userData, userErr := c.tracker.GetUserEventData(server, eventID, uid)
	if userErr == nil && userData != nil {
		candidates = append(candidates, strings.TrimSpace(userData.Name))
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, latestErr := c.tracker.GetLatestWorldBloomRankingByUser(server, eventID, *wlCharacterID, uid)
		if latestErr == nil && latest != nil {
			candidates = append(candidates, strings.TrimSpace(latest.UserData.Name))
		}
		return c.pickTrackerResolvedName(server, eventID, candidates...)
	}
	latest, latestErr := c.tracker.GetLatestRankingByUser(server, eventID, uid)
	if latestErr == nil && latest != nil {
		candidates = append(candidates, strings.TrimSpace(latest.UserData.Name))
	}
	return c.pickTrackerResolvedName(server, eventID, candidates...)
}

func (c *Controller) buildSingleUserFromTracker(server string, eventID int, userID int64, wlCharacterID *int) (drawing.RankInfo, error) {
	if wlCharacterID != nil && *wlCharacterID > 0 {
		latest, err := c.tracker.GetLatestWorldBloomRankingByUser(server, eventID, *wlCharacterID, userID)
		if err != nil {
			return drawing.RankInfo{}, err
		}
		rankValue := latest.RankData.Rank
		if rankValue <= 0 {
			rankValue = 1
		}
		score := latest.RankData.Score
		name := strings.TrimSpace(latest.UserData.Name)
		if c.shouldResolveTrackerNameByUser(server, eventID, name) {
			lookupUserID := strings.TrimSpace(latest.RankData.UserID)
			if lookupUserID == "" {
				lookupUserID = strconv.FormatInt(userID, 10)
			}
			if resolved := c.resolveTrackerNameByUserID(server, eventID, lookupUserID, wlCharacterID); strings.TrimSpace(resolved) != "" {
				name = resolved
			}
		}
		info := drawing.RankInfo{
			Rank:  rankValue,
			Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
			Score: drawing.IntPtr(score),
			Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
		}
		c.enrichRankInfoByUser(server, eventID, userID, wlCharacterID, &info)
		return info, nil
	}
	latest, err := c.tracker.GetLatestRankingByUser(server, eventID, userID)
	if err != nil {
		return drawing.RankInfo{}, err
	}
	rankValue := latest.RankData.Rank
	if rankValue <= 0 {
		rankValue = 1
	}
	score := latest.RankData.Score
	name := strings.TrimSpace(latest.UserData.Name)
	if c.shouldResolveTrackerNameByUser(server, eventID, name) {
		lookupUserID := strings.TrimSpace(latest.RankData.UserID)
		if lookupUserID == "" {
			lookupUserID = strconv.FormatInt(userID, 10)
		}
		if resolved := c.resolveTrackerNameByUserID(server, eventID, lookupUserID, wlCharacterID); strings.TrimSpace(resolved) != "" {
			name = resolved
		}
	}
	info := drawing.RankInfo{
		Rank:  rankValue,
		Name:  pickTrackerDisplayName(c.censorTrackerName(name, server), rankValue),
		Score: drawing.IntPtr(score),
		Time:  formatTrackerTimestamp(latest.RankData.Timestamp),
	}
	c.enrichRankInfoByUser(server, eventID, userID, wlCharacterID, &info)
	return info, nil
}

type trackerScoreSample struct {
	score     int
	timestamp int64
}

type trackerRankScoreSample struct {
	rank      int
	score     int
	timestamp int64
}

func (c *Controller) enrichRankInfoByRank(server string, eventID, rank int, wlCharacterID *int, info *drawing.RankInfo) {
	if c == nil || c.tracker == nil || info == nil || rank <= 0 {
		return
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil || trace == nil {
			return
		}
		name := strings.TrimSpace(c.censorTrackerName(trace.UserData.Name, server))
		if c.shouldResolveTrackerNameByUser(server, eventID, name) {
			ids := make([]string, 0, len(trace.RankData)+1)
			ids = append(ids, trace.UserData.UserID)
			for _, point := range trace.RankData {
				ids = append(ids, point.UserID)
			}
			resolved := c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...)
			name = strings.TrimSpace(c.censorTrackerName(resolved, server))
		}
		if c.shouldReplaceTrackerName(server, eventID, info.Name, name) {
			info.Name = name
		}
		samples := make([]trackerScoreSample, 0, len(trace.RankData))
		for _, point := range trace.RankData {
			samples = append(samples, trackerScoreSample{
				score:     point.Score,
				timestamp: point.Timestamp,
			})
		}
		applyRankInfoMetrics(info, samples)
		return
	}

	trace, err := c.tracker.TraceRankingByRank(server, eventID, rank)
	if err != nil || trace == nil {
		return
	}
	name := strings.TrimSpace(c.censorTrackerName(trace.UserData.Name, server))
	if c.shouldResolveTrackerNameByUser(server, eventID, name) {
		ids := make([]string, 0, len(trace.RankData)+1)
		ids = append(ids, trace.UserData.UserID)
		for _, point := range trace.RankData {
			ids = append(ids, point.UserID)
		}
		resolved := c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...)
		name = strings.TrimSpace(c.censorTrackerName(resolved, server))
	}
	if c.shouldReplaceTrackerName(server, eventID, info.Name, name) {
		info.Name = name
	}
	samples := make([]trackerScoreSample, 0, len(trace.RankData))
	for _, point := range trace.RankData {
		samples = append(samples, trackerScoreSample{
			score:     point.Score,
			timestamp: point.Timestamp,
		})
	}
	applyRankInfoMetrics(info, samples)
}

func (c *Controller) enrichRankInfoByUser(server string, eventID int, userID int64, wlCharacterID *int, info *drawing.RankInfo) {
	if c == nil || c.tracker == nil || info == nil || userID <= 0 {
		return
	}
	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByUser(server, eventID, *wlCharacterID, userID)
		if err != nil || trace == nil {
			return
		}
		name := strings.TrimSpace(c.censorTrackerName(trace.UserData.Name, server))
		if c.shouldResolveTrackerNameByUser(server, eventID, name) {
			ids := make([]string, 0, len(trace.RankData)+2)
			ids = append(ids, strconv.FormatInt(userID, 10), trace.UserData.UserID)
			for _, point := range trace.RankData {
				ids = append(ids, point.UserID)
			}
			resolved := c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...)
			name = strings.TrimSpace(c.censorTrackerName(resolved, server))
		}
		if c.shouldReplaceTrackerName(server, eventID, info.Name, name) {
			info.Name = name
		}
		samples := make([]trackerScoreSample, 0, len(trace.RankData))
		for _, point := range trace.RankData {
			samples = append(samples, trackerScoreSample{
				score:     point.Score,
				timestamp: point.Timestamp,
			})
		}
		applyRankInfoMetrics(info, samples)
		return
	}

	trace, err := c.tracker.TraceRankingByUser(server, eventID, userID)
	if err != nil || trace == nil {
		return
	}
	name := strings.TrimSpace(c.censorTrackerName(trace.UserData.Name, server))
	if c.shouldResolveTrackerNameByUser(server, eventID, name) {
		ids := make([]string, 0, len(trace.RankData)+2)
		ids = append(ids, strconv.FormatInt(userID, 10), trace.UserData.UserID)
		for _, point := range trace.RankData {
			ids = append(ids, point.UserID)
		}
		resolved := c.resolveTrackerNameByUserIDs(server, eventID, wlCharacterID, ids...)
		name = strings.TrimSpace(c.censorTrackerName(resolved, server))
	}
	if c.shouldReplaceTrackerName(server, eventID, info.Name, name) {
		info.Name = name
	}
	samples := make([]trackerScoreSample, 0, len(trace.RankData))
	for _, point := range trace.RankData {
		samples = append(samples, trackerScoreSample{
			score:     point.Score,
			timestamp: point.Timestamp,
		})
	}
	applyRankInfoMetrics(info, samples)
}

func (c *Controller) resolveTrackerNameByUserIDs(server string, eventID int, wlCharacterID *int, userIDs ...string) string {
	if c == nil {
		return ""
	}
	seen := map[string]struct{}{}
	for _, raw := range userIDs {
		uid := strings.TrimSpace(raw)
		if uid == "" {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		if name := strings.TrimSpace(c.resolveTrackerNameByUserID(server, eventID, uid, wlCharacterID)); name != "" {
			return name
		}
	}
	return ""
}

func parseTrackerUserID(userIDs ...string) (int64, bool) {
	for _, raw := range userIDs {
		uid, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err == nil && uid > 0 {
			return uid, true
		}
	}
	return 0, false
}

func hasRankInfoMetrics(info *drawing.RankInfo) bool {
	if info == nil {
		return false
	}
	return info.AveragePt != nil ||
		info.LatestPt != nil ||
		info.Speed != nil ||
		info.HourRound != nil ||
		info.Min20Time3Speed != nil
}

func (c *Controller) enrichRankInfoPreferUser(server string, eventID, rank int, userID int64, hasUserID bool, wlCharacterID *int, info *drawing.RankInfo) {
	if c == nil || info == nil {
		return
	}
	if hasUserID && userID > 0 {
		c.enrichRankInfoByUser(server, eventID, userID, wlCharacterID, info)
		if hasRankInfoMetrics(info) {
			return
		}
	}
	c.enrichRankInfoByRank(server, eventID, rank, wlCharacterID, info)
}

func (c *Controller) shouldReplaceTrackerName(server string, eventID int, current, candidate string) bool {
	next := strings.TrimSpace(candidate)
	if next == "" {
		return false
	}
	if c.shouldResolveTrackerNameByUser(server, eventID, next) {
		return false
	}
	prev := strings.TrimSpace(current)
	if prev == "" {
		return true
	}
	if c.shouldResolveTrackerNameByUser(server, eventID, prev) {
		return true
	}
	return isTrackerPlaceholderName(prev) && !isTrackerPlaceholderName(next)
}

func isTrackerPlaceholderName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "rank ") {
		return false
	}
	_, err := strconv.Atoi(strings.TrimSpace(trimmed[len("rank "):]))
	return err == nil
}

func (c *Controller) shouldResolveTrackerNameByUser(server string, eventID int, name string) bool {
	clean := strings.TrimSpace(name)
	if clean == "" || isTrackerPlaceholderName(clean) {
		return true
	}
	return c.isTrackerEventTitleName(server, eventID, clean)
}

func (c *Controller) eventTitleForNameCheck(server string, eventID int) string {
	if c == nil || eventID <= 0 {
		return ""
	}
	region := renderregion.Normalize(server)
	if region.IsZero() {
		region = renderregion.JP
	}
	eventSource := c.eventSourceForRegion(region.String())
	if eventSource == nil {
		return ""
	}
	eventInfo, err := eventSource.GetEventByID(eventID)
	if err != nil || eventInfo == nil {
		return ""
	}
	return strings.TrimSpace(eventInfo.Name)
}

func (c *Controller) pickTrackerResolvedName(server string, eventID int, candidates ...string) string {
	for _, raw := range candidates {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if c.shouldResolveTrackerNameByUser(server, eventID, name) {
			continue
		}
		return name
	}
	return ""
}

func (c *Controller) isTrackerEventTitleName(server string, eventID int, name string) bool {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return false
	}
	meta := strings.TrimSpace(c.eventTitleForNameCheck(server, eventID))
	if meta == "" {
		return false
	}
	return isTrackerEventTitleFuzzyMatch(clean, meta)
}

func isTrackerEventTitleFuzzyMatch(name, eventTitle string) bool {
	left := normalizeTrackerNameForCompare(name)
	right := normalizeTrackerNameForCompare(eventTitle)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	if len([]rune(left)) >= 6 && strings.Contains(left, right) {
		return true
	}
	if len([]rune(right)) >= 6 && strings.Contains(right, left) {
		return true
	}
	return false
}

func normalizeTrackerNameForCompare(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range strings.ToLower(raw) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func applyRankInfoMetrics(info *drawing.RankInfo, samples []trackerScoreSample) {
	if info == nil || len(samples) == 0 {
		return
	}

	normalized := make([]trackerScoreSample, 0, len(samples))
	for _, sample := range samples {
		if sample.timestamp <= 0 {
			continue
		}
		normalized = append(normalized, sample)
	}
	if len(normalized) == 0 {
		return
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalizeTrackerUnixSeconds(normalized[i].timestamp) < normalizeTrackerUnixSeconds(normalized[j].timestamp)
	})

	info.RecordStartAt = formatTrackerTimestamp(normalized[0].timestamp)
	if len(normalized) < 2 {
		return
	}

	deltas := make([]int, 0, len(normalized)-1)
	for i := 1; i < len(normalized); i++ {
		diff := normalized[i].score - normalized[i-1].score
		if diff > 0 {
			deltas = append(deltas, diff)
		}
	}
	if len(deltas) > 0 {
		latest := deltas[len(deltas)-1]
		info.LatestPt = drawing.IntPtr(latest)

		avgWindow := deltas
		if len(avgWindow) > 10 {
			avgWindow = avgWindow[len(avgWindow)-10:]
		}
		sum := 0
		for _, value := range avgWindow {
			sum += value
		}
		roundCount := len(avgWindow)
		if roundCount > 0 {
			avg := sum / roundCount
			info.AverageRound = drawing.IntPtr(roundCount)
			info.AveragePt = drawing.IntPtr(avg)
		}
	}

	last := normalized[len(normalized)-1]
	endSec := normalizeTrackerUnixSeconds(last.timestamp)

	// Speed/HourRound use the last ~1h window to match tracker "近1小时" semantics.
	hourStart := endSec - 60*60
	hourBaseIdx := findWindowBaselineIndex(normalized, hourStart)
	if hourBaseIdx >= 0 {
		hourBase := normalized[hourBaseIdx]
		hourBaseSec := normalizeTrackerUnixSeconds(hourBase.timestamp)
		if endSec > hourBaseSec && last.score > hourBase.score {
			hourGain := last.score - hourBase.score
			hourElapsed := endSec - hourBaseSec
			speed := int((int64(hourGain) * 3600) / hourElapsed)
			if speed > 0 {
				info.Speed = drawing.IntPtr(speed)
			}
		}

		hourRound := countPositiveDeltas(normalized[hourBaseIdx:])
		if hourRound > 0 {
			info.HourRound = drawing.IntPtr(hourRound)
		}
	}

	// 20min×3 is computed as gain in last 20min multiplied by 3.
	windowStart := endSec - 20*60
	windowBaseIdx := findWindowBaselineIndex(normalized, windowStart)
	if windowBaseIdx >= 0 {
		windowBase := normalized[windowBaseIdx]
		windowGain := last.score - windowBase.score
		if windowGain > 0 {
			windowSpeed := windowGain * 3
			if windowSpeed > 0 {
				info.Min20Time3Speed = drawing.IntPtr(windowSpeed)
			}
		}
	}
}

func normalizeTrackerUnixSeconds(ts int64) int64 {
	if ts > 1_000_000_000_000 {
		return ts / 1000
	}
	return ts
}

func findWindowBaselineIndex(samples []trackerScoreSample, windowStart int64) int {
	if len(samples) == 0 {
		return -1
	}
	baseline := -1
	for i := range samples {
		sec := normalizeTrackerUnixSeconds(samples[i].timestamp)
		if sec <= windowStart {
			baseline = i
			continue
		}
		break
	}
	if baseline >= 0 {
		return baseline
	}
	return 0
}

func countPositiveDeltas(samples []trackerScoreSample) int {
	if len(samples) < 2 {
		return 0
	}
	count := 0
	for i := 1; i < len(samples); i++ {
		if samples[i].score-samples[i-1].score > 0 {
			count++
		}
	}
	return count
}

func (c *Controller) buildSpeedInfosFromTracker(server string, eventID int, ranks []int, wlCharacterID *int, interval int, unitPeriodSeconds int64, skipMissing bool) ([]drawing.SpeedInfo, error) {
	var (
		points []sekaiapi.ScoreGrowthPoint
		err    error
	)
	if wlCharacterID != nil && *wlCharacterID > 0 {
		points, err = c.tracker.GetWorldBloomRankingScoreGrowth(server, eventID, *wlCharacterID, interval)
	} else {
		points, err = c.tracker.GetRankingScoreGrowth(server, eventID, interval)
	}
	if err != nil {
		points = nil
	}
	pointByRank := make(map[int]sekaiapi.ScoreGrowthPoint, len(points))
	for _, point := range points {
		if point.Rank <= 0 {
			continue
		}
		existing, ok := pointByRank[point.Rank]
		if !ok || point.TimestampLatest > existing.TimestampLatest {
			pointByRank[point.Rank] = point
		}
	}

	result := make([]drawing.SpeedInfo, 0, len(ranks))
	for _, rank := range ranks {
		if point, ok := pointByRank[rank]; ok {
			info := speedInfoFromGrowthPoint(point, unitPeriodSeconds)
			if info.Speed == nil {
				if traceInfo, traceOK := c.buildSpeedInfoFromTrace(server, eventID, rank, wlCharacterID, interval, unitPeriodSeconds); traceOK {
					// Prefer score/time from growth endpoint when present because it
					// reflects the tracker speed aggregation point used by drawing.
					if info.Score > 0 {
						traceInfo.Score = info.Score
					}
					if point.TimestampLatest > 0 {
						traceInfo.RecordTime = formatTrackerTimestamp(point.TimestampLatest)
					}
					result = append(result, traceInfo)
					continue
				}
			}
			result = append(result, info)
			continue
		}
		if traceInfo, traceOK := c.buildSpeedInfoFromTrace(server, eventID, rank, wlCharacterID, interval, unitPeriodSeconds); traceOK {
			result = append(result, traceInfo)
			continue
		}
		info, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
			if shouldSkipMissingTrackerRankError(skipMissing, err) {
				continue
			}
			return nil, fmt.Errorf("tracker speed rank %d query failed: %w", rank, err)
		}
		score := 0
		if info.Score != nil {
			score = *info.Score
		}
		result = append(result, drawing.SpeedInfo{
			Rank:       info.Rank,
			Score:      score,
			RecordTime: info.Time,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Rank < result[j].Rank
	})
	return result, nil
}

func (c *Controller) buildRankTraceFromTracker(server string, eventID, rank int, wlCharacterID *int) ([]drawing.RankInfo, error) {
	result := make([]drawing.RankInfo, 0)
	latestName := ""
	if latest, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID); err == nil {
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
		ti := fmt.Sprintf("%v", result[i].Time)
		tj := fmt.Sprintf("%v", result[j].Time)
		return ti < tj
	})
	if len(result) == 0 {
		latest, err := c.buildSingleRankFromTracker(server, eventID, rank, wlCharacterID)
		if err != nil {
			return nil, fmt.Errorf("tracker trace fallback rank %d query failed: %w", rank, err)
		}
		return []drawing.RankInfo{latest}, nil
	}
	return result, nil
}

func (c *Controller) buildSpeedInfoFromTrace(server string, eventID, rank int, wlCharacterID *int, interval int, unitPeriodSeconds int64) (drawing.SpeedInfo, bool) {
	if c == nil || c.tracker == nil || rank <= 0 {
		return drawing.SpeedInfo{}, false
	}
	if interval <= 0 {
		interval = 60 * 60
	}
	if unitPeriodSeconds <= 0 {
		unitPeriodSeconds = 60 * 60
	}
	samples := make([]trackerRankScoreSample, 0)
	if wlCharacterID != nil && *wlCharacterID > 0 {
		trace, err := c.tracker.TraceWorldBloomRankingByRank(server, eventID, *wlCharacterID, rank)
		if err != nil || trace == nil {
			return drawing.SpeedInfo{}, false
		}
		samples = make([]trackerRankScoreSample, 0, len(trace.RankData))
		for _, point := range trace.RankData {
			if point.Timestamp <= 0 {
				continue
			}
			rankValue := rank
			if point.Rank > 0 {
				rankValue = point.Rank
			}
			samples = append(samples, trackerRankScoreSample{
				rank:      rankValue,
				score:     point.Score,
				timestamp: point.Timestamp,
			})
		}
	} else {
		trace, err := c.tracker.TraceRankingByRank(server, eventID, rank)
		if err != nil || trace == nil {
			return drawing.SpeedInfo{}, false
		}
		samples = make([]trackerRankScoreSample, 0, len(trace.RankData))
		for _, point := range trace.RankData {
			if point.Timestamp <= 0 {
				continue
			}
			rankValue := rank
			if point.Rank > 0 {
				rankValue = point.Rank
			}
			samples = append(samples, trackerRankScoreSample{
				rank:      rankValue,
				score:     point.Score,
				timestamp: point.Timestamp,
			})
		}
	}
	if len(samples) == 0 {
		return drawing.SpeedInfo{}, false
	}

	sort.Slice(samples, func(i, j int) bool {
		return normalizeTrackerUnixSeconds(samples[i].timestamp) < normalizeTrackerUnixSeconds(samples[j].timestamp)
	})
	last := samples[len(samples)-1]
	info := drawing.SpeedInfo{
		Rank:       last.rank,
		Score:      last.score,
		RecordTime: formatTrackerTimestamp(last.timestamp),
	}
	if len(samples) < 2 {
		return info, true
	}

	endSec := normalizeTrackerUnixSeconds(last.timestamp)
	windowStart := endSec - int64(interval)
	baseIdx := 0
	for i := range samples {
		sec := normalizeTrackerUnixSeconds(samples[i].timestamp)
		if sec <= windowStart {
			baseIdx = i
			continue
		}
		break
	}
	base := samples[baseIdx]
	baseSec := normalizeTrackerUnixSeconds(base.timestamp)
	if endSec > baseSec && last.score > base.score {
		speed := int((int64(last.score-base.score) * unitPeriodSeconds) / (endSec - baseSec))
		if speed > 0 {
			info.Speed = drawing.IntPtr(speed)
		}
	}
	return info, true
}

func speedInfoFromGrowthPoint(point sekaiapi.ScoreGrowthPoint, unitPeriodSeconds int64) drawing.SpeedInfo {
	var speed *int
	if unitPeriodSeconds <= 0 {
		unitPeriodSeconds = 60 * 60
	}

	growth := point.Growth
	if (growth == nil || *growth <= 0) && point.ScoreEarlier != nil {
		val := point.ScoreLatest - *point.ScoreEarlier
		if val > 0 {
			growth = &val
		}
	}

	timeDiff := point.TimeDiff
	if (timeDiff == nil || *timeDiff <= 0) && point.TimestampEarlier != nil {
		latest := normalizeTrackerUnixSeconds(point.TimestampLatest)
		earlier := normalizeTrackerUnixSeconds(*point.TimestampEarlier)
		diff := latest - earlier
		if diff > 0 {
			timeDiff = &diff
		}
	}

	if growth != nil && *growth > 0 && timeDiff != nil && *timeDiff > 0 {
		val := int((int64(*growth) * unitPeriodSeconds) / *timeDiff)
		speed = &val
	}
	score := point.ScoreLatest
	if score <= 0 && point.ScoreEarlier != nil {
		score = *point.ScoreEarlier
	}
	recordTs := point.TimestampLatest
	if recordTs <= 0 && point.TimestampEarlier != nil {
		recordTs = *point.TimestampEarlier
	}
	return drawing.SpeedInfo{
		Rank:       point.Rank,
		Score:      score,
		Speed:      speed,
		RecordTime: formatTrackerTimestamp(recordTs),
	}
}

func (c *Controller) resolveEventMeta(eventID int, region renderregion.Value) eventMeta {
	const defaultWindow = int64(6 * time.Hour / time.Millisecond)
	now := time.Now().UnixMilli()
	meta := eventMeta{
		name:        fmt.Sprintf("Event #%d", eventID),
		startAt:     now - defaultWindow,
		aggregateAt: now + defaultWindow,
	}
	eventSource := c.eventSourceForRegion(region.String())
	if c == nil || eventSource == nil {
		return meta
	}
	eventInfo, err := eventSource.GetEventByID(eventID)
	if err != nil || eventInfo == nil {
		return meta
	}
	if strings.TrimSpace(eventInfo.Name) != "" {
		meta.name = strings.TrimSpace(eventInfo.Name)
	}
	if eventInfo.StartAt > 0 {
		meta.startAt = eventInfo.StartAt
	}
	if eventInfo.AggregateAt > 0 {
		meta.aggregateAt = eventInfo.AggregateAt
	}
	if path := c.resolveEventBannerPath(eventInfo.AssetBundleName, region); path != "" {
		meta.bannerPath = path
	}
	return meta
}

func (c *Controller) resolveEventBannerPath(assetBundleName string, region renderregion.Value) string {
	if c == nil || c.assets == nil || strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return renderassets.ResolveRegionAssetPath(
		c.assets, renderregion.WithDefault(region).String(),
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
	)
}

func (c *Controller) resolveCharacterIconPath(characterID int, _ renderregion.Value) string {
	if c == nil || c.assets == nil || characterID <= 0 {
		return ""
	}
	if nickname := renderassets.CharacterIDToNickname[characterID]; nickname != "" {
		return renderassets.ResolveAssetPath(
			c.assets,
			renderassets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
		)
	}
	return renderassets.ResolveAssetPath(
		c.assets,
		renderassets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
	)
}

func (c *Controller) pickCurrentOrNextEventID(region string) int {
	eventSource := c.eventSourceForRegion(region)
	if c == nil || eventSource == nil {
		return 0
	}
	now := time.Now().UnixMilli()
	var current *masterdata.Event
	var next *masterdata.Event
	var latest *masterdata.Event
	for _, eventInfo := range eventSource.GetEvents() {
		if eventInfo == nil {
			continue
		}
		if latest == nil || eventInfo.StartAt > latest.StartAt {
			latest = eventInfo
		}
		if eventInfo.StartAt <= now && now <= eventInfo.AggregateAt {
			if current == nil || eventInfo.StartAt > current.StartAt {
				current = eventInfo
			}
			continue
		}
		if eventInfo.StartAt > now {
			if next == nil || eventInfo.StartAt < next.StartAt {
				next = eventInfo
			}
		}
	}
	if current != nil {
		return current.ID
	}
	if next != nil {
		return next.ID
	}
	if latest != nil {
		return latest.ID
	}
	return 0
}

func (c *Controller) eventSourceForRegion(region string) EventSource {
	if c == nil || c.events == nil {
		return nil
	}
	src, ok := c.events.SourceForRegion(renderregion.Normalize(region))
	if !ok {
		return nil
	}
	return src
}

func normalizeTrackerServer(region string) string {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "jp", "cn", "tw", "kr", "en":
		return strings.ToLower(strings.TrimSpace(region))
	default:
		return ""
	}
}

func normalizeRanks(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	out := make([]int, 0, len(values))
	seen := make(map[int]struct{}, len(values))
	for _, rank := range values {
		if rank <= 0 {
			continue
		}
		if _, ok := seen[rank]; ok {
			continue
		}
		seen[rank] = struct{}{}
		out = append(out, rank)
	}
	sort.Ints(out)
	return out
}

func formatTrackerTimestamp(ts int64) string {
	if ts <= 0 {
		return time.Now().UTC().Format(time.RFC3339)
	}
	if ts > 1_000_000_000_000 {
		return time.UnixMilli(ts).UTC().Format(time.RFC3339)
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func pickTrackerDisplayName(name string, rank int) string {
	clean := strings.TrimSpace(name)
	if clean != "" {
		return clean
	}
	return fmt.Sprintf("Rank %d", rank)
}
