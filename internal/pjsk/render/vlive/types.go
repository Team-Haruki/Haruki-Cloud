package vlive

import (
	"context"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetLives(region renderregion.Value) ([]*Live, error)
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type Controller struct {
	source        DataSource
	defaultRegion renderregion.Value
}

// ProviderAdapter bridges provider.MasterDataProvider to vlive.DataSource.
type ProviderAdapter struct {
	provider.PjskProviderAdapterBase
}

type ListQuery struct {
	Region string    `json:"region,omitempty"`
	Now    time.Time `json:"-"`
}

type Schedule struct {
	StartAt int64 `json:"start_at"`
	EndAt   int64 `json:"end_at"`
}

type Live struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	StartAt   int64      `json:"start_at"`
	EndAt     int64      `json:"end_at"`
	Schedules []Schedule `json:"schedules,omitempty"`
}

type Window struct {
	StartAt time.Time
	EndAt   time.Time
}

type ResolvedLive struct {
	ID        int
	Name      string
	StartAt   time.Time
	EndAt     time.Time
	Current   *Window
	Living    bool
	RestCount int
}
