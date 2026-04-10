package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// PlayerFrameProvider exposes player-frame masterdata queries
// used by the profile module.
type PlayerFrameProvider interface {
	GetByID(ctx context.Context, id int) (*masterdata.PlayerFrame, error)
	GetGroupByID(ctx context.Context, id int) (*masterdata.PlayerFrameGroup, error)
}
