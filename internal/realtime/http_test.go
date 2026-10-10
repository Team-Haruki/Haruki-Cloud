package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/database/pjsk/realtimeevent"
)

func TestHealthz(t *testing.T) {
	env := newTestEnv(t, testConfig())
	status, body := env.get(RouteHealth)
	if status != http.StatusOK || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("healthz = %d %s", status, body)
	}
}

func TestSSERejectsMissingParameters(t *testing.T) {
	env := newTestEnv(t, testConfig())
	for _, query := range []string{
		"?subscription_id=1&subscription_version=v1",
		"?subscription_version=v1&token=t",
		"?subscription_id=1&token=t",
	} {
		if status, _ := env.get(RouteSSE + query); status != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", query, status)
		}
	}
}

func TestSSERejectsInvalidCredentialsWith401(t *testing.T) {
	env := newTestEnv(t, testConfig())
	active := env.activeSubscription()
	inactive := env.createSubscription(subscriptionSpec{active: false})
	expired := env.createSubscription(subscriptionSpec{active: true, expiresIn: -time.Minute})
	cases := map[string]string{
		"wrong token":          fmt.Sprintf("%s?subscription_id=%d&subscription_version=v1&token=v1.wrong", RouteSSE, active.ID),
		"wrong version":        fmt.Sprintf("%s?subscription_id=%d&subscription_version=v2&token=v1.secret", RouteSSE, active.ID),
		"unknown subscription": RouteSSE + "?subscription_id=999999&subscription_version=v1&token=v1.secret",
		"non-numeric id":       RouteSSE + "?subscription_id=abc&subscription_version=v1&token=v1.secret",
		"inactive":             streamPath(inactive, "v1"),
		"expired":              streamPath(expired, "v1"),
	}
	for name, path := range cases {
		status, body := env.get(path)
		if status != http.StatusUnauthorized || !strings.Contains(body, "invalid subscription token") {
			t.Fatalf("%s: %d %s, want 401", name, status, body)
		}
	}
}

func TestSSETransientFailuresAre503(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()

	env.readOnly.Store(true)
	if status, _ := env.get(streamPath(sub, "v1")); status != http.StatusServiceUnavailable {
		t.Fatalf("read-only status = %d, want 503", status)
	}
	env.readOnly.Store(false)

	_ = env.db.Close()
	if status, body := env.get(streamPath(sub, "v1")); status != http.StatusServiceUnavailable || !strings.Contains(body, "unavailable") {
		t.Fatalf("database down = %d %s, want 503", status, body)
	}
	if !strings.Contains(env.logs.String(), "sse validation failed") {
		t.Fatal("validation failure was not logged")
	}
}

func TestSSEStoppingIs503(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	env.svc.Shutdown()
	if status, _ := env.get(streamPath(sub, "v1")); status != http.StatusServiceUnavailable {
		t.Fatalf("status during shutdown = %d, want 503", status)
	}
}

func TestSSEReplaysPendingThenStreamsLive(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	if status := env.ingest(sub, "v1", "evt-1", "toolbox:ref:1"); status != http.StatusOK {
		t.Fatalf("ingest status = %d", status)
	}

	stream := env.connect(fmt.Sprintf("%s?subscription_id=%d&version=v1&token=v1.secret", RouteSSE, sub.ID), nil)
	if first, ok := stream.rawFrame(3 * time.Second); !ok || first.retry != "1500" {
		t.Fatalf("first frame = %+v, want the retry hint", first)
	}
	replayed := stream.mustNext()
	if replayed.event != BirthdayMonitorEventName || replayed.id == "" {
		t.Fatalf("replayed frame = %+v", replayed)
	}
	want := fmt.Sprintf(`{"event_id":"evt-1","subscription_id":"%d","subscription_version":"v1","payload_ref":"toolbox:ref:1","empty_result":false}`, sub.ID)
	if replayed.data != want {
		t.Fatalf("data = %s, want %s", replayed.data, want)
	}

	if status := env.ingest(sub, "v1", "evt-2", ""); status != http.StatusOK {
		t.Fatalf("live ingest status = %d", status)
	}
	live := stream.mustNext()
	var payload map[string]any
	if err := json.Unmarshal([]byte(live.data), &payload); err != nil {
		t.Fatalf("live data: %v", err)
	}
	if payload["event_id"] != "evt-2" || payload["empty_result"] != false {
		t.Fatalf("live payload = %v", payload)
	}
	if _, present := payload["payload_ref"]; present {
		t.Fatalf("empty payload_ref should be omitted: %s", live.data)
	}
	if live.id <= replayed.id && len(live.id) <= len(replayed.id) {
		t.Fatalf("ids not increasing: %s then %s", replayed.id, live.id)
	}
	stream.expectNone(200 * time.Millisecond)
}

