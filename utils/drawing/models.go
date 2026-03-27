package drawing

// =========================== Music Models ===========================

type MusicMD struct {
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	Composer     string   `json:"composer"`
	Lyricist     string   `json:"lyricist"`
	Arranger     string   `json:"arranger"`
	MvInfo       []string `json:"mv_info,omitempty"`
	Categories   []string `json:"categories"`
	ReleaseAt    int64    `json:"release_at"`
	IsFullLength bool     `json:"is_full_length"`
}

type DifficultyInfo struct {
	Level     []int    `json:"level"`
	NoteCount []int    `json:"note_count"`
	HasAppend bool     `json:"has_append"`
	Order     []string `json:"order,omitempty"`
}

type MusicVocalInfo struct {
	VocalInfo   map[string]interface{} `json:"vocal_info"`   // {"caption": str, "characters": [{"characterName": str}]}
	VocalAssets map[string]string      `json:"vocal_assets"` // {"xxx": path}
}

// LeaderboardInfo represents leaderboard item info
type LeaderboardInfo struct {
	Rank  int    `json:"rank"`
	Diff  string `json:"diff"`
	Value string `json:"value"`
}

// MusicDetailRequest represents request for /music/detail
type MusicDetailRequest struct {
	Region               string               `json:"region"`
	MusicInfo            MusicMD              `json:"music_info"`
	Bpm                  *int                 `json:"bpm,omitempty"`
	Vocal                MusicVocalInfo       `json:"vocal"`
	Alias                []string             `json:"alias,omitempty"`
	Length               *string              `json:"length,omitempty"`
	Difficulty           DifficultyInfo       `json:"difficulty"`
	EventID              *int                 `json:"event_id,omitempty"`
	CnName               *string              `json:"cn_name,omitempty"`
	MusicJacketPath      string               `json:"music_jacket_path"`
	EventBannerPath      *string              `json:"event_banner_path,omitempty"`
	LimitedTimes         [][]string           `json:"limited_times,omitempty"` // List of (start, end) tuples
	LeaderboardMatrix    [][]*LeaderboardInfo `json:"leaderboard_matrix,omitempty"`
	LeaderboardMusicNum  *int                 `json:"leaderboard_music_num,omitempty"`
	LeaderboardLiveTypes map[string]string    `json:"leaderboard_live_types,omitempty"`
	LeaderboardTargets   map[string]string    `json:"leaderboard_targets,omitempty"`
}

type MusicBriefList struct {
	ID              int            `json:"id,omitempty"`
	Level           int            `json:"level,omitempty"`
	PlayResult      string         `json:"play_result,omitempty"`
	Difficulty      DifficultyInfo `json:"difficulty"`
	MusicInfo       MusicMD        `json:"music_info"`
	MusicJacketPath string         `json:"music_jacket_path"`
}

// MusicBriefListRequest represents request for /music/brief-list
type MusicBriefListRequest struct {
	MusicList            []MusicBriefList `json:"music_list"`
	Region               string           `json:"region"`
	RequiredDifficulty   string           `json:"required_difficulty,omitempty"`
	RequiredDifficulties string           `json:"required_difficulties,omitempty"`
	Title                *string          `json:"title,omitempty"`
	TitleStyle           interface{}      `json:"title_style,omitempty"`
	TitleShadow          bool             `json:"title_shadow,omitempty"`
}

// MusicListRequest represents request for /music/list
type MusicListRequest struct {
	UserResults           map[int]interface{}        `json:"user_results"` // key: musicId
	MusicList             []map[string]interface{}   `json:"music_list"`   // [{"id": int, "difficulty": str}]
	JacketsPathList       map[int]string             `json:"jackets_path_list"`
	RequiredDifficulties  string                     `json:"required_difficulties"`
	Profile               DetailedProfileCardRequest `json:"profile"`
	PlayResultIconPathMap map[string]string          `json:"play_result_icon_path_map,omitempty"`
	Title                 *string                    `json:"title,omitempty"`
	TitleStyle            interface{}                `json:"title_style,omitempty"`
	TitleShadow           bool                       `json:"title_shadow,omitempty"`
}

type PlayProgressCount struct {
	Level    int `json:"level"`
	Total    int `json:"total"`
	NotClear int `json:"not_clear"`
	Clear    int `json:"clear"`
	Fc       int `json:"fc"`
	Ap       int `json:"ap"`
}

// PlayProgressRequest represents request for /music/progress
type PlayProgressRequest struct {
	Counts     []PlayProgressCount `json:"counts"`
	Difficulty string              `json:"difficulty"` // "easy", "normal", ...
	Profile    ProfileCardRequest  `json:"profile"`
}

