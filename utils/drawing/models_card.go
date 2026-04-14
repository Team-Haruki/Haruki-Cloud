package drawing

// =========================== Card Models ===========================

type CardFullThumbnailRequest struct {
	CardID            int            `json:"card_id"`
	CardThumbnailPath string         `json:"card_thumbnail_path"`
	Rare              string         `json:"rare"`
	FrameImgPath      string         `json:"frame_img_path"`
	AttrImgPath       string         `json:"attr_img_path"`
	RareImgPath       string         `json:"rare_img_path"`
	TrainRank         *int           `json:"train_rank"`
	TrainRankImgPath  *string        `json:"train_rank_img_path,omitempty"`
	Level             *int           `json:"level,omitempty"`
	BirthdayIconPath  *string        `json:"birthday_icon_path,omitempty"`
	IsAfterTraining   *bool          `json:"is_after_training,omitempty"`
	CustomText        *string        `json:"custom_text,omitempty"`
	CardLevel         map[string]any `json:"card_level,omitempty"`
	IsPcard           bool           `json:"is_pcard"`
}

type CardPower struct {
	PowerTotal int `json:"power_total"`
	Power1     int `json:"power1"`
	Power2     int `json:"power2"`
	Power3     int `json:"power3"`
}

type CardSkill struct {
	SkillID           int     `json:"skill_id"`
	SkillName         string  `json:"skill_name"`
	SkillType         string  `json:"skill_type"`
	SkillDetail       string  `json:"skill_detail"`
	SkillTypeIconPath *string `json:"skill_type_icon_path,omitempty"`
	SkillDetailCn     *string `json:"skill_detail_cn,omitempty"`
}

type CardEventInfo struct {
	EventID         int     `json:"event_id"`
	EventName       string  `json:"event_name"`
	StartAt         any     `json:"start_at"` // datetime | int | str
	EndAt           any     `json:"end_at"`
	EventBannerPath string  `json:"event_banner_path"`
	BonusAttr       *string `json:"bonus_attr,omitempty"`
	Unit            *string `json:"unit,omitempty"`
	BannerCid       *int    `json:"banner_cid,omitempty"`
}

type CardGachaInfo struct {
	GachaID         int    `json:"gacha_id"`
	GachaName       string `json:"gacha_name"`
	StartAt         any    `json:"start_at"`
	EndAt           any    `json:"end_at"`
	GachaBannerPath string `json:"gacha_banner_path"`
}

type CardBasic struct {
	CardID           int                        `json:"card_id"`
	CharacterID      *int                       `json:"character_id"`
	CharacterName    *string                    `json:"character_name,omitempty"`
	Unit             *string                    `json:"unit,omitempty"`
	ReleaseAt        *int64                     `json:"release_at,omitempty"`
	SupplyType       *string                    `json:"supply_type,omitempty"`
	Rare             *string                    `json:"rare,omitempty"`
	Attr             *string                    `json:"attr,omitempty"`
	Prefix           *string                    `json:"prefix,omitempty"`
	AssetBundleName  *string                    `json:"asset_bundle_name,omitempty"`
	Skill            *CardSkill                 `json:"skill,omitempty"`
	SpecialSkillInfo *CardSkill                 `json:"special_skill_info,omitempty"`
	ThumbnailInfo    []CardFullThumbnailRequest `json:"thumbnail_info,omitempty"`
	IsAfterTraining  *bool                      `json:"is_after_training,omitempty"`
	Power            *CardPower                 `json:"power,omitempty"`
}

type UserCard struct {
	Card    CardBasic `json:"card"`
	HasCard bool      `json:"has_card"`
}

// CardDetailRequest represents request for /card/detail
type CardDetailRequest struct {
	CardInfo            CardBasic      `json:"card_info"`
	Region              string         `json:"region"`
	EventInfo           *CardEventInfo `json:"event_info,omitempty"`
	GachaInfo           *CardGachaInfo `json:"gacha_info,omitempty"`
	CardImagesPath      []string       `json:"card_images_path"`
	CostumeImagesPath   []string       `json:"costume_images_path"`
	CharacterIconPath   string         `json:"character_icon_path"`
	UnitLogoPath        string         `json:"unit_logo_path"`
	BackgroundImagePath *string        `json:"background_image_path,omitempty"`
	EventAttrIconPath   *string        `json:"event_attr_icon_path,omitempty"`
	EventUnitIconPath   *string        `json:"event_unit_icon_path,omitempty"`
	EventCharaIconPath  *string        `json:"event_chara_icon_path,omitempty"`
}

// CardListRequest represents request for /card/list
type CardListRequest struct {
	Cards               []CardBasic                 `json:"cards"`
	Region              string                      `json:"region"`
	UserInfo            *DetailedProfileCardRequest `json:"user_info,omitempty"`
	BackgroundImgPath   *string                     `json:"background_img_path,omitempty"`
	TermLimitedIconPath *string                     `json:"term_limited_icon_path,omitempty"`
	FesLimitedIconPath  *string                     `json:"fes_limited_icon_path,omitempty"`
}

