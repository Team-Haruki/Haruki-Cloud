package drawing

import sekaiapi "haruki-cloud/internal/pjsk/sekai"

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
	BgAlpha      *int                `json:"bg_alpha,omitempty"`
	ErrorMessage *string             `json:"error_message,omitempty"`
}

type DetailedProfileCardRequest struct {
	ID              string  `json:"id"`
	Region          string  `json:"region"`
	Nickname        string  `json:"nickname"`
	Source          string  `json:"source"`
	UpdateTime      int64   `json:"update_time"`
	Mode            *string `json:"mode,omitempty"`
	IsHideUID       bool    `json:"is_hide_uid"`
	LeaderImagePath string  `json:"leader_image_path"`
	HasFrame        bool    `json:"has_frame"`
	FramePath       *string `json:"frame_path,omitempty"`
	UserCards       []any   `json:"user_cards,omitempty"`
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

type MultiLiveTopScoreCount struct {
	MVP       int `json:"mvp"`
	SuperStar int `json:"super_star"`
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
	MultiLive            *MultiLiveTopScoreCount    `json:"multi_live,omitempty"`
	UpdateTime           *int64                     `json:"update_time,omitempty"`
	LvRankBgPath         string                     `json:"lv_rank_bg_path"`
	XIconPath            string                     `json:"x_icon_path"`
	IconClearPath        string                     `json:"icon_clear_path"`
	IconFcPath           string                     `json:"icon_fc_path"`
	IconApPath           string                     `json:"icon_ap_path"`
	CharaRankIconPathMap map[string]string          `json:"chara_rank_icon_path_map"`
	FramePaths           *PlayerFramePaths          `json:"frame_paths,omitempty"`
}

const CustomProfileCardRequestKind = "pjsk_custom_profile_card"

const CustomProfileCardRenderVersion = 3

type CustomProfileContext struct {
	User                          sekaiapi.AnotherUser                            `json:"user"`
	UserProfile                   sekaiapi.UserProfile                            `json:"userProfile"`
	UserDeck                      sekaiapi.UserDeck                               `json:"userDeck"`
	UserCards                     []sekaiapi.AnotherUserCard                      `json:"userCards"`
	UserCharacters                []sekaiapi.AnotherUserCharacter                 `json:"userCharacters"`
	UserChallengeLiveSoloResult   sekaiapi.UserChallengeLiveSoloResult            `json:"userChallengeLiveSoloResult"`
	UserChallengeLiveSoloStages   []sekaiapi.AnotherUserChallengeLiveSoloStage    `json:"userChallengeLiveSoloStages"`
	UserMusicDifficultyClearCount []sekaiapi.AnotherUserMusicDifficultyClearCount `json:"userMusicDifficultyClearCount"`
	UserProfileHonors             []sekaiapi.UserProfileHonor                     `json:"userProfileHonors"`
	UserHonors                    []sekaiapi.UserHonor                            `json:"userHonors"`
	UserBondsHonors               []sekaiapi.UserBondsHonor                       `json:"userBondsHonors"`
	UserStoryFavorites            []sekaiapi.UserStoryFavorite                    `json:"userStoryFavorites"`
	UserConfig                    sekaiapi.AnotherUserConfig                      `json:"userConfig"`
	UserMultiLiveTopScoreCount    sekaiapi.AnotherUserMultiLiveTopScoreCount      `json:"userMultiLiveTopScoreCount"`
	TotalPower                    sekaiapi.AnotherTotalPower                      `json:"totalPower"`
	UserHonorMissions             []sekaiapi.UserHonorMission                     `json:"userHonorMissions"`
	IsMysekaiOwnerAcceptVisit     bool                                            `json:"isMysekaiOwnerAcceptVisit"`
}

type CustomProfileCardRenderRequest struct {
	SchemaVersion  int                            `json:"schema_version"`
	RenderVersion  int                            `json:"render_version,omitempty"`
	Kind           string                         `json:"kind"`
	Region         string                         `json:"region"`
	Card           sekaiapi.UserCustomProfileCard `json:"card"`
	Resources      CustomProfileResources         `json:"resources"`
	ProfileContext CustomProfileContext           `json:"profile_context"`
}

type CustomProfileResources map[string]any

func NewCustomProfileCardRenderRequest(region string, card sekaiapi.UserCustomProfileCard, resp *sekaiapi.GetAnotherProfileResponse, resources CustomProfileResources) *CustomProfileCardRenderRequest {
	if resources == nil {
		resources = CustomProfileResources{}
	}
	return &CustomProfileCardRenderRequest{
		SchemaVersion:  1,
		RenderVersion:  CustomProfileCardRenderVersion,
		Kind:           CustomProfileCardRequestKind,
		Region:         region,
		Card:           card,
		Resources:      resources,
		ProfileContext: NewCustomProfileContext(resp),
	}
}

func NewCustomProfileContext(resp *sekaiapi.GetAnotherProfileResponse) CustomProfileContext {
	if resp == nil {
		return CustomProfileContext{}
	}
	return CustomProfileContext{
		User:                          resp.User,
		UserProfile:                   resp.UserProfile,
		UserDeck:                      resp.UserDeck,
		UserCards:                     resp.UserCards,
		UserCharacters:                resp.UserCharacters,
		UserChallengeLiveSoloResult:   resp.UserChallengeLiveSoloResult,
		UserChallengeLiveSoloStages:   resp.UserChallengeLiveSoloStages,
		UserMusicDifficultyClearCount: resp.UserMusicDifficultyClearCount,
		UserProfileHonors:             resp.UserProfileHonors,
		UserHonors:                    resp.UserHonors,
		UserBondsHonors:               resp.UserBondsHonors,
		UserStoryFavorites:            resp.UserStoryFavorites,
		UserConfig:                    resp.UserConfig,
		UserMultiLiveTopScoreCount:    resp.UserMultiLiveTopScoreCount,
		TotalPower:                    resp.TotalPower,
		UserHonorMissions:             resp.UserHonorMissions,
		IsMysekaiOwnerAcceptVisit:     resp.IsMysekaiOwnerAcceptVisit,
	}
}
