package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// PlayerFrameProvider exposes player-frame masterdata queries
// used by the profile module.
type PlayerFrameProvider interface {
	GetPartsByGroupID(ctx context.Context, groupID int) (map[int]int, error)
	GetByID(ctx context.Context, id int) (*masterdata.PlayerFrame, error)
	GetGroupByID(ctx context.Context, id int) (*masterdata.PlayerFrameGroup, error)
}
