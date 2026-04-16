package drawing

// =========================== Music Models ===========================

type MusicMD struct {
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	Composer     string   `json:"composer"`
	Lyricist     string   `json:"lyricist"`
	Arranger     string   `json:"arranger"`
	MVInfo       []string `json:"mv_info,omitempty"`
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
	VocalInfo   map[string]any    `json:"vocal_info"`   // {"caption": str, "characters": [{"characterName": str}]}
	VocalAssets map[string]string `json:"vocal_assets"` // {"xxx": path}
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
	Alias                []string             `json:"alias"`
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
	TitleStyle           any              `json:"title_style,omitempty"`
	TitleShadow          bool             `json:"title_shadow,omitempty"`
}

// MusicListRequest represents request for /music/list
type MusicListRequest struct {
	UserResults           map[int]any                 `json:"user_results"` // key: musicId
	MusicList             []map[string]any            `json:"music_list"`   // [{"id": int, "difficulty": str}]
	JacketsPathList       map[int]string              `json:"jackets_path_list"`
	RequiredDifficulties  string                      `json:"required_difficulties"`
	Profile               *DetailedProfileCardRequest `json:"profile,omitempty"`
	PlayResultIconPathMap map[string]string           `json:"play_result_icon_path_map,omitempty"`
	Title                 *string                     `json:"title,omitempty"`
	TitleStyle            any                         `json:"title_style,omitempty"`
	TitleShadow           bool                        `json:"title_shadow,omitempty"`
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

// =========================== Chart Models ===========================

type GenerateMusicChartRequest struct {
	MusicID              any            `json:"music_id"` // str | int
	Title                string         `json:"title"`
	Artist               string         `json:"artist"`
	Difficulty           string         `json:"difficulty"`
	PlayLevel            any            `json:"play_level"` // str | int
	Skill                bool           `json:"skill"`
	JacketPath           string         `json:"jacket_path"`
	SusPath              string         `json:"sus_path"`
	StylePath            *string        `json:"style_path,omitempty"`
	NoteHost             string         `json:"note_host"`
	MusicMeta            map[string]any `json:"music_meta,omitempty"`
	TargetSegmentSeconds *float64       `json:"target_segment_seconds,omitempty"`
}
