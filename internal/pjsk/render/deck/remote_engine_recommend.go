package deck

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

type remoteReadyFlightToken byte

type remoteReadyFlightResult struct {
	err        error
	operations []commandtrace.Stats
	leader     *remoteReadyFlightToken
}

func (r *RemoteDeckRecommender) Recommend(req RecommendRequest) (*RecommendResult, error) {
	return r.RecommendContext(context.Background(), req)
}

func (r *RemoteDeckRecommender) RecommendContext(ctx context.Context, req RecommendRequest) (*RecommendResult, error) {
	ctx = normalizeRecommendContext(ctx)
	results, err := r.RecommendBatchContext(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return aggregateRemoteRecommendResults(req.RecommendType, req.BatchOption, results)
}

func (r *RemoteDeckRecommender) RecommendBatch(req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	return r.RecommendBatchContext(context.Background(), req)
}

func (r *RemoteDeckRecommender) RecommendBatchContext(ctx context.Context, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	ctx = normalizeRecommendContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateRemoteRecommendRequest(req); err != nil {
		return nil, err
	}
	exec, err := r.acquireExecution(ctx)
	if err != nil {
		return nil, err
	}
	defer exec.Release()

	if err := r.ensureCircuitClosed(ctx, exec.state); err != nil {
		return nil, err
	}
	if err := r.ensureReady(ctx, exec, req.Region, req.MusicMeta, req.MusicMetaFilePath); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		r.recordFailure(exec.state)
		return nil, err
	}
	return r.recommendWithRewarm(ctx, exec, req)
}

func (r *RemoteDeckRecommender) ensureCircuitClosed(ctx context.Context, state *remoteTargetState) error {
	failures := state.consecutiveFailures.Load()
	if failures < maxConsecutiveFailures {
		return nil
	}
	if r.tryResetCircuitBreakerAfterCooldown(state, failures) {
		if r.logger != nil {
			r.logger.InfoContext(ctx, deckCircuitBreakerResetLog, "reason", "cooldown")
		}
		return nil
	}
	if r.tryResetCircuitBreakerOnHealthyService(ctx, state, failures) {
		if r.logger != nil {
			r.logger.InfoContext(ctx, deckCircuitBreakerResetLog, "reason", "health_probe")
		}
		return nil
	}
	if r.logger != nil {
		r.logger.WarnContext(ctx, "deck circuit breaker open", "consecutive_failures", failures)
	}
	return fmt.Errorf("deck-service unavailable: %d consecutive failures (circuit breaker open)", failures)
}

func (r *RemoteDeckRecommender) recommendWithRewarm(ctx context.Context, exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	start := time.Now()
	results, err := r.recommendBatchOnce(ctx, exec, req)
	elapsed := time.Since(start)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err == nil {
		err = firstRemoteBatchError(results)
	}
	if err == nil {
		r.recordSuccess(exec.state)
		r.logger.DebugContext(ctx, "deck recommendation completed", "duration_ms", commandtrace.Milliseconds(elapsed))
		return results, nil
	}
	rewarmKind := classifyRemoteRewarm(err)
	if rewarmKind != remoteRewarmNone {
		return r.rewarmAndRetry(ctx, exec, req, err, rewarmKind)
	}
	r.recordRecommendError(ctx, exec.state, err, elapsed)
	return nil, err
}

func (r *RemoteDeckRecommender) recordRecommendError(ctx context.Context, state *remoteTargetState, err error, elapsed time.Duration) {
	if shouldCountCircuitBreakerFailure(err) {
		r.recordFailure(state)
		r.logger.WarnContext(ctx, "deck recommendation failed",
			"duration_ms", commandtrace.Milliseconds(elapsed),
			"error_type", fmt.Sprintf("%T", err),
		)
		return
	}
	r.recordSuccess(state)
	r.logger.InfoContext(ctx, "deck recommendation returned logical error",
		"duration_ms", commandtrace.Milliseconds(elapsed),
		"error_type", fmt.Sprintf("%T", err),
	)
}

