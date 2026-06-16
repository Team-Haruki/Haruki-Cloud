package deck

import (
	"encoding/json"
	"fmt"
	sonic "github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/utils/logger"
)

func newStandaloneTestRemoteDeckRecommender(baseURL string, client *http.Client) *RemoteDeckRecommender {
	targets := upstream.ResolveTargets(baseURL, nil, "deck-service")
	return newTestRemoteDeckRecommenderWithTargets(targets, client)
}

func newTestRemoteDeckRecommenderWithTargets(targets []upstream.TargetConfig, client *http.Client) *RemoteDeckRecommender {
	targetStates := make(map[string]*remoteTargetState, len(targets))
	for _, target := range targets {
		targetStates[remoteTargetKey(target)] = &remoteTargetState{
			target:          target,
			masterdataReady: true,
		}
	}
	return &RemoteDeckRecommender{
		client:       client,
		pool:         upstream.NewPool(targets),
		targetStates: targetStates,
	}
}

func testRemoteTargetState(t *testing.T, recommender *RemoteDeckRecommender) *remoteTargetState {
	t.Helper()
	for _, state := range recommender.targetStates {
		return state
	}
	t.Fatalf("expected at least one target state")
	return nil
}

func testRemoteExecution(t *testing.T, recommender *RemoteDeckRecommender) *remoteExecution {
	t.Helper()
	exec, err := recommender.acquireExecution()
	if err != nil {
		t.Fatalf("acquireExecution() error = %v", err)
	}
	return exec
}

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

	agg, err := aggregateRemoteRecommendResults("event", options, results)
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

func TestAggregateRemoteRecommendResultsMergesSameCardsInDifferentPositions(t *testing.T) {
	options := []map[string]any{
		{"algorithm": "ga", "target": "score", "live_type": "multi", "limit": 6},
		{"algorithm": "dfs_ga", "target": "score", "live_type": "multi", "limit": 6},
		{"algorithm": "rl", "target": "score", "live_type": "multi", "limit": 6},
	}
	results := []remoteBatchRecommendResult{
		{
			Alg: "ga",
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{{
				Score:            2904855,
				LiveScore:        2904855,
				TotalPower:       357857,
				EventBonusRate:   273,
				MultiLiveScoreUp: 200,
				Cards: []remoteRecommendCard{
					{CardID: 1252},
					{CardID: 1057},
					{CardID: 243},
					{CardID: 841},
					{CardID: 424},
				},
			}}},
		},
		{
			Alg: "dfs_ga",
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{{
				Score:            2904855,
				LiveScore:        2904855,
				TotalPower:       357857,
				EventBonusRate:   273,
				MultiLiveScoreUp: 200,
				Cards: []remoteRecommendCard{
					{CardID: 1252},
					{CardID: 1057},
					{CardID: 424},
					{CardID: 841},
					{CardID: 243},
				},
			}}},
		},
		{
			Alg: "rl",
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{{
				Score:            2904855,
				LiveScore:        2904855,
				TotalPower:       357857,
				EventBonusRate:   273,
				MultiLiveScoreUp: 200,
				Cards: []remoteRecommendCard{
					{CardID: 1252},
					{CardID: 1057},
					{CardID: 841},
					{CardID: 243},
					{CardID: 424},
				},
			}}},
		},
	}

	agg, err := aggregateRemoteRecommendResults("no_event", options, results)
	if err != nil {
		t.Fatalf("aggregateRemoteRecommendResults() error = %v", err)
	}
	if len(agg.Decks) != 1 {
		t.Fatalf("expected same card set to merge across positions, got %+v", agg.Decks)
	}
	if len(agg.DeckAlgs) != 1 || agg.DeckAlgs[0] != "DGA+GA+RL" {
		t.Fatalf("unexpected deck algs: %+v", agg.DeckAlgs)
	}
	gotOrder := make([]int, 0, len(agg.Decks[0].Cards))
	for _, card := range agg.Decks[0].Cards {
		gotOrder = append(gotOrder, card.CardID)
	}
	wantOrder := []int{1252, 1057, 243, 841, 424}
	if fmt.Sprint(gotOrder) != fmt.Sprint(wantOrder) {
		t.Fatalf("merged deck should keep first returned order, got %+v", gotOrder)
	}
}

