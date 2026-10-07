package secevent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memCounter struct {
	mu     sync.Mutex
	counts map[string]int64
	ttls   map[string]time.Duration
}

func (m *memCounter) Incr(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts == nil {
		m.counts = map[string]int64{}
		m.ttls = map[string]time.Duration{}
	}
	m.counts[key]++
	return m.counts[key], nil
}

func (m *memCounter) Expire(_ context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttls[key] = ttl
	return nil
}

func TestMonitorAlertsOncePerWindow(t *testing.T) {
	counter := &memCounter{}
	m := New(Config{WebhookURL: "http://example.invalid/hook", Threshold: 3, Window: time.Minute, Node: "test"}, counter)
	var delivered [][]byte
	m.post = func(_ context.Context, payload []byte) error {
		delivered = append(delivered, payload)
		return nil
	}
	m.spawn = func(f func()) { f() }

	ev := Event{Kind: KindAuthFailed, BotID: "42", SourceIP: "192.0.2.9", Reason: "bad credential"}
	for range 5 {
		m.Report(context.Background(), ev)
	}
	if len(delivered) != 1 {
		t.Fatalf("alerts delivered = %d, want exactly 1", len(delivered))
	}
	var alert alertPayload
	if err := json.Unmarshal(delivered[0], &alert); err != nil {
		t.Fatal(err)
	}
	if alert.Kind != "auth_failed" || alert.BotID != "42" || alert.Count != 3 || alert.Threshold != 3 || alert.WindowSeconds != 60 || alert.Node != "test" {
		t.Fatalf("alert = %+v", alert)
	}
	if ttl := counter.ttls["haruki:sec:auth_failed:42"]; ttl != time.Minute {
		t.Fatalf("window ttl = %v", ttl)
	}

	// A different subject counts separately; an event without a bot id is
	// keyed by its source address.
	m.Report(context.Background(), Event{Kind: KindAuthFailed, SourceIP: "198.51.100.1"})
	if counter.counts["haruki:sec:auth_failed:198.51.100.1"] != 1 {
		t.Fatalf("source-keyed count = %v", counter.counts)
	}
}

func TestMonitorDefaultsAndNilSafety(t *testing.T) {
	m := New(Config{}, nil)
	if m.cfg.Threshold != DefaultThreshold || m.cfg.Window != DefaultWindow {
		t.Fatalf("defaults = %+v", m.cfg)
	}
	m.Report(context.Background(), Event{Kind: KindReplayDetected, BotID: "1"}) // no counter: log only
	var nilMonitor *Monitor
	nilMonitor.Report(context.Background(), Event{Kind: KindReplayDetected})
	Report(context.Background(), nil, Event{Kind: KindReplayDetected})
}