func (r *RemoteDeckRecommender) rewarmAndRetry(ctx context.Context, exec *remoteExecution, req RecommendRequest, recommendErr error, rewarmKind remoteRewarmKind) ([]remoteBatchRecommendResult, error) {
	r.logger.InfoContext(ctx, "deck service rewarming", "error_type", fmt.Sprintf("%T", recommendErr))
	r.invalidate(exec.state, rewarmKind)
	if warmErr := r.ensureReady(ctx, exec, req.Region, req.MusicMeta, req.MusicMetaFilePath); warmErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		r.recordFailure(exec.state)
		return nil, warmErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	results, err := r.recommendBatchOnce(ctx, exec, req)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err == nil {
		err = firstRemoteBatchError(results)
	}
	if err != nil {
		r.recordFailure(exec.state)
		r.logger.WarnContext(ctx, "deck recommendation failed after rewarm", "error_type", fmt.Sprintf("%T", err))
		return nil, err
	}
	r.recordSuccess(exec.state)
	r.logger.DebugContext(ctx, "deck recommendation completed", "after_rewarm", true)
	return results, nil
}

func validateRemoteRecommendRequest(req RecommendRequest) error {
	if len(req.BatchOption) == 0 {
		return fmt.Errorf("deck remote engine requires batch_options")
	}
	if len(req.UserData) == 0 && strings.TrimSpace(req.UserDataFilePath) == "" {
		return fmt.Errorf("deck remote engine requires user_data bytes or file path")
	}
	if len(req.MusicMeta) == 0 && strings.TrimSpace(req.MusicMetaFilePath) == "" {
		return fmt.Errorf("deck remote engine requires music meta bytes or file path")
	}
	return nil
}

func (r *RemoteDeckRecommender) recommendBatchOnce(ctx context.Context, exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	return r.doRecommendCompatible(ctx, exec, req)
}

func firstRemoteBatchError(results []remoteBatchRecommendResult) error {
	if len(results) == 0 {
		return nil
	}

	var firstErr error
	for _, item := range results {
		if item.Result != nil && len(item.Result.Decks) > 0 {
			return nil
		}
		if len(item.Decks) > 0 {
			return nil
		}
		if firstErr == nil && strings.TrimSpace(item.Error) != "" {
			firstErr = fmt.Errorf("%s", strings.TrimSpace(item.Error))
		}
	}
	return firstErr
}

func (r *RemoteDeckRecommender) recordSuccess(state *remoteTargetState) {
	state.consecutiveFailures.Store(0)
	state.lastFailureAtNanos.Store(0)
	state.lastHealthProbeAtNanos.Store(0)
}

func (r *RemoteDeckRecommender) recordFailure(state *remoteTargetState) {
	now := r.timeNow().UnixNano()
	state.consecutiveFailures.Add(1)
	state.lastFailureAtNanos.Store(now)
	state.lastHealthProbeAtNanos.Store(now)
}

func (r *RemoteDeckRecommender) resetCircuitBreaker(state *remoteTargetState) {
	state.consecutiveFailures.Store(0)
	state.lastFailureAtNanos.Store(0)
	state.lastHealthProbeAtNanos.Store(0)
	if r.logger != nil {
		r.logger.Info(deckCircuitBreakerResetLog)
	}
}

func (r *RemoteDeckRecommender) tryResetCircuitBreakerAfterCooldown(state *remoteTargetState, failures int64) bool {
	lastNanos := state.lastFailureAtNanos.Load()
	if lastNanos <= 0 {
		return false
	}
	elapsed := r.timeNow().Sub(time.Unix(0, lastNanos))
	if elapsed < circuitBreakerCooldown {
		return false
	}
	if r.logger != nil {
		r.logger.Info("deck circuit breaker cooldown elapsed",
			"duration_ms", commandtrace.Milliseconds(elapsed),
			"consecutive_failures", failures,
		)
	}
	r.resetCircuitBreaker(state)
	return true
}