type MusicComboReward struct {
	Level  int `json:"level"`
	Reward int `json:"reward"`
}

// DetailMusicRewardsRequest represents request for /music/rewards/detail
type DetailMusicRewardsRequest struct {
	RankRewards   int                           `json:"rank_rewards"`
	ComboRewards  map[string][]MusicComboReward `json:"combo_rewards"`
	Profile       ProfileCardRequest            `json:"profile"`
	JewelIconPath *string                       `json:"jewel_icon_path,omitempty"`
	ShardIconPath *string                       `json:"shard_icon_path,omitempty"`
}

// BasicMusicRewardsRequest represents request for /music/rewards/basic
type BasicMusicRewardsRequest struct {
	RankRewards   string             `json:"rank_rewards"`
	ComboRewards  map[string]string  `json:"combo_rewards"`
	Profile       ProfileCardRequest `json:"profile"`
	JewelIconPath *string            `json:"jewel_icon_path,omitempty"`
	ShardIconPath *string            `json:"shard_icon_path,omitempty"`
}

// =========================== Profile Models ===========================

type BasicProfile struct {
	ID              string  `json:"id"`
	Region          string  `json:"region"`
	Nickname        string  `json:"nickname"`
	IsHideUID       bool    `json:"is_hide_uid"`
	LeaderImagePath string  `json:"leader_image_path"`
	HasFrame        bool    `json:"has_frame"`
	FramePath       *string `json:"frame_path,omitempty"`
}

type ProfileDataSource struct {
	Name       string  `json:"name"`
	Source     *string `json:"source,omitempty"`
	UpdateTime *int64  `json:"update_time,omitempty"`
	Mode       *string `json:"mode,omitempty"`
}

type ProfileCardRequest struct {
	Profile      *BasicProfile       `json:"profile,omitempty"`
	DataSources  []ProfileDataSource `json:"data_sources"`
	MysekaiLevel *int                `json:"mysekai_level,omitempty"`
	ErrorMessage *string             `json:"error_message,omitempty"`
}

type DetailedProfileCardRequest struct {
	ID              string        `json:"id"`
	Region          string        `json:"region"`
	Nickname        string        `json:"nickname"`
	Source          string        `json:"source"`
	UpdateTime      int64         `json:"update_time"`
	Mode            *string       `json:"mode,omitempty"`
	IsHideUID       bool          `json:"is_hide_uid"`
	LeaderImagePath string        `json:"leader_image_path"`
	HasFrame        bool          `json:"has_frame"`
	FramePath       *string       `json:"frame_path,omitempty"`
	UserCards       []interface{} `json:"user_cards,omitempty"`
}

type CardFullThumbnailRequest struct {
	CardID            int                    `json:"card_id"`
	CardThumbnailPath string                 `json:"card_thumbnail_path"`
	Rare              string                 `json:"rare"`
	FrameImgPath      string                 `json:"frame_img_path"`
	AttrImgPath       string                 `json:"attr_img_path"`
	RareImgPath       string                 `json:"rare_img_path"`
	TrainRank         *int                   `json:"train_rank"`
	TrainRankImgPath  *string                `json:"train_rank_img_path,omitempty"`
	Level             *int                   `json:"level,omitempty"`
	BirthdayIconPath  *string                `json:"birthday_icon_path,omitempty"`
	IsAfterTraining   *bool                  `json:"is_after_training,omitempty"`
	CustomText        *string                `json:"custom_text,omitempty"`
	CardLevel         map[string]interface{} `json:"card_level,omitempty"`
	IsPcard           bool                   `json:"is_pcard"`
}

type ProfileBgSettings struct {
	ImgPath  *string `json:"img_path,omitempty"`
	Blur     int     `json:"blur"`
	Alpha    int     `json:"alpha"`
	Vertical bool    `json:"vertical"`
}

type MusicClearCount struct {
	Difficulty string `json:"difficulty"`
	Clear      int    `json:"clear"`
	Fc         int    `json:"fc"`
	Ap         int    `json:"ap"`
}

type CharacterRank struct {
	CharacterID int `json:"character_id"`
	Rank        int `json:"rank"`
}

type SoloLiveRank struct {
	CharacterID int `json:"character_id"`
	Score       int `json:"score"`
	Rank        int `json:"rank"`
}

type PlayerFramePaths struct {
	Base        string `json:"base"`
	CenterTop   string `json:"centertop"`
	LeftBottom  string `json:"leftbottom"`
	LeftTop     string `json:"lefttop"`
	RightBottom string `json:"rightbottom"`
	RightTop    string `json:"righttop"`
}

