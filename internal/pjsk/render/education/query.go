package education

import (
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type ChallengeLiveQuery struct {
	Region   renderregion.Value                  `json:"region"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot *userdata.Service                   `json:"-"` // Optional: overrides controller snapshot
}

type PowerBonusQuery struct {
	Region   renderregion.Value                  `json:"region"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot *userdata.Service                   `json:"-"`
}

type AreaItemQuery struct {
	Region   renderregion.Value                  `json:"region"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot *userdata.Service                   `json:"-"`
}
