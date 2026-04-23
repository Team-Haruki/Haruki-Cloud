package drawing

type VLiveRewardItem struct {
	ImagePath string `json:"image_path"`
	Quantity  int    `json:"quantity"`
}

type VLiveCharacterItem struct {
	IconPath string `json:"icon_path"`
}

type VLiveBrief struct {
	ID             int                  `json:"id"`
	Name           string               `json:"name"`
	StartAt        any                  `json:"start_at"`
	EndAt          any                  `json:"end_at"`
	CurrentStartAt any                  `json:"current_start_at,omitempty"`
	CurrentEndAt   any                  `json:"current_end_at,omitempty"`
	Living         bool                 `json:"living"`
	RestCount      int                  `json:"rest_count"`
	BannerPath     string               `json:"banner_path,omitempty"`
	Rewards        []VLiveRewardItem    `json:"rewards,omitempty"`
	Characters     []VLiveCharacterItem `json:"characters,omitempty"`
}

type VLiveListRequest struct {
	Region   string       `json:"region"`
	TimeZone string       `json:"timezone,omitempty"`
	DT       int64        `json:"dt,omitempty"`
	Lives    []VLiveBrief `json:"lives"`
}
