package deck

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/core/upstream"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/utils/logger"
)

type additionalRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn additionalRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type additionalNetError struct{}

func (additionalNetError) Error() string   { return "temporary network error" }
func (additionalNetError) Timeout() bool   { return true }
func (additionalNetError) Temporary() bool { return true }

type additionalErrorReader struct{}

func (additionalErrorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func additionalRemoteExecution(baseURL string) *remoteExecution {
	return &remoteExecution{state: &remoteTargetState{target: upstream.TargetConfig{
		Name:        "additional",
		BaseURL:     baseURL,
		Concurrency: 1,
	}}}
}

func TestRemoteBatchParsingAdditional(t *testing.T) {
	options := []map[string]any{{"algorithm": "ga", "target": "score", "limit": 2}}
	if _, err := parseRemoteRecommendBatch(nil, options); err == nil {
		t.Fatal("expected empty response error")
	}
	if _, err := parseRemoteRecommendBatch(json.RawMessage(`[invalid`), options); err == nil {
		t.Fatal("expected invalid array error")
	}
	items, err := parseRemoteRecommendBatch(json.RawMessage(`[ {"decks":[]} ]`), options)
	if err != nil || len(items) != 1 || items[0].Alg != "ga" {
		t.Fatalf("array parse = %+v, %v", items, err)
	}
	if _, err := parseRemoteRecommendBatch(json.RawMessage(`{invalid`), options); err == nil {
		t.Fatal("expected invalid single response error")
	}
}

func TestRemoteAggregationAdditional(t *testing.T) {
	options := []map[string]any{{"algorithm": "ga", "target": "score", "limit": 2}}
	if _, err := aggregateRemoteRecommendResults("event", options, []remoteBatchRecommendResult{{Error: " bad option "}}); err == nil || err.Error() != "bad option" {
		t.Fatalf("aggregate logical error = %v", err)
	}
	empty, err := aggregateRemoteRecommendResults("event", options, []remoteBatchRecommendResult{{}})
	if err != nil || len(empty.Decks) != 0 {
		t.Fatalf("aggregate empty = %+v, %v", empty, err)
	}
	first := remoteRecommendDeck{Score: 10, Cards: []remoteRecommendCard{{CardID: 1}}}
	better := remoteRecommendDeck{Score: 20, Cards: []remoteRecommendCard{{CardID: 1}}}
	agg, err := aggregateRemoteRecommendResults("event", options, []remoteBatchRecommendResult{
		{Alg: "ga", Decks: []remoteRecommendDeck{first}},
		{Alg: "rl", Decks: []remoteRecommendDeck{better}},
	})
	if err != nil || len(agg.Decks) != 1 || agg.Decks[0].Score != 20 {
		t.Fatalf("aggregate replacement = %+v, %v", agg, err)
	}
}

func TestRemoteClassificationHelpers(t *testing.T) {
	if classifyRemoteRewarm(nil) != remoteRewarmNone || classifyRemoteRewarm(errors.New("other")) != remoteRewarmNone {
		t.Fatal("unexpected rewarm classification")
	}
	if classifyRemoteRewarm(errors.New("MASTER DATA NOT FOUND")) != remoteRewarmMasterdata {
		t.Fatal("expected masterdata rewarm")
	}
	if classifyRemoteRewarm(errors.New("music meta not found")) != remoteRewarmMusicMeta {
		t.Fatal("expected music-meta rewarm")
	}
	if !shouldRewarmRemoteService(errors.New("music metas not found")) {
		t.Fatal("expected rewarm")
	}
	for _, message := range []string{"HTTP 404", "missing field `live_type`", "unsupported media type"} {
		if !isUnsupportedBatchProtocolError(errors.New(message)) {
			t.Fatalf("expected unsupported error for %q", message)
		}
	}
	if isUnsupportedBatchProtocolError(nil) || isUnsupportedBatchProtocolError(errors.New("HTTP 400")) {
		t.Fatal("unexpected unsupported-protocol classification")
	}
	if isMissingUserdataHashError(nil) || isMissingUserdataHashError(errors.New("userdata_hash invalid")) {
		t.Fatal("unexpected missing-hash classification")
	}
	if !isMissingUserdataHashError(errors.New("userdata_hash user data not found")) {
		t.Fatal("expected missing-hash classification")
	}
}

func TestRemotePayloadHelpers(t *testing.T) {
	if hashPayload(nil) != "" || len(hashPayload([]byte("payload"))) != 64 {
		t.Fatal("unexpected payload hash")
	}
	if got := cloneRecommendOption(nil); len(got) != 0 {
		t.Fatalf("clone nil = %#v", got)
	}
	original := map[string]any{"algorithm": "ga"}
	cloned := cloneRecommendOption(original)
	cloned["algorithm"] = "rl"
	if original["algorithm"] != "ga" {
		t.Fatal("clone mutated original")
	}
}

func TestRemoteRetryAndHTTPErrorHelpersAdditional(t *testing.T) {
	if !isRetryableError(additionalNetError{}, 0) {
		t.Fatal("net.Error should be retryable")
	}
	for _, message := range []string{"connection refused", "no such host", "i/o timeout", "unexpected EOF"} {
		if !isRetryableError(errors.New(message), 0) {
			t.Fatalf("%q should be retryable", message)
		}
	}
	if isRetryableError(errors.New("bad request"), 0) || !isRetryableError(nil, 500) || isRetryableError(nil, 499) {
		t.Fatal("unexpected retry classification")
	}

	if got := parseRemoteHTTPError(400, []byte(`{"error":"remote bad"}`)); got.Error() != "remote bad" {
		t.Fatalf("structured HTTP error = %v", got)
	}
	if got := parseRemoteHTTPError(400, []byte(" plain bad ")); !strings.Contains(got.Error(), "plain bad") {
		t.Fatalf("plain HTTP error = %v", got)
	}
	if got := parseRemoteHTTPError(418, nil); got.Error() != "deck-service returned HTTP 418" {
		t.Fatalf("empty HTTP error = %v", got)
	}
	oversizedError := []byte(strings.Repeat("x", maxDeckErrorBodyBytes+10))
	if got := parseRemoteHTTPError(500, oversizedError); strings.Contains(got.Error(), strings.Repeat("x", maxDeckErrorBodyBytes+1)) {
		t.Fatal("HTTP error was not truncated")
	}
}

func TestRemoteResponseBodyAndWaitHelpersAdditional(t *testing.T) {
	payload, truncated, err := readDeckResponseBody(strings.NewReader("ok"))
	if err != nil || truncated || string(payload) != "ok" {
		t.Fatalf("read body = %q, %v, %v", payload, truncated, err)
	}
	if _, _, err := readDeckResponseBody(additionalErrorReader{}); err == nil {
		t.Fatal("expected read error")
	}
	large := strings.NewReader(strings.Repeat("z", maxDeckResponseBodyBytes+1))
	payload, truncated, err = readDeckResponseBody(large)
	if err != nil || !truncated || len(payload) != maxDeckResponseBodyBytes {
		t.Fatalf("oversized body = %d, %v, %v", len(payload), truncated, err)
	}

	if err := waitForDeckRetry(context.Background(), 0); err != nil {
		t.Fatalf("zero wait = %v", err)
	}
	if err := waitForDeckRetry(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("timer wait = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(waitForDeckRetry(canceled, time.Second), context.Canceled) {
		t.Fatal("expected canceled retry wait")
	}
	if len(buildMultipartPayload(context.Background(), []byte("a"), []byte("bc"))) == 0 {
		t.Fatal("multipart payload is empty")
	}
}

func TestRemotePostAndHealthAdditional(t *testing.T) {
	var retryCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusNoContent)
		case "/health-bad":
			http.Error(w, "bad", http.StatusServiceUnavailable)
		case "/ok":
			_, _ = io.WriteString(w, `{"value":"ok"}`)
		case "/empty":
			w.WriteHeader(http.StatusNoContent)
		case "/bad-json":
			_, _ = io.WriteString(w, `{bad`)
		case "/client-error":
			http.Error(w, `{"error":"client bad"}`, http.StatusBadRequest)
		case "/missing":
			http.Error(w, `{"error":"userdata_hash user data not found"}`, http.StatusNotFound)
		case "/retry":
			retryCalls++
			http.Error(w, "retry", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	r := newStandaloneTestRemoteDeckRecommender(server.URL, server.Client())
	r.maxRetries = 1
	r.retryWaitTime = 0
	r.logger = logger.NewLogger("DeckRemoteAdditional", "ERROR", nil)
	exec := testRemoteExecution(t, r)
	defer exec.Release()
	testRemotePostBranches(t, r, exec, &retryCalls)
	testRemoteHealthAndInputBranches(t, r, exec, server)
}

func testRemotePostBranches(t *testing.T, r *RemoteDeckRecommender, exec *remoteExecution, retryCalls *int) {
	t.Helper()
	var response map[string]any
	if err := r.postJSON(context.Background(), exec, "/ok", map[string]any{"x": 1}, &response); err != nil || response["value"] != "ok" {
		t.Fatalf("postJSON success = %#v, %v", response, err)
	}
	if err := r.postJSON(context.Background(), exec, "/empty", nil, nil); err != nil {
		t.Fatalf("postJSON empty = %v", err)
	}
	if err := r.postJSON(context.Background(), exec, "/bad-json", nil, &response); err == nil {
		t.Fatal("expected decode error")
	}
	if err := r.postJSON(context.Background(), exec, "/client-error", nil, nil); err == nil || !strings.Contains(err.Error(), "client bad") {
		t.Fatalf("postJSON client error = %v", err)
	}
	if err := r.postJSON(context.Background(), exec, "/missing", nil, nil); err == nil || *retryCalls != 0 {
		t.Fatalf("postJSON missing hash = %v, retryCalls=%d", err, *retryCalls)
	}
	if err := r.postJSON(context.Background(), exec, "/retry", nil, nil); err == nil || *retryCalls != 2 {
		t.Fatalf("postJSON retry = %v, calls=%d", err, *retryCalls)
	}
	if err := r.postBinary(context.Background(), exec, "/ok", []byte("x"), &response); err != nil {
		t.Fatalf("postBinary success = %v", err)
	}
	if err := r.postBinary(context.Background(), exec, "/empty", nil, nil); err != nil {
		t.Fatalf("postBinary empty = %v", err)
	}
	if err := r.postBinary(context.Background(), exec, "/bad-json", nil, &response); err == nil {
		t.Fatal("expected binary decode error")
	}
	if err := r.postBinary(context.Background(), exec, "/client-error", nil, nil); err == nil {
		t.Fatal("expected binary client error")
	}
	if err := r.postBinary(context.Background(), exec, "/missing", nil, nil); err == nil {
		t.Fatal("expected binary missing hash error")
	}
}

func testRemoteHealthAndInputBranches(t *testing.T, r *RemoteDeckRecommender, exec *remoteExecution, server *httptest.Server) {
	t.Helper()
	if !r.healthCheck(context.Background(), server.URL) || r.healthCheck(context.Background(), "") {
		t.Fatal("unexpected health result")
	}
	if (&RemoteDeckRecommender{}).healthCheck(context.Background(), server.URL) {
		t.Fatal("nil client should be unhealthy")
	}
	badHealth := newStandaloneTestRemoteDeckRecommender(server.URL+"/health-bad", server.Client())
	if badHealth.healthCheck(context.Background(), server.URL+"/health-bad") {
		t.Fatal("non-success health should fail")
	}

	if err := r.postJSON(context.Background(), additionalRemoteExecution(""), "/ok", nil, nil); err == nil {
		t.Fatal("expected empty base URL error")
	}
	if err := r.postBinary(context.Background(), additionalRemoteExecution(""), "/ok", nil, nil); err == nil {
		t.Fatal("expected empty binary base URL error")
	}
	if err := r.postJSON(context.Background(), exec, "/ok", make(chan int), nil); err == nil {
		t.Fatal("expected marshal error")
	}
	if err := r.postJSON(context.Background(), additionalRemoteExecution(":"), "/ok", nil, nil); err == nil {
		t.Fatal("expected invalid request URL")
	}
	if err := r.postBinary(context.Background(), additionalRemoteExecution(":"), "/ok", nil, nil); err == nil {
		t.Fatal("expected invalid binary request URL")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(r.postJSON(canceled, exec, "/ok", nil, nil), context.Canceled) {
		t.Fatal("expected canceled JSON request")
	}
	if !errors.Is(r.postBinary(canceled, exec, "/ok", nil, nil), context.Canceled) {
		t.Fatal("expected canceled binary request")
	}
}

func TestRemoteTransportFailuresAdditional(t *testing.T) {
	newRecommender := func(fn additionalRoundTripFunc) (*RemoteDeckRecommender, *remoteExecution) {
		r := newStandaloneTestRemoteDeckRecommender("http://deck.test", &http.Client{Transport: fn})
		r.maxRetries = 1
		r.retryWaitTime = 0
		r.logger = logger.NewLogger("DeckRemoteAdditional", "ERROR", nil)
		return r, testRemoteExecution(t, r)
	}

	r, exec := newRecommender(func(*http.Request) (*http.Response, error) {
		return nil, additionalNetError{}
	})
	defer exec.Release()
	if err := r.postJSON(context.Background(), exec, "/x", nil, nil); err == nil {
		t.Fatal("expected retryable transport error")
	}
	if err := r.postBinary(context.Background(), exec, "/x", nil, nil); err == nil {
		t.Fatal("expected retryable binary transport error")
	}

	r2, exec2 := newRecommender(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("permanent failure")
	})
	defer exec2.Release()
	if err := r2.postJSON(context.Background(), exec2, "/x", nil, nil); err == nil {
		t.Fatal("expected permanent transport error")
	}
	if err := r2.postBinary(context.Background(), exec2, "/x", nil, nil); err == nil {
		t.Fatal("expected permanent binary transport error")
	}

	r3, exec3 := newRecommender(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(additionalErrorReader{})}, nil
	})
	defer exec3.Release()
	if err := r3.postJSON(context.Background(), exec3, "/x", nil, nil); err == nil {
		t.Fatal("expected JSON read error")
	}
	if err := r3.postBinary(context.Background(), exec3, "/x", nil, nil); err == nil {
		t.Fatal("expected binary read error")
	}
}

