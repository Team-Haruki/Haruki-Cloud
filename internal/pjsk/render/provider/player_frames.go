package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// PlayerFrameProvider exposes player-frame masterdata queries
// used by the profile module.
type PlayerFrameProvider interface {
	GetByID(id int) (*masterdata.PlayerFrame, error)
	GetGroupByID(id int) (*masterdata.PlayerFrameGroup, error)
}
