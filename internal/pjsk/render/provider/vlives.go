package provider

import renderregion "haruki-cloud/internal/pjsk/render/region"

// VLive holds a virtual live entry.
type VLive struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	StartAt   int64  `json:"start_at"`
	EndAt     int64  `json:"end_at"`
	Schedules []VLiveSchedule `json:"schedules,omitempty"`
}

// VLiveSchedule is a single schedule entry within a virtual live.
type VLiveSchedule struct {
	StartAt int64 `json:"start_at"`
	EndAt   int64 `json:"end_at"`
}

// VLiveProvider exposes virtual-live masterdata queries.
type VLiveProvider interface {
	GetLives(region renderregion.Value) ([]*VLive, error)
}
