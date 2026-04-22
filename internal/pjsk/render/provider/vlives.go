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
