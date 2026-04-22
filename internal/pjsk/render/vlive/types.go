package vlive

import (
	"context"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetLives(region renderregion.Value) ([]*Live, error)
	GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error)
	GetResourceBoxByPurpose(purpose string, id int) *provider.ResourceBox
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type eventBannerDataSource interface {
	GetEventByVirtualLiveID(id int) (*masterdata.Event, error)
}

type Controller struct {
	sources *regionsource.Registry[DataSource]
	drawing *drawing.HarukiDrawingClient
	assets  *assets.AssetHelper
}

// ProviderAdapter bridges provider.MasterDataProvider to vlive.DataSource.
type ProviderAdapter struct {
	provider.PjskProviderAdapterBase
}

type ListQuery struct {
	Region   string    `json:"region,omitempty"`
	TimeZone string    `json:"timezone,omitempty"`
	Now      time.Time `json:"-"`
}

type Schedule struct {
	StartAt int64 `json:"start_at"`
	EndAt   int64 `json:"end_at"`
}

type Reward struct {
	VirtualLiveType string `json:"virtual_live_type"`
	ResourceBoxID   int    `json:"resource_box_id"`
}

type Character struct {
	GameCharacterUnitID        int    `json:"game_character_unit_id"`
	VirtualLivePerformanceType string `json:"virtual_live_performance_type"`
}

type Live struct {
	ID              int         `json:"id"`
	Name            string      `json:"name"`
	AssetBundleName string      `json:"asset_bundle_name,omitempty"`
	StartAt         int64       `json:"start_at"`
	EndAt           int64       `json:"end_at"`
	Schedules       []Schedule  `json:"schedules,omitempty"`
	Rewards         []Reward    `json:"rewards,omitempty"`
	Characters      []Character `json:"characters,omitempty"`
}

type Window struct {
	StartAt time.Time
	EndAt   time.Time
}

type ResolvedLive struct {
	ID              int
	Name            string
	AssetBundleName string
	StartAt         time.Time
	EndAt           time.Time
	Current         *Window
	Living          bool
	RestCount       int
	Rewards         []Reward
	Characters      []Character
}