func TestReplayLatestPolicyKeepsOnlyNewest(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	env.ingest(sub, "v1", "first", "")
	env.ingest(sub, "v1", "latest", "redis-key")

	stream := env.connect(streamPath(sub, "v1"), nil)
	got := stream.mustNext()
	if !strings.Contains(got.data, `"event_id":"latest"`) || !strings.Contains(got.data, `"payload_ref":"redis-key"`) {
		t.Fatalf("replayed = %s", got.data)
	}
	stream.expectNone(200 * time.Millisecond)

	row, err := env.db.RealtimeEvent.Query().Where(realtimeevent.EventID("first")).Only(context.Background())
	if err != nil || row.SupersededAt == nil || row.SupersededReason != ReasonReplaced {
		t.Fatalf("first row = %+v, %v", row, err)
	}
}

func TestReplayAllPolicyKeepsNewestN(t *testing.T) {
	cfg := testConfig()
	cfg.ReplayPolicy = config.EventsReplayAll
	cfg.ReplayMax = 2
	env := newTestEnv(t, cfg)
	sub := env.activeSubscription()
	for _, id := range []string{"a", "b", "c"} {
		env.ingest(sub, "v1", id, "")
	}
	stream := env.connect(streamPath(sub, "v1"), nil)
	for _, want := range []string{"b", "c"} {
		if got := stream.mustNext(); !strings.Contains(got.data, `"event_id":"`+want+`"`) {
			t.Fatalf("replayed %s, want %s", got.data, want)
		}
	}
	stream.expectNone(200 * time.Millisecond)
	row, err := env.db.RealtimeEvent.Query().Where(realtimeevent.EventID("a")).Only(context.Background())
	if err != nil || row.SupersededReason != ReasonOverflow {
		t.Fatalf("oldest row = %+v, %v", row, err)
	}
}

func TestReplaySurvivesRestart(t *testing.T) {
	db := openTestDB(t)
	first := newTestEnvWithDB(t, testConfig(), db)
	sub := first.activeSubscription()
	if status := first.ingest(sub, "v1", "persisted", ""); status != http.StatusOK {
		t.Fatalf("ingest status = %d", status)
	}
	first.svc.Shutdown()
	_ = first.app.Shutdown()

	second := newTestEnvWithDB(t, testConfig(), db)
	stream := second.connect(streamPath(sub, "v1"), nil)
	if got := stream.mustNext(); !strings.Contains(got.data, `"event_id":"persisted"`) {
		t.Fatalf("replayed after restart = %s", got.data)
	}
}

func TestLastEventIDSkipsDeliveredEvents(t *testing.T) {
	cfg := testConfig()
	cfg.ReplayPolicy = config.EventsReplayAll
	env := newTestEnv(t, cfg)
	sub := env.activeSubscription()
	env.ingest(sub, "v1", "old", "")
	env.ingest(sub, "v1", "new", "")
	old, err := env.db.RealtimeEvent.Query().Where(realtimeevent.EventID("old")).Only(context.Background())
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	stream := env.connect(streamPath(sub, "v1"), map[string]string{"Last-Event-ID": fmt.Sprint(old.ID)})
	if got := stream.mustNext(); !strings.Contains(got.data, `"event_id":"new"`) {
		t.Fatalf("replayed = %s", got.data)
	}
	stream.expectNone(200 * time.Millisecond)
	if parseLastEventID("-4") != 0 || parseLastEventID("x") != 0 || parseLastEventID(" 7 ") != 7 {
		t.Fatal("parseLastEventID")
	}
}

