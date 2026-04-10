package deck

import (
	"encoding/json"
	"testing"
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
	if len(agg.DeckAlgs) != 1 || agg.DeckAlgs[0] != "dfs+ga" {
		t.Fatalf("unexpected deck algs: %+v", agg.DeckAlgs)
	}
	if agg.CostTimes["dfs"] != 1.2 || agg.CostTimes["ga"] != 2.3 {
		t.Fatalf("unexpected cost times: %+v", agg.CostTimes)
	}
}