func (r *RemoteDeckRecommender) tryResetCircuitBreakerOnHealthyService(ctx context.Context, state *remoteTargetState, failures int64) bool {
	if !r.healthCheck(ctx, state.target.BaseURL) {
		return false
	}
	if r.logger != nil {
		r.logger.InfoContext(ctx, "deck circuit breaker health probe succeeded", "consecutive_failures", failures)
	}
	r.resetCircuitBreaker(state)
	return true
}

func (r *RemoteDeckRecommender) timeNow() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}

func shouldCountCircuitBreakerFailure(err error) bool {
	if err == nil {
		return false
	}
	if shouldRewarmRemoteService(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "deck-service returned http 5") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "eof")
}

func (r *RemoteDeckRecommender) doRecommendCompatible(ctx context.Context, exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	results, err := r.doRecommendBatch(ctx, exec, req)
	if err == nil {
		return results, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if !isUnsupportedBatchProtocolError(err) && !isMissingUserdataHashError(err) {
		return nil, err
	}
	return r.doRecommendLegacy(ctx, exec, req)
}

func (r *RemoteDeckRecommender) doRecommendBatch(ctx context.Context, exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	userData := req.UserData
	if len(userData) == 0 {
		return nil, fmt.Errorf("deck remote engine: no user data bytes available")
	}

	var cacheResp remoteUserDataCacheResponse
	userPayload := buildMultipartPayload(ctx, userData)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.postBinary(ctx, exec, "/cache_userdata", userPayload, &cacheResp); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cacheResp.UserdataHash) == "" {
		return nil, fmt.Errorf("deck-service cache_userdata returned empty userdata_hash")
	}

	recommendPayload := map[string]any{
		"region":        strings.ToLower(strings.TrimSpace(req.Region)),
		"batch_options": req.BatchOption,
		"userdata_hash": cacheResp.UserdataHash,
	}
	finishEncode := commandtrace.MeasureOperation(ctx, "deck.encode")
	recommendJSON, err := json.Marshal(recommendPayload)
	finishEncode()
	if err != nil {
		return nil, err
	}

	var raw json.RawMessage
	recommendPayloadBytes := buildMultipartPayload(ctx, recommendJSON)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.postBinary(ctx, exec, "/recommend", recommendPayloadBytes, &raw); err != nil {
		return nil, err
	}
	return parseRemoteRecommendBatch(raw, req.BatchOption)
}

