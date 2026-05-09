package sk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteForecastProviderFetchesLocalForecast(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prediction/cn/total" && r.URL.Path != "/prediction/cn" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"region": "cn",
			"event_id": 165,
			"updated_at": 1777537304,
			"lines": [{
				"leaderboard_scope": "total",
				"current_timestamp": 1777536872,
				"rows": [
					{"rank": 100, "prediction": 58833709, "current_timestamp": 1777536873},
					{"rank": 200, "prediction": null, "synthetic_rank": true},
					{"rank": 300, "prediction": 26613992}
				]
			}]
		}`))
	}))
	defer server.Close()

	provider := NewRemoteForecastProviderWithConfig(ForecastConfig{LocalBaseURL: server.URL})
	got, err := provider.fetchLocalForecast(context.Background(), "cn", 165, map[int]struct{}{
		100: {},
		200: {},
		300: {},
	})
	if err != nil {
		t.Fatalf("fetch local forecast: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected score count: %d", len(got))
	}
	if score := got[100]; score.Score != 58_833_709 || score.Timestamp != 1_777_536_873_000 || score.Source != "local" {
		t.Fatalf("unexpected p100 score: %+v", score)
	}
	if score := got[300]; score.Score != 26_613_992 || score.Timestamp != 1_777_536_872_000 || score.Source != "local" {
		t.Fatalf("unexpected p300 score: %+v", score)
	}
	if _, ok := got[200]; ok {
		t.Fatalf("null prediction should be skipped: %+v", got[200])
	}
}

func TestRemoteForecastProviderFetchesLocalForecastChapterByCharacter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prediction/tw/chapter" && r.URL.Path != "/prediction/tw" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"region": "tw",
			"event_id": 167,
			"leaderboard_scope": "chapter",
			"updated_at": 1777537304,
			"lines": [
				{
					"character_id": 21,
					"leaderboard_scope": "chapter",
					"current_timestamp": 1777536872,
					"rows": [
						{"rank": 100, "prediction": 11111111, "current_timestamp": 1777536873}
					]
				},
				{
					"character_id": 22,
					"leaderboard_scope": "chapter",
					"current_timestamp": 1777536972,
					"rows": [
						{"rank": 100, "prediction": 22222222, "current_timestamp": 1777536973}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	charaID := 22
	provider := NewRemoteForecastProviderWithConfig(ForecastConfig{LocalBaseURL: server.URL})
	got, err := provider.fetchLocalForecastByQuery(context.Background(), ForecastQuery{
		Region:        "tw",
		EventID:       167,
		Scope:         ForecastScopeChapter,
		WlCharacterID: &charaID,
	}, map[int]struct{}{100: {}})
	if err != nil {
		t.Fatalf("fetch local chapter forecast: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("unexpected score count: %d", len(got))
	}
	if score := got[100]; score.Score != 22_222_222 || score.Timestamp != 1_777_536_973_000 || score.Source != "local" {
		t.Fatalf("unexpected chapter p100 score: %+v", score)
	}
}

func TestRemoteForecastProviderLocalForecastRejectsEventMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"region":"cn","event_id":166,"lines":[]}`))
	}))
	defer server.Close()

	provider := NewRemoteForecastProviderWithConfig(ForecastConfig{LocalBaseURL: server.URL})
	if _, err := provider.fetchLocalForecast(context.Background(), "cn", 165, nil); err == nil {
		t.Fatal("expected event mismatch error, got nil")
	}
}
