package drawing

// =========================== Gacha Models ===========================

type GachaFilter struct {
	Page int `json:"page"`
}

type GachaBehavior struct {
	Type         string  `json:"type"`
	SpinCount    int     `json:"spin_count"`
	CostType     *string `json:"cost_type,omitempty"`
	CostIconPath *string `json:"cost_icon_path,omitempty"`
	CostQuantity *int    `json:"cost_quantity,omitempty"`
	ExecuteLimit *int    `json:"execute_limit,omitempty"`
	ColorfulPass bool    `json:"colorful_pass"`
}

type GachaInfo struct {
	ID                  int             `json:"id"`
	Name                string          `json:"name"`
	GachaType           string          `json:"gacha_type"`
	Summary             string          `json:"summary"`
	Desc                string          `json:"desc"`
	StartAt             int64           `json:"start_at"`
	EndAt               int64           `json:"end_at"`
	AssetName           string          `json:"asset_name"`
	CeilItemImgPath     *string         `json:"ceil_item_img_path,omitempty"`
	Behaviors           []GachaBehavior `json:"behaviors"`
	Rarity1Count        int             `json:"rarity_1_count"`
	Rarity2Count        int             `json:"rarity_2_count"`
	Rarity3Count        int             `json:"rarity_3_count"`
	Rarity4Count        int             `json:"rarity_4_count"`
	RarityBirthdayCount int             `json:"rarity_birthday_count"`
	PickupCount         int             `json:"pickup_count"`
}

type GachaBrief struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	GachaType string `json:"gacha_type"`
	StartAt   any    `json:"start_at"`
	EndAt     any    `json:"end_at"`
	AssetName string `json:"asset_name"`
}

type GachaCardWeight struct {
	ID               int                      `json:"id"`
	Rarity           string                   `json:"rarity"`
	Rate             float64                  `json:"rate"`
	ThumbnailRequest CardFullThumbnailRequest `json:"thumbnail_request"`
}

type GachaWeight struct {
	Rarity1Rate        *float64           `json:"rarity_1_rate,omitempty"`
	Rarity2Rate        *float64           `json:"rarity_2_rate,omitempty"`
	Rarity3Rate        *float64           `json:"rarity_3_rate,omitempty"`
	Rarity4Rate        *float64           `json:"rarity_4_rate,omitempty"`
	RarityBirthdayRate *float64           `json:"rarity_birthday_rate,omitempty"`
	GuaranteedRates    map[string]float64 `json:"guaranteed_rates"`
}

type GachaListRequest struct {
	Gachas       []GachaBrief   `json:"gachas"`
	PageSize     int            `json:"page_size"`
	Region       string         `json:"region"`
	GachaLogos   map[int]string `json:"gacha_logos"`
	GachaBanners map[int]string `json:"gacha_banners,omitempty"`
	Filter       GachaFilter    `json:"filter"`
	CurrentPage  int            `json:"current_page,omitempty"`
	TotalPage    int            `json:"total_page,omitempty"`
	PrePaginated bool           `json:"pre_paginated,omitempty"`
}

type GachaDetailRequest struct {
	Gacha         GachaInfo         `json:"gacha"`
	WeightInfo    GachaWeight       `json:"weight_info"`
	PickupCards   []GachaCardWeight `json:"pickup_cards"`
	LogoImgPath   *string           `json:"logo_img_path,omitempty"`
	BannerImgPath *string           `json:"banner_img_path,omitempty"`
	BgImgPath     *string           `json:"bg_img_path,omitempty"`
	Region        string            `json:"region"`
}
