package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// CostumeProvider exposes 3D costume masterdata queries.
type CostumeProvider interface {
	GetByID(ctx context.Context, id int) (*masterdata.Costume3d, error)
	Filter(ctx context.Context, filter *CostumeFilter) ([]*masterdata.Costume3d, error)
	GetVariants(ctx context.Context, groupID int, partType string, characterID int) ([]*masterdata.Costume3d, error)
	GetSourceCardIDs(ctx context.Context, costumeIDs []int) (map[int][]int, error)
}

// CostumeFilter describes filtering criteria for costume queries.
// Individual fields are optional; zero-value fields are ignored.
type CostumeFilter struct {
	PartType     string
	CostumeType  string
	CharacterID  int
	CharacterIDs []int
	ColorID      int
	Keyword      string
	Limit        int
	Offset       int
}
