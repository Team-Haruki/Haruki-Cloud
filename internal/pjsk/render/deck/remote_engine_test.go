package deck

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/utils/logger"
)

func TestParseRemoteRecommendBatchSupportsLegacySingleResponse(t *testing.T) {
	raw := json.RawMessage(`{"decks":[{"score":100,"live_score":100,"mysekai_event_point":0,"total_power":200,"event_bonus_rate":25,"support_deck_bonus_rate":0,"multi_live_score_up":120,"cards":[]} ]}`)
	results, err := parseRemoteRecommendBatch(raw, []map[string]any{
		{"algorithm": "ga"},
	})
	if err != nil {
		t.Fatalf("parseRemoteRecommendBatch() error = %v", err)
	}
	if len(results) != 1 || results[0].Alg != "ga" {
		t.Fatalf("unexpected parsed results: %+v", results)
	}
	if results[0].Result == nil || len(results[0].Result.Decks) != 1 {
		t.Fatalf("unexpected parsed deck payload: %+v", results[0])
	}
}

func TestAggregateRemoteRecommendResultsMergesAlgorithmsForSameDeck(t *testing.T) {
	options := []map[string]any{
		{"algorithm": "dfs", "target": "score", "live_type": "multi", "limit": 1},
		{"algorithm": "ga", "target": "score", "live_type": "multi", "limit": 1},
	}
	results := []remoteBatchRecommendResult{
		{
			Alg:      "dfs",
			CostTime: 1.2,
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{{
				Score:            100,
				LiveScore:        100,
				TotalPower:       200,
				MultiLiveScoreUp: 120,
				Cards:            []remoteRecommendCard{{CardID: 1001}},
			}}},
		},
		{
			Alg:      "ga",
			CostTime: 2.3,
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{{
				Score:            100,
				LiveScore:        100,
				TotalPower:       200,
				MultiLiveScoreUp: 120,
				Cards:            []remoteRecommendCard{{CardID: 1001}},
			}}},
		},
	}

	agg, err := aggregateRemoteRecommendResults(options, results)
	if err != nil {
		t.Fatalf("aggregateRemoteRecommendResults() error = %v", err)
	}
	if len(agg.Decks) != 1 {
		t.Fatalf("expected 1 merged deck, got %+v", agg.Decks)
	}
	if len(agg.DeckAlgs) != 1 || agg.DeckAlgs[0] != "DFS+GA" {
		t.Fatalf("unexpected deck algs: %+v", agg.DeckAlgs)
	}
	if agg.CostTimes["DFS"] != 1.2 || agg.CostTimes["GA"] != 2.3 {
		t.Fatalf("unexpected cost times: %+v", agg.CostTimes)
	}
}

func TestNormalizeRecommendAlgorithmAliases(t *testing.T) {
	testCases := map[string]string{
		"dfs":     "dfs",
		"sa":      "ga",
		"ga":      "ga",
		"dfs-ga":  "dfs_ga",
		"dfs_ga":  "dfs_ga",
		"ga_dfs":  "dfs_ga",
		"rl":      "rl",
		"all":     "all",
		"unknown": "",
	}

	for input, expected := range testCases {
		if got := normalizeRecommendAlgorithm(input); got != expected {
			t.Fatalf("normalizeRecommendAlgorithm(%q) = %q, want %q", input, got, expected)
		}
	}

	if got := normalizeRecommendAlgorithmForService("dfs-ga"); got != "dfs_ga" {
		t.Fatalf("normalizeRecommendAlgorithmForService(dfs-ga) = %q", got)
	}
	if got := normalizeRecommendAlgorithmForService("sa"); got != "ga" {
		t.Fatalf("normalizeRecommendAlgorithmForService(sa) = %q", got)
	}
}

