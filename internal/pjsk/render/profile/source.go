package profile

import (
	"haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type Source interface {
	honor.DataSource
	GetPlayerFrameByID(id int) (*masterdata.PlayerFrame, error)
	GetPlayerFrameGroupByID(id int) (*masterdata.PlayerFrameGroup, error)
	GetCardByID(id int) (*masterdata.Card, error)
	GetEventIDByHonorID(honorID int) int
}
