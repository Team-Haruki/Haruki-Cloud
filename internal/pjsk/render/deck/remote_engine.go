package deck

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"haruki-cloud/utils/logger"
)

const (
	defaultMaxRetries            = 3
	defaultRetryWaitTime         = time.Second
	maxConsecutiveFailures int64 = 5
	circuitBreakerCooldown       = time.Minute
)

type remoteEngineProvider struct {
	cfg          RecommendConfig
	client       *http.Client
	mu           sync.Mutex
	recommenders map[string]DeckRecommender
}

func newRemoteEngineProvider(cfg RecommendConfig) engineProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 75 * time.Second
	}
	return &remoteEngineProvider{
		cfg:          cfg,
		client:       &http.Client{Timeout: timeout},
		recommenders: make(map[string]DeckRecommender),
	}
}

func (p *remoteEngineProvider) Get(region string) (DeckRecommender, error) {
	if p == nil {
		return nil, fmt.Errorf("deck remote engine provider is not initialized")
	}
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = "jp"
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if recommender, ok := p.recommenders[region]; ok && recommender != nil {
		return recommender, nil
	}

	masterdataDir := resolveDeckRemoteMasterdataDir(p.cfg.MasterdataDir)
	if masterdataDir == "" {
		return nil, fmt.Errorf("deck remote engine requires local masterdata dir")
	}

	algs := append([]string(nil), p.cfg.DefaultAlgs...)
	if len(algs) == 0 {
		algs = []string{"dfs", "sa", "ga"}
	}

	maxRetries := p.cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}
	retryWait := p.cfg.RetryWaitTime
	if retryWait <= 0 {
		retryWait = defaultRetryWaitTime
	}

	recommender := &RemoteDeckRecommender{
		baseURL:       strings.TrimRight(strings.TrimSpace(p.cfg.ServiceBaseURL), "/"),
		client:        p.client,
		defaultAlgs:   algs,
		masterdataDir: masterdataDir,
		region:        region,
		maxRetries:    maxRetries,
		retryWaitTime: retryWait,
		logger:        logger.NewLoggerFromGlobal("DeckRemote"),
	}
	p.recommenders[region] = recommender
	return recommender, nil
}

type RemoteDeckRecommender struct {
	baseURL       string
	client        *http.Client
	defaultAlgs   []string
	masterdataDir string
	region        string
	maxRetries    int
	retryWaitTime time.Duration
	logger        *logger.Logger

	mu              sync.Mutex
	masterdataReady bool
	musicMetaHash   string

	consecutiveFailures atomic.Int64
	lastFailureAtNanos  atomic.Int64

	now func() time.Time
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

func (r *RemoteDeckRecommender) Enabled() bool {
	return r != nil && r.client != nil && r.baseURL != ""
}

func (r *RemoteDeckRecommender) Close() {}

func (r *RemoteDeckRecommender) ExpandAlgorithms(option map[string]any) []map[string]any {
	if option == nil {
		return nil
	}
	alg, _ := option["algorithm"].(string)
	alg = strings.ToLower(strings.TrimSpace(alg))
	if alg != "all" {
		return []map[string]any{option}
	}
	result := make([]map[string]any, 0, len(r.defaultAlgs))
	for _, a := range r.defaultAlgs {
		copied := make(map[string]any, len(option))
		for k, v := range option {
			copied[k] = v
		}
		copied["algorithm"] = a
		result = append(result, copied)
	}
	return result
}