func TestExpandAlgorithmsNormalizesConfiguredAndRequestedAlgorithms(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		MasterdataDir:  "/masterdata",
		DefaultAlgs:    []string{"dfs", "dfs-ga", "sa", "rl", "dfs_ga"},
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	remote, ok := recommender.(*RemoteDeckRecommender)
	if !ok {
		t.Fatalf("unexpected recommender type: %T", recommender)
	}
	if strings.Join(remote.defaultAlgs, ",") != "dfs,dfs_ga,ga,rl" {
		t.Fatalf("unexpected default algorithms: %+v", remote.defaultAlgs)
	}

	single := remote.ExpandAlgorithms(map[string]any{"algorithm": "dfs-ga", "target": "score"})
	if len(single) != 1 || single[0]["algorithm"] != "dfs_ga" {
		t.Fatalf("unexpected normalized single algorithm payload: %+v", single)
	}

	all := remote.ExpandAlgorithms(map[string]any{"algorithm": "all", "target": "score"})
	if len(all) != 3 {
		t.Fatalf("unexpected expanded algorithm count: %+v", all)
	}
	if all[0]["algorithm"] != "dfs_ga" || all[1]["algorithm"] != "ga" || all[2]["algorithm"] != "rl" {
		t.Fatalf("unexpected expanded algorithms: %+v", all)
	}

	skill := remote.ExpandAlgorithms(map[string]any{"algorithm": "all", "target": "skill"})
	if len(skill) != 4 {
		t.Fatalf("unexpected skill algorithm count: %+v", skill)
	}
	if skill[0]["algorithm"] != "dfs" || skill[1]["algorithm"] != "dfs_ga" || skill[2]["algorithm"] != "ga" || skill[3]["algorithm"] != "rl" {
		t.Fatalf("unexpected expanded skill algorithms: %+v", skill)
	}
}

func TestAggregateRemoteRecommendResultsUsesDisplayAlgorithmNames(t *testing.T) {
	options := []map[string]any{
		{"algorithm": "dfs_ga", "target": "score", "live_type": "multi", "limit": 1},
		{"algorithm": "rl", "target": "score", "live_type": "multi", "limit": 1},
	}
	results := []remoteBatchRecommendResult{
		{
			Alg:      "dfs_ga",
			CostTime: 1.5,
			WaitTime: 0.5,
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{{
				Score:            200,
				LiveScore:        200,
				TotalPower:       300,
				MultiLiveScoreUp: 140,
				Cards:            []remoteRecommendCard{{CardID: 1001}},
			}}},
		},
		{
			Alg:      "rl",
			CostTime: 0.9,
			WaitTime: 0.2,
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{{
				Score:            200,
				LiveScore:        200,
				TotalPower:       300,
				MultiLiveScoreUp: 140,
				Cards:            []remoteRecommendCard{{CardID: 1001}},
			}}},
		},
	}

	agg, err := aggregateRemoteRecommendResults(options, results)
	if err != nil {
		t.Fatalf("aggregateRemoteRecommendResults() error = %v", err)
	}
	if len(agg.DeckAlgs) != 1 || agg.DeckAlgs[0] != "DGA+RL" {
		t.Fatalf("unexpected display deck algs: %+v", agg.DeckAlgs)
	}
	if agg.CostTimes["DGA"] != 1.5 || agg.WaitTimes["DGA"] != 0.5 {
		t.Fatalf("unexpected dfs_ga timings: cost=%+v wait=%+v", agg.CostTimes, agg.WaitTimes)
	}
	if agg.CostTimes["RL"] != 0.9 || agg.WaitTimes["RL"] != 0.2 {
		t.Fatalf("unexpected rl timings: cost=%+v wait=%+v", agg.CostTimes, agg.WaitTimes)
	}
}