func TestIngestAuthorization(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	body := fmt.Sprintf(`{"event_id":"e","subscription_id":"%d","subscription_version":"v1"}`, sub.ID)
	if status, _ := env.post(RouteIngest, "", body); status != http.StatusUnauthorized {
		t.Fatalf("no token = %d", status)
	}
	if status, _ := env.post(RouteIngest, "Bearer wrong", body); status != http.StatusUnauthorized {
		t.Fatalf("wrong token = %d", status)
	}
	if status, _ := env.post(RouteIngest, testIngestToken, body); status != http.StatusOK {
		t.Fatalf("bare token = %d", status)
	}
	if status, _ := env.post(RouteIngest, "bearer "+testIngestToken, body); status != http.StatusOK {
		t.Fatalf("bearer token = %d", status)
	}

	cfg := testConfig()
	cfg.IngestTokenSHA256 = ""
	unconfigured := newTestEnv(t, cfg)
	if status, _ := unconfigured.post(RouteIngest, testIngestToken, body); status != http.StatusUnauthorized {
		t.Fatalf("unconfigured hash = %d", status)
	}
	if !strings.Contains(unconfigured.logs.String(), "realtime_ingest_disabled") {
		t.Fatal("missing hash was not logged")
	}
	if unconfigured.svc.IngestAuthorized("Bearer ") {
		t.Fatal("empty token authorized")
	}
}

func TestIngestValidatesAndTrims(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	auth := "Bearer " + testIngestToken
	if status, body := env.post(RouteIngest, auth, "not-json"); status != http.StatusBadRequest || !strings.Contains(body, "invalid json") {
		t.Fatalf("bad json = %d %s", status, body)
	}
	if status, _ := env.post(RouteIngest, auth, fmt.Sprintf(`{"event_id":" ","subscription_id":"%d","subscription_version":"v1"}`, sub.ID)); status != http.StatusBadRequest {
		t.Fatalf("blank event id = %d", status)
	}
	status, body := env.post(RouteIngest, auth, fmt.Sprintf(`{"event_id":" 12 ","subscription_id":" %d ","subscription_version":" v1 ","payload_ref":" ref "}`, sub.ID))
	if status != http.StatusOK || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("trimmed ingest = %d %s", status, body)
	}
	row, err := env.db.RealtimeEvent.Query().Only(context.Background())
	if err != nil || row.EventID != "12" || row.PayloadRef != "ref" || row.SubscriptionVersion != "v1" || row.Topic != TopicBirthdayMonitor {
		t.Fatalf("stored row = %+v, %v", row, err)
	}
	if !row.ExpiresAt.After(sub.ExpiresAt) {
		t.Fatalf("expires_at %v should be after the subscription expiry %v", row.ExpiresAt, sub.ExpiresAt)
	}
}

func TestIngestIsIdempotent(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	for range 3 {
		if status := env.ingest(sub, "v1", "same", ""); status != http.StatusOK {
			t.Fatalf("ingest status = %d", status)
		}
	}
	stream.mustNext()
	stream.expectNone(300 * time.Millisecond)
	if count, _ := env.db.RealtimeEvent.Query().Count(context.Background()); count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}
}

