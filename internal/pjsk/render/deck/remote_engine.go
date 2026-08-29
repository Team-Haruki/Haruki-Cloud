package deck

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

func newRemoteEngineProvider(cfg RecommendConfig) engineProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 75 * time.Second
	}
	refreshInterval := cfg.MasterdataRefreshInterval
	if refreshInterval == 0 {
		refreshInterval = defaultMasterdataRefresh
	}
	targets := upstream.ResolveTargets(cfg.ServiceBaseURL, cfg.Targets, "deck-service")
	provider := &remoteEngineProvider{
		cfg:                       cfg,
		client:                    &http.Client{Timeout: timeout, Transport: upstream.NewTunedTransport(upstream.TunedTransportConfig{})},
		pool:                      upstream.NewPoolWithResources(targets, cfg.SharedResources),
		targets:                   targets,
		masterdataRefreshInterval: refreshInterval,
		recommenders:              make(map[string]PjskDeckRecommender),
	}
	provider.startMasterdataRefreshLoop()
	return provider
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
		algs = []string{"ga", "dfs_ga", "rl"}
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
		recommender.targetStates[remoteTargetKey(target)] = &remoteTargetState{
			target:          target,
			masterdataReady: true,
		}
	}
	recommender.captureMasterdataSignature()
	p.recommenders[region] = recommender
	return recommender, nil
}

func (r *RemoteDeckRecommender) Enabled() bool {
	return r != nil && r.client != nil && r.pool != nil && r.pool.Enabled() && len(r.targetStates) > 0
}

func (r *RemoteDeckRecommender) Close() {
	// HTTP clients and the shared upstream pool are owned by the composition root.
}

func (r *RemoteDeckRecommender) ExpandAlgorithms(option map[string]any) []map[string]any {
	if option == nil {
		return nil
	}
	target := optionString(option, "target")
	alg, _ := option["algorithm"].(string)
	if normalized := normalizeRecommendAlgorithmForTarget(alg, target); normalized != "" && normalized != alg {
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
	target := optionString(option, "target")

	filtered := make([]string, 0, len(r.defaultAlgs))
	seen := make(map[string]struct{}, len(r.defaultAlgs))
	for _, alg := range r.defaultAlgs {
		normalized := normalizeRecommendAlgorithmForTarget(alg, target)
		if normalized == "" || normalized == "all" {
			continue
		}
		if normalized == "dfs" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		filtered = append(filtered, normalized)
	}
	if len(filtered) == 0 {
		return normalizeRecommendAlgorithmsForService(r.defaultAlgs)
	}
	return filtered
}

func (r *RemoteDeckRecommender) acquireExecution(ctx context.Context) (*remoteExecution, error) {
	if r == nil || r.client == nil || r.pool == nil || !r.pool.Enabled() {
		return nil, fmt.Errorf("deck recommend service is not configured")
	}
	ctx = normalizeRecommendContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	finishQueue := commandtrace.MeasureOperation(ctx, "deck.queue")
	lease, err := r.pool.AcquireFunc(ctx, func(target upstream.TargetConfig) bool {
		return r.targetAcceptsAssignment(ctx, target)
	})
	finishQueue()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
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

func (r *RemoteDeckRecommender) targetAcceptsAssignment(ctx context.Context, target upstream.TargetConfig) bool {
	if err := normalizeRecommendContext(ctx).Err(); err != nil {
		return false
	}
	state := r.targetStates[remoteTargetKey(target)]
	if state == nil {
		return true
	}
	failures := state.consecutiveFailures.Load()
	if failures < targetAssignmentSkipFailures {
		return true
	}
	if r.tryResetCircuitBreakerAfterCooldown(state, failures) {
		return true
	}
	if r.shouldProbeTargetHealth(state) {
		if r.tryResetCircuitBreakerOnHealthyService(ctx, state, failures) {
			return true
		}
		if r.logger != nil {
			r.logger.WarnContext(ctx, "deck target health probe failed",
				"target", state.target.Name,
				"consecutive_failures", failures,
				"assignment_skipped", true,
			)
		}
		return false
	}
	if r.logger != nil {
		r.logger.DebugContext(ctx, "deck target assignment skipped",
			"target", state.target.Name,
			"consecutive_failures", failures,
		)
	}
	return false
}

func (r *RemoteDeckRecommender) shouldProbeTargetHealth(state *remoteTargetState) bool {
	if state == nil {
		return false
	}
	nowNanos := r.timeNow().UnixNano()
	for {
		lastNanos := state.lastHealthProbeAtNanos.Load()
		if lastNanos > 0 && time.Duration(nowNanos-lastNanos) < targetHealthProbeInterval {
			return false
		}
		if state.lastHealthProbeAtNanos.CompareAndSwap(lastNanos, nowNanos) {
			return true
		}
	}
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
