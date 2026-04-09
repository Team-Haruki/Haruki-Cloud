package deck

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
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

	recommender := &RemoteDeckRecommender{
		baseURL:       strings.TrimRight(strings.TrimSpace(p.cfg.ServiceBaseURL), "/"),
		client:        p.client,
		defaultAlgs:   algs,
		masterdataDir: masterdataDir,
		region:        region,
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

	mu              sync.Mutex
	masterdataReady bool
	musicMetaHash   string
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

func (r *RemoteDeckRecommender) ExpandAlgorithms(option map[string]interface{}) []map[string]interface{} {
	if option == nil {
		return nil
	}
	alg, _ := option["algorithm"].(string)
	alg = strings.ToLower(strings.TrimSpace(alg))
	if alg != "all" {
		return []map[string]interface{}{option}
	}
	result := make([]map[string]interface{}, 0, len(r.defaultAlgs))
	for _, a := range r.defaultAlgs {
		copied := make(map[string]interface{}, len(option))
		for k, v := range option {
			copied[k] = v
		}
		copied["algorithm"] = a
		result = append(result, copied)
	}
	return result
}
