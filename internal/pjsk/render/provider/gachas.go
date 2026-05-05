package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// GachaProvider exposes gacha-related masterdata queries.
type GachaProvider interface {
	GetByID(ctx context.Context, id int) (*masterdata.Gacha, error)
	GetAll(ctx context.Context) []*masterdata.Gacha
	GetCardByID(ctx context.Context, id int) (*masterdata.Card, error)
	GetCeilItemAssetbundleName(ctx context.Context, id int) (string, error)
}
