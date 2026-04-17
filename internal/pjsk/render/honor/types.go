package honor

import (
	"context"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

// ── Data source ─────────────────────────────────────────────────────────────

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetHonorByID(id int) (*masterdata.Honor, error)
	GetHonorGroupByID(id int) (*masterdata.HonorGroup, error)
	GetBondsHonorByID(id int) (*masterdata.BondsHonor, error)
	GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool)
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

// ── Controller & builder ────────────────────────────────────────────────────

type Controller struct {
	sources *regionsource.Registry[DataSource]
	drawing *drawing.HarukiDrawingClient
	assets  *assets.AssetHelper
}

type Builder struct {
	source DataSource
	assets *assets.AssetHelper
}

// ── Query ───────────────────────────────────────────────────────────────────

type Query struct {
	Region              renderregion.Value `json:"region"`
	HonorID             int                `json:"honor_id"`
	HonorLevel          int                `json:"honor_level,omitempty"`
	IsMain              bool               `json:"is_main,omitempty"`
	BondsHonorViewType  string             `json:"bonds_honor_view_type,omitempty"`
	BondsHonorWordID    int                `json:"bonds_honor_word_id,omitempty"`
	FcOrApLevelOverride *int               `json:"fc_or_ap_level_override,omitempty"`
}
