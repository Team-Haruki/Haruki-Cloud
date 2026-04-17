package deck

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"haruki-cloud/utils/logger"
)

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