func TestRemoteRecommendHelpersAdditional(t *testing.T) {
	testRemoteRecommendValidation(t)
	testRemoteCircuitBreakerHelpers(t)
	testRemoteReadyTimeoutAndInvalidation(t)
}

func testRemoteRecommendValidation(t *testing.T) {
	t.Helper()
	valid := testRemoteRecommendRequest()
	for _, req := range []RecommendRequest{
		{},
		{BatchOption: valid.BatchOption},
		{BatchOption: valid.BatchOption, UserData: valid.UserData},
	} {
		if validateRemoteRecommendRequest(req) == nil {
			t.Fatalf("expected validation error for %#v", req)
		}
	}
	if err := validateRemoteRecommendRequest(valid); err != nil {
		t.Fatalf("valid request = %v", err)
	}
	if firstRemoteBatchError(nil) != nil || firstRemoteBatchError([]remoteBatchRecommendResult{{Decks: []remoteRecommendDeck{{}}}}) != nil {
		t.Fatal("successful batch reported error")
	}
	if firstRemoteBatchError([]remoteBatchRecommendResult{{Result: &remoteRecommendResult{Decks: []remoteRecommendDeck{{}}}}}) != nil {
		t.Fatal("successful result reported error")
	}
	if err := firstRemoteBatchError([]remoteBatchRecommendResult{{Error: " first "}, {Error: "second"}}); err == nil || err.Error() != "first" {
		t.Fatalf("first batch error = %v", err)
	}
}