// HonorRequest from src/sekai/honor/model.py
type HonorRequest struct {
	HonorType               *string `json:"honor_type,omitempty"`
	GroupType               *string `json:"group_type,omitempty"`
	HonorRarity             *string `json:"honor_rarity,omitempty"`
	HonorLevel              *int    `json:"honor_level,omitempty"`
	FcOrApLevel             *string `json:"fc_or_ap_level,omitempty"`
	IsEmpty                 bool    `json:"is_empty"`
	IsMainHonor             bool    `json:"is_main_honor"`
	HonorImgPath            *string `json:"honor_img_path,omitempty"`
	RankImgPath             *string `json:"rank_img_path,omitempty"`
	LvImgPath               *string `json:"lv_img_path,omitempty"`
	Lv6ImgPath              *string `json:"lv6_img_path,omitempty"`
	EmptyHonorPath          *string `json:"empty_honor_path,omitempty"`
	ScrollImgPath           *string `json:"scroll_img_path,omitempty"`
	WordImgPath             *string `json:"word_img_path,omitempty"`
	CharaIconPath           *string `json:"chara_icon_path,omitempty"`
	CharaIconPath2          *string `json:"chara_icon_path2,omitempty"`
	CharaID                 *string `json:"chara_id,omitempty"`
	CharaID2                *string `json:"chara_id2,omitempty"`
	BondsBgPath             *string `json:"bonds_bg_path,omitempty"`
	BondsBgPath2            *string `json:"bonds_bg_path2,omitempty"`
	MaskImgPath             *string `json:"mask_img_path,omitempty"`
	FrameImgPath            *string `json:"frame_img_path,omitempty"`
	FrameDegreeLevelImgPath *string `json:"frame_degree_level_img_path,omitempty"`
}

// ProfileRequest represents request for /profile/profile
type ProfileRequest struct {
	Profile              BasicProfile               `json:"profile"`
	Rank                 int                        `json:"rank"`
	TwitterID            string                     `json:"twitter_id"`
	Word                 string                     `json:"word"`
	Pcards               []CardFullThumbnailRequest `json:"pcards"`
	BgSettings           *ProfileBgSettings         `json:"bg_settings,omitempty"`
	Honors               []HonorRequest             `json:"honors"`
	MusicDifficultyCount []MusicClearCount          `json:"music_difficulty_count"`
	CharacterRank        []CharacterRank            `json:"character_rank"`
	SoloLive             *SoloLiveRank              `json:"solo_live,omitempty"`
	UpdateTime           *int64                     `json:"update_time,omitempty"`
	LvRankBgPath         string                     `json:"lv_rank_bg_path"`
	XIconPath            string                     `json:"x_icon_path"`
	IconClearPath        string                     `json:"icon_clear_path"`
	IconFcPath           string                     `json:"icon_fc_path"`
	IconApPath           string                     `json:"icon_ap_path"`
	CharaRankIconPathMap map[string]string          `json:"chara_rank_icon_path_map"`
	FramePaths           *PlayerFramePaths          `json:"frame_paths,omitempty"`
}

// =========================== Card Models ===========================

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
	EventID         int         `json:"event_id"`
	EventName       string      `json:"event_name"`
	StartAt         interface{} `json:"start_at"` // datetime | int | str
	EndAt           interface{} `json:"end_at"`
	EventBannerPath string      `json:"event_banner_path"`
	BonusAttr       *string     `json:"bonus_attr,omitempty"`
	Unit            *string     `json:"unit,omitempty"`
	BannerCid       *int        `json:"banner_cid,omitempty"`
}