func TestIngestStaleSubscriptionIs409(t *testing.T) {
	env := newTestEnv(t, testConfig())
	active := env.activeSubscription()
	inactive := env.createSubscription(subscriptionSpec{active: false})
	expired := env.createSubscription(subscriptionSpec{active: true, expiresIn: -time.Minute})
	auth := "Bearer " + testIngestToken
	for name, body := range map[string]string{
		"stale version": fmt.Sprintf(`{"event_id":"e","subscription_id":"%d","subscription_version":"v0"}`, active.ID),
		"inactive":      fmt.Sprintf(`{"event_id":"e","subscription_id":"%d","subscription_version":"v1"}`, inactive.ID),
		"expired":       fmt.Sprintf(`{"event_id":"e","subscription_id":"%d","subscription_version":"v1"}`, expired.ID),
		"unknown":       `{"event_id":"e","subscription_id":"424242","subscription_version":"v1"}`,
		"non-numeric":   `{"event_id":"e","subscription_id":"x","subscription_version":"v1"}`,
	} {
		if status, resp := env.post(RouteIngest, auth, body); status != http.StatusConflict {
			t.Fatalf("%s: %d %s, want 409", name, status, resp)
		}
	}
	if count, _ := env.db.RealtimeEvent.Query().Count(context.Background()); count != 0 {
		t.Fatalf("stale ingests stored %d rows", count)
	}
}

func TestIngestTransientFailuresAre503(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	env.readOnly.Store(true)
	if status := env.ingest(sub, "v1", "e", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("read-only ingest = %d", status)
	}
	env.readOnly.Store(false)
	_ = env.db.Close()
	if status := env.ingest(sub, "v1", "e", ""); status != http.StatusServiceUnavailable {
		t.Fatalf("database down ingest = %d", status)
	}
}

func TestUnackedEventIsRedeliveredThenSuperseded(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	env.ingest(sub, "v1", "needs-ack", "")
	first := stream.mustNext()

	env.svc.Sweep(context.Background())
	stream.expectNone(150 * time.Millisecond)

	for delivery := 2; delivery <= 3; delivery++ {
		env.clock.Advance(91 * time.Second)
		env.svc.Sweep(context.Background())
		again := stream.mustNext()
		if again.id != first.id || again.data != first.data {
			t.Fatalf("redelivery %d = %+v, want %+v", delivery, again, first)
		}
	}

	env.clock.Advance(91 * time.Second)
	env.svc.Sweep(context.Background())
	stream.expectNone(200 * time.Millisecond)
	row, err := env.db.RealtimeEvent.Query().Only(context.Background())
	if err != nil || row.DeliveryCount != 3 || row.SupersededReason != ReasonDeliveryLimit {
		t.Fatalf("row after limit = %+v, %v", row, err)
	}
}

func TestAckedEventIsNotRedelivered(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	env.ingest(sub, "v1", "acked", "")
	stream.mustNext()
	if n, err := env.svc.Store().Ack(context.Background(), sub.ID, "v1", "acked", time.Now()); err != nil || n != 1 {
		t.Fatalf("ack = %d, %v", n, err)
	}
	env.clock.Advance(5 * time.Minute)
	env.svc.Sweep(context.Background())
	stream.expectNone(200 * time.Millisecond)

	// A reconnect does not replay it either.
	second := env.connect(streamPath(sub, "v1"), nil)
	second.expectNone(200 * time.Millisecond)
}

func TestUndeliveredEventIsSentBySweepAfterQueueLoss(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	// Store without delivering, as when the live push failed.
	if _, _, err := env.svc.Store().Insert(context.Background(), NewEvent{
		SubscriptionID: sub.ID, SubscriptionVersion: "v1", EventID: "lost", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	env.clock.Advance(91 * time.Second)
	env.svc.Sweep(context.Background())
	if got := stream.mustNext(); !strings.Contains(got.data, `"event_id":"lost"`) {
		t.Fatalf("swept = %s", got.data)
	}
}

func TestPerSubscriptionLimitEvictsOldestStream(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	streams := make([]*sseClient, 0, 4)
	for range 3 {
		streams = append(streams, env.connect(streamPath(sub, "v1"), nil))
	}
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 3 })
	streams = append(streams, env.connect(streamPath(sub, "v1"), nil))
	if !streams[0].waitClosed(3 * time.Second) {
		t.Fatal("oldest stream was not closed")
	}
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 3 })
	for _, open := range streams[1:] {
		if open.waitClosed(50 * time.Millisecond) {
			t.Fatal("a newer stream was closed")
		}
	}
	if !strings.Contains(env.logs.String(), "reason="+CloseReasonEvicted) {
		t.Fatal("eviction was not logged on disconnect")
	}
}

