package stamp

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetStamps() ([]masterdata.Stamp, error)
}