func testRemoteCircuitBreakerHelpers(t *testing.T) {
	t.Helper()
	fixedNow := time.Unix(1000, 0)
	r := &RemoteDeckRecommender{now: func() time.Time { return fixedNow }, logger: logger.NewLogger("DeckRemoteAdditional", "ERROR", nil)}
	state := &remoteTargetState{}
	r.recordFailure(state)
	if state.consecutiveFailures.Load() != 1 || state.lastFailureAtNanos.Load() != fixedNow.UnixNano() {
		t.Fatalf("recordFailure state = %+v", state)
	}
	r.recordSuccess(state)
	if state.consecutiveFailures.Load() != 0 || state.lastFailureAtNanos.Load() != 0 {
		t.Fatal("recordSuccess did not clear state")
	}
	state.consecutiveFailures.Store(5)
	state.lastFailureAtNanos.Store(fixedNow.Add(-circuitBreakerCooldown - time.Second).UnixNano())
	if !r.tryResetCircuitBreakerAfterCooldown(state, 5) {
		t.Fatal("expected cooldown reset")
	}
	state.lastFailureAtNanos.Store(0)
	if r.tryResetCircuitBreakerAfterCooldown(state, 5) {
		t.Fatal("missing timestamp should not reset")
	}
	state.lastFailureAtNanos.Store(fixedNow.UnixNano())
	if r.tryResetCircuitBreakerAfterCooldown(state, 5) {
		t.Fatal("recent failure should not reset")
	}
	if !r.timeNow().Equal(fixedNow) || (&RemoteDeckRecommender{}).timeNow().IsZero() {
		t.Fatal("unexpected timeNow result")
	}
	for _, err := range []error{
		errors.New("master data not found"),
		errors.New("deck-service returned HTTP 503"),
		errors.New("connection refused"),
		errors.New("no such host"),
		errors.New("i/o timeout"),
		errors.New("context deadline exceeded"),
		errors.New("EOF"),
	} {
		if !shouldCountCircuitBreakerFailure(err) {
			t.Fatalf("expected circuit failure for %v", err)
		}
	}
	if shouldCountCircuitBreakerFailure(nil) || shouldCountCircuitBreakerFailure(errors.New("bad option")) {
		t.Fatal("logical error counted as circuit failure")
	}
}

