package vlive

import "time"

type ListQuery struct {
	Region string    `json:"region,omitempty"`
	Now    time.Time `json:"-"`
}

type Schedule struct {
	StartAt int64 `json:"start_at"`
	EndAt   int64 `json:"end_at"`
}

type Live struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	StartAt   int64      `json:"start_at"`
	EndAt     int64      `json:"end_at"`
	Schedules []Schedule `json:"schedules,omitempty"`
}

type Window struct {
	StartAt time.Time
	EndAt   time.Time
}

type ResolvedLive struct {
	ID        int
	Name      string
	StartAt   time.Time
	EndAt     time.Time
	Current   *Window
	Living    bool
	RestCount int
}