type CardGachaInfo struct {
	GachaID         int         `json:"gacha_id"`
	GachaName       string      `json:"gacha_name"`
	StartAt         interface{} `json:"start_at"`
	EndAt           interface{} `json:"end_at"`
	GachaBannerPath string      `json:"gacha_banner_path"`
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
	Cards             []CardBasic                 `json:"cards"`
	Region            string                      `json:"region"`
	UserInfo          *DetailedProfileCardRequest `json:"user_info,omitempty"`
	BackgroundImgPath *string                     `json:"background_img_path,omitempty"`
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
	TermLimitedIconPath *string                     `json:"term_limited_icon_path,omitempty"`
	FesLimitedIconPath  *string                     `json:"fes_limited_icon_path,omitempty"`
}

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
	Region                     string                     `json:"region"`
	Profile                    DetailedProfileCardRequest `json:"profile"`
	DeckData                   []DeckData                 `json:"deck_data"`
	EventName                  *string                    `json:"event_name,omitempty"`
	MusicTitle                 *string                    `json:"music_title,omitempty"`
	MusicID                    *int                       `json:"music_id,omitempty"`
	MusicDiff                  *string                    `json:"music_diff,omitempty"`
	EventBannerPath            *string                    `json:"event_banner_path,omitempty"`
	MusicCoverPath             *string                    `json:"music_cover_path,omitempty"`
	IsMaxDeck                  bool                       `json:"is_max_deck"`
	RecommendType              string                     `json:"recommend_type"`
	WlCharaName                *string                    `json:"wl_chara_name,omitempty"`
	WlCharaIconPath            *string                    `json:"wl_chara_icon_path,omitempty"`
	EventID                    *int                       `json:"event_id,omitempty"`
	LiveType                   *string                    `json:"live_type,omitempty"`
	LiveName                   *string                    `json:"live_name,omitempty"`
	CharaIconPath              *string                    `json:"chara_icon_path,omitempty"`
	CharaName                  *string                    `json:"chara_name,omitempty"`
	UnitLogoPath               *string                    `json:"unit_logo_path,omitempty"`
	AttrIconPath               *string                    `json:"attr_icon_path,omitempty"`
	IsWl                       bool                       `json:"is_wl"`
	MultiLiveTeammatePower     *int                       `json:"multi_live_teammate_power,omitempty"`
	MultiLiveTeammateScoreUp   *float64                   `json:"multi_live_teammate_score_up,omitempty"`
	Target                     *string                    `json:"target,omitempty"`
	UnitFilter                 *string                    `json:"unit_filter,omitempty"`
	AttrFilter                 *string                    `json:"attr_filter,omitempty"`
	ExcludedCards              []int                      `json:"excluded_cards,omitempty"`
	MultiLiveScoreUpLowerBound *float64                   `json:"multi_live_score_up_lower_bound,omitempty"`
	KeepAfterTrainingState     bool                       `json:"keep_after_training_state"`
	ModelName                  []interface{}              `json:"model_name,omitempty"`
	CanvasThumbnailPath        *string                    `json:"canvas_thumbnail_path,omitempty"`
	FixedCardsID               []int                      `json:"fixed_cards_id,omitempty"`
	FixedCharactersID          []int                      `json:"fixed_characters_id,omitempty"`
	CostTimes                  map[string]interface{}     `json:"cost_times,omitempty"`
	WaitTimes                  map[string]interface{}     `json:"wait_times,omitempty"`
}

// =========================== Education Models ===========================

type CharacterChallengeInfo struct {
	CharaID       int    `json:"chara_id"`
	Rank          int    `json:"rank"`
	Score         int    `json:"score"`
	Jewel         int    `json:"jewel"`
	Shard         int    `json:"shard"`
	CharaIconPath string `json:"chara_icon_path"`
}

type ChallengeLiveDetailsRequest struct {
	Profile             DetailedProfileCardRequest `json:"profile"`
	CharacterChallenges []CharacterChallengeInfo   `json:"character_challenges"`
	MaxScore            int                        `json:"max_score"`
	JewelIconPath       *string                    `json:"jewel_icon_path,omitempty"`
	ShardIconPath       *string                    `json:"shard_icon_path,omitempty"`
}

type CharacterBonus struct {
	CharaID       int     `json:"chara_id"`
	CharaIconPath string  `json:"chara_icon_path"`
	AreaItem      float64 `json:"area_item"`
	Rank          float64 `json:"rank"`
	Fixture       float64 `json:"fixture"`
	Total         float64 `json:"total"`
}

type UnitBonus struct {
	Unit         string  `json:"unit"`
	UnitIconPath string  `json:"unit_icon_path"`
	AreaItem     float64 `json:"area_item"`
	Gate         float64 `json:"gate"`
	Total        float64 `json:"total"`
}

type AttrBonus struct {
	Attr         string  `json:"attr"`
	AttrIconPath string  `json:"attr_icon_path"`
	AreaItem     float64 `json:"area_item"`
	Total        float64 `json:"total"`
}

type PowerBonusDetailRequest struct {
	Profile      DetailedProfileCardRequest `json:"profile"`
	CharaBonuses []CharacterBonus           `json:"chara_bonuses"`
	UnitBonuses  []UnitBonus                `json:"unit_bonuses"`
	AttrBonuses  []AttrBonus                `json:"attr_bonuses"`
}

type AreaItemMaterial struct {
	MaterialID       int    `json:"material_id"`
	MaterialIconPath string `json:"material_icon_path"`
	Quantity         int    `json:"quantity"`
	HaveQuantity     int    `json:"have_quantity"`
	SumQuantity      int    `json:"sum_quantity"`
	IsEnough         bool   `json:"is_enough"`
}

type AreaItemLevel struct {
	Level      int                `json:"level"`
	Bonus      float64            `json:"bonus"`
	CanUpgrade bool               `json:"can_upgrade"`
	Materials  []AreaItemMaterial `json:"materials"`
}

