package deck

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/snapshot"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

// ── Data sources ────────────────────────────────────────────────────────────

type CardSource interface {
	DefaultRegion() renderregion.Value
	GetCardByID(id int) (*masterdata.Card, error)
}

type EventSource interface {
	DefaultRegion() renderregion.Value
	GetEventByID(id int) (*masterdata.Event, error)
	GetEvents() []*masterdata.Event
}

type MusicSource interface {
	DefaultRegion() renderregion.Value
	GetMusicByID(id int) (*masterdata.Music, error)
}

type contextualCardSource interface {
	WithContext(ctx context.Context) rendercard.DataSource
}

type contextualEventSource interface {
	WithContext(ctx context.Context) renderevent.DataSource
}

type contextualMusicSource interface {
	WithContext(ctx context.Context) rendermusic.DataSource
}

type allCardSource interface {
	GetAllCards() ([]*masterdata.Card, error)
}

type areaItemLevelCapSource interface {
	AreaItemLevelCaps(limit int) map[int]int
}

type cardUnitSource interface {
	GetUnitByCardID(cardID int) (string, error)
}

type cardEpisodeSource interface {
	GetCardEpisodes(cardID int) ([]snapshot.RawUserCardEpisode, error)
}

type characterSource interface {
	GetCharacterByID(id int) (*masterdata.Character, error)
}

type maxProfileMySekaiSource interface {
	GetMaxProfileMysekaiGates() []snapshot.RawUserMysekaiGate
	GetMaxProfileMysekaiFixtureBonuses() []snapshot.RawUserFixtureBonus
}

type eventRankingHonorSource interface {
	GetEventRankingHonorRewards(eventID int) ([]masterdata.EventRankingHonorReward, error)
}

// ── Controller ──────────────────────────────────────────────────────────────

type Controller struct {
	ctx           context.Context
	cardSources   *regionsource.Registry[CardSource]
	eventSources  *regionsource.Registry[EventSource]
	musicSources  *regionsource.Registry[MusicSource]
	drawing       *drawing.HarukiDrawingClient
	assets        *assets.AssetHelper
	snapshot      snapshot.Snapshot
	defaultRegion renderregion.Value
	recommendCfg  RecommendConfig
	metaLoader    MusicMetaSource
	engine        engineProvider
}

// ── Recommender ─────────────────────────────────────────────────────────────

type RecommendConfig struct {
	Enabled                   bool
	Disable                   bool
	DisableReason             string
	ServiceBaseURL            string
	Targets                   []upstream.TargetConfig
	SharedResources           *upstream.SharedResources
	MasterdataDir             string
	MasterdataRefreshInterval time.Duration
	Timeout                   time.Duration
	MaxRetries                int
	RetryWaitTime             time.Duration
	DefaultAlgs               []string
}

type MusicMetaSource interface {
	Get(region string) []byte
}

type PjskDeckRecommender interface {
	Enabled() bool
	ExpandAlgorithms(option map[string]any) []map[string]any
	Recommend(req RecommendRequest) (*RecommendResult, error)
	Close()
}

type engineProvider interface {
	Get(region string) (PjskDeckRecommender, error)
}

type RecommendRequest struct {
	Region            string
	RecommendType     string
	UserData          []byte
	UserDataFilePath  string
	MusicMeta         []byte
	MusicMetaFilePath string
	BatchOption       []map[string]any
}

type RecommendResult struct {
	Decks     []RecommendDeck `json:"decks"`
	CostTimes map[string]float64
	WaitTimes map[string]float64
	DeckAlgs  []string
}

type RecommendDeck struct {
	Cards                []RecommendCard `json:"cards"`
	Score                int             `json:"score"`
	LiveScore            int             `json:"live_score"`
	MysekaiEventPoint    int             `json:"mysekai_event_point"`
	TotalPower           int             `json:"total_power"`
	EventBonusRate       float64         `json:"event_bonus_rate"`
	SupportDeckBonusRate float64         `json:"support_deck_bonus_rate"`
	MultiLiveScoreUp     float64         `json:"multi_live_score_up"`
	ChallengeScoreDelta  int             `json:"challenge_score_delta"`
	Algs                 []string        `json:"-"`
}

type RecommendCard struct {
	CardID          int     `json:"card_id"`
	Level           int     `json:"level"`
	MasterRank      int     `json:"master_rank"`
	DefaultImage    string  `json:"default_image"`
	SkillLevel      int     `json:"skill_level"`
	SkillRate       float64 `json:"skill_rate"`
	EventBonusRate  float64 `json:"event_bonus_rate"`
	IsBeforeStory   bool    `json:"is_before_story"`
	IsAfterStory    bool    `json:"is_after_story"`
	IsAfterTraining bool    `json:"is_after_training"`
	HasCanvasBonus  bool    `json:"has_canvas_bonus"`
}

// ── Remote engine ───────────────────────────────────────────────────────────

