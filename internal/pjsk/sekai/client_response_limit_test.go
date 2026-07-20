package sekai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/config"
)

func TestSekaiAndTrackerClientsLimitResponseBodies(t *testing.T) {
	sekaiClient := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: "http://sekai.invalid"})
	trackerClient := NewTrackerClient(&config.TrackerConfig{BaseURL: "http://tracker.invalid"})

	for name, limit := range map[string]int{
		"sekai":   sekaiClient.http.ResponseBodyLimit,
		"tracker": trackerClient.http.ResponseBodyLimit,
	} {
		if limit != maxUpstreamResponseBytes {
			t.Fatalf("%s response body limit = %d, want %d", name, limit, maxUpstreamResponseBytes)
		}
	}
}

func TestSekaiAndTrackerClientsRejectOversizedResponseWithoutLeakingBody(t *testing.T) {
	const (
		testLimit = 32
		secret    = "upstream-secret-that-must-not-appear"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(secret, 2)))
	}))
	defer server.Close()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "sekai",
			call: func() error {
				client := NewSekaiAPIClient(&config.SekaiAPIConfig{BaseURL: server.URL})
				client.http.SetResponseBodyLimit(testLimit)
				_, err := client.GetSystem("jp")
				return err
			},
		},
		{
			name: "tracker",
			call: func() error {
				client := NewTrackerClient(&config.TrackerConfig{BaseURL: server.URL})
				client.http.SetResponseBodyLimit(testLimit)
				_, err := client.GetEventStatus("jp", 1)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected oversized response error")
			}
			if !strings.Contains(err.Error(), "response body too large") {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error leaked upstream response body: %v", err)
			}
		})
	}
}
