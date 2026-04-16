package drawing

// =========================== Misc Models ===========================

type PlayerFramePaths struct {
	Base        string `json:"base"`
	CenterTop   string `json:"centertop"`
	LeftBottom  string `json:"leftbottom"`
	LeftTop     string `json:"lefttop"`
	RightBottom string `json:"rightbottom"`
	RightTop    string `json:"righttop"`
}

type CharaBirthdayCard struct {
	ID            int    `json:"id"`
	ThumbnailPath string `json:"thumbnail_path"`
}

type BirthdayEventTime struct {
	StartText string `json:"start_text"`
	EndText   string `json:"end_text"`
}

type CharaBirthdayData struct {
	Cid      int    `json:"cid"`
	Month    int    `json:"month"`
	Day      int    `json:"day"`
	IconPath string `json:"icon_path"`
}

type CharaBirthdayRequest struct {
	Cid               int                 `json:"cid"`
	Month             int                 `json:"month"`
	Day               int                 `json:"day"`
	RegionName        string              `json:"region_name"`
	DaysUntilBirthday int                 `json:"days_until_birthday"`
	ColorCode         string              `json:"color_code"`
	SdImagePath       string              `json:"sd_image_path"`
	TitleImagePath    string              `json:"title_image_path"`
	CardImagePath     string              `json:"card_image_path"`
	Cards             []CharaBirthdayCard `json:"cards"`
	IsFifthAnniv      bool                `json:"is_fifth_anniv"`
	GachaTime         BirthdayEventTime   `json:"gacha_time"`
	LiveTime          BirthdayEventTime   `json:"live_time"`
	DropTime          *BirthdayEventTime  `json:"drop_time,omitempty"`
	FlowerTime        *BirthdayEventTime  `json:"flower_time,omitempty"`
	PartyTime         *BirthdayEventTime  `json:"party_time,omitempty"`
	AllCharacters     []CharaBirthdayData `json:"all_characters"`
}

// =========================== Stamp Models ===========================

type StampData struct {
	ID        int    `json:"id"`
	ImagePath string `json:"image_path"`
	TextColor []int  `json:"text_color"` // Color (r,g,b,a)
}

type StampListRequest struct {
	PromptMessage *string     `json:"prompt_message,omitempty"`
	PageMessage   *string     `json:"page_message,omitempty"`
	Stamps        []StampData `json:"stamps"`
}

// =========================== Helper Functions ===========================

// Helper methods for pointer creation
func StringPtr(s string) *string {
	return &s
}

func IntPtr(i int) *int {
	return &i
}

func Int64Ptr(i int64) *int64 {
	return &i
}
