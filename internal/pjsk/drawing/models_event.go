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
	EventPoint      int     `json:"event_point"`
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
}

type EventListRequest struct {
	EventInfo []EventBrief `json:"event_info"`
}
