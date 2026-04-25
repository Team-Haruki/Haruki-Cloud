package sk

import (
	"context"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"github.com/go-resty/resty/v2"
)

// ── SK query ────────────────────────────────────────────────────────────────

type LineRequest struct {
	drawing.SklRequest
	Full bool `json:"full,omitempty"`
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

// ── Data sources ────────────────────────────────────────────────────────────

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

type contextualTrackerSource interface {
	WithContext(ctx context.Context) *sekaiapi.TrackerClient
}

type EventSource interface {
	DefaultRegion() renderregion.Value
	GetEventByID(id int) (*masterdata.Event, error)
	GetEvents() []*masterdata.Event
}

// CensorService is a minimal interface for name censoring, satisfied by *censor.Service.
type CensorService interface {
	CensorName(ctx context.Context, harukiUserID int, userID string, name string, server string) bool
}

// ── Controller ──────────────────────────────────────────────────────────────

type Controller struct {
	drawing      *drawing.HarukiDrawingClient
	tracker      TrackerSource
	forecast     ForecastProvider
	events       *regionsource.Registry[EventSource]
	assets       *renderassets.AssetHelper
	censor       CensorService
	predictCache *predictRenderCache
	requestCtx   context.Context
}

// ── Forecast ────────────────────────────────────────────────────────────────

type ForecastScore struct {
	Score     int
	Timestamp int64
	Source    string
}

type ForecastSourceData struct {
	Scores    map[int]ForecastScore
	FetchedAt int64
}

type ForecastProvider interface {
	Fetch(ctx context.Context, region string, eventID int, ranks []int) (map[int]ForecastScore, error)
}

type ForecastProviderBySource interface {
	FetchBySource(ctx context.Context, region string, eventID int, ranks []int) (map[string]ForecastSourceData, error)
}

type RemoteForecastProvider struct {
	http *resty.Client
}

// ── Internal helpers ────────────────────────────────────────────────────────

type eventMeta struct {
	name        string
	startAt     int64
	aggregateAt int64
	bannerPath  string
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
