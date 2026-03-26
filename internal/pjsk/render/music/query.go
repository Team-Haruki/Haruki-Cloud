package music

import "haruki-cloud/utils/drawing"

type Query struct {
	Query      string `json:"query"`
	Region     string `json:"region"`
	Difficulty string `json:"difficulty,omitempty"`
	UserID     string `json:"user_id,omitempty"`
}

type NoteCountQuery struct {
	NoteCount int    `json:"note_count"`
	Region    string `json:"region"`
}

type ChartQuery struct {
	Query      string `json:"query"`
	Region     string `json:"region"`
	Difficulty string `json:"difficulty,omitempty"`
	Skill      bool   `json:"skill,omitempty"`
	Style      string `json:"style,omitempty"`
}

type BriefListQuery struct {
	MusicIDs   []int  `json:"music_ids"`
	Difficulty string `json:"difficulty,omitempty"`
	Region     string `json:"region"`
}

type ListQuery struct {
	Difficulty      string                              `json:"difficulty"`
	Level           int                                 `json:"level,omitempty"`
	LevelMin        int                                 `json:"level_min,omitempty"`
	LevelMax        int                                 `json:"level_max,omitempty"`
	Region          string                              `json:"region"`
	IncludeLeaks    bool                                `json:"include_leaks,omitempty"`
	UserResults     map[int]string                      `json:"user_results,omitempty"`
	Title           *string                             `json:"title,omitempty"`
	TitleStyle      map[string]interface{}              `json:"title_style,omitempty"`
	TitleShadow     bool                                `json:"title_shadow,omitempty"`
	Keyword         string                              `json:"keyword,omitempty"`
	ShowID          bool                                `json:"show_id,omitempty"`
	DetailedProfile *drawing.DetailedProfileCardRequest `json:"-"`
}

type ProgressQuery struct {
	Difficulty string                      `json:"difficulty"`
	Region     string                      `json:"region"`
	Counts     []drawing.PlayProgressCount `json:"counts,omitempty"`
	Title      *string                     `json:"title,omitempty"`
	TitleStyle map[string]interface{}      `json:"title_style,omitempty"`
	Profile    *drawing.ProfileCardRequest `json:"-"`
}

type RewardsDetailQuery struct {
	Region        string                                `json:"region"`
	RankRewards   int                                   `json:"rank_rewards"`
	ComboRewards  map[string][]drawing.MusicComboReward `json:"combo_rewards"`
	Title         *string                               `json:"title,omitempty"`
	TitleStyle    map[string]interface{}                `json:"title_style,omitempty"`
	JewelIconPath *string                               `json:"jewel_icon_path,omitempty"`
	ShardIconPath *string                               `json:"shard_icon_path,omitempty"`
	Profile       *drawing.ProfileCardRequest           `json:"-"`
}

type RewardsBasicQuery struct {
	Region        string                      `json:"region"`
	RankRewards   string                      `json:"rank_rewards"`
	ComboRewards  map[string]string           `json:"combo_rewards"`
	Title         *string                     `json:"title,omitempty"`
	TitleStyle    map[string]interface{}      `json:"title_style,omitempty"`
	JewelIconPath *string                     `json:"jewel_icon_path,omitempty"`
	ShardIconPath *string                     `json:"shard_icon_path,omitempty"`
	Profile       *drawing.ProfileCardRequest `json:"-"`
}
