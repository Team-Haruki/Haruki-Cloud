package music

import (
	"context"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	"haruki-cloud/internal/pjsk/render/snapshot"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

// ── Data source ─────────────────────────────────────────────────────────────

type DataSource interface {
	DefaultRegion() renderregion.Value
	SearchMusic(query string) (*masterdata.Music, error)
	GetMusicByID(id int) (*masterdata.Music, error)
	GetMusicByEventID(eventID int) (*masterdata.Music, error)
	GetMusics() []*masterdata.Music
	GetBanEvents(charID int) []*masterdata.Event
	GetMusicLocalizedTitles(musicID int) ([]string, error)
	GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error)
	GetMusicVocals(musicID int) ([]*masterdata.MusicVocal, error)
	GetMusicTags(musicID int) ([]string, error)
	GetCharacterByID(id int) (*masterdata.Character, error)
	GetOutsideCharacterByID(id int) (string, error)
	GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error)
	GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic
}

type contextualDataSource interface {
	WithContext(ctx context.Context) DataSource
}

type musicAliasResolver interface {
	TryResolveMusicID(ctx context.Context, token string) (int, bool, error)
	TryResolveMusicTitleOrAliasID(ctx context.Context, token string) (int, bool, error)
}

// ── Controller & builder ────────────────────────────────────────────────────

type Controller struct {
	sources               *regionsource.Registry[DataSource]
	drawing               *drawing.HarukiDrawingClient
	assets                *assets.AssetHelper
	banCharacterNicknames map[string]int
	aliases               musicAliasResolver
	snapshot              snapshot.Snapshot
	metaLoader            *meta.Loader
	requestCtx            context.Context
}

type Builder struct {
	source   DataSource
	fallback DataSource
	assets   *assets.AssetHelper
}

// ProviderAdapter bridges provider.MasterDataProvider to music.DataSource.
type ProviderAdapter struct {
	provider.PjskProviderAdapterBase
}

type SearchService struct {
	source        DataSource
	parser        *Parser
	titleResolver func(string) (*masterdata.Music, error)
}

// ── Parser ──────────────────────────────────────────────────────────────────

type QueryType int

const (
	QueryTypeUnknown QueryType = iota
	QueryTypeID
	QueryTypeSeq
	QueryTypeEvent
	QueryTypeBan
	QueryTypeTitle
	QueryTypeChart
)

type QueryInfo struct {
	Type               QueryType
	Value              int
	Diff               string
	Difficulty         string
	MusicID            int
	Keyword            string
	BanCharID          int
	BanSeq             int
	Original           string
	AllowTitleFallback bool
}

type Parser struct {
	banCharacterNicknames map[string]int
}

// ── Lookup result types ─────────────────────────────────────────────────────

type NoteCountMatch struct {
	Music          *masterdata.Music
	Difficulty     string
	PlayLevel      int
	TotalNoteCount int
}

type CoverResult struct {
	Music      *masterdata.Music
	JacketPath string
}

type BPMEvent struct {
	Bar      float64
	BPM      float64
	Duration float64
}

type BPMResult struct {
	Music      *masterdata.Music
	JacketPath string
	Difficulty string
	MainBPM    float64
	Events     []BPMEvent
	BarCount   int
	Duration   float64
}

type BPMMatch struct {
	Music      *masterdata.Music
	Difficulty string
	MainBPM    float64
	Events     []BPMEvent
}

type noteCountFinder interface {
	FindMusicDifficultiesByNoteCount(noteCount int) ([]*masterdata.MusicDifficulty, error)
}

// ── Query types ─────────────────────────────────────────────────────────────

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

type ListItemQuery struct {
	MusicID    int    `json:"music_id"`
	Difficulty string `json:"difficulty,omitempty"`
	Level      int    `json:"level,omitempty"`
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
	Items           []ListItemQuery                     `json:"items,omitempty"`
	Difficulty      string                              `json:"difficulty"`
	ResultFilter    string                              `json:"result_filter,omitempty"`
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
