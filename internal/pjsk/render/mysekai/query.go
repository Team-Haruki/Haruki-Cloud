package mysekai

import "haruki-cloud/utils/drawing"

type ResourceQuery struct {
	Region  string                      `json:"region,omitempty"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}

type FixtureListQuery struct {
	Region  string                      `json:"region,omitempty"`
	ShowID  *bool                       `json:"show_id,omitempty"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}

type FixtureDetailQuery struct {
	Region string `json:"region,omitempty"`
	Query  string `json:"query"`
}

type DoorUpgradeQuery struct {
	Region  string                      `json:"region,omitempty"`
	Query   string                      `json:"query,omitempty"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}

type MusicRecordQuery struct {
	Region  string                      `json:"region,omitempty"`
	ShowID  *bool                       `json:"show_id,omitempty"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}

type TalkListQuery struct {
	Region  string                      `json:"region,omitempty"`
	Query   string                      `json:"query"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}
