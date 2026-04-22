package deck

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/utils/logger"
)

func newRemoteEngineProvider(cfg RecommendConfig) engineProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 75 * time.Second
	}
	targets := upstream.ResolveTargets(cfg.ServiceBaseURL, cfg.Targets, "deck-service")
	return &remoteEngineProvider{
		cfg:          cfg,
		client:       &http.Client{Timeout: timeout},
		pool:         upstream.NewPoolWithResources(targets, cfg.SharedResources),
		targets:      targets,
		recommenders: make(map[string]PjskDeckRecommender),
	}
}

func (p *remoteEngineProvider) Get(region string) (PjskDeckRecommender, error) {
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
	if p.pool == nil || !p.pool.Enabled() || len(p.targets) == 0 {
		return nil, fmt.Errorf("deck recommend service is not configured")
	}

	algs := normalizeRecommendAlgorithmsForService(p.cfg.DefaultAlgs)
	if len(algs) == 0 {
		algs = []string{"dfs", "ga", "dfs_ga", "rl"}
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
		client:        p.client,
		pool:          p.pool,
		targetStates:  make(map[string]*remoteTargetState, len(p.targets)),
		defaultAlgs:   algs,
		masterdataDir: masterdataDir,
		region:        region,
		maxRetries:    maxRetries,
		retryWaitTime: retryWait,
		logger:        logger.NewLoggerFromGlobal("DeckRemote"),
	}
	for _, target := range p.targets {
		recommender.targetStates[remoteTargetKey(target)] = &remoteTargetState{target: target}
	}
	p.recommenders[region] = recommender
	return recommender, nil
}

func (r *RemoteDeckRecommender) Enabled() bool {
	return r != nil && r.client != nil && r.pool != nil && r.pool.Enabled() && len(r.targetStates) > 0
}

func (r *RemoteDeckRecommender) Close() {}

func (r *RemoteDeckRecommender) ExpandAlgorithms(option map[string]any) []map[string]any {
	if option == nil {
		return nil
	}
	alg, _ := option["algorithm"].(string)
	if normalized := normalizeRecommendAlgorithmForService(alg); normalized != "" && normalized != alg {
		option = cloneRecommendOption(option)
		option["algorithm"] = normalized
		alg = normalized
	} else {
		alg = strings.ToLower(strings.TrimSpace(alg))
	}
	if alg != "all" {
		return []map[string]any{sanitizeLocalRecommendOption(option)}
	}
	selected := r.defaultAlgorithmsForOption(option)
	if subset := selectRecommendAlgorithmSubset(option, selected); len(subset) > 0 {
		selected = subset
	}
	baseOption := sanitizeLocalRecommendOption(option)
	result := make([]map[string]any, 0, len(selected))
	for _, a := range selected {
		copied := cloneRecommendOption(baseOption)
		copied["algorithm"] = a
		result = append(result, copied)
	}
	return result
}

func (r *RemoteDeckRecommender) defaultAlgorithmsForOption(option map[string]any) []string {
	if len(r.defaultAlgs) == 0 {
		return nil
	}
	if optionString(option, "target") == "skill" {
		return r.defaultAlgs
	}

	filtered := make([]string, 0, len(r.defaultAlgs))
	for _, alg := range r.defaultAlgs {
		if normalizeRecommendAlgorithmForService(alg) == "dfs" {
			continue
		}
		filtered = append(filtered, alg)
	}
	if len(filtered) == 0 {
		return r.defaultAlgs
	}
	return filtered
}

func (r *RemoteDeckRecommender) acquireExecution() (*remoteExecution, error) {
	if r == nil || r.client == nil || r.pool == nil || !r.pool.Enabled() {
		return nil, fmt.Errorf("deck recommend service is not configured")
	}

	lease, err := r.pool.Acquire(nil)
	if err != nil {
		return nil, fmt.Errorf("deck-service upstream is unavailable: %w", err)
	}
	state := r.targetStates[remoteTargetKey(lease.Target)]
	if state == nil {
		lease.Release()
		return nil, fmt.Errorf("deck-service target state is not initialized: %s", lease.Target.Name)
	}
	return &remoteExecution{
		lease: lease,
		state: state,
	}, nil
}

func sanitizeLocalRecommendOption(option map[string]any) map[string]any {
	if option == nil {
		return nil
	}
	if _, ok := option[recommendAlgorithmSubsetKey]; !ok {
		return option
	}
	copied := cloneRecommendOption(option)
	delete(copied, recommendAlgorithmSubsetKey)
	return copied
}
