package vlive

import (
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// Source provides VLive master data.
type Source interface {
	DefaultRegion() renderregion.Value
	GetLives(region renderregion.Value) ([]*Live, error)
}
