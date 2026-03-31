package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// HonorProvider exposes honor-related masterdata queries.
type HonorProvider interface {
	GetByID(id int) (*masterdata.Honor, error)
	GetGroupByID(id int) (*masterdata.HonorGroup, error)
	GetBondsHonorByID(id int) (*masterdata.BondsHonor, error)
	GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool)
	GetEventIDByHonorID(honorID int) int
}
