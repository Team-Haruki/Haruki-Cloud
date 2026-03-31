package sk

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	drawing *drawing.HarukiDrawingClient
	tracker TrackerSource
	events  *regionsource.Registry[EventSource]
	assets  *renderassets.AssetHelper
	censor  CensorService
}

// CensorService is a minimal interface for name censoring, satisfied by *censor.Service.
type CensorService interface {
	CensorName(ctx context.Context, harukiUserID int, userID string, name string, server string) bool
}

func NewController(drawingClient *drawing.HarukiDrawingClient) *Controller {
	return &Controller{
		drawing: drawingClient,
		events:  regionsource.NewRegistry[EventSource](renderregion.JP),
		assets:  renderassets.NewAssetHelper("", nil),
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

// censorTrackerName runs the name through CensorService for logging only and
// always returns the original tracker name so ranking outputs keep in-game names.
func (c *Controller) censorTrackerName(name, server string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return ""
	}
	if c.censor != nil {
		_ = c.censor.CensorName(context.Background(), 0, "", clean, server)
	}
	return clean
}

func (c *Controller) BuildLineRequest(req LineRequest) (*LineRequest, error) {
	if len(req.Ranks) == 0 {
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
	rankInfos, err := c.buildRanksOrUserFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.UserID, normalized.WlCharacterID)
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
	rankInfos, err := c.buildRanksOrUserFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.UserID, normalized.WlCharacterID)
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
	rankInfos, err := c.buildRanksFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.WlCharacterID)
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
	const speedPeriodSeconds = int64(20 * 60)
	speedInfos, err := c.buildSpeedInfosFromTracker(normalized.Region, normalized.EventID, normalized.Ranks, normalized.WlCharacterID, int(speedPeriodSeconds))
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
		RequestType:      "tracker",
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
		userID := *normalized.UserID
		var ranks []drawing.RankInfo
		if normalized.WlCharacterID != nil && *normalized.WlCharacterID > 0 {
			trace, err := c.tracker.TraceWorldBloomRankingByUser(normalized.Region, normalized.EventID, *normalized.WlCharacterID, userID)
			if err != nil {
				return nil, fmt.Errorf("tracker trace user %d query failed: %w", userID, err)
			}
			if trace == nil {
				return nil, fmt.Errorf("tracker trace user %d returned empty data", userID)
			}
			name := c.censorTrackerName(trace.UserData.Name, normalized.Region)
			for _, point := range trace.RankData {
				rankValue := point.Rank
				score := point.Score
				ranks = append(ranks, drawing.RankInfo{
					Rank:  rankValue,
					Name:  name,
					Score: drawing.IntPtr(score),
					Time:  formatTrackerTimestamp(point.Timestamp),
				})
			}
		} else {
			trace, err := c.tracker.TraceRankingByUser(normalized.Region, normalized.EventID, userID)
			if err != nil {
				return nil, fmt.Errorf("tracker trace user %d query failed: %w", userID, err)
			}
			if trace == nil {
				return nil, fmt.Errorf("tracker trace user %d returned empty data", userID)
			}
			name := c.censorTrackerName(trace.UserData.Name, normalized.Region)
			for _, point := range trace.RankData {
				rankValue := point.Rank
				score := point.Score
				ranks = append(ranks, drawing.RankInfo{
					Rank:  rankValue,
					Name:  name,
					Score: drawing.IntPtr(score),
					Time:  formatTrackerTimestamp(point.Timestamp),
				})
			}
		}

		sort.Slice(ranks, func(i, j int) bool {
			return fmt.Sprintf("%v", ranks[i].Time) < fmt.Sprintf("%v", ranks[j].Time)
		})
		if len(ranks) == 0 {
			return nil, fmt.Errorf("no trace data available for user %d", userID)
		}
		payload.Ranks = ranks

	case len(normalized.Ranks) > 0:
		if len(normalized.Ranks) > 2 {
			return nil, fmt.Errorf("player-trace 最多支持两个排名")
		}
		trace1, err := c.buildRankTraceFromTracker(normalized.Region, normalized.EventID, normalized.Ranks[0], normalized.WlCharacterID)
		if err != nil {
			return nil, err
		}
		if len(trace1) == 0 {
			return nil, fmt.Errorf("no trace data available for rank %d", normalized.Ranks[0])
		}
		payload.Ranks = trace1
		if len(normalized.Ranks) > 1 {
			trace2, err := c.buildRankTraceFromTracker(normalized.Region, normalized.EventID, normalized.Ranks[1], normalized.WlCharacterID)
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