const (
	defaultMaxRetries                  = 3
	defaultRetryWaitTime               = time.Second
	defaultMasterdataRefresh           = 5 * time.Minute
	maxConsecutiveFailures       int64 = 5
	targetAssignmentSkipFailures       = 1
	targetHealthProbeInterval          = 10 * time.Second
	circuitBreakerCooldown             = time.Minute
)

type remoteEngineProvider struct {
	cfg                       RecommendConfig
	client                    *http.Client
	pool                      *upstream.Pool
	targets                   []upstream.TargetConfig
	masterdataRefreshInterval time.Duration
	mu                        sync.Mutex
	recommenders              map[string]PjskDeckRecommender
}

type RemoteDeckRecommender struct {
	client        *http.Client
	pool          *upstream.Pool
	targetStates  map[string]*remoteTargetState
	defaultAlgs   []string
	masterdataDir string
	masterdataMu  sync.Mutex
	masterdataSig string
	region        string
	maxRetries    int
	retryWaitTime time.Duration
	logger        *logger.Logger

	now func() time.Time
}

type remoteTargetState struct {
	target upstream.TargetConfig

	mu              sync.Mutex
	masterdataReady bool
	musicMetaHash   string
	readyGroup      singleflight.Group

	consecutiveFailures    atomic.Int64
	lastFailureAtNanos     atomic.Int64
	lastHealthProbeAtNanos atomic.Int64
}

type remoteExecution struct {
	lease *upstream.Lease
	state *remoteTargetState
}

func (e *remoteExecution) Release() {
	if e == nil || e.lease == nil {
		return
	}
	e.lease.Release()
	e.lease = nil
}

func (e *remoteExecution) BaseURL() string {
	if e == nil || e.state == nil {
		return ""
	}
	return e.state.target.BaseURL
}

func remoteTargetKey(target upstream.TargetConfig) string {
	return target.Name + "\x00" + target.BaseURL
}

type remoteRecommendResult struct {
	Decks []remoteRecommendDeck `json:"decks"`
}

