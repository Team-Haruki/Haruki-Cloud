package vlive

import (
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// DataSource provides VLive master data.
type DataSource interface {
	DefaultRegion() renderregion.Value
	GetLives(region renderregion.Value) ([]*Live, error)
}
