package censor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func baiduTestResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestBaiduTextCensorResultError(t *testing.T) {
	result := map[string]any{
		"error_code": float64(17),
		"error_msg":  "Open api daily request limit reached",
	}

	if err := baiduTextCensorResultError(result); err == nil {
		t.Fatal("baiduTextCensorResultError() = nil, want error")
	}
}

func TestBaiduTextCensorResultErrorAllowsNormalResponse(t *testing.T) {
	result := map[string]any{
		"conclusion": string(ResultCompliant),
	}

	if err := baiduTextCensorResultError(result); err != nil {
		t.Fatalf("baiduTextCensorResultError() = %v, want nil", err)
	}
}

func TestBaiduTextCensorClientConfiguresResponseBodyLimit(t *testing.T) {
	client := NewBaiduTextCensorClient("api-key", "secret-key")
	if got := client.httpClient().ResponseBodyLimit; got != baiduMaxResponseBytes {
		t.Fatalf("ResponseBodyLimit = %d, want %d", got, baiduMaxResponseBytes)
	}

	client.httpClient().SetResponseBodyLimit(16)
	client.httpClient().SetTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		response := baiduTestResponse(req, strings.Repeat("x", 32))
		response.ContentLength = 32
		return response, nil
	}))
	if err := client.InitContext(context.Background()); !errors.Is(err, errBaiduResponseTooLarge) {
		t.Fatalf("InitContext() error = %v, want errBaiduResponseTooLarge", err)
	}
}

func TestBaiduTextCensorSanitizesCredentialsFromNetworkErrors(t *testing.T) {
	t.Run("oauth client secret", func(t *testing.T) {
		const apiKey = "sensitive-api-key"
		const secretKey = "sensitive-client-secret"
		client := NewBaiduTextCensorClient(apiKey, secretKey)
		client.httpClient().SetTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("dial failed for %s", req.URL.String())
		}))

		err := client.InitContext(context.Background())
		assertSanitizedBaiduError(t, err, apiKey, secretKey, "client_id", "client_secret")
	})

	t.Run("moderation access token", func(t *testing.T) {
		const accessToken = "sensitive-access-token"
		client := NewBaiduTextCensorClient("api-key", "secret-key")
		client.storeAccessToken(accessToken)
		client.httpClient().SetTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("dial failed for %s", req.URL.String())
		}))

		_, err := client.TextCensorContext(context.Background(), "hello")
		assertSanitizedBaiduError(t, err, accessToken, "access_token", "aip.baidubce.com")
	})
}

func assertSanitizedBaiduError(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want sanitized request error")
	}
	message := err.Error()
	for _, value := range forbidden {
		if strings.Contains(message, value) {
			t.Fatalf("error leaked %q: %q", value, message)
		}
	}
}

func TestBaiduTextCensorContextAndTrace(t *testing.T) {
	type contextKey string
	const key contextKey = "request"
	const value = "censor-test"

	client := NewBaiduTextCensorClient("api-key", "secret-key")
	client.accessToken = "token"
	client.client.SetTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Context().Value(key); got != value {
			return nil, fmt.Errorf("request context value = %v, want %q", got, value)
		}
		return baiduTestResponse(req, `{"conclusion":"合规"}`), nil
	}))

	base := context.WithValue(context.Background(), key, value)
	ctx, trace := commandtrace.WithTrace(base)
	result, err := client.TextCensorContext(ctx, "hello")
	if err != nil {
		t.Fatalf("TextCensorContext() error = %v", err)
	}
	if got := result["conclusion"]; got != string(ResultCompliant) {
		t.Fatalf("conclusion = %v, want %q", got, ResultCompliant)
	}
	snapshot := trace.Snapshot()
	if got := operationCount(snapshot, "censor.http"); got != 1 {
		t.Fatalf("censor.http count = %d, want 1", got)
	}
	if got := operationCount(snapshot, "censor.decode"); got != 1 {
		t.Fatalf("censor.decode count = %d, want 1", got)
	}
}