func TestGlobalConnectionLimitIs503(t *testing.T) {
	cfg := testConfig()
	cfg.MaxConns = 1
	env := newTestEnv(t, cfg)
	first := env.activeSubscription()
	second := env.activeSubscription()
	env.connect(streamPath(first, "v1"), nil)
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 1 })
	if status, body := env.get(streamPath(second, "v1")); status != http.StatusServiceUnavailable || !strings.Contains(body, "too many connections") {
		t.Fatalf("over the cap = %d %s", status, body)
	}
}

func TestCloseRoute(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 1 })
	path := fmt.Sprintf("/internal/subscriptions/%d/close", sub.ID)

	if status, _ := env.post(path+"?subscription_version=v1", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("unauthorized close = %d", status)
	}
	if status, _ := env.post(path, "Bearer internal", "not-json"); status != http.StatusBadRequest {
		t.Fatalf("bad json close = %d", status)
	}
	if status, _ := env.post(path, "Bearer internal", ""); status != http.StatusBadRequest {
		t.Fatalf("missing version close = %d", status)
	}
	if status, _ := env.post("/internal/subscriptions/x/close?version=v1", "Bearer internal", ""); status != http.StatusBadRequest {
		t.Fatalf("bad id close = %d", status)
	}
	status, body := env.post(path+"?subscription_version=v1", "Bearer internal", "")
	if status != http.StatusOK || !strings.Contains(body, `"closed_clients":1`) {
		t.Fatalf("close = %d %s", status, body)
	}
	if !stream.waitClosed(3 * time.Second) {
		t.Fatal("stream not closed")
	}
	status, body = env.post(path, "Bearer internal", `{"subscription_version":" v1 "}`)
	if status != http.StatusOK || !strings.Contains(body, `"closed_clients":0`) {
		t.Fatalf("body close = %d %s", status, body)
	}
	if status, _ := env.post(path+"?version=v3", "Bearer internal", ""); status != http.StatusOK {
		t.Fatalf("version alias close = %d", status)
	}
}

func TestSweepClosesStaleStreams(t *testing.T) {
	env := newTestEnv(t, testConfig())
	stale := env.activeSubscription()
	kept := env.activeSubscription()
	staleStream := env.connect(streamPath(stale, "v1"), nil)
	keptStream := env.connect(streamPath(kept, "v1"), nil)
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 2 })

	if err := env.db.MysekaiBirthdaySubscription.UpdateOneID(stale.ID).SetToken("v2.secret").Exec(context.Background()); err != nil {
		t.Fatalf("rotate token: %v", err)
	}
	env.svc.Sweep(context.Background())
	if !staleStream.waitClosed(3 * time.Second) {
		t.Fatal("stale stream not closed")
	}
	if keptStream.waitClosed(100 * time.Millisecond) {
		t.Fatal("live stream closed")
	}
	if err := env.db.MysekaiBirthdaySubscription.DeleteOneID(kept.ID).Exec(context.Background()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	env.svc.Sweep(context.Background())
	if !keptStream.waitClosed(3 * time.Second) {
		t.Fatal("deleted subscription's stream not closed")
	}
}

func TestShutdownSendsRetryAndEndsStreams(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	if f, ok := stream.rawFrame(3 * time.Second); !ok || f.retry == "" {
		t.Fatalf("initial frame = %+v", f)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		env.svc.Run(ctx)
		close(done)
	}()
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 1 })
	cancel()
	<-done
	f, ok := stream.rawFrame(3 * time.Second)
	if !ok || f.retry != "1500" {
		t.Fatalf("shutdown frame = %+v", f)
	}
	if !stream.waitClosed(3 * time.Second) {
		t.Fatal("stream not ended on shutdown")
	}
}

