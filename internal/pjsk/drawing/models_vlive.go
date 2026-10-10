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
	// Set only on an entry standing for a whole solo virtual live group.
	VirtualLiveType string `json:"virtual_live_type,omitempty"`
	GroupID         *int   `json:"group_id,omitempty"`
	GroupName       string `json:"group_name,omitempty"`
	GroupCount      *int   `json:"group_count,omitempty"`
}

type VLiveListRequest struct {
	Region   string       `json:"region"`
	TimeZone string       `json:"timezone,omitempty"`
	DT       int64        `json:"dt,omitempty"`
	Lives    []VLiveBrief `json:"lives"`
}

// VLiveDetailRequest is the solo virtual live group detail view.
type VLiveDetailRequest struct {
	Region                 string                     `json:"region"`
	ID                     int                        `json:"id"`
	Title                  string                     `json:"title"`
	VirtualLiveType        string                     `json:"virtual_live_type,omitempty"`
	BannerPath             string                     `json:"banner_path,omitempty"`
	StartAt                any                        `json:"start_at"`
	EndAt                  any                        `json:"end_at"`
	Lives                  []VLiveDetailLive          `json:"lives"`
	TotalCheerPoint        *int                       `json:"total_cheer_point,omitempty"`
	TotalCheerPointRewards []VLiveCheerPointRewardRow `json:"total_cheer_point_rewards,omitempty"`
	SurplusReward          *VLiveSurplusReward        `json:"surplus_reward,omitempty"`
	OverrideCost           *VLiveOverrideCost         `json:"override_cost,omitempty"`
}

type VLiveDetailLive struct {
	ID                int    `json:"id"`
	Name              string `json:"name,omitempty"`
	ShortName         string `json:"short_name,omitempty"` // the part of Name that tells the lives of a group apart
	CharacterIconPath string `json:"character_icon_path,omitempty"`
	CurrentStartAt    any    `json:"current_start_at,omitempty"`
	CurrentEndAt      any    `json:"current_end_at,omitempty"`
	Living            bool   `json:"living"`
	RestCount         int    `json:"rest_count"`
	ScheduleCount     *int   `json:"schedule_count,omitempty"`
}

type VLiveCheerPointRewardRow struct {
	Threshold int               `json:"threshold"`
	Rewards   []VLiveRewardItem `json:"rewards"`
	Received  bool              `json:"received,omitempty"`
}

type VLiveSurplusReward struct {
	BasePoint     int               `json:"base_point"`
	Rewards       []VLiveRewardItem `json:"rewards"`
	ReceivedCount *int              `json:"received_count,omitempty"`
}

type VLiveOverrideCost struct {
	ImagePath    string `json:"image_path"`
	Name         string `json:"name,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   *int   `json:"resource_id,omitempty"`
	HaveQuantity *int   `json:"have_quantity,omitempty"`
}
