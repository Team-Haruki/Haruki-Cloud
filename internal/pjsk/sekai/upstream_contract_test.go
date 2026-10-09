package sekai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/upstreamerr"
)

// TestToolboxResponsesClassifyByContract drives the real Toolbox client with
// each error answer the Toolbox sends and checks the classification the
// replies depend on (see upstreamerr/contract.go).
func TestToolboxResponsesClassifyByContract(t *testing.T) {
	cases := []struct {
		status   int
		body     string
		kind     upstreamerr.Kind
		sentinel error
	}{
		{http.StatusForbidden, `{"message":"forbidden: invalid platform or platform_user_id for this user"}`, upstreamerr.KindAccessDenied, ErrInvalidPlatformUser},
		{http.StatusForbidden, `{"message":"forbidden: account owner is banned"}`, upstreamerr.KindOwnerBanned, ErrAccountOwnerBanned},
		{http.StatusForbidden, `{"message":"forbidden"}`, upstreamerr.KindAccessDenied, nil},
		{http.StatusNotFound, `{"message":"account binding not found"}`, upstreamerr.KindAccountNotBound, ErrAccountBindingNotFound},
		{http.StatusNotFound, `{"message":"game data not found"}`, upstreamerr.KindDataNotUploaded, ErrGameDataNotFound},
		{http.StatusUnauthorized, `{"message":"invalid token"}`, upstreamerr.KindAuth, nil},
		{http.StatusBadRequest, `{"message":"bad request"}`, upstreamerr.KindRejected, nil},
	}
	for _, tc := range cases {
		client := newToolboxResponseClient(t, tc.status, tc.body)
		_, err := client.GetSuiteData("jp", 1001, "qq", "10001")
		class, ok := upstreamerr.Classify(err)
		if !ok || class.Service != upstreamerr.ServiceToolbox || class.Kind != tc.kind {
			t.Errorf("toolbox %d %s: Classify = %+v (ok %v), want %s", tc.status, tc.body, class, ok, tc.kind)
		}
		if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
			t.Errorf("toolbox %d %s: error %v is not %v", tc.status, tc.body, err, tc.sentinel)
		}
	}
}

// TestSekaiAPIResponsesClassifyByContract drives the real SekaiAPI client.
func TestSekaiAPIResponsesClassifyByContract(t *testing.T) {
	cases := []struct {
		status int
		body   string
		kind   upstreamerr.Kind
	}{
		{http.StatusNotFound, `{"message":"user not found"}`, upstreamerr.KindPlayerNotFound},
		{http.StatusUnauthorized, `{"message":"missing token"}`, upstreamerr.KindAuth},
		{http.StatusForbidden, `{"message":"token is not authorized for this server"}`, upstreamerr.KindAuth},
		{http.StatusBadRequest, `{"message":"invalid api type"}`, upstreamerr.KindRejected},
	}
	for _, tc := range cases {
		client := newSekaiResponseClient(t, tc.status, tc.body)
		_, err := client.GetUserProfile("jp", "1001")
		class, ok := upstreamerr.Classify(err)
		if !ok || class.Kind != tc.kind {
			t.Errorf("sekai api %d %s: Classify = %+v (ok %v), want %s", tc.status, tc.body, class, ok, tc.kind)
		}
	}
}

// TestTrackerResponsesClassifyByContract drives the real tracker client.
func TestTrackerResponsesClassifyByContract(t *testing.T) {
	cases := []struct {
		status int
		body   string
		kind   upstreamerr.Kind
	}{
		{http.StatusNotFound, `{"message":"ranking record not found"}`, upstreamerr.KindRankingNotFound},
		{http.StatusTooManyRequests, `{"message":"slow down"}`, upstreamerr.KindRateLimited},
		{http.StatusBadRequest, `{"message":"invalid server"}`, upstreamerr.KindRegionUnsupported},
		{http.StatusBadRequest, `{"message":"no heartbeat found for event 1"}`, upstreamerr.KindNoRankingData},
	}
	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		client := NewTrackerClient(&config.TrackerConfig{BaseURL: server.URL})
		_, err := client.GetEventStatus("jp", 1)
		server.Close()
		class, ok := upstreamerr.Classify(err)
		if !ok || class.Kind != tc.kind {
			t.Errorf("tracker %d %s: Classify = %+v (ok %v), want %s", tc.status, tc.body, class, ok, tc.kind)
		}
	}
}

// TestTransportFailuresAreTaggedWithTheirService checks that an unreachable
// upstream is attributed to the right service and its URL is not in the
// error text.
func TestTransportFailuresAreTaggedWithTheirService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: url})
	client.http.SetRetryCount(0)
	_, err := client.GetSuiteData("jp", 1001, "qq", "10001")
	class, ok := upstreamerr.Classify(err)
	if !ok || class.Service != upstreamerr.ServiceToolbox || class.Kind != upstreamerr.KindUnavailable {
		t.Fatalf("Classify(unreachable toolbox) = %+v (ok %v), error %v", class, ok, err)
	}
}
