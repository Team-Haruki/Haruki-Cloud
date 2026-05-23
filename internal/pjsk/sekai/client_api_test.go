package sekai

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"
)

func TestSekaiAPIClientDistributesRequestsAcrossTargets(t *testing.T) {
	var firstHits atomic.Int32
	var secondHits atomic.Int32

	newServer := func(counter *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/jp/system" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if token := r.Header.Get(tokenHeader); token != "sekai-secret" {
				t.Fatalf("unexpected token header: %q", token)
			}
			counter.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}))
	}

	first := newServer(&firstHits)
	defer first.Close()
	second := newServer(&secondHits)
	defer second.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{
		Token: "sekai-secret",
		Targets: []upstream.TargetConfig{
			{Name: "first", BaseURL: first.URL, Concurrency: 1},
			{Name: "second", BaseURL: second.URL, Concurrency: 1},
		},
	})

	if _, err := client.GetSystem("jp"); err != nil {
		t.Fatalf("first GetSystem() error = %v", err)
	}
	if _, err := client.GetSystem("jp"); err != nil {
		t.Fatalf("second GetSystem() error = %v", err)
	}

	if got := firstHits.Load(); got != 1 {
		t.Fatalf("expected first target to receive 1 request, got %d", got)
	}
	if got := secondHits.Load(); got != 1 {
		t.Fatalf("expected second target to receive 1 request, got %d", got)
	}
}

func TestSekaiAPIClientSupportsLegacyBaseURL(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/jp/information" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if token := r.Header.Get(tokenHeader); token != "legacy-secret" {
			t.Fatalf("unexpected token header: %q", token)
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{
		BaseURL: server.URL,
		Token:   "legacy-secret",
	})

	if _, err := client.GetInformation("jp"); err != nil {
		t.Fatalf("GetInformation() error = %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected legacy base_url to receive 1 request, got %d", got)
	}
}

func TestSekaiAPIClientGetsCustomMusicScoreBlob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/image/jp/blob/custom-music-score/full/hash-a/hash-b" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if token := r.Header.Get(tokenHeader); token != "legacy-secret" {
			t.Fatalf("unexpected token header: %q", token)
		}
		_, _ = w.Write([]byte(`{"NoteList":[]}`))
	}))
	defer server.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{
		BaseURL: server.URL,
		Token:   "legacy-secret",
	})

	body, err := client.GetCustomMusicScore("jp", "/hash-a/hash-b")
	if err != nil {
		t.Fatalf("GetCustomMusicScore() error = %v", err)
	}
	if string(body) != `{"NoteList":[]}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestSekaiAPIClientGetsCustomMusicScorePublishedByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/jp/user/%25user_id/custom-music-score/published/search/_g5yakrvqobnfq6hafdob7ed8jwm" {
			t.Fatalf("unexpected path: %s", r.URL.EscapedPath())
		}
		if token := r.Header.Get(tokenHeader); token != "legacy-secret" {
			t.Fatalf("unexpected token header: %q", token)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"userCustomMusicScoreInfoJson": {
				"userCustomMusicScoreId": "_g5yakrvqobnfq6hafdob7ed8jwm",
				"userName": "Maker",
				"musicId": 47,
				"customMusicScoreTags": [9, 6, 4],
				"musicDifficultyType": "master",
				"playLevel": 31,
				"userCustomMusicScoreInfoJson": {
					"musicId": 47,
					"title": "Direct Custom",
					"userCustomMusicScorePath": "hash-a/hash-b"
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{
		BaseURL: server.URL,
		Token:   "legacy-secret",
	})

	item, err := client.GetCustomMusicScorePublished("jp", "_g5yakrvqobnfq6hafdob7ed8jwm")
	if err != nil {
		t.Fatalf("GetCustomMusicScorePublished() error = %v", err)
	}
	if item.UserCustomMusicScoreID != "_g5yakrvqobnfq6hafdob7ed8jwm" {
		t.Fatalf("unexpected score id: %#v", item.UserCustomMusicScoreID)
	}
	if item.UserCustomMusicScoreInfoJSON == nil || item.UserCustomMusicScoreInfoJSON.UserCustomMusicScorePath != "hash-a/hash-b" {
		t.Fatalf("unexpected score info: %#v", item.UserCustomMusicScoreInfoJSON)
	}
	if len(item.CustomMusicScoreTags) != 3 || item.CustomMusicScoreTags[0] != 9 || item.CustomMusicScoreTags[2] != 4 {
		t.Fatalf("unexpected custom score tags: %#v", item.CustomMusicScoreTags)
	}
}
