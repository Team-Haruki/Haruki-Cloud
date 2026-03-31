package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// GachaProvider exposes gacha-related masterdata queries.
type GachaProvider interface {
	GetByID(id int) (*masterdata.Gacha, error)
	GetAll() []*masterdata.Gacha
	GetCardByID(id int) (*masterdata.Card, error)
}
