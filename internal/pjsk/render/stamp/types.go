package stamp

import (
	"context"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetStamps() ([]masterdata.Stamp, error)
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type Controller struct {
	sources *regionsource.Registry[DataSource]
	drawing *drawing.HarukiDrawingClient
	assets  *assets.AssetHelper
}

// ProviderAdapter bridges provider.MasterDataProvider to stamp.DataSource.
type ProviderAdapter struct {
	provider.ProviderAdapterBase
}

const stampPageSize = 25

type ListQuery struct {
	Region        renderregion.Value `json:"region"`
	PromptMessage string             `json:"prompt_message,omitempty"`
	IDs           []int              `json:"ids,omitempty"`
	CharacterIDs  []int              `json:"character_ids,omitempty"`
	Limit         int                `json:"limit,omitempty"`
	Page          int                `json:"page,omitempty"`
	All           bool               `json:"all,omitempty"`
}