func TestHeartbeatComment(t *testing.T) {
	cfg := testConfig()
	cfg.HeartbeatInterval = 50 * time.Millisecond
	env := newTestEnv(t, cfg)
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case f := <-stream.frames:
			if f.comment == heartbeatTag {
				return
			}
		case <-deadline:
			t.Fatal("no heartbeat comment")
		}
	}
}

func TestDisconnectIsLoggedAndUnregistered(t *testing.T) {
	cfg := testConfig()
	cfg.HeartbeatInterval = 30 * time.Millisecond
	env := newTestEnv(t, cfg)
	sub := env.activeSubscription()
	stream := env.connect(streamPath(sub, "v1"), nil)
	waitFor(t, 2*time.Second, func() bool { return env.svc.Hub().Count() == 1 })
	stream.close()
	waitFor(t, 5*time.Second, func() bool { return env.svc.Hub().Count() == 0 })
	logs := env.logs.String()
	if !strings.Contains(logs, "sse connected") || !strings.Contains(logs, "sse disconnected") {
		t.Fatalf("connect/disconnect not logged: %s", logs)
	}
}

func TestRunCollectsGarbage(t *testing.T) {
	env := newTestEnv(t, testConfig())
	sub := env.activeSubscription()
	ctx := context.Background()
	store := env.svc.Store()
	for _, spec := range []struct {
		id      string
		expires time.Time
	}{
		{"old", time.Now().Add(-8 * 24 * time.Hour)},
		{"recent", time.Now().Add(-time.Hour)},
		{"live", time.Now().Add(time.Hour)},
	} {
		if _, _, err := store.Insert(ctx, NewEvent{SubscriptionID: sub.ID, SubscriptionVersion: "v1", EventID: spec.id, ExpiresAt: spec.expires}); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		env.svc.Run(runCtx)
		close(done)
	}()
	waitFor(t, 3*time.Second, func() bool {
		count, _ := env.db.RealtimeEvent.Query().Count(ctx)
		return count == 2
	})
	cancel()
	<-done
	if _, err := env.db.RealtimeEvent.Query().Where(realtimeevent.EventID("old")).Only(ctx); err == nil {
		t.Fatal("expired row survived GC")
	}

	env.readOnly.Store(true)
	env.svc.collectGarbage(ctx)
	env.svc.Sweep(ctx)
}

func TestLegacyForwardTeesNewEvents(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	var auths []string
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(data))
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer legacy.Close()

	cfg := testConfig()
	cfg.LegacyForwardURL = legacy.URL + "/internal/events"
	cfg.LegacyForwardToken = "old-token"
	env := newTestEnv(t, cfg)
	sub := env.activeSubscription()
	body := fmt.Sprintf(`{"event_id":"tee","subscription_id":"%d","subscription_version":"v1","empty_result":true}`, sub.ID)
	for range 2 {
		if status, _ := env.post(RouteIngest, "Bearer "+testIngestToken, body); status != http.StatusOK {
			t.Fatalf("ingest = %d", status)
		}
	}
	env.svc.forwarder.wait(3 * time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 || bodies[0] != body || auths[0] != "Bearer old-token" {
		t.Fatalf("forwarded = %v %v", bodies, auths)
	}
}

func TestLegacyForwardFailuresAreLogged(t *testing.T) {
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	f := newForwarder(legacy.URL, "Bearer x", nil)
	if err := f.post([]byte(`{}`)); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("status error = %v", err)
	}
	legacy.Close()
	if err := f.post([]byte(`{}`)); err == nil {
		t.Fatal("closed server should fail")
	}
	bad := newForwarder("://bad", "", nil)
	if err := bad.post(nil); err == nil {
		t.Fatal("bad url should fail")
	}
	if bearer("") != "" || bearer("Bearer a") != "Bearer a" || bearer("a") != "Bearer a" {
		t.Fatal("bearer")
	}
}
