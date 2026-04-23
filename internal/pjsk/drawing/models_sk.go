package drawing

// =========================== SK Models ===========================

type RankInfo struct {
	Rank            int    `json:"rank"`
	Name            string `json:"name"`
	Score           *int   `json:"score,omitempty"`
	Time            int64  `json:"time"`
	AverageRound    *int   `json:"average_round,omitempty"`
	AveragePt       *int   `json:"average_pt,omitempty"`
	LatestPt        *int   `json:"latest_pt,omitempty"`
	Speed           *int   `json:"speed,omitempty"`
	Min20Time3Speed *int   `json:"min20_times_3_speed,omitempty"`
	HourRound       *int   `json:"hour_round,omitempty"`
	RecordStartAt   *int64 `json:"record_start_at,omitempty"`
}

type SpeedInfo struct {
	Rank       int   `json:"rank"`
	Score      int   `json:"score"`
	Speed      *int  `json:"speed,omitempty"`
	RecordTime int64 `json:"record_time"`
}

type TeamInfo struct {
	TeamID       int     `json:"team_id"`
	TeamName     string  `json:"team_name"`
	WinRate      float64 `json:"win_rate"`
	IsRecruiting bool    `json:"is_recruiting"`
	TeamCnName   *string `json:"team_cn_name,omitempty"`
	TeamIconPath *string `json:"team_icon_path,omitempty"`
}

type SKForecastColumn struct {
	Key          string     `json:"key"`
	Name         string     `json:"name"`
	Ranks        []RankInfo `json:"ranks"`
	ForecastTime *int64     `json:"forecast_time,omitempty"`
	UpdateTime   *int64     `json:"update_time,omitempty"`
}

type SklRequest struct {
	ID              int                `json:"id"`
	Region          string             `json:"region"`
	StartAt         int64              `json:"start_at"`
	AggregateAt     int64              `json:"aggregate_at"`
	Name            string             `json:"name"`
	BannerImgPath   string             `json:"banner_img_path"`
	WlCid           *int               `json:"wl_cid,omitempty"`
	CharaIconPath   *string            `json:"chara_icon_path,omitempty"`
	Ranks           []RankInfo         `json:"ranks"`
	CurrentRanks    []RankInfo         `json:"current_ranks,omitempty"`
	ForecastColumns []SKForecastColumn `json:"forecast_columns,omitempty"`
}

type SKRequest struct {
	ID              int        `json:"id"`
	Region          string     `json:"region"`
	Name            string     `json:"name"`
	AggregateAt     int64      `json:"aggregate_at"`
	Ranks           []RankInfo `json:"ranks"`
	WlCharaIconPath *string    `json:"wl_chara_icon_path,omitempty"`
	CharaIconPath   *string    `json:"chara_icon_path,omitempty"`
	PrevRanks       *RankInfo  `json:"prev_ranks,omitempty"`
	NextRanks       *RankInfo  `json:"next_ranks,omitempty"`
}

type CFRequest struct {
	Eid             int        `json:"eid"`
	EventName       string     `json:"event_name"`
	Region          string     `json:"region"`
	Ranks           []RankInfo `json:"ranks"`
	PrevRank        *RankInfo  `json:"prev_rank,omitempty"`
	NextRank        *RankInfo  `json:"next_rank,omitempty"`
	AggregateAt     int64      `json:"aggregate_at"`
	UpdateAt        int64      `json:"update_at"`
	WlCharaIconPath *string    `json:"wl_chara_icon_path,omitempty"`
}

type CSBRequest struct {
	Eid             int        `json:"eid"`
	EventName       string     `json:"event_name"`
	Region          string     `json:"region"`
	Ranks           []RankInfo `json:"ranks"`
	AggregateAt     int64      `json:"aggregate_at"`
	UpdateAt        int64      `json:"update_at"`
	WlCharaIconPath *string    `json:"wl_chara_icon_path,omitempty"`
}

type SpeedRequest struct {
	EventID          int         `json:"event_id"`
	Region           string      `json:"region"`
	EventName        string      `json:"event_name"`
	EventStartAt     int64       `json:"event_start_at"`
	EventAggregateAt int64       `json:"event_aggregate_at"`
	Ranks            []SpeedInfo `json:"ranks"`
	IsWlEvent        bool        `json:"is_wl_event"`
	RequestType      string      `json:"request_type"`
	Period           int64       `json:"period"` // timedelta -> int64 (seconds)
	BannerImgPath    *string     `json:"banner_img_path,omitempty"`
	WlCharaIconPath  *string     `json:"wl_chara_icon_path,omitempty"`
}

type PlayerTraceRequest struct {
	EventID         int        `json:"event_id"`
	Region          string     `json:"region"`
	WlCharaIconPath *string    `json:"wl_chara_icon_path,omitempty"`
	Ranks           []RankInfo `json:"ranks"`
	Ranks2          []RankInfo `json:"ranks2,omitempty"`
}

type RankTraceRequest struct {
	EventID         int        `json:"event_id"`
	Region          string     `json:"region"`
	WlCharaIconPath *string    `json:"wl_chara_icon_path,omitempty"`
	TargetRank      int        `json:"target_rank"`
	Ranks           []RankInfo `json:"ranks"`
	PredictRanks    *RankInfo  `json:"predict_ranks,omitempty"`
}

type WinRateRequest struct {
	WlCharaIconPath  *string    `json:"wl_chara_icon_path,omitempty"`
	UpdatedAt        int64      `json:"updated_at"`
	EventStartAt     int64      `json:"event_start_at"`
	EventAggregateAt int64      `json:"event_aggregate_at"`
	BannerImgPath    *string    `json:"banner_img_path,omitempty"`
	TeamInfo         []TeamInfo `json:"team_info"`
}