func (r *RemoteDeckRecommender) doRecommendLegacy(ctx context.Context, exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	ctx = normalizeRecommendContext(ctx)
	group, groupCtx := errgroup.WithContext(ctx)
	partials := make([]remoteLegacyPartial, len(req.BatchOption))
	var launchErr error
	for index, option := range req.BatchOption {
		if err := groupCtx.Err(); err != nil {
			launchErr = err
			break
		}
		index := index
		opt := cloneRecommendOption(option)
		group.Go(func() error {
			partials[index] = r.recommendLegacyPartial(groupCtx, exec, req, opt, index)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if launchErr != nil {
		return nil, launchErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	out := make([]remoteBatchRecommendResult, len(req.BatchOption))
	var firstErr error
	successCount := 0
	for _, item := range partials {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
		} else {
			successCount++
		}
		out[item.index] = item.item
	}
	if successCount == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

type remoteLegacyPartial struct {
	index int
	item  remoteBatchRecommendResult
	err   error
}

func (r *RemoteDeckRecommender) recommendLegacyPartial(ctx context.Context, exec *remoteExecution, req RecommendRequest, option map[string]any, index int) remoteLegacyPartial {
	alg, _ := option["algorithm"].(string)
	start := time.Now()
	decks, err := r.doRecommendLegacyOption(ctx, exec, req, option)
	if err != nil {
		return remoteLegacyPartial{
			index: index, err: err,
			item: remoteBatchRecommendResult{Alg: alg, Error: err.Error()},
		}
	}
	return remoteLegacyPartial{
		index: index,
		item: remoteBatchRecommendResult{
			Alg: alg, CostTime: time.Since(start).Seconds(),
			Result: &remoteRecommendResult{Decks: decks},
		},
	}
}

func (r *RemoteDeckRecommender) doRecommendLegacyOption(ctx context.Context, exec *remoteExecution, req RecommendRequest, option map[string]any) ([]remoteRecommendDeck, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	payload := cloneRecommendOption(option)
	payload["region"] = strings.ToLower(strings.TrimSpace(req.Region))
	if len(req.UserData) > 0 {
		payload["user_data_str"] = string(req.UserData)
	} else if path := strings.TrimSpace(req.UserDataFilePath); path != "" {
		payload["user_data_file_path"] = path
	} else {
		return nil, fmt.Errorf("deck remote engine: no user data available")
	}

	var response remoteRecommendResult
	if err := r.postJSON(ctx, exec, "/recommend", payload, &response); err != nil {
		return nil, err
	}
	return response.Decks, nil
}

func (r *RemoteDeckRecommender) ensureReady(ctx context.Context, exec *remoteExecution, region string, musicMeta []byte, musicMetaPath string) error {
	ctx = normalizeRecommendContext(ctx)
	region, musicMetaPath, hash, err := r.normalizeReadyInputs(ctx, region, musicMeta, musicMetaPath)
	if err != nil {
		return err
	}
	state := exec.state
	if remoteStateReady(state, hash) {
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		callerToken := new(remoteReadyFlightToken)
		result := state.readyGroup.DoChan("ready", func() (any, error) {
			return r.runReadyFlight(state, region, hash, musicMeta, musicMetaPath, callerToken), nil
		})
		retry, err := waitForReadyFlight(ctx, result, callerToken)
		if err != nil {
			return err
		}
		if retry {
			continue
		}
		if remoteStateReady(state, hash) {
			return nil
		}
	}
}

func (r *RemoteDeckRecommender) normalizeReadyInputs(ctx context.Context, region string, musicMeta []byte, musicMetaPath string) (string, string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", "", err
	}
	musicMetaPath = strings.TrimSpace(musicMetaPath)
	hash := hashPayload(musicMeta)
	if err := ctx.Err(); err != nil {
		return "", "", "", err
	}
	if hash == "" && musicMetaPath != "" {
		hash = "path:" + musicMetaPath
	}
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = r.region
	}
	return region, musicMetaPath, hash, nil
}

func remoteStateReady(state *remoteTargetState, hash string) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	musicReady := hash != "" && hash == state.musicMetaHash
	return state.masterdataReady && (hash == "" || musicReady)
}

func (r *RemoteDeckRecommender) runReadyFlight(state *remoteTargetState, region, hash string, musicMeta []byte, musicMetaPath string, leader *remoteReadyFlightToken) remoteReadyFlightResult {
	detached := logger.WithContextAttrs(context.Background(), slog.Bool("shared_work", true))
	sharedBase, cancel := context.WithTimeout(detached, r.readySharedTimeout())
	defer cancel()
	sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
	workErr := r.updateRemoteReadyState(sharedCtx, &remoteExecution{state: state}, region, hash, musicMeta, musicMetaPath)
	return remoteReadyFlightResult{
		err:        workErr,
		operations: trace.Snapshot().Operations,
		leader:     leader,
	}
}

func (r *RemoteDeckRecommender) updateRemoteReadyState(ctx context.Context, exec *remoteExecution, region, hash string, musicMeta []byte, musicMetaPath string) error {
	state := exec.state
	if remoteStateReady(state, hash) {
		return nil
	}
	state.mu.Lock()
	masterReady := state.masterdataReady
	musicReady := hash != "" && hash == state.musicMetaHash
	state.mu.Unlock()
	if !masterReady {
		req := map[string]any{"base_dir": r.masterdataDir, "region": region}
		if err := r.postJSON(ctx, exec, "/update/masterdata", req, nil); err != nil {
			return fmt.Errorf("deck remote engine: update masterdata: %w", err)
		}
	}
	if hash != "" && !musicReady {
		if err := r.updateRemoteMusicMeta(ctx, exec, region, musicMeta, musicMetaPath); err != nil {
			return err
		}
	}
	state.mu.Lock()
	state.masterdataReady = true
	if hash != "" {
		state.musicMetaHash = hash
	}
	state.mu.Unlock()
	return nil
}

func (r *RemoteDeckRecommender) updateRemoteMusicMeta(ctx context.Context, exec *remoteExecution, region string, musicMeta []byte, musicMetaPath string) error {
	req := map[string]any{"region": region}
	path := "/update/musicmetas"
	if len(musicMeta) > 0 {
		req["data"] = string(musicMeta)
		path = "/update/musicmetas/string"
	} else {
		req["file_path"] = musicMetaPath
	}
	if err := r.postJSON(ctx, exec, path, req, nil); err != nil {
		return fmt.Errorf("deck remote engine: update music metas: %w", err)
	}
	return nil
}

func waitForReadyFlight(ctx context.Context, result <-chan singleflight.Result, callerToken *remoteReadyFlightToken) (bool, error) {
	finishReadyWait := commandtrace.MeasureOperation(ctx, "deck.ready_wait")
	select {
	case <-ctx.Done():
		finishReadyWait()
		return false, ctx.Err()
	case readyResult := <-result:
		finishReadyWait()
		if readyResult.Err != nil {
			return classifyReadyFlightError(ctx, readyResult.Err)
		}
		completed, ok := readyResult.Val.(remoteReadyFlightResult)
		if !ok {
			return false, fmt.Errorf("deck ready returned unexpected shared result %T", readyResult.Val)
		}
		commandtrace.MergeOperations(ctx, completed.operations)
		if completed.leader != callerToken {
			commandtrace.RecordOperation(ctx, "deck.ready_shared", 0)
		}
		if completed.err != nil {
			return classifyReadyFlightError(ctx, completed.err)
		}
		return false, nil
	}
}

func classifyReadyFlightError(ctx context.Context, flightErr error) (bool, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	if errors.Is(flightErr, context.Canceled) || errors.Is(flightErr, context.DeadlineExceeded) {
		return true, nil
	}
	return false, flightErr
}

func (r *RemoteDeckRecommender) readySharedTimeout() time.Duration {
	requestTimeout := 30 * time.Second
	if r != nil && r.client != nil && r.client.Timeout > 0 {
		requestTimeout = r.client.Timeout
	}
	retries := 0
	if r != nil && r.maxRetries > 0 {
		retries = min(r.maxRetries, 10)
	}
	retryWait := time.Duration(0)
	if r != nil && r.retryWaitTime > 0 {
		retryWait = r.retryWaitTime
	}
	// A cold target may need both masterdata and music-meta updates. Each may
	// consume every retry attempt, so budget both without inheriting a caller's
	// shorter cancellation deadline.
	timeout := 2 * (requestTimeout*time.Duration(retries+1) + retryWait*time.Duration(retries))
	if timeout < 30*time.Second {
		return 30 * time.Second
	}
	return timeout
}

func (r *RemoteDeckRecommender) invalidate(state *remoteTargetState, kind remoteRewarmKind) {
	state.mu.Lock()
	defer state.mu.Unlock()
	switch kind {
	case remoteRewarmMasterdata:
		state.masterdataReady = false
	case remoteRewarmMusicMeta:
		state.musicMetaHash = ""
	default:
		state.masterdataReady = false
		state.musicMetaHash = ""
	}
}