type remoteBatchRecommendResult struct {
	Alg      string                 `json:"alg,omitempty"`
	CostTime float64                `json:"cost_time,omitempty"`
	WaitTime float64                `json:"wait_time,omitempty"`
	Result   *remoteRecommendResult `json:"result,omitempty"`
	Decks    []remoteRecommendDeck  `json:"decks,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

type remoteRecommendDeck struct {
	Score                int                   `json:"score"`
	LiveScore            int                   `json:"live_score"`
	MysekaiEventPoint    int                   `json:"mysekai_event_point"`
	TotalPower           int                   `json:"total_power"`
	EventBonusRate       float64               `json:"event_bonus_rate"`
	SupportDeckBonusRate float64               `json:"support_deck_bonus_rate"`
	MultiLiveScoreUp     float64               `json:"multi_live_score_up"`
	Cards                []remoteRecommendCard `json:"cards"`
}

type remoteRecommendCard struct {
	CardID         int     `json:"card_id"`
	Level          int     `json:"level"`
	MasterRank     int     `json:"master_rank"`
	SkillLevel     int     `json:"skill_level"`
	SkillScoreUp   float64 `json:"skill_score_up"`
	EventBonusRate float64 `json:"event_bonus_rate"`
	Episode1Read   bool    `json:"episode1_read"`
	Episode2Read   bool    `json:"episode2_read"`
	AfterTraining  bool    `json:"after_training"`
	DefaultImage   string  `json:"default_image"`
	HasCanvasBonus bool    `json:"has_canvas_bonus"`
}

type remoteErrorResponse struct {
	Error string `json:"error"`
}

type remoteUserDataCacheResponse struct {
	UserdataHash string `json:"userdata_hash"`
}

// ── Query types ─────────────────────────────────────────────────────────────

type MusicCompareSelection struct {
	MusicID        int    `json:"music_id"`
	MusicDiff      string `json:"music_diff,omitempty"`
	MusicTitle     string `json:"music_title,omitempty"`
	MusicCoverPath string `json:"music_cover_path,omitempty"`
	MusicQuery     string `json:"music_query,omitempty"`
}

type CardConfigPatch struct {
	Disable     bool `json:"disable,omitempty"`
	LevelMax    bool `json:"level_max,omitempty"`
	EpisodeRead bool `json:"episode_read,omitempty"`
	MasterMax   bool `json:"master_max,omitempty"`
	SkillMax    bool `json:"skill_max,omitempty"`
	Canvas      bool `json:"canvas,omitempty"`
}

type SingleCardConfigPatch struct {
	CardID      int  `json:"card_id"`
	Disable     bool `json:"disable,omitempty"`
	LevelMax    bool `json:"level_max,omitempty"`
	EpisodeRead bool `json:"episode_read,omitempty"`
	MasterMax   bool `json:"master_max,omitempty"`
	SkillMax    bool `json:"skill_max,omitempty"`
	Canvas      bool `json:"canvas,omitempty"`
}

type AutoQuery struct {
	Region                        string                              `json:"region"`
	RecommendType                 string                              `json:"recommend_type"`
	EventID                       *int                                `json:"event_id,omitempty"`
	Limit                         int                                 `json:"limit,omitempty"`
	TargetBonuses                 []int                               `json:"target_bonuses,omitempty"`
	Boost                         *int                                `json:"boost,omitempty"`
	AreaItemLevel                 *int                                `json:"area_item_level,omitempty"`
	UnitFilter                    string                              `json:"unit_filter,omitempty"`
	AttrFilter                    string                              `json:"attr_filter,omitempty"`
	ExcludedCards                 []int                               `json:"excluded_cards,omitempty"`
	UseCurrentDeck                bool                                `json:"use_current_deck,omitempty"`
	UseExactCardState             bool                                `json:"use_exact_card_state,omitempty"`
	MaxProfile                    bool                                `json:"max_profile,omitempty"`
	SubMaxProfile                 bool                                `json:"sub_max_profile,omitempty"`
	SupportMasterMax              bool                                `json:"support_master_max,omitempty"`
	SupportSkillMax               bool                                `json:"support_skill_max,omitempty"`
	MusicCompare                  bool                                `json:"music_compare,omitempty"`
	MusicCompareQueries           []string                            `json:"music_compare_queries,omitempty"`
	MusicCompareSelections        []MusicCompareSelection             `json:"music_compare_selections,omitempty"`
	SpecificSkillOrder            []int                               `json:"specific_skill_order,omitempty"`
	Args                          string                              `json:"args,omitempty"`
	Algorithm                     string                              `json:"algorithm,omitempty"`
	LiveType                      string                              `json:"live_type,omitempty"`
	Target                        string                              `json:"target,omitempty"`
	MusicQuery                    string                              `json:"music_query,omitempty"`
	MusicID                       *int                                `json:"music_id,omitempty"`
	MusicDiff                     string                              `json:"music_diff,omitempty"`
	MusicTitle                    string                              `json:"music_title,omitempty"`
	MusicCoverPath                string                              `json:"music_cover_path,omitempty"`
	EventAttr                     string                              `json:"event_attr,omitempty"`
	EventUnit                     string                              `json:"event_unit,omitempty"`
	WorldBloomCharacterID         *int                                `json:"world_bloom_character_id,omitempty"`
	WorldBloomCharacterQuery      string                              `json:"world_bloom_character_query,omitempty"`
	WorldBloomEventTurn           *int                                `json:"world_bloom_event_turn,omitempty"`
	MetadataWorldBloomCharacterID *int                                `json:"-"`
	MetadataWorldBloomEventTurn   *int                                `json:"-"`
	ForcedLeaderCharacterID       *int                                `json:"forced_leader_character_id,omitempty"`
	ForcedLeaderCharacterQuery    string                              `json:"forced_leader_character_query,omitempty"`
	ChallengeLiveCharacterID      *int                                `json:"challenge_live_character_id,omitempty"`
	ChallengeLiveCharacterQuery   string                              `json:"challenge_live_character_query,omitempty"`
	FixedCards                    []int                               `json:"fixed_cards,omitempty"`
	FixedCharacters               []int                               `json:"fixed_characters,omitempty"`
	FixedCharacterQueries         []string                            `json:"fixed_character_queries,omitempty"`
	Rarity1Config                 *CardConfigPatch                    `json:"rarity_1_config,omitempty"`
	Rarity2Config                 *CardConfigPatch                    `json:"rarity_2_config,omitempty"`
	Rarity3Config                 *CardConfigPatch                    `json:"rarity_3_config,omitempty"`
	Rarity4Config                 *CardConfigPatch                    `json:"rarity_4_config,omitempty"`
	RarityBirthdayConfig          *CardConfigPatch                    `json:"rarity_birthday_config,omitempty"`
	SingleCardConfigs             []SingleCardConfigPatch             `json:"single_card_configs,omitempty"`
	MultiLiveTeammatePower        *int                                `json:"multi_live_teammate_power,omitempty"`
	MultiLiveTeammateScoreUp      *int                                `json:"multi_live_teammate_score_up,omitempty"`
	MultiLiveScoreUpLowerBound    *float64                            `json:"multi_live_score_up_lower_bound,omitempty"`
	SkillOrderChooseStrategy      string                              `json:"skill_order_choose_strategy,omitempty"`
	SkillReferenceChooseStrategy  string                              `json:"skill_reference_choose_strategy,omitempty"`
	KeepAfterTrainingState        bool                                `json:"keep_after_training_state,omitempty"`
	Profile                       *drawing.DetailedProfileCardRequest `json:"-"`
	PublicProfileResp             *sekai.GetAnotherProfileResponse    `json:"-"`
}

// ── Compare ─────────────────────────────────────────────────────────────────

const challengeCharacterCount = 26

const (
	musicCompareDefaultShowNum = 8
	musicCompareCandidateNum   = 40
)

type musicCompareResultItem struct {
	selection MusicCompareSelection
	deck      RecommendDeck
	alg       string
}
