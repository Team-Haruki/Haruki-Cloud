package deck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	sonic "github.com/bytedance/sonic"
	"golang.org/x/sync/errgroup"
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

	// Circuit breaker: reject early when service is consistently failing.
	if failures := exec.state.consecutiveFailures.Load(); failures >= maxConsecutiveFailures {
		if r.tryResetCircuitBreakerAfterCooldown(exec.state, failures) {
			if r.logger != nil {
				r.logger.InfoContext(ctx, "deck circuit breaker reset", "reason", "cooldown")
			}
		} else if r.tryResetCircuitBreakerOnHealthyService(ctx, exec.state, failures) {
			if r.logger != nil {
				r.logger.InfoContext(ctx, "deck circuit breaker reset", "reason", "health_probe")
			}
		} else {
			if r.logger != nil {
				r.logger.WarnContext(ctx, "deck circuit breaker open", "consecutive_failures", failures)
			}
			return nil, fmt.Errorf("deck-service unavailable: %d consecutive failures (circuit breaker open)", failures)
		}
	}

	if err := r.ensureReady(ctx, exec, req.Region, req.MusicMeta, req.MusicMetaFilePath); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		r.recordFailure(exec.state)
		return nil, err
	}

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
	if rewarmKind == remoteRewarmNone {
		if shouldCountCircuitBreakerFailure(err) {
			r.recordFailure(exec.state)
			r.logger.WarnContext(ctx, "deck recommendation failed",
				"duration_ms", commandtrace.Milliseconds(elapsed),
				"error_type", fmt.Sprintf("%T", err),
			)
		} else {
			r.recordSuccess(exec.state)
			r.logger.InfoContext(ctx, "deck recommendation returned logical error",
				"duration_ms", commandtrace.Milliseconds(elapsed),
				"error_type", fmt.Sprintf("%T", err),
			)
		}
		return nil, err
	}

	// Rewarm and retry once.
	r.logger.InfoContext(ctx, "deck service rewarming", "error_type", fmt.Sprintf("%T", err))
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
	results, err = r.recommendBatchOnce(ctx, exec, req)
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
		r.logger.Info("deck circuit breaker reset")
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
	recommendJSON, err := sonic.Marshal(recommendPayload)
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
	type partial struct {
		index int
		item  remoteBatchRecommendResult
		err   error
	}

	ctx = normalizeRecommendContext(ctx)
	group, groupCtx := errgroup.WithContext(ctx)
	partials := make([]partial, len(req.BatchOption))
	var launchErr error
	for index, option := range req.BatchOption {
		if err := groupCtx.Err(); err != nil {
			launchErr = err
			break
		}
		index := index
		opt := cloneRecommendOption(option)
		group.Go(func() error {
			alg, _ := opt["algorithm"].(string)
			start := time.Now()
			decks, err := r.doRecommendLegacyOption(groupCtx, exec, req, opt)
			if err != nil {
				partials[index] = partial{
					index: index,
					item: remoteBatchRecommendResult{
						Alg:   alg,
						Error: err.Error(),
					},
					err: err,
				}
				return nil
			}
			partials[index] = partial{
				index: index,
				item: remoteBatchRecommendResult{
					Alg:      alg,
					CostTime: time.Since(start).Seconds(),
					Result:   &remoteRecommendResult{Decks: decks},
				},
			}
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
	if err := ctx.Err(); err != nil {
		return err
	}
	musicMetaPath = strings.TrimSpace(musicMetaPath)
	hash := hashPayload(musicMeta)
	if err := ctx.Err(); err != nil {
		return err
	}
	if hash == "" && musicMetaPath != "" {
		hash = "path:" + musicMetaPath
	}

	state := exec.state
	state.mu.Lock()
	masterReady := state.masterdataReady
	musicReady := hash != "" && hash == state.musicMetaHash
	state.mu.Unlock()

	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = r.region
	}

	if masterReady && (hash == "" || musicReady) {
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		callerToken := new(remoteReadyFlightToken)
		result := state.readyGroup.DoChan("ready", func() (any, error) {
			detached := context.Background()
			detached = logger.WithContextAttrs(detached, slog.Bool("shared_work", true))
			sharedBase, cancel := context.WithTimeout(detached, r.readySharedTimeout())
			defer cancel()
			sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)
			sharedExec := &remoteExecution{state: state}

			workErr := func() error {
				state.mu.Lock()
				masterReady = state.masterdataReady
				musicReady = hash != "" && hash == state.musicMetaHash
				state.mu.Unlock()

				if masterReady && (hash == "" || musicReady) {
					return nil
				}

				if !masterReady {
					req := map[string]any{
						"base_dir": r.masterdataDir,
						"region":   region,
					}
					if err := r.postJSON(sharedCtx, sharedExec, "/update/masterdata", req, nil); err != nil {
						return fmt.Errorf("deck remote engine: update masterdata: %w", err)
					}
				}

				if hash != "" && !musicReady {
					req := map[string]any{"region": region}
					// Prefer sending bytes directly — container cannot access host file paths.
					if len(musicMeta) > 0 {
						req["data"] = string(musicMeta)
						if err := r.postJSON(sharedCtx, sharedExec, "/update/musicmetas/string", req, nil); err != nil {
							return fmt.Errorf("deck remote engine: update music metas: %w", err)
						}
					} else if musicMetaPath != "" {
						req["file_path"] = musicMetaPath
						if err := r.postJSON(sharedCtx, sharedExec, "/update/musicmetas", req, nil); err != nil {
							return fmt.Errorf("deck remote engine: update music metas: %w", err)
						}
					}
				}

				state.mu.Lock()
				state.masterdataReady = true
				if hash != "" {
					state.musicMetaHash = hash
				}
				state.mu.Unlock()
				return nil
			}()

			return remoteReadyFlightResult{
				err:        workErr,
				operations: trace.Snapshot().Operations,
				leader:     callerToken,
			}, nil
		})
		finishReadyWait := commandtrace.MeasureOperation(ctx, "deck.ready_wait")
		select {
		case <-ctx.Done():
			finishReadyWait()
			return ctx.Err()
		case readyResult := <-result:
			finishReadyWait()
			if readyResult.Err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				if errors.Is(readyResult.Err, context.Canceled) || errors.Is(readyResult.Err, context.DeadlineExceeded) {
					continue
				}
				return readyResult.Err
			}
			completed, ok := readyResult.Val.(remoteReadyFlightResult)
			if !ok {
				return fmt.Errorf("deck ready returned unexpected shared result %T", readyResult.Val)
			}
			commandtrace.MergeOperations(ctx, completed.operations)
			if completed.leader != callerToken {
				commandtrace.RecordOperation(ctx, "deck.ready_shared", 0)
			}
			if completed.err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				if errors.Is(completed.err, context.Canceled) || errors.Is(completed.err, context.DeadlineExceeded) {
					continue
				}
				return completed.err
			}
		}

		state.mu.Lock()
		masterReady = state.masterdataReady
		musicReady = hash != "" && hash == state.musicMetaHash
		state.mu.Unlock()
		if masterReady && (hash == "" || musicReady) {
			return nil
		}
	}
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