// TestAlertPayloadShape pins the webhook body. Toolbox's alert ingest is
// built against exactly these field names, types and omitempty rules; change
// this test only together with the receiver.
func TestAlertPayloadShape(t *testing.T) {
	full, err := json.Marshal(alertPayload{
		Kind:          "auth_failed",
		BotID:         "30042042",
		BuildID:       "b1",
		ClientVersion: "1.2.3",
		SourceIP:      "192.0.2.9",
		Reason:        "bad credential",
		Enforced:      true,
		Count:         5,
		Threshold:     5,
		WindowSeconds: 600,
		Node:          "node-a",
		Time:          "2026-09-02T09:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	const wantFull = `{"kind":"auth_failed","bot_id":"30042042","build_id":"b1","client_version":"1.2.3",` +
		`"source_ip":"192.0.2.9","reason":"bad credential","enforced":true,"count":5,"threshold":5,` +
		`"window_seconds":600,"node":"node-a","time":"2026-09-02T09:00:00Z"}`
	if string(full) != wantFull {
		t.Fatalf("full payload =\n%s\nwant\n%s", full, wantFull)
	}

	minimal, err := json.Marshal(alertPayload{Kind: "policy_unavailable", Count: 1, Threshold: 1, WindowSeconds: 60, Time: "2026-09-02T09:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	const wantMinimal = `{"kind":"policy_unavailable","enforced":false,"count":1,"threshold":1,"window_seconds":60,"time":"2026-09-02T09:00:00Z"}`
	if string(minimal) != wantMinimal {
		t.Fatalf("minimal payload =\n%s\nwant\n%s", minimal, wantMinimal)
	}
}

const testWebhookToken = "tok-3f9a1c-do-not-log"

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// alertOnce builds a monitor with threshold 1 and synchronous delivery,
// raises one alert and returns the captured log output.
func alertOnce(t *testing.T, url, token string) string {
	t.Helper()
	logs := &syncBuffer{}
	m := New(Config{WebhookURL: url, WebhookToken: token, Threshold: 1, Window: time.Minute, Node: "node-a"}, &memCounter{})
	m.logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	m.spawn = func(f func()) { f() }
	m.Report(context.Background(), Event{
		Kind: KindBuildRejected, BotID: "42", BuildID: "b1", ClientVersion: "1.2.3",
		SourceIP: "192.0.2.9", Reason: "unknown build", Enforced: true,
	})
	return logs.String()
}

func assertNoToken(t *testing.T, logs string) {
	t.Helper()
	if strings.Contains(logs, testWebhookToken) {
		t.Fatalf("token leaked into logs:\n%s", logs)
	}
}

func TestWebhookPostHeadersAndBody(t *testing.T) {
	for _, tc := range []struct {
		name, token, wantAuth string
	}{
		{name: "with token", token: testWebhookToken, wantAuth: "Bearer " + testWebhookToken},
		{name: "token is trimmed", token: "  " + testWebhookToken + "\n", wantAuth: "Bearer " + testWebhookToken},
		{name: "without token", token: ""},
		{name: "blank token", token: "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type received struct {
				method, contentType, auth string
				hasAuth                   bool
				body                      []byte
			}
			got := make(chan received, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_, hasAuth := r.Header["Authorization"]
				got <- received{r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), hasAuth, body}
				w.WriteHeader(http.StatusAccepted)
			}))
			defer srv.Close()

			logs := alertOnce(t, srv.URL+"/internal/bot-security/alerts", tc.token)
			assertNoToken(t, logs)
			if strings.Contains(logs, "webhook failed") {
				t.Fatalf("2xx logged as failure:\n%s", logs)
			}

			var r received
			select {
			case r = <-got:
			default:
				t.Fatal("webhook not called")
			}
			if r.method != http.MethodPost || r.contentType != "application/json" {
				t.Fatalf("method = %q, content-type = %q", r.method, r.contentType)
			}
			if tc.wantAuth == "" {
				if r.hasAuth {
					t.Fatalf("Authorization sent without a token: %q", r.auth)
				}
			} else if r.auth != tc.wantAuth {
				t.Fatalf("Authorization = %q, want %q", r.auth, tc.wantAuth)
			}

			var body map[string]any
			if err := json.Unmarshal(r.body, &body); err != nil {
				t.Fatal(err)
			}
			wantTypes := map[string]string{
				"kind": "string", "bot_id": "string", "build_id": "string", "client_version": "string",
				"source_ip": "string", "reason": "string", "enforced": "bool", "count": "number",
				"threshold": "number", "window_seconds": "number", "node": "string", "time": "string",
			}
			if len(body) != len(wantTypes) {
				t.Fatalf("payload keys = %v, want %v", body, wantTypes)
			}
			for key, typ := range wantTypes {
				v, ok := body[key]
				if !ok {
					t.Fatalf("payload missing %q: %s", key, r.body)
				}
				var gotType string
				switch v.(type) {
				case string:
					gotType = "string"
				case bool:
					gotType = "bool"
				case float64:
					gotType = "number"
				}
				if gotType != typ {
					t.Fatalf("payload %q has type %T, want %s", key, v, typ)
				}
			}
			if body["kind"] != "build_rejected" || body["bot_id"] != "42" || body["enforced"] != true ||
				body["count"] != float64(1) || body["threshold"] != float64(1) || body["window_seconds"] != float64(60) || body["node"] != "node-a" {
				t.Fatalf("payload = %s", r.body)
			}
			if _, err := time.Parse(time.RFC3339, body["time"].(string)); err != nil {
				t.Fatalf("time %q: %v", body["time"], err)
			}
		})
	}
}

func TestWebhookDoesNotFollowRedirects(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	logs := alertOnce(t, redirector.URL, testWebhookToken)
	assertNoToken(t, logs)
	if n := targetHits.Load(); n != 0 {
		t.Fatalf("redirect followed: target hit %d times", n)
	}
	if !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, `msg="security alert webhook failed"`) || !strings.Contains(logs, "status=307") {
		t.Fatalf("redirect not reported as a WARN failure with its status:\n%s", logs)
	}
}

func TestWebhookNon2xxLogsStatusOnly(t *testing.T) {
	const body = "upstream-said-something-private"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	logs := alertOnce(t, srv.URL, testWebhookToken)
	assertNoToken(t, logs)
	var failure string
	for line := range strings.Lines(logs) {
		if strings.Contains(line, "webhook failed") {
			failure = line
		}
	}
	if !strings.Contains(failure, "level=WARN") || !strings.Contains(failure, "status=401") || !strings.Contains(failure, "kind=build_rejected") {
		t.Fatalf("failure line = %q", failure)
	}
	if strings.Contains(logs, body) || strings.Contains(failure, "error_type") {
		t.Fatalf("failure log carries more than the status:\n%s", logs)
	}
}

func TestWebhookTransportErrorDoesNotLeakToken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	logs := alertOnce(t, url+"/hook", testWebhookToken)
	assertNoToken(t, logs)
	if !strings.Contains(logs, "webhook failed") || !strings.Contains(logs, "error_type=") {
		t.Fatalf("transport error not reported:\n%s", logs)
	}
}

func TestWebhookEmptyURLSkipsDelivery(t *testing.T) {
	logs := alertOnce(t, "  ", testWebhookToken)
	assertNoToken(t, logs)
	if strings.Contains(logs, "webhook failed") || !strings.Contains(logs, "security alert") {
		t.Fatalf("logs = %s", logs)
	}
}
