package education

import (
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/internal/pjsk/drawing"
)

type ChallengeLiveQuery struct {
	Region   renderregion.Value                  `json:"region,omitempty"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot userdata.Snapshot                   `json:"-"` // Optional: overrides controller snapshot
}

type PowerBonusQuery struct {
	Region   renderregion.Value                  `json:"region,omitempty"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot userdata.Snapshot                   `json:"-"`
}

type BondsQuery struct {
	Region         renderregion.Value                  `json:"region,omitempty"`
	Profile        *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot       userdata.Snapshot                   `json:"-"`
	Cid            int                                 `json:"cid,omitempty"`
	CharacterQuery string                              `json:"character_query,omitempty"`
}

type LeaderCountQuery struct {
	Region   renderregion.Value                  `json:"region,omitempty"`
	Profile  *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot userdata.Snapshot                   `json:"-"`
}

type AreaItemQuery struct {
	Region         renderregion.Value                  `json:"region,omitempty"`
	Profile        *drawing.DetailedProfileCardRequest `json:"-"`
	Snapshot       userdata.Snapshot                   `json:"-"`
	Unit           string                              `json:"unit,omitempty"`
	Cid            int                                 `json:"cid,omitempty"`
	CharacterQuery string                              `json:"character_query,omitempty"`
	Attr           string                              `json:"attr,omitempty"`
	Tree           bool                                `json:"tree,omitempty"`
	Flower         bool                                `json:"flower,omitempty"`
}
