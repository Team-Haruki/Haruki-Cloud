package deck

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
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

func (r *RemoteDeckRecommender) postJSON(path string, requestBody interface{}, responseBody interface{}) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, r.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var remoteErr remoteErrorResponse
		if json.Unmarshal(payload, &remoteErr) == nil && strings.TrimSpace(remoteErr.Error) != "" {
			return fmt.Errorf("%s", remoteErr.Error)
		}
		if trimmed := strings.TrimSpace(string(payload)); trimmed != "" {
			return fmt.Errorf("deck-service returned HTTP %d: %s", resp.StatusCode, trimmed)
		}
		return fmt.Errorf("deck-service returned HTTP %d", resp.StatusCode)
	}
	if responseBody == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, responseBody)
}

func (r *RemoteDeckRecommender) postBinary(path string, payload []byte, responseBody interface{}) error {
	req, err := http.NewRequest(http.MethodPost, r.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var remoteErr remoteErrorResponse
		if json.Unmarshal(body, &remoteErr) == nil && strings.TrimSpace(remoteErr.Error) != "" {
			return fmt.Errorf("%s", remoteErr.Error)
		}
		if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
			return fmt.Errorf("deck-service returned HTTP %d: %s", resp.StatusCode, trimmed)
		}
		return fmt.Errorf("deck-service returned HTTP %d", resp.StatusCode)
	}
	if responseBody == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, responseBody)
}

func buildMultipartPayload(segments ...[]byte) []byte {
	var raw bytes.Buffer
	for _, segment := range segments {
		if err := binary.Write(&raw, binary.BigEndian, uint32(len(segment))); err != nil {
			return nil
		}
		if _, err := raw.Write(segment); err != nil {
			return nil
		}
	}

	var compressed bytes.Buffer
	writer, err := zstd.NewWriter(&compressed)
	if err != nil {
		return nil
	}
	if _, err := writer.Write(raw.Bytes()); err != nil {
		_ = writer.Close()
		return nil
	}
	if err := writer.Close(); err != nil {
		return nil
	}
	return compressed.Bytes()
}

func convertRemoteDecks(src []remoteRecommendDeck) []RecommendDeck {
	out := make([]RecommendDeck, 0, len(src))
	for _, d := range src {
		cards := make([]RecommendCard, 0, len(d.Cards))
		for _, c := range d.Cards {
			cards = append(cards, RecommendCard{
				CardID:          c.CardID,
				Level:           c.Level,
				MasterRank:      c.MasterRank,
				DefaultImage:    c.DefaultImage,
				SkillLevel:      c.SkillLevel,
				SkillRate:       c.SkillScoreUp,
				EventBonusRate:  c.EventBonusRate,
				IsAfterStory:    c.Episode2Read,
				IsBeforeStory:   c.Episode1Read,
				IsAfterTraining: c.AfterTraining,
				HasCanvasBonus:  c.HasCanvasBonus,
			})
		}
		out = append(out, RecommendDeck{
			Cards:                cards,
			Score:                d.Score,
			LiveScore:            d.LiveScore,
			MysekaiEventPoint:    d.MysekaiEventPoint,
			TotalPower:           d.TotalPower,
			EventBonusRate:       d.EventBonusRate,
			SupportDeckBonusRate: d.SupportDeckBonusRate,
			MultiLiveScoreUp:     d.MultiLiveScoreUp,
		})
	}
	return out
}

func parseRemoteRecommendBatch(raw json.RawMessage, options []map[string]interface{}) ([]remoteBatchRecommendResult, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("deck-service returned empty response")
	}

	if trimmed[0] == '[' {
		var items []remoteBatchRecommendResult
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		for i := range items {
			if strings.TrimSpace(items[i].Alg) == "" && i < len(options) {
				if alg, _ := options[i]["algorithm"].(string); strings.TrimSpace(alg) != "" {
					items[i].Alg = alg
				}
			}
		}
		return items, nil
	}

	var single remoteRecommendResult
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return nil, err
	}
	item := remoteBatchRecommendResult{Result: &single}
	if len(options) > 0 {
		if alg, _ := options[0]["algorithm"].(string); strings.TrimSpace(alg) != "" {
			item.Alg = alg
		}
	}
	return []remoteBatchRecommendResult{item}, nil
}