func testRemoteReadyTimeoutAndInvalidation(t *testing.T) {
	t.Helper()
	if got := (&RemoteDeckRecommender{}).readySharedTimeout(); got != time.Minute {
		t.Fatalf("default shared timeout = %v", got)
	}
	configured := &RemoteDeckRecommender{client: &http.Client{Timeout: 40 * time.Second}, maxRetries: 2, retryWaitTime: time.Second}
	if got := configured.readySharedTimeout(); got != 244*time.Second {
		t.Fatalf("configured shared timeout = %v", got)
	}
	r := &RemoteDeckRecommender{}
	state := &remoteTargetState{}
	state.masterdataReady = true
	state.musicMetaHash = "hash"
	r.invalidate(state, remoteRewarmMasterdata)
	if state.masterdataReady || state.musicMetaHash != "hash" {
		t.Fatal("masterdata invalidation failed")
	}
	state.masterdataReady = true
	r.invalidate(state, remoteRewarmMusicMeta)
	if !state.masterdataReady || state.musicMetaHash != "" {
		t.Fatal("music invalidation failed")
	}
	state.musicMetaHash = "hash"
	r.invalidate(state, remoteRewarmNone)
	if state.masterdataReady || state.musicMetaHash != "" {
		t.Fatal("full invalidation failed")
	}
}

