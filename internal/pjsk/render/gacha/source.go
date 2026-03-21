package gacha

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetGachaByID(id int) (*masterdata.Gacha, error)
	GetGachas() []*masterdata.Gacha
	GetCardByID(id int) (*masterdata.Card, error)
}
