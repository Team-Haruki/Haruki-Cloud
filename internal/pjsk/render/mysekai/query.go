package mysekai

type ResourceQuery struct {
	Region string `json:"region,omitempty"`
}

type FixtureListQuery struct {
	Region string `json:"region,omitempty"`
	ShowID *bool  `json:"show_id,omitempty"`
}

type FixtureDetailQuery struct {
	Region string `json:"region,omitempty"`
	Query  string `json:"query"`
}

type DoorUpgradeQuery struct {
	Region string `json:"region,omitempty"`
	Query  string `json:"query,omitempty"`
}

type MusicRecordQuery struct {
	Region string `json:"region,omitempty"`
	ShowID *bool  `json:"show_id,omitempty"`
}

type TalkListQuery struct {
	Region string `json:"region,omitempty"`
	Query  string `json:"query"`
}
