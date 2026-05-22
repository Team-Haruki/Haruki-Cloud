package drawing

// =========================== Event Models ===========================

type EventInfo struct {
	ID            any              `json:"id"` // str | int
	EventType     string           `json:"event_type"`
	EventTypeName string           `json:"event_type_name"`
	StartAt       any              `json:"start_at"` // datetime (int64 ts)
	EndAt         any              `json:"end_at"`
	IsWlEvent     bool             `json:"is_wl_event"`
	BannerCid     int              `json:"banner_cid"`
	BannerIndex   int              `json:"banner_index"`
	BonusAttr     string           `json:"bonus_attr"`
	BonusCharaID  []int            `json:"bonus_chara_id,omitempty"`
	WlTimeList    []map[string]any `json:"wl_time_list,omitempty"`
}

type EventHistory struct {
	ID              any     `json:"id"` // str | int
	EventName       string  `json:"event_name"`
	StartAt         any     `json:"start_at"`
	EndAt           any     `json:"end_at"`
	Rank            *int    `json:"rank,omitempty"`
	RankDisplay     *string `json:"rank_display,omitempty"`
	RankTier        *int    `json:"rank_tier,omitempty"`
	EventPoint      *int    `json:"event_point,omitempty"`
	IsWlEvent       bool    `json:"is_wl_event"`
	BannerPath      string  `json:"banner_path"`
	WlCharaIconPath *string `json:"wl_chara_icon_path,omitempty"`
}

type EventAssets struct {
	EventBgPath        string   `json:"event_bg_path"`
	EventLogoPath      string   `json:"event_logo_path"`
	EventStoryBgPath   string   `json:"event_story_bg_path"`
	EventAttrImagePath string   `json:"event_attr_image_path"`
	EventBanCharaImg   string   `json:"event_ban_chara_img"`
	BanCharaIconPath   string   `json:"ban_chara_icon_path"`
	BonusCharaPath     []string `json:"bonus_chara_path,omitempty"`
}

type EventBrief struct {
	ID              int                        `json:"id"`
	EventName       string                     `json:"event_name"`
	EventType       string                     `json:"event_type"`
	EventTypeName   string                     `json:"event_type_name"`
	StartAt         any                        `json:"start_at"`
	EndAt           any                        `json:"end_at"`
	EventBannerPath string                     `json:"event_banner_path"`
	EventCards      []CardFullThumbnailRequest `json:"event_cards,omitempty"`
	EventAttrPath   *string                    `json:"event_attr_path,omitempty"`
	EventCharaPath  *string                    `json:"event_chara_path,omitempty"`
	EventUnitPath   *string                    `json:"event_unit_path,omitempty"`
}

type EventDetailRequest struct {
	Region      string                     `json:"region"`
	EventInfo   EventInfo                  `json:"event_info"`
	EventAssets EventAssets                `json:"event_assets"`
	EventCards  []CardFullThumbnailRequest `json:"event_cards"`
}

type EventRecordRequest struct {
	EventInfo   []EventHistory             `json:"event_info"`
	WlEventInfo []EventHistory             `json:"wl_event_info"`
	UserInfo    DetailedProfileCardRequest `json:"user_info"`
	RankNote    *string                    `json:"rank_note,omitempty"`
}

type EventListRequest struct {
	EventInfo []EventBrief `json:"event_info"`
}

type EventPlannerDeckCard struct {
	CardThumbnail  CardFullThumbnailRequest `json:"card_thumbnail"`
	SkillLevel     string                   `json:"skill_level,omitempty"`
	SkillRate      float64                  `json:"skill_rate,omitempty"`
	EventBonusRate float64                  `json:"event_bonus_rate,omitempty"`
}

type EventPlannerBoostRow struct {
	Boost        int   `json:"boost"`
	PointPerPlay int64 `json:"point_per_play"`
	Plays        int64 `json:"plays"`
	Energy       int64 `json:"energy"`
}

type EventPlannerSong struct {
	MusicID        int                    `json:"music_id,omitempty"`
	Query          string                 `json:"query,omitempty"`
	Title          string                 `json:"title"`
	MusicCoverPath string                 `json:"music_cover_path,omitempty"`
	Difficulty     string                 `json:"difficulty,omitempty"`
	Rows           []EventPlannerBoostRow `json:"rows"`
}

type EventPlannerRequest struct {
	Title           string                      `json:"title"`
	Region          string                      `json:"region"`
	EventID         int                         `json:"event_id,omitempty"`
	EventName       string                      `json:"event_name,omitempty"`
	EventBannerPath string                      `json:"event_banner_path,omitempty"`
	LiveName        string                      `json:"live_name,omitempty"`
	Profile         *DetailedProfileCardRequest `json:"profile,omitempty"`
	TargetPoint     int64                       `json:"target_point"`
	CurrentPoint    int64                       `json:"current_point,omitempty"`
	RemainingPoint  int64                       `json:"remaining_point"`
	DailyPoint      int64                       `json:"daily_point,omitempty"`
	TargetSource    string                      `json:"target_source,omitempty"`
	DeckSummary     string                      `json:"deck_summary,omitempty"`
	DeckCards       []EventPlannerDeckCard      `json:"deck_cards,omitempty"`
	DeckTotalPower  int                         `json:"deck_total_power,omitempty"`
	DeckEventBonus  float64                     `json:"deck_event_bonus,omitempty"`
	DeckSkillUp     float64                     `json:"deck_skill_up,omitempty"`
	Songs           []EventPlannerSong          `json:"songs"`
	Warnings        []string                    `json:"warnings,omitempty"`
	DeckRequest     *DeckRequest                `json:"deck_request,omitempty"`
}
