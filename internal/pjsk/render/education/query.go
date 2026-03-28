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
	Region         renderregion.Value                  `json:"region"`
	Profile        *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot       *userdata.Service                   `json:"-"`
	Unit           string                              `json:"unit,omitempty"`
	Cid            int                                 `json:"cid,omitempty"`
	CharacterQuery string                              `json:"character_query,omitempty"`
	Attr           string                              `json:"attr,omitempty"`
	Tree           bool                                `json:"tree,omitempty"`
	Flower         bool                                `json:"flower,omitempty"`
}