type AreaItemInfo struct {
	ItemID         int             `json:"item_id"`
	CurrentLevel   int             `json:"current_level"`
	ItemIconPath   string          `json:"item_icon_path"`
	TargetIconPath *string         `json:"target_icon_path,omitempty"`
	Levels         []AreaItemLevel `json:"levels"`
}

type AreaItemUpgradeMaterialsRequest struct {
	Profile    *DetailedProfileCardRequest `json:"profile,omitempty"`
	AreaItems  []AreaItemInfo              `json:"area_items"`
	HasProfile bool                        `json:"has_profile"`
}

type BondInfo struct {
	CharaID1       int    `json:"chara_id1"`
	CharaID2       int    `json:"chara_id2"`
	CharaIconPath1 string `json:"chara_icon_path1"`
	CharaIconPath2 string `json:"chara_icon_path2"`
	CharaRank1     int    `json:"chara_rank1"`
	CharaRank2     int    `json:"chara_rank2"`
	BondLevel      int    `json:"bond_level"`
	NeedExp        *int   `json:"need_exp,omitempty"`
	HasBond        bool   `json:"has_bond"`
	Color1         []int  `json:"color1,omitempty"` // tuple in python, []int in go
	Color2         []int  `json:"color2,omitempty"`
}

type BondsRequest struct {
	Profile  DetailedProfileCardRequest `json:"profile"`
	Bonds    []BondInfo                 `json:"bonds"`
	MaxLevel int                        `json:"max_level"`
}

type LeaderCountInfo struct {
	CharaID       int    `json:"chara_id"`
	CharaIconPath string `json:"chara_icon_path"`
	PlayCount     int    `json:"play_count"`
	ExLevel       int    `json:"ex_level"`
	ExCount       int    `json:"ex_count"`
}

type LeaderCountRequest struct {
	Profile      DetailedProfileCardRequest `json:"profile"`
	LeaderCounts []LeaderCountInfo          `json:"leader_counts"`
	MaxPlayCount int                        `json:"max_play_count"`
}

// =========================== Event Models ===========================

type EventInfo struct {
	ID           interface{}              `json:"id"` // str | int
	EventType    string                   `json:"event_type"`
	StartAt      interface{}              `json:"start_at"` // datetime (int64 ts)
	EndAt        interface{}              `json:"end_at"`
	IsWlEvent    bool                     `json:"is_wl_event"`
	BannerCid    int                      `json:"banner_cid"`
	BannerIndex  int                      `json:"banner_index"`
	BonusAttr    string                   `json:"bonus_attr"`
	BonusCharaID []int                    `json:"bonus_chara_id,omitempty"`
	WlTimeList   []map[string]interface{} `json:"wl_time_list,omitempty"`
}