func TestBaiduTextCensorConcurrentColdStartDeduplicatesAccessToken(t *testing.T) {
	const callers = 32

	client := NewBaiduTextCensorClient("api-key", "secret-key")
	var tokenRequests atomic.Int32
	var censorRequests atomic.Int32
	oauthStarted := make(chan struct{})
	releaseOAuth := make(chan struct{})
	var oauthStartedOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseOAuth) }) }
	t.Cleanup(release)

	client.client.SetTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/oauth/2.0/token":
			tokenRequests.Add(1)
			oauthStartedOnce.Do(func() { close(oauthStarted) })
			select {
			case <-releaseOAuth:
				return baiduTestResponse(req, `{"access_token":"shared-token"}`), nil
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		case "/rest/2.0/solution/v1/text_censor/v2/user_defined":
			if got := req.URL.Query().Get("access_token"); got != "shared-token" {
				return nil, fmt.Errorf("access_token = %q, want shared-token", got)
			}
			censorRequests.Add(1)
			return baiduTestResponse(req, `{"conclusion":"合规"}`), nil
		default:
			return nil, fmt.Errorf("unexpected request path %q", req.URL.Path)
		}
	}))

	start := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, err := client.TextCensorContext(context.Background(), "hello")
			errs <- err
		}()
	}
	close(start)
	select {
	case <-oauthStarted:
	case <-time.After(time.Second):
		t.Fatal("OAuth request did not start")
	}
	time.Sleep(25 * time.Millisecond)
	if got := tokenRequests.Load(); got != 1 {
		t.Fatalf("OAuth request count while cold start is blocked = %d, want 1", got)
	}
	release()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("TextCensorContext() error = %v", err)
		}
	}
	if got := tokenRequests.Load(); got != 1 {
		t.Fatalf("OAuth request count = %d, want 1", got)
	}
	if got := censorRequests.Load(); got != callers {
		t.Fatalf("text censor request count = %d, want %d", got, callers)
	}
}

func TestBaiduTextCensorCanceledLeaderDoesNotPoisonSharedTokenFetch(t *testing.T) {
	client := NewBaiduTextCensorClient("api-key", "secret-key")
	var tokenRequests atomic.Int32
	oauthStarted := make(chan struct{})
	releaseOAuth := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseOAuth) }) }
	t.Cleanup(release)

	client.client.SetTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/oauth/2.0/token" {
			return nil, fmt.Errorf("unexpected request path %q", req.URL.Path)
		}
		tokenRequests.Add(1)
		startedOnce.Do(func() { close(oauthStarted) })
		select {
		case <-releaseOAuth:
			return baiduTestResponse(req, `{"access_token":"shared-token"}`), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}))

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := client.accessTokenContext(leaderCtx, false)
		leaderErr <- err
	}()
	select {
	case <-oauthStarted:
	case <-time.After(time.Second):
		t.Fatal("OAuth request did not start")
	}
	cancelLeader()
	select {
	case err := <-leaderErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled leader did not return promptly")
	}

	timer := time.AfterFunc(50*time.Millisecond, release)
	t.Cleanup(func() { timer.Stop() })
	followerCtx, followerTrace := commandtrace.WithTrace(context.Background())
	token, err := client.accessTokenContext(followerCtx, false)
	if err != nil {
		t.Fatalf("follower accessTokenContext() error = %v", err)
	}
	if token != "shared-token" {
		t.Fatalf("follower token = %q, want shared-token", token)
	}
	if got := tokenRequests.Load(); got != 1 {
		t.Fatalf("OAuth request count = %d, want 1", got)
	}
	snapshot := followerTrace.Snapshot()
	for _, operation := range []string{"censor.token_wait", "censor.http", "censor.decode", "censor.token_shared"} {
		if got := operationCount(snapshot, operation); got != 1 {
			t.Fatalf("%s count = %d, want 1; operations=%+v", operation, got, snapshot.Operations)
		}
	}
}

func TestBaiduTextCensorConcurrentLazyClientInitialization(t *testing.T) {
	client := &BaiduTextCensorClient{
		apiKey:      "api-key",
		secretKey:   "secret-key",
		accessToken: "token",
	}

	const callers = 64
	start := make(chan struct{})
	clients := make(chan *http.Client, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			<-start
			err := client.InitContext(context.Background())
			errs <- err
			clients <- client.httpClient().GetClient()
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(clients)
	for err := range errs {
		if err != nil {
			t.Fatalf("InitContext() error = %v", err)
		}
	}
	var initialized *http.Client
	for current := range clients {
		if initialized == nil {
			initialized = current
			continue
		}
		if current != initialized {
			t.Fatal("concurrent initialization returned different HTTP clients")
		}
	}
	if initialized == nil {
		t.Fatal("concurrent initialization left HTTP client nil")
	}
}
