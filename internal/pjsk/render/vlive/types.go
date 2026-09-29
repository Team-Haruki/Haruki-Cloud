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
	sources    *regionsource.Registry[DataSource]
	drawing    *drawing.HarukiDrawingClient
	assets     *assets.AssetHelper
	requestCtx context.Context
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

	VirtualLiveType string `json:"virtual_live_type,omitempty"`
	GroupID         int    `json:"group_id,omitempty"`

	TotalCheerPointRewards []CheerPointReward `json:"total_cheer_point_rewards,omitempty"`
	SurplusReward          *SurplusReward     `json:"surplus_reward,omitempty"`
	OverrideCost           *OverrideCost      `json:"override_cost,omitempty"`
}

// CheerPointReward is one virtualLiveTotalCheerPointRewards threshold.
type CheerPointReward struct {
	Threshold     int `json:"threshold"`
	ResourceBoxID int `json:"resource_box_id"`
}

// SurplusReward is virtualLiveTotalCheerPointSurplusReward.
type SurplusReward struct {
	BasePoint     int `json:"base_point"`
	ResourceBoxID int `json:"resource_box_id"`
}

// OverrideCost is virtualLiveVirtualItemOverrideCost.
type OverrideCost struct {
	ResourceType    string `json:"resource_type"`
	ResourceID      int    `json:"resource_id"`
	AssetBundleName string `json:"asset_bundle_name,omitempty"`
}

// Group is a virtualLiveGroups row.
type Group struct {
	ID              int
	Name            string
	Type            string
	AssetBundleName string
}

// groupDataSource is implemented by sources that can serve
// virtualLiveGroups; ok is false when the region has no such table.
type groupDataSource interface {
	GetGroups(region renderregion.Value) (map[int]*Group, bool)
}

// materialNameSource resolves material names for the override-cost item.
type materialNameSource interface {
	GetMaterialName(id int) string
}

type DetailQuery struct {
	Region   string    `json:"region,omitempty"`
	TimeZone string    `json:"timezone,omitempty"`
	Query    string    `json:"query,omitempty"`
	Now      time.Time `json:"-"`
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

	VirtualLiveType string
	GroupID         int
	// GroupName and Members are set on a list entry that stands for a whole
	// solo virtual live group.
	GroupName        string
	GroupBannerAsset string
	Members          []ResolvedLive
}
