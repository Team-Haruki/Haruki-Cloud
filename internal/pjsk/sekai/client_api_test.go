package sekai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
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

func TestSekaiAPIClientGetsMySekaiHousingCompetitionList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/jp/user/mysekai/housing-competition/123/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("isLottery"); got != "True" {
			t.Fatalf("unexpected isLottery query: %q", got)
		}
		if token := r.Header.Get(tokenHeader); token != "legacy-secret" {
			t.Fatalf("unexpected token header: %q", token)
		}
		_, _ = w.Write([]byte(`{"entries":[]}`))
	}))
	defer server.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{
		BaseURL: server.URL,
		Token:   "legacy-secret",
	})

	body, err := client.GetMySekaiHousingCompetitionList("jp", 123, true)
	if err != nil {
		t.Fatalf("GetMySekaiHousingCompetitionList() error = %v", err)
	}
	if string(body) != `{"entries":[]}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestSekaiAPIClientEntersMySekaiHousingCompetitionEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/jp/user/mysekai/housing-competition/123/mysekai-owner/456/entry" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("mysekaiOwnerUserSubmittedAt"); got != "1710000000000" {
			t.Fatalf("unexpected submittedAt query: %q", got)
		}
		if got := r.URL.Query().Get("isBackNumber"); got != "true" {
			t.Fatalf("unexpected isBackNumber query: %q", got)
		}
		if token := r.Header.Get(tokenHeader); token != "legacy-secret" {
			t.Fatalf("unexpected token header: %q", token)
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Fatalf("expected empty POST body, got %q", string(body))
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{
		BaseURL: server.URL,
		Token:   "legacy-secret",
	})

	body, err := client.EnterMySekaiHousingCompetitionEntry("jp", 123, 456, 1710000000000, true)
	if err != nil {
		t.Fatalf("EnterMySekaiHousingCompetitionEntry() error = %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestSekaiAPIClientGetsMySekaiHousingBackNumbersAndThumbnail(t *testing.T) {
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = true
		switch r.URL.Path {
		case "/api/jp/mysekai/housing-competition/back-number-top-list":
			_, _ = w.Write([]byte(`{"top":[]}`))
		case "/api/jp/mysekai/housing-competition/987/back-number-list":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case "/image/jp/mysekai-housing/hash/uuid":
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL})

	if _, err := client.GetMySekaiHousingCompetitionBackNumberTopList("jp"); err != nil {
		t.Fatalf("GetMySekaiHousingCompetitionBackNumberTopList() error = %v", err)
	}
	if _, err := client.GetMySekaiHousingCompetitionBackNumberList("jp", 987); err != nil {
		t.Fatalf("GetMySekaiHousingCompetitionBackNumberList() error = %v", err)
	}
	body, err := client.GetMySekaiHousingThumbnail("jp", "/hash/uuid")
	if err != nil {
		t.Fatalf("GetMySekaiHousingThumbnail() error = %v", err)
	}
	if string(body) != "png" {
		t.Fatalf("unexpected thumbnail body: %s", body)
	}
	for _, path := range []string{
		"/api/jp/mysekai/housing-competition/back-number-top-list",
		"/api/jp/mysekai/housing-competition/987/back-number-list",
		"/image/jp/mysekai-housing/hash/uuid",
	} {
		if !seen[path] {
			t.Fatalf("expected path %s to be requested", path)
		}
	}
}

func TestSekaiAPIClientContextSupportsCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.WithContext(ctx).GetSystem("jp")
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("sekai request did not start")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("GetSystem() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("sekai request did not stop after context cancellation")
	}
}

func TestSekaiAPIClientContextRecordsHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL})
	ctx, trace := commandtrace.WithTrace(context.Background())
	if _, err := client.WithContext(ctx).GetSystem("jp"); err != nil {
		t.Fatalf("GetSystem() error = %v", err)
	}

	operations := trace.Snapshot().Operations
	var httpCount int
	for _, operation := range operations {
		if operation.Name == "sekai.http" {
			httpCount = operation.Count
		}
	}
	if httpCount != 1 {
		t.Fatalf("unexpected operations: %+v", operations)
	}
}
