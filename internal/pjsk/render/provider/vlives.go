package provider

import (
	"context"

	renderregion "haruki-cloud/internal/pjsk/region"
)

// VLive holds a virtual live entry.
type VLive struct {
	ID              int              `json:"id"`
	Name            string           `json:"name"`
	AssetBundleName string           `json:"asset_bundle_name,omitempty"`
	StartAt         int64            `json:"start_at"`
	EndAt           int64            `json:"end_at"`
	Schedules       []VLiveSchedule  `json:"schedules,omitempty"`
	Rewards         []VLiveReward    `json:"rewards,omitempty"`
	Characters      []VLiveCharacter `json:"characters,omitempty"`
	// VirtualLiveType and VirtualLiveGroupID are empty/0 on data that
	// predates them.
	VirtualLiveType    string `json:"virtual_live_type,omitempty"`
	VirtualLiveGroupID int    `json:"virtual_live_group_id,omitempty"`
	// The JP 7.0.0 solo virtual live cheer-point fields; nil/empty when the
	// region's data does not carry them.
	TotalCheerPointRewards       []VLiveTotalCheerPointReward `json:"total_cheer_point_rewards,omitempty"`
	TotalCheerPointSurplusReward *VLiveSurplusReward          `json:"total_cheer_point_surplus_reward,omitempty"`
	VirtualItemOverrideCost      *VLiveOverrideCost           `json:"virtual_item_override_cost,omitempty"`
}

// VLiveTypeSolo is the virtualLiveType of the JP 7.0.0 per-character solo
// virtual lives.
const VLiveTypeSolo = "solo_virtual_live"

type VLiveTotalCheerPointReward struct {
	Threshold     int `json:"threshold"`
	ResourceBoxID int `json:"resource_box_id"`
}

type VLiveSurplusReward struct {
	BasePoint     int `json:"base_point"`
	ResourceBoxID int `json:"resource_box_id"`
}

type VLiveOverrideCost struct {
	CostResourceType string `json:"cost_resource_type"`
	CostResourceID   int    `json:"cost_resource_id"`
	AssetBundleName  string `json:"asset_bundle_name,omitempty"`
}

// VLiveGroup is a virtualLiveGroups row.
type VLiveGroup struct {
	ID                   int
	Name                 string
	VirtualLiveGroupType string
	AssetBundleName      string
	StartAt              int64
	EndAt                int64
}

// VLiveSchedule is a single schedule entry within a virtual live.
type VLiveSchedule struct {
	StartAt int64 `json:"start_at"`
	EndAt   int64 `json:"end_at"`
}

type VLiveReward struct {
	VirtualLiveType string `json:"virtual_live_type"`
	ResourceBoxID   int    `json:"resource_box_id"`
}

type VLiveCharacter struct {
	GameCharacterUnitID        int    `json:"game_character_unit_id"`
	VirtualLivePerformanceType string `json:"virtual_live_performance_type"`
}

// VLiveProvider exposes virtual-live masterdata queries.
type VLiveProvider interface {
	GetLives(ctx context.Context, region renderregion.Value) ([]*VLive, error)
}

// VLiveGroupProvider serves virtualLiveGroups. ok is false when the region's
// data has no such table.
type VLiveGroupProvider interface {
	GetGroups(ctx context.Context, region renderregion.Value) (groups map[int]*VLiveGroup, ok bool)
}
