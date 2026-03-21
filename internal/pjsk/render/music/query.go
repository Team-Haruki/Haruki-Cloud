package music

type Query struct {
	Query      string `json:"query"`
	Region     string `json:"region"`
	Difficulty string `json:"difficulty,omitempty"`
	UserID     string `json:"user_id,omitempty"`
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
	Difficulty   string                 `json:"difficulty"`
	Level        int                    `json:"level,omitempty"`
	LevelMin     int                    `json:"level_min,omitempty"`
	LevelMax     int                    `json:"level_max,omitempty"`
	Region       string                 `json:"region"`
	IncludeLeaks bool                   `json:"include_leaks,omitempty"`
	UserResults  map[int]string         `json:"user_results,omitempty"`
	Title        *string                `json:"title,omitempty"`
	TitleStyle   map[string]interface{} `json:"title_style,omitempty"`
	TitleShadow  bool                   `json:"title_shadow,omitempty"`
	Keyword      string                 `json:"keyword,omitempty"`
	ShowID       bool                   `json:"show_id,omitempty"`
}
