package sk

import (
	"context"
	"fmt"
	"strings"

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
