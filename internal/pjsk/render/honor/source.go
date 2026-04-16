package honor

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	GetHonorByID(id int) (*masterdata.Honor, error)
	GetHonorGroupByID(id int) (*masterdata.HonorGroup, error)
	GetBondsHonorByID(id int) (*masterdata.BondsHonor, error)
	GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool)
}