func aggregateRemoteRecommendResults(options []map[string]interface{}, results []remoteBatchRecommendResult) (*RecommendResult, error) {
	agg := &RecommendResult{
		CostTimes: make(map[string]float64),
		WaitTimes: make(map[string]float64),
	}
	seen := make(map[string]*RecommendDeck)
	var order []string
	var firstErr error

	for _, item := range results {
		if strings.TrimSpace(item.Error) != "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s", strings.TrimSpace(item.Error))
			}
			continue
		}

		var decks []RecommendDeck
		if item.Result != nil {
			decks = convertRemoteDecks(item.Result.Decks)
		} else {
			decks = convertRemoteDecks(item.Decks)
		}
		if len(decks) == 0 {
			continue
		}

		alg := strings.ToLower(strings.TrimSpace(item.Alg))
		if alg != "" {
			agg.CostTimes[alg] = item.CostTime
			agg.WaitTimes[alg] = item.WaitTime
		}

		for _, deck := range decks {
			h := deckHash(deck)
			if existing, ok := seen[h]; ok {
				if alg != "" {
					existing.Algs = append(existing.Algs, alg)
				}
				continue
			}
			deckCopy := deck
			if alg != "" {
				deckCopy.Algs = []string{alg}
			}
			seen[h] = &deckCopy
			order = append(order, h)
		}
	}
	if len(order) == 0 && firstErr != nil {
		return nil, firstErr
	}

	type pair struct {
		Deck RecommendDeck
		Alg  string
	}
	var pairs []pair
	for _, h := range order {
		deck := seen[h]
		algsMap := make(map[string]struct{})
		for _, a := range deck.Algs {
			algsMap[a] = struct{}{}
		}
		var algs []string
		for alg := range algsMap {
			algs = append(algs, alg)
		}
		sort.Strings(algs)
		pairs = append(pairs, pair{Deck: *deck, Alg: strings.Join(algs, "+")})
	}

	liveType, _ := options[0]["live_type"].(string)
	target, _ := options[0]["target"].(string)
	sort.SliceStable(pairs, func(i, j int) bool {
		d1 := pairs[i].Deck
		d2 := pairs[j].Deck
		if liveType == "mysekai" {
			if d1.MysekaiEventPoint != d2.MysekaiEventPoint {
				return d1.MysekaiEventPoint > d2.MysekaiEventPoint
			}
			return d1.TotalPower > d2.TotalPower
		}
		if target == "power" {
			return d1.TotalPower > d2.TotalPower
		}
		if target == "skill" {
			return d1.MultiLiveScoreUp > d2.MultiLiveScoreUp
		}
		if target == "bonus" {
			if d1.EventBonusRate != d2.EventBonusRate {
				return d1.EventBonusRate < d2.EventBonusRate
			}
			if d1.Score != d2.Score {
				return d1.Score > d2.Score
			}
			return d1.MultiLiveScoreUp > d2.MultiLiveScoreUp
		}
		if d1.Score != d2.Score {
			return d1.Score > d2.Score
		}
		return d1.MultiLiveScoreUp > d2.MultiLiveScoreUp
	})

	limitFloat, _ := options[0]["limit"].(float64)
	limitInt, ok := options[0]["limit"].(int)
	if !ok {
		limitInt = int(limitFloat)
	}
	if limitInt <= 0 {
		limitInt = len(pairs)
	}
	if limitInt > len(pairs) {
		limitInt = len(pairs)
	}

	for i := 0; i < limitInt; i++ {
		agg.Decks = append(agg.Decks, pairs[i].Deck)
		agg.DeckAlgs = append(agg.DeckAlgs, pairs[i].Alg)
	}
	return agg, nil
}

func shouldRewarmRemoteService(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "master data not found") ||
		strings.Contains(message, "music metas not found") ||
		strings.Contains(message, "music meta not found")
}

func isUnsupportedBatchProtocolError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "http 404") ||
		strings.Contains(message, "missing field `live_type`") ||
		strings.Contains(message, "unsupported media type")
}

func hashPayload(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cloneRecommendOption(option map[string]interface{}) map[string]interface{} {
	if option == nil {
		return map[string]interface{}{}
	}
	copied := make(map[string]interface{}, len(option)+1)
	for k, v := range option {
		copied[k] = v
	}
	return copied
}