// CardBoxRequest represents request for /card/box
type CardBoxRequest struct {
	Cards               []UserCard                  `json:"cards"`
	Region              string                      `json:"region"`
	UserInfo            *DetailedProfileCardRequest `json:"user_info,omitempty"`
	ShowID              bool                        `json:"show_id"`
	ShowBox             bool                        `json:"show_box"`
	BackgroundImgPath   *string                     `json:"background_img_path,omitempty"`
	CharacterIconPaths  map[int]string              `json:"character_icon_paths"`
	CharacterColorCodes map[int]string              `json:"character_color_codes,omitempty"`
	TermLimitedIconPath *string                     `json:"term_limited_icon_path,omitempty"`
	FesLimitedIconPath  *string                     `json:"fes_limited_icon_path,omitempty"`
}

// =========================== Deck Models ===========================

type DeckCardData struct {
	CardThumbnail   CardFullThumbnailRequest `json:"card_thumbnail"`
	CharaID         int                      `json:"chara_id"`
	SkillLevel      string                   `json:"skill_level"`
	IsAfterTraining bool                     `json:"is_after_training"`
	SkillRate       float64                  `json:"skill_rate"`
	EventBonusRate  float64                  `json:"event_bonus_rate"`
	IsBeforeStory   bool                     `json:"is_before_story"`
	IsAfterStory    bool                     `json:"is_after_story"`
	HasCanvasBonus  bool                     `json:"has_canvas_bonus"`
}

type DeckData struct {
	CardData             []DeckCardData `json:"card_data"`
	MusicTitle           *string        `json:"music_title,omitempty"`
	MusicID              *int           `json:"music_id,omitempty"`
	MusicDiff            *string        `json:"music_diff,omitempty"`
	MusicCoverPath       *string        `json:"music_cover_path,omitempty"`
	MusicQuery           *string        `json:"music_query,omitempty"`
	Pt                   *int           `json:"pt,omitempty"`
	EventBonusRate       *float64       `json:"event_bonus_rate,omitempty"`
	ScoreUp              *float64       `json:"score_up,omitempty"`
	TotalPower           *int           `json:"total_power,omitempty"`
	ChallengeScoreDelta  *int           `json:"challenge_score_delta,omitempty"`
	Score                *int           `json:"score,omitempty"`
	LiveScore            *int           `json:"live_score,omitempty"`
	MySekaiEventPoint    *int           `json:"mysekai_event_point,omitempty"`
	SupportDeckBonusRate *float64       `json:"support_deck_bonus_rate,omitempty"`
	MultiLiveScoreUp     *float64       `json:"multi_live_score_up,omitempty"`
}

type DeckRequest struct {
	Region                       string                     `json:"region"`
	Profile                      DetailedProfileCardRequest `json:"profile"`
	DeckData                     []DeckData                 `json:"deck_data"`
	MusicCompare                 bool                       `json:"music_compare,omitempty"`
	EventName                    *string                    `json:"event_name,omitempty"`
	MusicTitle                   *string                    `json:"music_title,omitempty"`
	MusicID                      *int                       `json:"music_id,omitempty"`
	MusicDiff                    *string                    `json:"music_diff,omitempty"`
	EventBannerPath              *string                    `json:"event_banner_path,omitempty"`
	MusicCoverPath               *string                    `json:"music_cover_path,omitempty"`
	IsMaxDeck                    bool                       `json:"is_max_deck"`
	RecommendType                string                     `json:"recommend_type"`
	WlCharaName                  *string                    `json:"wl_chara_name,omitempty"`
	WlCharaIconPath              *string                    `json:"wl_chara_icon_path,omitempty"`
	EventID                      *int                       `json:"event_id,omitempty"`
	LiveType                     *string                    `json:"live_type,omitempty"`
	LiveName                     *string                    `json:"live_name,omitempty"`
	CharaIconPath                *string                    `json:"chara_icon_path,omitempty"`
	CharaName                    *string                    `json:"chara_name,omitempty"`
	UnitLogoPath                 *string                    `json:"unit_logo_path,omitempty"`
	AttrIconPath                 *string                    `json:"attr_icon_path,omitempty"`
	IsWl                         bool                       `json:"is_wl"`
	MultiLiveTeammatePower       *int                       `json:"multi_live_teammate_power,omitempty"`
	MultiLiveTeammateScoreUp     *float64                   `json:"multi_live_teammate_score_up,omitempty"`
	Target                       *string                    `json:"target,omitempty"`
	UnitFilter                   *string                    `json:"unit_filter,omitempty"`
	AttrFilter                   *string                    `json:"attr_filter,omitempty"`
	ExcludedCards                []int                      `json:"excluded_cards,omitempty"`
	MultiLiveScoreUpLowerBound   *float64                   `json:"multi_live_score_up_lower_bound,omitempty"`
	SkillOrderChooseStrategy     *string                    `json:"skill_order_choose_strategy,omitempty"`
	SkillReferenceChooseStrategy *string                    `json:"skill_reference_choose_strategy,omitempty"`
	KeepAfterTrainingState       bool                       `json:"keep_after_training_state"`
	ModelName                    []interface{}              `json:"model_name,omitempty"`
	CanvasThumbnailPath          *string                    `json:"canvas_thumbnail_path,omitempty"`
	FixedCardsID                 []int                      `json:"fixed_cards_id,omitempty"`
	FixedCharactersID            []int                      `json:"fixed_characters_id,omitempty"`
	CostTimes                    map[string]any             `json:"cost_times,omitempty"`
	WaitTimes                    map[string]any             `json:"wait_times,omitempty"`
}
