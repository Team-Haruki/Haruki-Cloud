package deck

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (r *RemoteDeckRecommender) Recommend(req RecommendRequest) (*RecommendResult, error) {
	if len(req.BatchOption) == 0 {
		return nil, fmt.Errorf("deck remote engine requires batch_options")
	}
	if len(req.UserData) == 0 && strings.TrimSpace(req.UserDataFilePath) == "" {
		return nil, fmt.Errorf("deck remote engine requires user_data bytes or file path")
	}
	if len(req.MusicMeta) == 0 && strings.TrimSpace(req.MusicMetaFilePath) == "" {
		return nil, fmt.Errorf("deck remote engine requires music meta bytes or file path")
	}

	if err := r.ensureReady(req.Region, req.MusicMeta, req.MusicMetaFilePath); err != nil {
		return nil, err
	}

	results, err := r.doRecommendCompatible(req)
	if err == nil {
		return aggregateRemoteRecommendResults(req.BatchOption, results)
	}
	if !shouldRewarmRemoteService(err) {
		return nil, err
	}
	r.invalidate()
	if warmErr := r.ensureReady(req.Region, req.MusicMeta, req.MusicMetaFilePath); warmErr != nil {
		return nil, warmErr
	}
	results, err = r.doRecommendCompatible(req)
	if err != nil {
		return nil, err
	}
	return aggregateRemoteRecommendResults(req.BatchOption, results)
}

func (r *RemoteDeckRecommender) doRecommendCompatible(req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	results, err := r.doRecommendBatch(req)
	if err == nil {
		return results, nil
	}
	if !isUnsupportedBatchProtocolError(err) {
		return nil, err
	}
	return r.doRecommendLegacy(req)
}

func (r *RemoteDeckRecommender) doRecommendBatch(req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	userData := req.UserData
	if len(userData) == 0 {
		return nil, fmt.Errorf("deck remote engine: no user data bytes available")
	}

	var cacheResp remoteUserDataCacheResponse
	if err := r.postBinary("/cache_userdata", buildMultipartPayload(userData), &cacheResp); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cacheResp.UserdataHash) == "" {
		return nil, fmt.Errorf("deck-service cache_userdata returned empty userdata_hash")
	}

	recommendPayload := map[string]interface{}{
		"region":        strings.ToLower(strings.TrimSpace(req.Region)),
		"batch_options": req.BatchOption,
		"userdata_hash": cacheResp.UserdataHash,
	}
	recommendJSON, err := json.Marshal(recommendPayload)
	if err != nil {
		return nil, err
	}

	var raw json.RawMessage
	if err := r.postBinary("/recommend", buildMultipartPayload(recommendJSON), &raw); err != nil {
		return nil, err
	}
	return parseRemoteRecommendBatch(raw, req.BatchOption)
}

func (r *RemoteDeckRecommender) doRecommendLegacy(req RecommendRequest) ([]remoteBatchRecommendResult, error) {
	type partial struct {
		item remoteBatchRecommendResult
		err  error
	}

	results := make(chan partial, len(req.BatchOption))
	for _, option := range req.BatchOption {
		opt := cloneRecommendOption(option)
		go func() {
			alg, _ := opt["algorithm"].(string)
			start := time.Now()
			decks, err := r.doRecommendLegacyOption(req, opt)
			if err != nil {
				results <- partial{err: err}
				return
			}
			results <- partial{
				item: remoteBatchRecommendResult{
					Alg:      alg,
					CostTime: time.Since(start).Seconds(),
					Result:   &remoteRecommendResult{Decks: decks},
				},
			}
		}()
	}

	out := make([]remoteBatchRecommendResult, 0, len(req.BatchOption))
	var firstErr error
	for range req.BatchOption {
		item := <-results
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		out = append(out, item.item)
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func (r *RemoteDeckRecommender) doRecommendLegacyOption(req RecommendRequest, option map[string]interface{}) ([]remoteRecommendDeck, error) {
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
	if err := r.postJSON("/recommend", payload, &response); err != nil {
		return nil, err
	}
	return response.Decks, nil
}

func (r *RemoteDeckRecommender) ensureReady(region string, musicMeta []byte, musicMetaPath string) error {
	musicMetaPath = strings.TrimSpace(musicMetaPath)
	hash := hashPayload(musicMeta)
	if hash == "" && musicMetaPath != "" {
		hash = "path:" + musicMetaPath
	}

	r.mu.Lock()
	masterReady := r.masterdataReady
	musicReady := hash != "" && hash == r.musicMetaHash
	r.mu.Unlock()

	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = r.region
	}

	if !masterReady {
		req := map[string]interface{}{
			"base_dir": r.masterdataDir,
			"region":   region,
		}
		if err := r.postJSON("/update/masterdata", req, nil); err != nil {
			return fmt.Errorf("deck remote engine: update masterdata: %w", err)
		}
	}

	if hash != "" && !musicReady {
		req := map[string]interface{}{"region": region}
		// Prefer sending bytes directly — container cannot access host file paths.
		if len(musicMeta) > 0 {
			req["data"] = string(musicMeta)
			if err := r.postJSON("/update/musicmetas/string", req, nil); err != nil {
				return fmt.Errorf("deck remote engine: update music metas: %w", err)
			}
		} else if musicMetaPath != "" {
			req["file_path"] = musicMetaPath
			if err := r.postJSON("/update/musicmetas", req, nil); err != nil {
				return fmt.Errorf("deck remote engine: update music metas: %w", err)
			}
		}
	}

	r.mu.Lock()
	r.masterdataReady = true
	if hash != "" {
		r.musicMetaHash = hash
	}
	r.mu.Unlock()
	return nil
}

func (r *RemoteDeckRecommender) invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.masterdataReady = false
	r.musicMetaHash = ""
}