func TestAggregateRemoteRecommendResultsKeepsDifferentCardsWithSameScoreAndLeader(t *testing.T) {
	options := []map[string]any{
		{"algorithm": "ga", "target": "score", "live_type": "multi", "limit": 2},
	}
	results := []remoteBatchRecommendResult{
		{
			Alg: "ga",
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{
				{
					Score:            100,
					LiveScore:        100,
					TotalPower:       200,
					MultiLiveScoreUp: 120,
					Cards: []remoteRecommendCard{
						{CardID: 1001},
						{CardID: 1002},
					},
				},
				{
					Score:            100,
					LiveScore:        100,
					TotalPower:       200,
					MultiLiveScoreUp: 120,
					Cards: []remoteRecommendCard{
						{CardID: 1001},
						{CardID: 1003},
					},
				},
			}},
		},
	}

	agg, err := aggregateRemoteRecommendResults("event", options, results)
	if err != nil {
		t.Fatalf("aggregateRemoteRecommendResults() error = %v", err)
	}
	if len(agg.Decks) != 2 {
		t.Fatalf("expected different card sets to stay separate, got %+v", agg.Decks)
	}
	if agg.Decks[0].Cards[1].CardID == agg.Decks[1].Cards[1].CardID {
		t.Fatalf("expected distinct second cards, got %+v", agg.Decks)
	}
}

