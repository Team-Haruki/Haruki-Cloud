package sekai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"haruki-cloud/config"
	json "haruki-cloud/internal/jsonutil"
)

type birthdayRequestRecorder struct {
	mu            sync.Mutex
	requests      map[string]int
	authorization string
	userAgent     string
	deleteVersion string
}

func newBirthdayRequestRecorder() *birthdayRequestRecorder {
	return &birthdayRequestRecorder{requests: make(map[string]int)}
}

func (recorder *birthdayRequestRecorder) handler(w http.ResponseWriter, request *http.Request) {
	recorder.record(request)

	switch {
	case request.Method == http.MethodPut && request.URL.Path == "/internal/mysekai-birthday-monitors/7":
		recorder.handleUpsert(w, request)
	case request.Method == http.MethodDelete && request.URL.Path == "/internal/mysekai-birthday-monitors/7":
		w.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodGet && request.URL.Path == "/internal/mysekai-birthday-events/9":
		recorder.handleEventLookup(w, request)
	case request.Method == http.MethodPost && request.URL.Path == "/internal/mysekai-birthday-events/9/ack":
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, request)
	}
}

func (recorder *birthdayRequestRecorder) record(request *http.Request) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.requests[request.Method+" "+request.URL.Path]++
	recorder.authorization = request.Header.Get("Authorization")
	recorder.userAgent = request.Header.Get("User-Agent")
	if request.Method == http.MethodDelete {
		recorder.deleteVersion = request.URL.Query().Get("subscription_version")
	}
}

func (recorder *birthdayRequestRecorder) handleUpsert(w http.ResponseWriter, request *http.Request) {
	var body MysekaiBirthdayMonitorUpsertRequest
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.SubscriptionVersion != "v7" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (recorder *birthdayRequestRecorder) handleEventLookup(w http.ResponseWriter, request *http.Request) {
	_ = json.NewEncoder(w).Encode(MysekaiBirthdayEvent{
		EventID:             "9",
		SubscriptionID:      request.URL.Query().Get("subscription_id"),
		SubscriptionVersion: request.URL.Query().Get("subscription_version"),
		Region:              "jp",
		UID:                 "123",
	})
}

func TestToolboxBirthdayMonitorLifecycle(t *testing.T) {
	recorder := newBirthdayRequestRecorder()
	server := httptest.NewServer(http.HandlerFunc(recorder.handler))
	defer server.Close()

	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL + "/", APIToken: "toolbox-token", UserAgent: "toolbox-test"})
	ctx := context.Background()
	upsert := MysekaiBirthdayMonitorUpsertRequest{SubscriptionID: "7", SubscriptionVersion: "v7", Region: "jp", UID: "123", Materials: []string{"diamond"}}
	if err := client.UpsertMysekaiBirthdayMonitor(ctx, upsert); err != nil {
		t.Fatalf("upsert monitor: %v", err)
	}
	if err := client.DeleteMysekaiBirthdayMonitor(ctx, "7", "v7"); err != nil {
		t.Fatalf("delete monitor: %v", err)
	}
	lookup := MysekaiBirthdayEventLookupRequest{EventID: "9", SubscriptionID: "7", SubscriptionVersion: "v7"}
	event, err := client.GetMysekaiBirthdayEvent(ctx, lookup)
	if err != nil || event.EventID != "9" || event.SubscriptionID != "7" || event.SubscriptionVersion != "v7" {
		t.Fatalf("get birthday event = %#v, %v", event, err)
	}
	if err := client.AckMysekaiBirthdayEvent(ctx, lookup); err != nil {
		t.Fatalf("ack birthday event: %v", err)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.authorization != "toolbox-token" || recorder.userAgent != "toolbox-test" || recorder.deleteVersion != "v7" {
		t.Fatalf("request metadata = auth:%q ua:%q version:%q", recorder.authorization, recorder.userAgent, recorder.deleteVersion)
	}
	for _, key := range []string{
		"PUT /internal/mysekai-birthday-monitors/7",
		"DELETE /internal/mysekai-birthday-monitors/7",
		"GET /internal/mysekai-birthday-events/9",
		"POST /internal/mysekai-birthday-events/9/ack",
	} {
		if recorder.requests[key] != 1 {
			t.Fatalf("request %q count = %d", key, recorder.requests[key])
		}
	}
}

func TestToolboxBirthdayMonitorErrors(t *testing.T) {
	var responseStatus = http.StatusBadGateway
	var responseBody = `{"message":"upstream unavailable"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(responseStatus)
		_, _ = w.Write([]byte(responseBody))
	}))
	defer server.Close()
	client := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
	client.http.SetRetryCount(0)
	ctx := context.Background()

	assertAPIError := func(t *testing.T, err error) {
		t.Helper()
		var apiErr *ToolboxAPIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != responseStatus {
			t.Fatalf("error = %T %v", err, err)
		}
	}
	assertAPIError(t, client.UpsertMysekaiBirthdayMonitor(ctx, MysekaiBirthdayMonitorUpsertRequest{SubscriptionID: "1"}))
	assertAPIError(t, client.DeleteMysekaiBirthdayMonitor(ctx, "1", "v"))
	_, err := client.GetMysekaiBirthdayEvent(ctx, MysekaiBirthdayEventLookupRequest{EventID: "1"})
	assertAPIError(t, err)
	assertAPIError(t, client.AckMysekaiBirthdayEvent(ctx, MysekaiBirthdayEventLookupRequest{EventID: "1"}))

	responseStatus = http.StatusOK
	responseBody = "not-json"
	if _, err := client.GetMysekaiBirthdayEvent(ctx, MysekaiBirthdayEventLookupRequest{EventID: "1"}); err == nil || !strings.Contains(err.Error(), "failed to parse") {
		t.Fatalf("invalid event response error = %v", err)
	}

	for _, missing := range []*HarukiToolboxClient{nil, NewToolboxClient(nil), NewToolboxClient(&config.ToolboxConfig{})} {
		if _, err := missing.internalRequest(context.Background()); !errors.Is(err, ErrClientNotConfigured) {
			t.Fatalf("missing internal request error = %v", err)
		}
		if err := missing.UpsertMysekaiBirthdayMonitor(ctx, MysekaiBirthdayMonitorUpsertRequest{}); !errors.Is(err, ErrClientNotConfigured) {
			t.Fatalf("missing upsert error = %v", err)
		}
	}

	defaultUA := NewToolboxClient(&config.ToolboxConfig{BaseURL: server.URL})
	if strings.TrimSpace(defaultUA.userAgent()) == "" || (*HarukiToolboxClient)(nil).userAgent() == "" {
		t.Fatal("default Toolbox user agent is empty")
	}
}
