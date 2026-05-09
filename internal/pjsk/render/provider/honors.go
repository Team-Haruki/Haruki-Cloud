package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// HonorProvider exposes honor-related masterdata queries.
type HonorProvider interface {
	GetByID(ctx context.Context, id int) (*masterdata.Honor, error)
	GetGroupByID(ctx context.Context, id int) (*masterdata.HonorGroup, error)
	GetBondsHonorByID(ctx context.Context, id int) (*masterdata.BondsHonor, error)
	GetBondsHonorWordByID(ctx context.Context, id int) (*masterdata.BondsHonorWord, error)
	GetGameCharacterUnitByID(ctx context.Context, id int) (*masterdata.GameCharacterUnit, bool)
	GetEventIDByHonorID(ctx context.Context, honorID int) int
}