type EventHistory struct {
	ID              interface{} `json:"id"` // str | int
	EventName       string      `json:"event_name"`
	StartAt         interface{} `json:"start_at"`
	EndAt           interface{} `json:"end_at"`
	Rank            *int        `json:"rank,omitempty"`
	EventPoint      int         `json:"event_point"`
	IsWlEvent       bool        `json:"is_wl_event"`
	BannerPath      string      `json:"banner_path"`
	WlCharaIconPath *string     `json:"wl_chara_icon_path,omitempty"`
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
	StartAt         interface{}                `json:"start_at"`
	EndAt           interface{}                `json:"end_at"`
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
	ID        int         `json:"id"`
	Name      string      `json:"name"`
	GachaType string      `json:"gacha_type"`
	StartAt   interface{} `json:"start_at"`
	EndAt     interface{} `json:"end_at"`
	AssetName string      `json:"asset_name"`
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
	Gachas     []GachaBrief   `json:"gachas"`
	PageSize   int            `json:"page_size"`
	Region     string         `json:"region"`
	GachaLogos map[int]string `json:"gacha_logos"`
	Filter     GachaFilter    `json:"filter"`
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

// =========================== Misc Models ===========================

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

// =========================== MySekai Models ===========================

type MysekaiPhenomRequest struct {
	RefreshReason  string `json:"refresh_reason"`
	ImagePath      string `json:"image_path"`
	BackgroundFill []int  `json:"background_fill"` // Color tuple
	Text           string `json:"text"`
	TextFill       []int  `json:"text_fill"`
}

type MysekaiVisitCharacter struct {
	SdImagePath         string  `json:"sd_image_path"`
	MemoriaImagePath    *string `json:"memoria_image_path,omitempty"`
	IsRead              bool    `json:"is_read"`
	IsReservation       bool    `json:"is_reservation"`
	ReservationIconPath *string `json:"reservation_icon_path,omitempty"`
}

type MysekaiSiteResourceNumber struct {
	ImagePath       string                  `json:"image_path"`
	ResourceNumbers []MysekaiResourceNumber `json:"resource_numbers"`
}

type MysekaiResourceNumber struct {
	ImagePath           string  `json:"image_path"`
	Number              int     `json:"number"`
	TextColor           []int   `json:"text_color"`
	HasMusicRecord      bool    `json:"has_music_record"`
	MusicRecordIconPath *string `json:"music_record_icon_path,omitempty"`
}

type MysekaiResourceRequest struct {
	Profile             ProfileCardRequest          `json:"profile"`
	BackgroundImagePath *string                     `json:"background_image_path,omitempty"`
	Phenoms             []MysekaiPhenomRequest      `json:"phenoms"`
	GateID              int                         `json:"gate_id"`
	GateLevel           int                         `json:"gate_level"`
	GateIconPath        string                      `json:"gate_icon_path"`
	VisitCharacters     []MysekaiVisitCharacter     `json:"visit_characters"`
	SiteResourceNumbers []MysekaiSiteResourceNumber `json:"site_resource_numbers,omitempty"`
}

type MysekaiFixture struct {
	ID            int     `json:"id"`
	ImagePath     string  `json:"image_path"`
	CharacterID   *int    `json:"character_id,omitempty"`
	CharaIconPath *string `json:"chara_icon_path,omitempty"`
	Obtained      bool    `json:"obtained"`
}

type MysekaiFixtureSubGenre struct {
	Name            *string          `json:"name,omitempty"`
	ImagePath       *string          `json:"image_path,omitempty"`
	ProgressMessage *string          `json:"progress_message,omitempty"`
	Fixtures        []MysekaiFixture `json:"fixtures"`
}

type MysekaiFixtureMainGenre struct {
	Name            string                   `json:"name"`
	ImagePath       string                   `json:"image_path"`
	ProgressMessage *string                  `json:"progress_message,omitempty"`
	SubGenres       []MysekaiFixtureSubGenre `json:"sub_genres"`
}

type MysekaiFixtureListRequest struct {
	Profile         *ProfileCardRequest       `json:"profile,omitempty"`
	ProgressMessage *string                   `json:"progress_message,omitempty"`
	ShowID          bool                      `json:"show_id"`
	MainGenres      []MysekaiFixtureMainGenre `json:"main_genres"`
}

type MysekaiFixtureColorImage struct {
	ImagePath string  `json:"image_path"`
	ColorCode *string `json:"color_code,omitempty"`
}

type MysekaiFixtureMaterial struct {
	ImagePath string `json:"image_path"`
	Quantity  int    `json:"quantity"`
}

type MysekaiReactionCharacterGroups struct {
	Number                int        `json:"number"`
	CharacterUintIDGroups [][]int    `json:"character_uint_id_groups,omitempty"`
	CharaIconPathGroups   [][]string `json:"chara_icon_path_groups,omitempty"`
}

type MysekaiFixtureDetailRequest struct {
	Title                   string                           `json:"title"`
	Images                  []MysekaiFixtureColorImage       `json:"images"`
	MainGenreName           string                           `json:"main_genre_name"`
	MainGenreImagePath      string                           `json:"main_genre_image_path"`
	SubGenreName            *string                          `json:"sub_genre_name,omitempty"`
	SubGenreImagePath       *string                          `json:"sub_genre_image_path,omitempty"`
	Size                    map[string]int                   `json:"size"` // width, depth, height
	FirstPutCost            int                              `json:"first_put_cost"`
	SecondPutCost           int                              `json:"second_put_cost"`
	BasicInfo               []string                         `json:"basic_info,omitempty"`
	CostMaterials           []MysekaiFixtureMaterial         `json:"cost_materials,omitempty"`
	RecycleMaterials        []MysekaiFixtureMaterial         `json:"recycle_materials,omitempty"`
	ReactionCharacterGroups []MysekaiReactionCharacterGroups `json:"reaction_character_groups,omitempty"`
	Tags                    []string                         `json:"tags,omitempty"`
	Friendcodes             []string                         `json:"friendcodes,omitempty"`
	FriendcodeSource        string                           `json:"friendcode_source"`
}

type MysekaiGateMaterialItem struct {
	ImagePath   string `json:"image_path"`
	Quantity    int    `json:"quantity"`
	Color       []int  `json:"color"`
	SumQuantity string `json:"sum_quantity"`
}

type MysekaiGateLevelMaterials struct {
	Level int                       `json:"level"`
	Color []int                     `json:"color"`
	Items []MysekaiGateMaterialItem `json:"items"`
}

type MysekaiGateMaterials struct {
	ID             int                         `json:"id"`
	Level          *int                        `json:"level,omitempty"`
	LevelMaterials []MysekaiGateLevelMaterials `json:"level_materials"`
}

type MysekaiDoorUpgradeRequest struct {
	Profile       *ProfileCardRequest    `json:"profile,omitempty"`
	GateMaterials []MysekaiGateMaterials `json:"gate_materials"`
}

type MysekaiMusicrecord struct {
	ID        *int   `json:"id,omitempty"`
	ImagePath string `json:"image_path"`
	Obtained  bool   `json:"obtained"`
}

type MysekaiCategoryMusicrecord struct {
	Tag             string               `json:"tag"`
	TagIconPath     string               `json:"tag_icon_path"`
	ProgressMessage *string              `json:"progress_message,omitempty"`
	Musicrecords    []MysekaiMusicrecord `json:"musicrecords"`
}

type MysekaiMusicrecordRequest struct {
	Profile              ProfileCardRequest           `json:"profile"`
	ProgressMessage      *string                      `json:"progress_message,omitempty"`
	CategoryMusicrecords []MysekaiCategoryMusicrecord `json:"category_musicrecords"`
}

type MysekaiTalkFixtures struct {
	Fixtures            []MysekaiFixture `json:"fixtures"`
	NoreadNum           int              `json:"noread_num"`
	CharacterIDs        [][]int          `json:"character_ids,omitempty"`
	CharaIconPathGroups [][]string       `json:"chara_icon_path_groups,omitempty"`
}

type MysekaiSingleTalkMainGenre struct {
	Name      string                  `json:"name"`
	ImagePath string                  `json:"image_path"`
	SubGenres [][]MysekaiTalkFixtures `json:"sub_genres"`
}

type MysekaiTalkListRequest struct {
	Profile          *ProfileCardRequest          `json:"profile,omitempty"`
	SdImagePath      string                       `json:"sd_image_path"`
	ProgressMessage  *string                      `json:"progress_message,omitempty"`
	PromptMessage    *string                      `json:"prompt_message,omitempty"`
	ShowID           bool                         `json:"show_id"`
	SingleMainGenres []MysekaiSingleTalkMainGenre `json:"single_main_genres"`
	MultiReads       []MysekaiTalkFixtures        `json:"multi_reads"`
}

// =========================== Score Models ===========================

type ScoreData struct {
	EventBonus int `json:"event_bonus"`
	Boost      int `json:"boost"`
	ScoreMin   int `json:"score_min"`
	ScoreMax   int `json:"score_max"`
}

type ScoreControlRequest struct {
	MusicCoverPath  string      `json:"music_cover_path"`
	MusicID         int         `json:"music_id"`
	MusicTitle      string      `json:"music_title"`
	MusicBasicPoint int         `json:"music_basic_point"`
	TargetPoint     int         `json:"target_point"`
	ValidScores     []ScoreData `json:"valid_scores"`
}

type CustomRoomScoreRequest struct {
	TargetPoint    int                              `json:"target_point"`
	CandidatePairs [][]int                          `json:"candidate_pairs"` // List of (event_rate, event_bonus) tuples
	MusicListMap   map[int][]map[string]interface{} `json:"music_list_map"`
}

type MusicMetaInfo struct {
	Difficulty      string    `json:"difficulty"`
	MusicTime       float64   `json:"music_time"`
	TapCount        int       `json:"tap_count"`
	EventRate       float64   `json:"event_rate"`
	BaseScore       float64   `json:"base_score"`
	BaseScoreAuto   float64   `json:"base_score_auto"`
	SkillScoreSolo  []float64 `json:"skill_score_solo"`
	SkillScoreAuto  []float64 `json:"skill_score_auto"`
	SkillScoreMulti []float64 `json:"skill_score_multi"`
	FeverScore      float64   `json:"fever_score"`
}

type MusicMetaRequest struct {
	MusicID        int             `json:"music_id"`
	MusicTitle     string          `json:"music_title"`
	MusicCoverPath string          `json:"music_cover_path"`
	Metas          []MusicMetaInfo `json:"metas"`
}

type MusicBoardItem struct {
	Rank                 int      `json:"rank"`
	MusicID              int      `json:"music_id"`
	Difficulty           string   `json:"difficulty"`
	Level                int      `json:"level"`
	MusicTitle           string   `json:"music_title"`
	MusicCoverPath       string   `json:"music_cover_path"`
	LiveTypePt           *float64 `json:"live_type_pt,omitempty"`
	LiveTypeRealScore    *float64 `json:"live_type_real_score,omitempty"`
	LiveTypeScore        *float64 `json:"live_type_score,omitempty"`
	LiveTypeSkillAccount *float64 `json:"live_type_skill_account,omitempty"`
	LiveTypePtPerHour    *float64 `json:"live_type_pt_per_hour,omitempty"`
	PlayCountPerHour     *float64 `json:"play_count_per_hour,omitempty"`
	EventRate            float64  `json:"event_rate"`
	MusicTime            float64  `json:"music_time"`
	Tps                  float64  `json:"tps"`
}

type MusicBoardRequest struct {
	LiveType     string           `json:"live_type"`
	Target       string           `json:"target"`
	Ascend       bool             `json:"ascend"`
	Page         int              `json:"page"`
	TotalPage    int              `json:"total_page"`
	TitleText    string           `json:"title_text"`
	Items        []MusicBoardItem `json:"items"`
	SpecMidDiffs [][]interface{}  `json:"spec_mid_diffs,omitempty"` // Tuple (int, str)
	Description  string           `json:"description"`
}

// =========================== Stamp Models ===========================

type StampData struct {
	ID        int    `json:"id"`
	ImagePath string `json:"image_path"`
	TextColor []int  `json:"text_color"` // Color (r,g,b,a)
}

type StampListRequest struct {
	PromptMessage *string     `json:"prompt_message,omitempty"`
	Stamps        []StampData `json:"stamps"`
}

// =========================== Chart Models ===========================

type GenerateMusicChartRequest struct {
	MusicID              interface{}            `json:"music_id"` // str | int
	Title                string                 `json:"title"`
	Artist               string                 `json:"artist"`
	Difficulty           string                 `json:"difficulty"`
	PlayLevel            interface{}            `json:"play_level"` // str | int
	Skill                bool                   `json:"skill"`
	JacketPath           string                 `json:"jacket_path"`
	SusPath              string                 `json:"sus_path"`
	StylePath            *string                `json:"style_path,omitempty"`
	NoteHost             string                 `json:"note_host"`
	MusicMeta            map[string]interface{} `json:"music_meta,omitempty"`
	TargetSegmentSeconds *float64               `json:"target_segment_seconds,omitempty"`
}

// =========================== SK Models ===========================

type RankInfo struct {
	Rank            int         `json:"rank"`
	Name            string      `json:"name"`
	Score           *int        `json:"score,omitempty"`
	Time            interface{} `json:"time"` // datetime
	AverageRound    *int        `json:"average_round,omitempty"`
	AveragePt       *int        `json:"average_pt,omitempty"`
	LatestPt        *int        `json:"latest_pt,omitempty"`
	Speed           *int        `json:"speed,omitempty"`
	Min20Time3Speed *int        `json:"min20_times_3_speed,omitempty"`
	HourRound       *int        `json:"hour_round,omitempty"`
	RecordStartAt   interface{} `json:"record_start_at,omitempty"` // datetime
}

type SpeedInfo struct {
	Rank       int         `json:"rank"`
	Score      int         `json:"score"`
	Speed      *int        `json:"speed,omitempty"`
	RecordTime interface{} `json:"record_time"` // datetime
}

type TeamInfo struct {
	TeamID       int     `json:"team_id"`
	TeamName     string  `json:"team_name"`
	WinRate      float64 `json:"win_rate"`
	IsRecruiting bool    `json:"is_recruiting"`
	TeamCnName   *string `json:"team_cn_name,omitempty"`
	TeamIconPath *string `json:"team_icon_path,omitempty"`
}

type SklRequest struct {
	ID            int        `json:"id"`
	Region        string     `json:"region"`
	StartAt       int64      `json:"start_at"`
	AggregateAt   int64      `json:"aggregate_at"`
	Name          string     `json:"name"`
	BannerImgPath string     `json:"banner_img_path"`
	WlCid         *int       `json:"wl_cid,omitempty"`
	CharaIconPath *string    `json:"chara_icon_path,omitempty"`
	Ranks         []RankInfo `json:"ranks"`
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
	Eid             int         `json:"eid"`
	EventName       string      `json:"event_name"`
	Region          string      `json:"region"`
	Ranks           []RankInfo  `json:"ranks"`
	PrevRank        *RankInfo   `json:"prev_rank,omitempty"`
	NextRank        *RankInfo   `json:"next_rank,omitempty"`
	AggregateAt     int64       `json:"aggregate_at"`
	UpdateAt        interface{} `json:"update_at"` // datetime
	WlCharaIconPath *string     `json:"wl_chara_icon_path,omitempty"`
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
	WlCharaIconPath  *string     `json:"wl_chara_icon_path,omitempty"`
	UpdatedAt        interface{} `json:"updated_at"` // datetime
	EventStartAt     int64       `json:"event_start_at"`
	EventAggregateAt int64       `json:"event_aggregate_at"`
	BannerImgPath    *string     `json:"banner_img_path,omitempty"`
	TeamInfo         []TeamInfo  `json:"team_info"`
}