func TestAggregateRemoteRecommendResultsSortsNoEventByLiveScore(t *testing.T) {
	options := []map[string]any{
		{"algorithm": "rl", "target": "score", "live_type": "multi", "limit": 2},
	}
	results := []remoteBatchRecommendResult{
		{
			Alg: "rl",
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{
				{
					Score:            2000,
					LiveScore:        1800,
					TotalPower:       300000,
					MultiLiveScoreUp: 100,
					Cards:            []remoteRecommendCard{{CardID: 1001}},
				},
				{
					Score:            1900,
					LiveScore:        2200,
					TotalPower:       290000,
					MultiLiveScoreUp: 90,
					Cards:            []remoteRecommendCard{{CardID: 1002}},
				},
			}},
		},
	}

	agg, err := aggregateRemoteRecommendResults("no_event", options, results)
	if err != nil {
		t.Fatalf("aggregateRemoteRecommendResults() error = %v", err)
	}
	if len(agg.Decks) != 2 {
		t.Fatalf("expected 2 no-event decks, got %+v", agg.Decks)
	}
	if agg.Decks[0].LiveScore != 2200 {
		t.Fatalf("expected higher live_score deck first, got %+v", agg.Decks)
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

func TestDeckMasterdataDirSignatureChangesWhenFileChanges(t *testing.T) {
	root := t.TempDir()
	regionDir := filepath.Join(root, "jp")
	if err := os.MkdirAll(regionDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(regionDir, "areaItemLevels.json"), []byte(`[{"id":1}]`), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(regionDir, "cards.json"), []byte(`[{"id":1}]`), 0o644); err != nil {
		t.Fatalf("write cards: %v", err)
	}

	first, err := deckMasterdataDirSignature(root, "jp")
	if err != nil {
		t.Fatalf("first signature: %v", err)
	}
	if first.Files != 2 {
		t.Fatalf("expected 2 files, got %+v", first)
	}

	if err := os.WriteFile(filepath.Join(regionDir, "cards.json"), []byte(`[{"id":1},{"id":2}]`), 0o644); err != nil {
		t.Fatalf("rewrite cards: %v", err)
	}
	second, err := deckMasterdataDirSignature(root, "jp")
	if err != nil {
		t.Fatalf("second signature: %v", err)
	}
	if first.Hash == second.Hash {
		t.Fatalf("signature did not change after file update: %s", first.Hash)
	}
}

func TestRemoteMasterdataRefreshInvalidatesReadyTargets(t *testing.T) {
	root := t.TempDir()
	regionDir := filepath.Join(root, "jp")
	if err := os.MkdirAll(regionDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(regionDir, "areaItemLevels.json"), []byte(`[{"id":1}]`), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(regionDir, "cards.json"), []byte(`[{"id":1}]`), 0o644); err != nil {
		t.Fatalf("write cards: %v", err)
	}

	recommender := newStandaloneTestRemoteDeckRecommender("http://127.0.0.1:1", http.DefaultClient)
	recommender.masterdataDir = root
	recommender.region = "jp"
	recommender.logger = logger.NewLogger("DeckRemoteTest", "ERROR", nil)
	state := testRemoteTargetState(t, recommender)
	state.masterdataReady = true
	recommender.captureMasterdataSignature()

	if err := os.WriteFile(filepath.Join(regionDir, "cards.json"), []byte(`[{"id":1},{"id":2}]`), 0o644); err != nil {
		t.Fatalf("rewrite cards: %v", err)
	}
	recommender.refreshMasterdataSignature()

	state.mu.Lock()
	ready := state.masterdataReady
	state.mu.Unlock()
	if ready {
		t.Fatalf("expected target masterdata to be invalidated")
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
	if len(skill) != 3 {
		t.Fatalf("unexpected skill algorithm count: %+v", skill)
	}
	if skill[0]["algorithm"] != "dfs_ga" || skill[1]["algorithm"] != "ga" || skill[2]["algorithm"] != "rl" {
		t.Fatalf("unexpected expanded skill algorithms: %+v", skill)
	}
}

func TestExpandAlgorithmsSkillDefaultsExcludeDFS(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		MasterdataDir:  "/masterdata",
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	remote, ok := recommender.(*RemoteDeckRecommender)
	if !ok {
		t.Fatalf("unexpected recommender type: %T", recommender)
	}
	if strings.Join(remote.defaultAlgs, ",") != "ga,dfs_ga,rl" {
		t.Fatalf("unexpected default algorithms: %+v", remote.defaultAlgs)
	}

	skill := remote.ExpandAlgorithms(map[string]any{"algorithm": "all", "target": "skill"})
	if len(skill) != 3 {
		t.Fatalf("unexpected skill algorithm count: %+v", skill)
	}
	if skill[0]["algorithm"] != "ga" || skill[1]["algorithm"] != "dfs_ga" || skill[2]["algorithm"] != "rl" {
		t.Fatalf("unexpected expanded skill algorithms: %+v", skill)
	}
}

func TestExpandAlgorithmsSkillNormalizesExplicitDFSAndSubset(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		MasterdataDir:  "/masterdata",
		DefaultAlgs:    []string{"dfs", "dfs_ga", "ga", "rl"},
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	remote, ok := recommender.(*RemoteDeckRecommender)
	if !ok {
		t.Fatalf("unexpected recommender type: %T", recommender)
	}

	single := remote.ExpandAlgorithms(map[string]any{"algorithm": "dfs", "target": "skill"})
	if len(single) != 1 || single[0]["algorithm"] != "dfs_ga" {
		t.Fatalf("unexpected explicit skill dfs expansion: %+v", single)
	}

	subset := remote.ExpandAlgorithms(map[string]any{
		"algorithm":                 "all",
		"target":                    "skill",
		recommendAlgorithmSubsetKey: []string{"dfs", "ga"},
	})
	if len(subset) != 2 {
		t.Fatalf("unexpected skill subset algorithm count: %+v", subset)
	}
	if subset[0]["algorithm"] != "dfs_ga" || subset[1]["algorithm"] != "ga" {
		t.Fatalf("unexpected skill subset algorithms: %+v", subset)
	}
}

func TestExpandAlgorithmsAppliesAlgorithmSubsetWithoutLeakingLocalKeys(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		MasterdataDir:  "/masterdata",
		DefaultAlgs:    []string{"dfs", "dfs_ga", "ga", "rl"},
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	remote, ok := recommender.(*RemoteDeckRecommender)
	if !ok {
		t.Fatalf("unexpected recommender type: %T", recommender)
	}

	options := remote.ExpandAlgorithms(map[string]any{
		"algorithm":                 "all",
		"target":                    "score",
		recommendAlgorithmSubsetKey: []string{"dfs_ga", "rl", "ga"},
	})
	if len(options) != 3 {
		t.Fatalf("unexpected subset option count: %+v", options)
	}
	if options[0]["algorithm"] != "dfs_ga" || options[1]["algorithm"] != "rl" || options[2]["algorithm"] != "ga" {
		t.Fatalf("unexpected subset algorithms: %+v", options)
	}
	for _, option := range options {
		if _, ok := option[recommendAlgorithmSubsetKey]; ok {
			t.Fatalf("local subset key should not be forwarded: %+v", option)
		}
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

	agg, err := aggregateRemoteRecommendResults("event", options, results)
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

func TestAggregateRemoteRecommendResultsSortsBonusByHigherRateFirst(t *testing.T) {
	options := []map[string]any{
		{"algorithm": "ga", "target": "bonus", "live_type": "multi", "limit": 2},
	}
	results := []remoteBatchRecommendResult{
		{
			Alg: "ga",
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{
				{
					Score:            900,
					LiveScore:        900,
					TotalPower:       300000,
					EventBonusRate:   20,
					MultiLiveScoreUp: 100,
					Cards:            []remoteRecommendCard{{CardID: 1001}},
				},
				{
					Score:            800,
					LiveScore:        800,
					TotalPower:       290000,
					EventBonusRate:   25,
					MultiLiveScoreUp: 90,
					Cards:            []remoteRecommendCard{{CardID: 1002}},
				},
			}},
		},
	}

	agg, err := aggregateRemoteRecommendResults("bonus", options, results)
	if err != nil {
		t.Fatalf("aggregateRemoteRecommendResults() error = %v", err)
	}
	if len(agg.Decks) != 2 {
		t.Fatalf("expected 2 bonus decks, got %+v", agg.Decks)
	}
	if agg.Decks[0].EventBonusRate != 25 {
		t.Fatalf("expected higher bonus deck first, got %+v", agg.Decks)
	}
}

func TestAggregateRemoteRecommendResultsPrefersMysekaiInternalPointOnTie(t *testing.T) {
	options := []map[string]any{
		{"algorithm": "ga", "target": "score", "live_type": "mysekai", "limit": 2},
	}
	results := []remoteBatchRecommendResult{
		{
			Alg: "ga",
			Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{
				{
					MysekaiEventPoint:    1000,
					TotalPower:           460000,
					EventBonusRate:       20,
					SupportDeckBonusRate: 0,
					Cards:                []remoteRecommendCard{{CardID: 1001}},
				},
				{
					MysekaiEventPoint:    1000,
					TotalPower:           460000,
					EventBonusRate:       20,
					SupportDeckBonusRate: 5,
					Cards:                []remoteRecommendCard{{CardID: 1002}},
				},
			}},
		},
	}

	agg, err := aggregateRemoteRecommendResults("mysekai", options, results)
	if err != nil {
		t.Fatalf("aggregateRemoteRecommendResults() error = %v", err)
	}
	if len(agg.Decks) != 2 {
		t.Fatalf("expected 2 mysekai decks, got %+v", agg.Decks)
	}
	if agg.Decks[0].SupportDeckBonusRate != 5 {
		t.Fatalf("expected higher-support mysekai deck to win tie-break, got %+v", agg.Decks[0])
	}
}

func TestExpandRecommendBatchOptionsAddsFallbacksForMysekaiRL(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		MasterdataDir:  "/masterdata",
		DefaultAlgs:    []string{"dfs_ga", "ga", "rl"},
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	options := expandRecommendBatchOptions(recommender, "mysekai", map[string]any{
		"algorithm": "rl",
		"live_type": "mysekai",
		"target":    "score",
	})
	if len(options) != 3 {
		t.Fatalf("expected rl mysekai fallback batch to include 3 algorithms, got %+v", options)
	}
	if options[0]["algorithm"] != "rl" || options[1]["algorithm"] != "ga" || options[2]["algorithm"] != "dfs_ga" {
		t.Fatalf("unexpected rl mysekai fallback batch order: %+v", options)
	}
	for _, item := range options {
		gaOptions, ok := item["ga_options"].(map[string]any)
		if !ok {
			t.Fatalf("expected rl mysekai fallback to include ga_options, got %+v", item)
		}
		if gaOptions["max_iter"] != 256 || gaOptions["max_no_improve_iter"] != 6 {
			t.Fatalf("unexpected rl mysekai ga_options: %+v", gaOptions)
		}
		if gaOptions["pop_size"] != 8000 || gaOptions["parent_size"] != 800 || gaOptions["elite_size"] != 80 {
			t.Fatalf("unexpected rl mysekai population tuning: %+v", gaOptions)
		}
	}

	plain := expandRecommendBatchOptions(recommender, "event", map[string]any{
		"algorithm": "rl",
		"live_type": "multi",
		"target":    "score",
	})
	if len(plain) != 1 || plain[0]["algorithm"] != "rl" {
		t.Fatalf("non-mysekai rl request should stay single-algorithm, got %+v", plain)
	}
	if _, ok := plain[0]["ga_options"]; ok {
		t.Fatalf("non-mysekai rl request should not inject ga_options, got %+v", plain[0])
	}
}

func TestExpandRecommendBatchOptionsForcesBonusToExactDFS(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		MasterdataDir:  "/masterdata",
		DefaultAlgs:    []string{"dfs_ga", "ga", "rl"},
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	options := expandRecommendBatchOptions(recommender, "bonus", map[string]any{
		"algorithm":                 "all",
		"live_type":                 "solo",
		"target":                    "bonus",
		"target_bonus_list":         []int{150, 160},
		recommendAlgorithmSubsetKey: []string{"rl", "ga", "dfs_ga"},
		"ga_options": map[string]any{
			"max_iter": 512,
		},
	})
	if len(options) != 1 {
		t.Fatalf("expected bonus batch to collapse to one exact dfs request, got %+v", options)
	}
	if options[0]["algorithm"] != "dfs" {
		t.Fatalf("expected bonus batch to force dfs, got %+v", options[0])
	}
	if _, ok := options[0][recommendAlgorithmSubsetKey]; ok {
		t.Fatalf("bonus dfs request should not keep algorithm subset, got %+v", options[0])
	}
	if _, ok := options[0]["ga_options"]; ok {
		t.Fatalf("bonus dfs request should not keep ga_options, got %+v", options[0])
	}
}

func TestExpandRecommendBatchOptionsPreservesExplicitMysekaiRLGaOptions(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		MasterdataDir:  "/masterdata",
		DefaultAlgs:    []string{"dfs_ga", "ga", "rl"},
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	options := expandRecommendBatchOptions(recommender, "mysekai", map[string]any{
		"algorithm": "rl",
		"live_type": "mysekai",
		"target":    "score",
		"ga_options": map[string]any{
			"max_iter": 1024,
			"seed":     42,
		},
	})
	if len(options) != 3 {
		t.Fatalf("expected rl mysekai fallback batch to include 3 algorithms, got %+v", options)
	}

	gaOptions, ok := options[0]["ga_options"].(map[string]any)
	if !ok {
		t.Fatalf("expected explicit ga_options to survive, got %+v", options[0])
	}
	if gaOptions["max_iter"] != 1024 {
		t.Fatalf("expected explicit max_iter to be preserved, got %+v", gaOptions)
	}
	if gaOptions["seed"] != 42 {
		t.Fatalf("expected explicit seed to be preserved, got %+v", gaOptions)
	}
	if gaOptions["pop_size"] != 8000 || gaOptions["parent_size"] != 800 || gaOptions["elite_size"] != 80 {
		t.Fatalf("expected tuned defaults to be merged, got %+v", gaOptions)
	}
}

func TestExpandRecommendBatchOptionsAddsTuningForMysekaiAll(t *testing.T) {
	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: "http://example.com",
		MasterdataDir:  "/masterdata",
		DefaultAlgs:    []string{"dfs_ga", "ga", "rl"},
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	options := expandRecommendBatchOptions(recommender, "mysekai", map[string]any{
		"algorithm": "all",
		"live_type": "mysekai",
		"target":    "score",
	})
	if len(options) != 3 {
		t.Fatalf("expected mysekai all batch to include 3 algorithms, got %+v", options)
	}
	if options[0]["algorithm"] != "dfs_ga" || options[1]["algorithm"] != "ga" || options[2]["algorithm"] != "rl" {
		t.Fatalf("unexpected mysekai all batch order: %+v", options)
	}
	for _, item := range options {
		gaOptions, ok := item["ga_options"].(map[string]any)
		if !ok {
			t.Fatalf("expected mysekai all batch to include ga_options, got %+v", item)
		}
		if gaOptions["max_iter"] != 256 || gaOptions["pop_size"] != 8000 {
			t.Fatalf("unexpected mysekai all ga_options: %+v", gaOptions)
		}
	}

	plain := expandRecommendBatchOptions(recommender, "event", map[string]any{
		"algorithm": "all",
		"live_type": "multi",
		"target":    "score",
	})
	for _, item := range plain {
		if _, ok := item["ga_options"]; ok {
			t.Fatalf("non-mysekai all request should not inject ga_options, got %+v", item)
		}
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
			if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&payload); err != nil {
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
	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.defaultAlgs = []string{"ga"}
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	recommender.maxRetries = 0
	recommender.logger = logger.NewLogger("DeckRemoteTest", "DEBUG", nil)

	// Simulate a stale Cloud-side ready cache after deck-service restart.
	state := testRemoteTargetState(t, recommender)
	state.masterdataReady = true
	state.musicMetaHash = hashPayload(musicMeta)

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
	if masterdataCalls.Load() != 0 {
		t.Fatalf("music meta rewarm should not refresh masterdata, got %d", masterdataCalls.Load())
	}
	if musicMetaCalls.Load() != 1 {
		t.Fatalf("expected 1 music meta rewarm call, got %d", musicMetaCalls.Load())
	}
	if recommendCalls.Load() != 2 {
		t.Fatalf("expected 2 recommend calls, got %d", recommendCalls.Load())
	}
}

func TestRemoteRecommendRewarmsOnLogicalMasterdataError(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/cache_userdata":
			_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
		case "/recommend":
			call := recommendCalls.Add(1)
			if call == 1 {
				_, _ = w.Write([]byte(`[
					{
						"alg": "ga",
						"error": "Master data not found for region: jp"
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
	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.defaultAlgs = []string{"ga"}
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	recommender.maxRetries = 0
	recommender.logger = logger.NewLogger("DeckRemoteTest", "DEBUG", nil)

	state := testRemoteTargetState(t, recommender)
	state.masterdataReady = true
	state.musicMetaHash = hashPayload(musicMeta)

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
	if musicMetaCalls.Load() != 0 {
		t.Fatalf("masterdata rewarm should not refresh music metas, got %d", musicMetaCalls.Load())
	}
	if recommendCalls.Load() != 2 {
		t.Fatalf("expected 2 recommend calls, got %d", recommendCalls.Load())
	}
}

func TestEnsureReadyDeduplicatesConcurrentWarmups(t *testing.T) {
	var masterdataCalls atomic.Int32
	var musicMetaCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/update/masterdata":
			masterdataCalls.Add(1)
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/update/musicmetas/string":
			musicMetaCalls.Add(1)
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newRemoteEngineProvider(RecommendConfig{
		ServiceBaseURL: server.URL,
		MasterdataDir:  "/masterdata",
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	remote, ok := recommender.(*RemoteDeckRecommender)
	if !ok {
		t.Fatalf("unexpected recommender type: %T", recommender)
	}

	firstMeta := []byte(`[{"music_id":10000,"difficulty":"master"}]`)

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			exec := testRemoteExecution(t, remote)
			defer exec.Release()
			errs <- remote.ensureReady(exec, "jp", firstMeta, "")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("ensureReady() error = %v", err)
		}
	}

	if masterdataCalls.Load() != 0 {
		t.Fatalf("expected 0 masterdata updates, got %d", masterdataCalls.Load())
	}
	if musicMetaCalls.Load() != 1 {
		t.Fatalf("expected 1 music meta update, got %d", musicMetaCalls.Load())
	}

	exec := testRemoteExecution(t, remote)
	if err := remote.ensureReady(exec, "jp", firstMeta, ""); err != nil {
		t.Fatalf("ensureReady() repeat error = %v", err)
	}
	exec.Release()
	if masterdataCalls.Load() != 0 || musicMetaCalls.Load() != 1 {
		t.Fatalf("repeat ensureReady should not rewarm, got masterdata=%d music=%d", masterdataCalls.Load(), musicMetaCalls.Load())
	}

	secondMeta := []byte(`[{"music_id":10001,"difficulty":"expert"}]`)
	exec = testRemoteExecution(t, remote)
	if err := remote.ensureReady(exec, "jp", secondMeta, ""); err != nil {
		t.Fatalf("ensureReady() new meta error = %v", err)
	}
	exec.Release()
	if masterdataCalls.Load() != 0 {
		t.Fatalf("new meta should not refresh masterdata, got %d", masterdataCalls.Load())
	}
	if musicMetaCalls.Load() != 2 {
		t.Fatalf("new meta should refresh music metas once, got %d", musicMetaCalls.Load())
	}
}

func TestRemoteRecommendBatchKeepsSingleRequestOnSameTarget(t *testing.T) {
	type counts struct {
		masterdata atomic.Int32
		musicMeta  atomic.Int32
		cache      atomic.Int32
		recommend  atomic.Int32
	}

	newServer := func(counter *counts) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			switch strings.TrimSuffix(r.URL.Path, "/") {
			case "/update/masterdata":
				counter.masterdata.Add(1)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			case "/update/musicmetas/string":
				counter.musicMeta.Add(1)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			case "/cache_userdata":
				counter.cache.Add(1)
				_, _ = w.Write([]byte(`{"userdata_hash":"test-userdata-hash"}`))
			case "/recommend":
				counter.recommend.Add(1)
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
	}

	var first counts
	var second counts
	firstServer := newServer(&first)
	defer firstServer.Close()
	secondServer := newServer(&second)
	defer secondServer.Close()

	provider := newRemoteEngineProvider(RecommendConfig{
		MasterdataDir: "/masterdata",
		Targets: []upstream.TargetConfig{
			{Name: "first", BaseURL: firstServer.URL, Concurrency: 1},
			{Name: "second", BaseURL: secondServer.URL, Concurrency: 1},
		},
		DefaultAlgs: []string{"ga"},
	})

	recommender, err := provider.Get("jp")
	if err != nil {
		t.Fatalf("provider.Get() error = %v", err)
	}

	result, err := recommender.Recommend(RecommendRequest{
		Region:    "jp",
		UserData:  []byte(`{"user":"ok"}`),
		MusicMeta: []byte(`[{"music_id":10000,"difficulty":"master"}]`),
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

	type total struct {
		masterdata int32
		musicMeta  int32
		cache      int32
		recommend  int32
	}
	totals := []total{
		{masterdata: first.masterdata.Load(), musicMeta: first.musicMeta.Load(), cache: first.cache.Load(), recommend: first.recommend.Load()},
		{masterdata: second.masterdata.Load(), musicMeta: second.musicMeta.Load(), cache: second.cache.Load(), recommend: second.recommend.Load()},
	}

	activeServers := 0
	for _, item := range totals {
		if item.cache > 0 || item.recommend > 0 {
			activeServers++
			if item.cache != 1 || item.recommend != 1 {
				t.Fatalf("expected chosen target to handle cache and recommend exactly once, got %+v", item)
			}
			if item.masterdata != 0 || item.musicMeta != 1 {
				t.Fatalf("expected warmup calls to stay on the same target, got %+v", item)
			}
		}
	}
	if activeServers != 1 {
		t.Fatalf("expected exactly one target to serve the whole request, got %+v", totals)
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
	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.defaultAlgs = []string{"ga"}
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	recommender.maxRetries = 0
	recommender.logger = logger.NewLogger("DeckRemoteTest", "DEBUG", nil)

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
			if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&payload); err != nil {
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

	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.defaultAlgs = []string{"ga"}
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	recommender.maxRetries = 0
	recommender.logger = logger.NewLogger("DeckRemoteTest", "DEBUG", nil)

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
	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.defaultAlgs = []string{"ga"}
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	recommender.maxRetries = 0
	recommender.logger = logger.NewLogger("DeckRemoteTest", "DEBUG", nil)
	recommender.now = func() time.Time {
		return now
	}
	state := testRemoteTargetState(t, recommender)
	state.consecutiveFailures.Store(maxConsecutiveFailures)
	state.lastFailureAtNanos.Store(now.Add(-circuitBreakerCooldown - time.Second).UnixNano())

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
	if failures := state.consecutiveFailures.Load(); failures != 0 {
		t.Fatalf("expected circuit breaker to reset after successful request, got %d failures", failures)
	}
}

func TestRemoteRecommendSkipsUnhealthyTargetDuringAcquire(t *testing.T) {
	var unhealthyCalls atomic.Int32
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unhealthyCalls.Add(1)
		defer r.Body.Close()
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
	}))
	defer unhealthyServer.Close()

	var cacheCalls atomic.Int32
	var recommendCalls atomic.Int32
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						"total_power": 200,
						"multi_live_score_up": 110,
						"cards": []
					}]
				}
			}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer healthyServer.Close()

	targets := upstream.ResolveTargets("", []upstream.TargetConfig{
		{Name: "unhealthy", BaseURL: unhealthyServer.URL},
		{Name: "healthy", BaseURL: healthyServer.URL},
	}, "deck-service")
	now := time.Now()
	recommender := newTestRemoteDeckRecommenderWithTargets(targets, http.DefaultClient)
	recommender.defaultAlgs = []string{"ga"}
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	recommender.maxRetries = 0
	recommender.logger = logger.NewLogger("DeckRemoteTest", "DEBUG", nil)
	recommender.now = func() time.Time {
		return now
	}

	unhealthyState := recommender.targetStates[remoteTargetKey(targets[0])]
	unhealthyState.consecutiveFailures.Store(targetAssignmentSkipFailures)
	unhealthyState.lastFailureAtNanos.Store(now.UnixNano())
	unhealthyState.lastHealthProbeAtNanos.Store(now.UnixNano())

	result, err := recommender.Recommend(RecommendRequest{
		Region:    "jp",
		UserData:  []byte(`{"user":"ok"}`),
		MusicMeta: []byte(`[{"music_id":10000,"difficulty":"master"}]`),
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
	if got := unhealthyCalls.Load(); got != 0 {
		t.Fatalf("expected unhealthy target not to be called, got %d calls", got)
	}
	if got := cacheCalls.Load(); got != 1 {
		t.Fatalf("expected healthy cache_userdata to run once, got %d", got)
	}
	if got := recommendCalls.Load(); got != 1 {
		t.Fatalf("expected healthy recommend to run once, got %d", got)
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

	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.defaultAlgs = []string{"ga"}
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	recommender.maxRetries = 0
	recommender.logger = logger.NewLogger("DeckRemoteTest", "DEBUG", nil)

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
	state := testRemoteTargetState(t, recommender)
	if failures := state.consecutiveFailures.Load(); failures != 0 {
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

	recommender := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	recommender.defaultAlgs = []string{"ga"}
	recommender.masterdataDir = "/masterdata"
	recommender.region = "jp"
	recommender.maxRetries = 0
	recommender.logger = logger.NewLogger("DeckRemoteTest", "DEBUG", nil)
	recommender.now = func() time.Time {
		return now
	}
	state := testRemoteTargetState(t, recommender)
	state.consecutiveFailures.Store(maxConsecutiveFailures)
	state.lastFailureAtNanos.Store(now.UnixNano())

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
	if failures := state.consecutiveFailures.Load(); failures != 0 {
		t.Fatalf("expected health probe to reset circuit breaker, got %d failures", failures)
	}
}
