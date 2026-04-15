package music

import "haruki-cloud/utils/drawing"

type Query struct {
	Query      string `json:"query"`
	Region     string `json:"region"`
	Difficulty string `json:"difficulty,omitempty"`
	UserID     string `json:"user_id,omitempty"`
}

type BPMQuery struct {
	BPM        float64 `json:"bpm"`
	Region     string  `json:"region"`
	Difficulty string  `json:"difficulty,omitempty"`
}

type NoteCountQuery struct {
	NoteCount  int    `json:"note_count"`
	Difficulty string `json:"difficulty,omitempty"`
	Region     string `json:"region"`
}

type BriefListItemQuery struct {
	MusicID    int    `json:"music_id"`
	Difficulty string `json:"difficulty,omitempty"`
}

type ChartQuery struct {
	Query      string `json:"query"`
	Region     string `json:"region"`
	Difficulty string `json:"difficulty,omitempty"`
	Skill      bool   `json:"skill,omitempty"`
	Style      string `json:"style,omitempty"`
}

type BriefListQuery struct {
	MusicIDs    []int                `json:"music_ids"`
	Items       []BriefListItemQuery `json:"items,omitempty"`
	Difficulty  string               `json:"difficulty,omitempty"`
	Region      string               `json:"region"`
	Title       *string              `json:"title,omitempty"`
	TitleStyle  map[string]any       `json:"title_style,omitempty"`
	TitleShadow bool                 `json:"title_shadow,omitempty"`
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
	TitleStyle      map[string]any                      `json:"title_style,omitempty"`
	TitleShadow     bool                                `json:"title_shadow,omitempty"`
	Keyword         string                              `json:"keyword,omitempty"`
	ShowID          bool                                `json:"show_id,omitempty"`
	DetailedProfile *drawing.DetailedProfileCardRequest `json:"-"`
}

type ProgressQuery struct {
	Difficulty  string                      `json:"difficulty"`
	Region      string                      `json:"region"`
	Counts      []drawing.PlayProgressCount `json:"counts,omitempty"`
	UserResults map[int]string              `json:"user_results,omitempty"`
	Title       *string                     `json:"title,omitempty"`
	TitleStyle  map[string]any              `json:"title_style,omitempty"`
	Profile     *drawing.ProfileCardRequest `json:"-"`
}

type BoardQuery struct {
	LiveType      string    `json:"live_type,omitempty"`
	Target        string    `json:"target,omitempty"`
	Ascend        bool      `json:"ascend,omitempty"`
	Page          int       `json:"page,omitempty"`
	SkillStrategy string    `json:"skill_strategy,omitempty"`
	Skills        []float64 `json:"skills,omitempty"`
	Power         int       `json:"power,omitempty"`
	DeckBonus     float64   `json:"deck_bonus,omitempty"`
	PlayInterval  float64   `json:"play_interval,omitempty"`
	DiffFilter    []string  `json:"diff_filter,omitempty"`
	LevelFilter   string    `json:"level_filter,omitempty"`
	SpecQueries   []string  `json:"spec_queries,omitempty"`
}

type RewardsDetailQuery struct {
	Region        string                                `json:"region"`
	RankRewards   int                                   `json:"rank_rewards"`
	ComboRewards  map[string][]drawing.MusicComboReward `json:"combo_rewards"`
	Title         *string                               `json:"title,omitempty"`
	TitleStyle    map[string]any                        `json:"title_style,omitempty"`
	JewelIconPath *string                               `json:"jewel_icon_path,omitempty"`
	ShardIconPath *string                               `json:"shard_icon_path,omitempty"`
	Profile       *drawing.ProfileCardRequest           `json:"-"`
}

type RewardsBasicQuery struct {
	Region        string                      `json:"region"`
	RankRewards   string                      `json:"rank_rewards"`
	ComboRewards  map[string]string           `json:"combo_rewards"`
	Title         *string                     `json:"title,omitempty"`
	TitleStyle    map[string]any              `json:"title_style,omitempty"`
	JewelIconPath *string                     `json:"jewel_icon_path,omitempty"`
	ShardIconPath *string                     `json:"shard_icon_path,omitempty"`
	Profile       *drawing.ProfileCardRequest `json:"-"`
}
