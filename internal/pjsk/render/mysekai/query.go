package mysekai

import (
	"time"

	"haruki-cloud/utils/drawing"
)

type ResourceQuery struct {
	Region  string                      `json:"region,omitempty"`
	Profile *drawing.ProfileCardRequest `json:"-"`
}

type FixtureListQuery struct {
	Region        string                      `json:"region,omitempty"`
	ShowID        *bool                       `json:"show_id,omitempty"`
	OnlyCraftable *bool                       `json:"only_craftable,omitempty"`
	Profile       *drawing.ProfileCardRequest `json:"-"`
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
	Region       string                      `json:"region,omitempty"`
	Query        string                      `json:"query"`
	ShowAllTalks *bool                       `json:"show_all_talks,omitempty"`
	Profile      *drawing.ProfileCardRequest `json:"-"`
}

type PhotoQuery struct {
	Region string `json:"region,omitempty"`
	Seq    int    `json:"seq"`
}

type PhotoResult struct {
	Region     string    `json:"region"`
	Seq        int       `json:"seq"`
	Total      int       `json:"total"`
	ImagePath  string    `json:"image_path"`
	ObtainedAt time.Time `json:"obtained_at"`
}
