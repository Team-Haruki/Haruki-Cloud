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