func TestRemoteCircuitGateAndErrorRecording(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	fixedNow := time.Unix(2000, 0)
	r := &RemoteDeckRecommender{
		client: server.Client(),
		now:    func() time.Time { return fixedNow },
		logger: logger.NewLogger("DeckRemoteAdditional", "ERROR", nil),
	}
	state := &remoteTargetState{target: upstream.TargetConfig{BaseURL: server.URL}}
	if err := r.ensureCircuitClosed(context.Background(), state); err != nil {
		t.Fatalf("closed circuit error = %v", err)
	}
	state.consecutiveFailures.Store(maxConsecutiveFailures)
	state.lastFailureAtNanos.Store(fixedNow.Add(-circuitBreakerCooldown - time.Second).UnixNano())
	if err := r.ensureCircuitClosed(context.Background(), state); err != nil {
		t.Fatalf("cooldown circuit reset error = %v", err)
	}
	state.consecutiveFailures.Store(maxConsecutiveFailures)
	state.lastFailureAtNanos.Store(fixedNow.UnixNano())
	if err := r.ensureCircuitClosed(context.Background(), state); err != nil {
		t.Fatalf("health-probe circuit reset error = %v", err)
	}
	state.target.BaseURL = ""
	state.consecutiveFailures.Store(maxConsecutiveFailures)
	state.lastFailureAtNanos.Store(fixedNow.UnixNano())
	if err := r.ensureCircuitClosed(context.Background(), state); err == nil {
		t.Fatal("open circuit was accepted")
	}

	r.recordRecommendError(context.Background(), state, errors.New("connection refused"), time.Millisecond)
	if state.consecutiveFailures.Load() != maxConsecutiveFailures+1 {
		t.Fatal("transport error did not increment circuit failures")
	}
	r.recordRecommendError(context.Background(), state, errors.New("bad option"), time.Millisecond)
	if state.consecutiveFailures.Load() != 0 {
		t.Fatal("logical error did not reset circuit failures")
	}
}

func TestRemoteReadyInputAndFlightErrorHelpers(t *testing.T) {
	r := &RemoteDeckRecommender{region: "jp"}
	region, path, hash, err := r.normalizeReadyInputs(context.Background(), " ", nil, " /tmp/meta.json ")
	if err != nil || region != "jp" || path != "/tmp/meta.json" || hash != "path:/tmp/meta.json" {
		t.Fatalf("normalized ready inputs = %q, %q, %q, %v", region, path, hash, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := r.normalizeReadyInputs(canceled, "jp", nil, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ready inputs error = %v", err)
	}
	if retry, err := classifyReadyFlightError(context.Background(), context.DeadlineExceeded); !retry || err != nil {
		t.Fatalf("deadline flight error = retry %v, err %v", retry, err)
	}
	if retry, err := classifyReadyFlightError(canceled, errors.New("remote")); retry || !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation = retry %v, err %v", retry, err)
	}
	wantErr := errors.New("remote")
	if retry, err := classifyReadyFlightError(context.Background(), wantErr); retry || !errors.Is(err, wantErr) {
		t.Fatalf("remote flight error = retry %v, err %v", retry, err)
	}
}

var _ net.Error = additionalNetError{}
