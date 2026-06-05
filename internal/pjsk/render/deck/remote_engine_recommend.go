package deck

import (
	"encoding/json"
	"fmt"
	sonic "github.com/bytedance/sonic"
	"strings"
	"time"
)

func (r *RemoteDeckRecommender) Recommend(req RecommendRequest) (*RecommendResult, error) {
	results, err := r.RecommendBatch(req)
	if err != nil {
		return nil, err
	}
	return aggregateRemoteRecommendResults(req.RecommendType, req.BatchOption, results)
}

func (r *RemoteDeckRecommender) RecommendBatch(req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	if err := validateRemoteRecommendRequest(req); err != nil {
		return nil, err
	}
	exec, err := r.acquireExecution()
	if err != nil {
		return nil, err
	}
	defer exec.Release()

	// Circuit breaker: reject early when service is consistently failing.
	if failures := exec.state.consecutiveFailures.Load(); failures >= maxConsecutiveFailures {
		if r.tryResetCircuitBreakerAfterCooldown(exec.state, failures) {
			r.logger.Infof("circuit breaker auto-reset after cooldown; allowing request to proceed")
		} else if r.tryResetCircuitBreakerOnHealthyService(exec.state, failures) {
			r.logger.Infof("circuit breaker reset after successful health probe; allowing request to proceed")
		} else {
			r.logger.Warnf("circuit breaker open: %d consecutive failures, rejecting request", failures)
			return nil, fmt.Errorf("deck-service unavailable: %d consecutive failures (circuit breaker open)", failures)
		}
	}

	if err := r.ensureReady(exec, req.Region, req.MusicMeta, req.MusicMetaFilePath); err != nil {
		r.recordFailure(exec.state)
		return nil, err
	}

	start := time.Now()
	results, err := r.recommendBatchOnce(exec, req)
	elapsed := time.Since(start)
	if err == nil {
		err = firstRemoteBatchError(results)
	}
	if err == nil {
		r.recordSuccess(exec.state)
		r.logger.Debugf("recommend completed in %v", elapsed)
		return results, nil
	}
	rewarmKind := classifyRemoteRewarm(err)
	if rewarmKind == remoteRewarmNone {
		if shouldCountCircuitBreakerFailure(err) {
			r.recordFailure(exec.state)
			r.logger.Warnf("recommend failed after %v: %v", elapsed, err)
		} else {
			r.recordSuccess(exec.state)
			r.logger.Infof("recommend returned logical error after %v: %v", elapsed, err)
		}
		return nil, err
	}

	// Rewarm and retry once.
	r.logger.Infof("rewarming remote service after: %v", err)
	r.invalidate(exec.state, rewarmKind)
	if warmErr := r.ensureReady(exec, req.Region, req.MusicMeta, req.MusicMetaFilePath); warmErr != nil {
		r.recordFailure(exec.state)
		return nil, warmErr
	}
	results, err = r.recommendBatchOnce(exec, req)
	if err == nil {
		err = firstRemoteBatchError(results)
	}
	if err != nil {
		r.recordFailure(exec.state)
		r.logger.Warnf("recommend failed after rewarm: %v", err)
		return nil, err
	}
	r.recordSuccess(exec.state)
	r.logger.Debugf("recommend completed after rewarm")
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

func (r *RemoteDeckRecommender) recommendBatchOnce(exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	return r.doRecommendCompatible(exec, req)
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
}

func (r *RemoteDeckRecommender) recordFailure(state *remoteTargetState) {
	state.consecutiveFailures.Add(1)
	state.lastFailureAtNanos.Store(r.timeNow().UnixNano())
}

func (r *RemoteDeckRecommender) resetCircuitBreaker(state *remoteTargetState) {
	state.consecutiveFailures.Store(0)
	state.lastFailureAtNanos.Store(0)
	r.logger.Infof("circuit breaker reset")
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
	r.logger.Infof("circuit breaker cooldown elapsed after %v; resetting from %d consecutive failures", elapsed, failures)
	r.resetCircuitBreaker(state)
	return true
}

func (r *RemoteDeckRecommender) tryResetCircuitBreakerOnHealthyService(state *remoteTargetState, failures int64) bool {
	if !r.healthCheck(state.target.BaseURL) {
		return false
	}
	r.logger.Infof("circuit breaker health probe succeeded; resetting from %d consecutive failures", failures)
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

func (r *RemoteDeckRecommender) doRecommendCompatible(exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	results, err := r.doRecommendBatch(exec, req)
	if err == nil {
		return results, nil
	}
	if !isUnsupportedBatchProtocolError(err) && !isMissingUserdataHashError(err) {
		return nil, err
	}
	return r.doRecommendLegacy(exec, req)
}

func (r *RemoteDeckRecommender) doRecommendBatch(exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	userData := req.UserData
	if len(userData) == 0 {
		return nil, fmt.Errorf("deck remote engine: no user data bytes available")
	}

	var cacheResp remoteUserDataCacheResponse
	if err := r.postBinary(exec, "/cache_userdata", buildMultipartPayload(userData), &cacheResp); err != nil {
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
	recommendJSON, err := sonic.Marshal(recommendPayload)
	if err != nil {
		return nil, err
	}

	var raw json.RawMessage
	if err := r.postBinary(exec, "/recommend", buildMultipartPayload(recommendJSON), &raw); err != nil {
		return nil, err
	}
	return parseRemoteRecommendBatch(raw, req.BatchOption)
}

func (r *RemoteDeckRecommender) doRecommendLegacy(exec *remoteExecution, req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	type partial struct {
		index int
		item  remoteBatchRecommendResult
		err   error
	}

	results := make(chan partial, len(req.BatchOption))
	for index, option := range req.BatchOption {
		opt := cloneRecommendOption(option)
		go func(index int) {
			alg, _ := opt["algorithm"].(string)
			start := time.Now()
			decks, err := r.doRecommendLegacyOption(exec, req, opt)
			if err != nil {
				results <- partial{
					index: index,
					item: remoteBatchRecommendResult{
						Alg:   alg,
						Error: err.Error(),
					},
					err: err,
				}
				return
			}
			results <- partial{
				index: index,
				item: remoteBatchRecommendResult{
					Alg:      alg,
					CostTime: time.Since(start).Seconds(),
					Result:   &remoteRecommendResult{Decks: decks},
				},
			}
		}(index)
	}

	out := make([]remoteBatchRecommendResult, len(req.BatchOption))
	var firstErr error
	successCount := 0
	for range req.BatchOption {
		item := <-results
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

func (r *RemoteDeckRecommender) doRecommendLegacyOption(exec *remoteExecution, req RecommendRequest, option map[string]any) ([]remoteRecommendDeck, error) {
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
	if err := r.postJSON(exec, "/recommend", payload, &response); err != nil {
		return nil, err
	}
	return response.Decks, nil
}

func (r *RemoteDeckRecommender) ensureReady(exec *remoteExecution, region string, musicMeta []byte, musicMetaPath string) error {
	musicMetaPath = strings.TrimSpace(musicMetaPath)
	hash := hashPayload(musicMeta)
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
		_, err, _ := state.readyGroup.Do("ready", func() (any, error) {
			state.mu.Lock()
			masterReady = state.masterdataReady
			musicReady = hash != "" && hash == state.musicMetaHash
			state.mu.Unlock()

			if masterReady && (hash == "" || musicReady) {
				return nil, nil
			}

			if !masterReady {
				req := map[string]any{
					"base_dir": r.masterdataDir,
					"region":   region,
				}
				if err := r.postJSON(exec, "/update/masterdata", req, nil); err != nil {
					return nil, fmt.Errorf("deck remote engine: update masterdata: %w", err)
				}
			}

			if hash != "" && !musicReady {
				req := map[string]any{"region": region}
				// Prefer sending bytes directly — container cannot access host file paths.
				if len(musicMeta) > 0 {
					req["data"] = string(musicMeta)
					if err := r.postJSON(exec, "/update/musicmetas/string", req, nil); err != nil {
						return nil, fmt.Errorf("deck remote engine: update music metas: %w", err)
					}
				} else if musicMetaPath != "" {
					req["file_path"] = musicMetaPath
					if err := r.postJSON(exec, "/update/musicmetas", req, nil); err != nil {
						return nil, fmt.Errorf("deck remote engine: update music metas: %w", err)
					}
				}
			}

			state.mu.Lock()
			state.masterdataReady = true
			if hash != "" {
				state.musicMetaHash = hash
			}
			state.mu.Unlock()
			return nil, nil
		})
		if err != nil {
			return err
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