func TestRemoteRecommendRewarmsOnLogicalMusicMetaError(t *testing.T) {
	var masterdataCalls atomic.Int32
	var musicMetaCalls atomic.Int32
	var recommendCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/update/masterdata":
			masterdataCalls.Add(1)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/update/musicmetas/string":
			musicMetaCalls.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode music meta update payload: %v", err)
			}
			if strings.TrimSpace(payload["data"].(string)) == "" {
				t.Fatalf("expected non-empty music meta payload")
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			call := recommendCalls.Add(1)
			if call == 1 {
				_, _ = w.Write([]byte(`[
					{
						"alg": "ga",
						"error": "Music metas not found for region: jp"
					}
				]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{
					"alg": "ga",
					"cost_time": 0.1,
					"wait_time": 0.0,
					"result": {
						"decks": [{
							"score": 100,
							"live_score": 100,
							"mysekai_event_point": 0,
							"total_power": 200,
							"event_bonus_rate": 20,
							"support_deck_bonus_rate": 0,
							"multi_live_score_up": 110,
							"cards": [{"card_id": 1001, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false}]
						}]
					}
				}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	musicMeta := []byte(`[{"music_id":10000,"difficulty":"master"}]`)
	recommender := &RemoteDeckRecommender{
		baseURL:       server.URL,
		client:        server.Client(),
		defaultAlgs:   []string{"ga"},
		masterdataDir: "/masterdata",
		region:        "jp",
		maxRetries:    0,
		logger:        logger.NewLogger("DeckRemoteTest", "DEBUG", nil),
	}

	// Simulate a stale Cloud-side ready cache after deck-service restart.
	recommender.masterdataReady = true
	recommender.musicMetaHash = hashPayload(musicMeta)

	result, err := recommender.Recommend(RecommendRequest{
		Region:    "jp",
		UserData:  []byte(`{"user":"ok"}`),
		MusicMeta: musicMeta,
		BatchOption: []map[string]any{
			{"algorithm": "ga", "target": "score", "live_type": "multi", "limit": 1},
		},
	})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if result == nil || len(result.Decks) != 1 {
		t.Fatalf("unexpected recommend result: %+v", result)
	}
	if masterdataCalls.Load() != 1 {
		t.Fatalf("expected 1 masterdata rewarm call, got %d", masterdataCalls.Load())
	}
	if musicMetaCalls.Load() != 1 {
		t.Fatalf("expected 1 music meta rewarm call, got %d", musicMetaCalls.Load())
	}
	if recommendCalls.Load() != 2 {
		t.Fatalf("expected 2 recommend calls, got %d", recommendCalls.Load())
	}
}

func TestRemoteRecommendFallsBackToLegacyWhenUserdataHashMissing(t *testing.T) {
	var batchRecommendCalls atomic.Int32
	var legacyRecommendCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			if strings.Contains(r.Header.Get("Content-Type"), "application/octet-stream") {
				batchRecommendCalls.Add(1)
				http.Error(w, `{"error":"User data not found for userdata_hash: test-userdata-hash"}`, http.StatusInternalServerError)
				return
			}
			legacyRecommendCalls.Add(1)
			_, _ = w.Write([]byte(`{"decks":[{"score":100,"live_score":100,"mysekai_event_point":0,"total_power":200,"event_bonus_rate":20,"support_deck_bonus_rate":0,"multi_live_score_up":110,"cards":[{"card_id":1001,"level":60,"master_rank":5,"skill_level":4,"skill_score_up":120,"event_bonus_rate":20,"episode1_read":true,"episode2_read":true,"after_training":true,"default_image":"special_training","has_canvas_bonus":false}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	musicMeta := []byte(`[{"music_id":10000,"difficulty":"master"}]`)
	recommender := &RemoteDeckRecommender{
		baseURL:       server.URL,
		client:        server.Client(),
		defaultAlgs:   []string{"ga"},
		masterdataDir: "/masterdata",
		region:        "jp",
		maxRetries:    0,
		logger:        logger.NewLogger("DeckRemoteTest", "DEBUG", nil),
	}

	result, err := recommender.Recommend(RecommendRequest{
		Region:    "jp",
		UserData:  []byte(`{"user":"ok"}`),
		MusicMeta: musicMeta,
		BatchOption: []map[string]any{
			{"algorithm": "ga", "target": "score", "live_type": "multi", "limit": 1},
		},
	})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if result == nil || len(result.Decks) != 1 {
		t.Fatalf("unexpected recommend result: %+v", result)
	}
	if batchRecommendCalls.Load() != 1 {
		t.Fatalf("expected 1 batch recommend call, got %d", batchRecommendCalls.Load())
	}
	if legacyRecommendCalls.Load() != 1 {
		t.Fatalf("expected 1 legacy recommend call, got %d", legacyRecommendCalls.Load())
	}
}

func TestRemoteRecommendBatchFallsBackToLegacyAndPreservesOptionOrder(t *testing.T) {
	var batchRecommendCalls atomic.Int32
	var legacyRecommendCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			if strings.Contains(r.Header.Get("Content-Type"), "application/octet-stream") {
				batchRecommendCalls.Add(1)
				http.NotFound(w, r)
				return
			}

			legacyRecommendCalls.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode legacy recommend payload: %v", err)
			}
			charID, _ := payload["challenge_live_character_id"].(float64)
			if int(charID) == 1 {
				time.Sleep(50 * time.Millisecond)
			}

			score := 1000 + int(charID)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"decks":[{"score":%d,"live_score":%d,"mysekai_event_point":0,"total_power":200,"event_bonus_rate":20,"support_deck_bonus_rate":0,"multi_live_score_up":110,"cards":[{"card_id":1001,"level":60,"master_rank":5,"skill_level":4,"skill_score_up":120,"event_bonus_rate":20,"episode1_read":true,"episode2_read":true,"after_training":true,"default_image":"special_training","has_canvas_bonus":false}]}]}`, score, score)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	recommender := &RemoteDeckRecommender{
		baseURL:       server.URL,
		client:        server.Client(),
		defaultAlgs:   []string{"ga"},
		masterdataDir: "/masterdata",
		region:        "jp",
		maxRetries:    0,
		logger:        logger.NewLogger("DeckRemoteTest", "DEBUG", nil),
	}

	results, err := recommender.RecommendBatch(RecommendRequest{
		Region:    "jp",
		UserData:  []byte(`{"user":"ok"}`),
		MusicMeta: []byte(`[{"music_id":10000,"difficulty":"master"}]`),
		BatchOption: []map[string]any{
			{"algorithm": "ga", "target": "score", "live_type": "challenge", "limit": 1, "challenge_live_character_id": 1},
			{"algorithm": "ga", "target": "score", "live_type": "challenge", "limit": 1, "challenge_live_character_id": 2},
		},
	})
	if err != nil {
		t.Fatalf("RecommendBatch() error = %v", err)
	}
	if batchRecommendCalls.Load() != 1 {
		t.Fatalf("expected 1 batch recommend call, got %d", batchRecommendCalls.Load())
	}
	if legacyRecommendCalls.Load() != 2 {
		t.Fatalf("expected 2 legacy recommend calls, got %d", legacyRecommendCalls.Load())
	}
	if len(results) != 2 {
		t.Fatalf("unexpected RecommendBatch result count: %d", len(results))
	}
	if results[0].Result == nil || len(results[0].Result.Decks) != 1 || results[0].Result.Decks[0].Score != 1001 {
		t.Fatalf("unexpected first RecommendBatch result: %+v", results[0])
	}
	if results[1].Result == nil || len(results[1].Result.Decks) != 1 || results[1].Result.Decks[0].Score != 1002 {
		t.Fatalf("unexpected second RecommendBatch result: %+v", results[1])
	}
}

func TestRemoteRecommendAutoResetsCircuitBreakerAfterCooldown(t *testing.T) {
	var cacheCalls atomic.Int32
	var recommendCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			cacheCalls.Add(1)
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			recommendCalls.Add(1)
			_, _ = w.Write([]byte(`[{
				"alg": "ga",
				"cost_time": 0.1,
				"wait_time": 0.0,
				"result": {
					"decks": [{
						"score": 100,
						"live_score": 100,
						"mysekai_event_point": 0,
						"total_power": 200,
						"event_bonus_rate": 20,
						"support_deck_bonus_rate": 0,
						"multi_live_score_up": 110,
						"cards": [{"card_id": 1001, "level": 60, "master_rank": 5, "skill_level": 4, "skill_score_up": 120, "event_bonus_rate": 20, "episode1_read": true, "episode2_read": true, "after_training": true, "default_image": "special_training", "has_canvas_bonus": false}]
					}]
				}
			}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := time.Now()
	musicMeta := []byte(`[{"music_id":10000,"difficulty":"master"}]`)
	recommender := &RemoteDeckRecommender{
		baseURL:       server.URL,
		client:        server.Client(),
		defaultAlgs:   []string{"ga"},
		masterdataDir: "/masterdata",
		region:        "jp",
		maxRetries:    0,
		logger:        logger.NewLogger("DeckRemoteTest", "DEBUG", nil),
		now: func() time.Time {
			return now
		},
	}
	recommender.consecutiveFailures.Store(maxConsecutiveFailures)
	recommender.lastFailureAtNanos.Store(now.Add(-circuitBreakerCooldown - time.Second).UnixNano())

	result, err := recommender.Recommend(RecommendRequest{
		Region:    "jp",
		UserData:  []byte(`{"user":"ok"}`),
		MusicMeta: musicMeta,
		BatchOption: []map[string]any{
			{"algorithm": "ga", "target": "score", "live_type": "multi", "limit": 1},
		},
	})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if result == nil || len(result.Decks) != 1 {
		t.Fatalf("unexpected recommend result: %+v", result)
	}
	if got := cacheCalls.Load(); got != 1 {
		t.Fatalf("expected cache_userdata to run once after auto-reset, got %d", got)
	}
	if got := recommendCalls.Load(); got != 1 {
		t.Fatalf("expected recommend to run once after auto-reset, got %d", got)
	}
	if failures := recommender.consecutiveFailures.Load(); failures != 0 {
		t.Fatalf("expected circuit breaker to reset after successful request, got %d failures", failures)
	}
}

func TestRemoteRecommendLogicalErrorsDoNotTripCircuitBreaker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			_, _ = w.Write([]byte(`[{
				"alg": "ga",
				"error": "Event not found for eventId: 202"
			}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	recommender := &RemoteDeckRecommender{
		baseURL:       server.URL,
		client:        server.Client(),
		defaultAlgs:   []string{"ga"},
		masterdataDir: "/masterdata",
		region:        "jp",
		maxRetries:    0,
		logger:        logger.NewLogger("DeckRemoteTest", "DEBUG", nil),
	}

	_, err := recommender.Recommend(RecommendRequest{
		Region:    "jp",
		UserData:  []byte(`{"user":"ok"}`),
		MusicMeta: []byte(`[{"music_id":10000,"difficulty":"master"}]`),
		BatchOption: []map[string]any{
			{"algorithm": "ga", "target": "score", "live_type": "multi", "limit": 1},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Event not found for eventId: 202") {
		t.Fatalf("expected logical event-not-found error, got %v", err)
	}
	if failures := recommender.consecutiveFailures.Load(); failures != 0 {
		t.Fatalf("expected logical error not to trip circuit breaker, got %d failures", failures)
	}
}

func TestRemoteRecommendAutoResetsCircuitBreakerWhenHealthProbeSucceeds(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/health":
			_, _ = w.Write([]byte(`ok`))
		case "/update/masterdata", "/update/musicmetas/string":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			_, _ = w.Write([]byte(`[{
				"alg": "ga",
				"error": "Event not found for eventId: 202"
			}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	recommender := &RemoteDeckRecommender{
		baseURL:       server.URL,
		client:        server.Client(),
		defaultAlgs:   []string{"ga"},
		masterdataDir: "/masterdata",
		region:        "jp",
		maxRetries:    0,
		logger:        logger.NewLogger("DeckRemoteTest", "DEBUG", nil),
		now: func() time.Time {
			return now
		},
	}
	recommender.consecutiveFailures.Store(maxConsecutiveFailures)
	recommender.lastFailureAtNanos.Store(now.UnixNano())

	_, err := recommender.Recommend(RecommendRequest{
		Region:    "jp",
		UserData:  []byte(`{"user":"ok"}`),
		MusicMeta: []byte(`[{"music_id":10000,"difficulty":"master"}]`),
		BatchOption: []map[string]any{
			{"algorithm": "ga", "target": "score", "live_type": "multi", "limit": 1},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Event not found for eventId: 202") {
		t.Fatalf("expected logical event-not-found error after health reset, got %v", err)
	}
	if failures := recommender.consecutiveFailures.Load(); failures != 0 {
		t.Fatalf("expected health probe to reset circuit breaker, got %d failures", failures)
	}
}
