package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	json "haruki-cloud/internal/jsonutil"
)

func TestFetchCachedSekaiUserProfileUsesCloudCache(t *testing.T) {
	const userID = "12345678901231"
	oldTTL := harukiConfig.Cfg.Backend.APICacheTTL
	harukiConfig.Cfg.Backend.APICacheTTL = time.Minute
	t.Cleanup(func() {
		harukiConfig.Cfg.Backend.APICacheTTL = oldTTL
		resetSekaiProfileCacheForTest()
	})

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		resp := &sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{
				UserID: 12345678901231,
				Name:   "cached-user",
				Rank:   88,
			},
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	app := &renderapp.App{
		SekaiAPI: sekaiapi.NewSekaiAPIClient(&harukiConfig.SekaiAPIConfig{BaseURL: server.URL}),
	}

	first, err := fetchCachedSekaiUserProfile(context.Background(), app, "jp", userID)
	if err != nil {
		t.Fatalf("first fetch error: %v", err)
	}
	first.User.Name = "mutated"

	second, err := fetchCachedSekaiUserProfile(context.Background(), app, "jp", userID)
	if err != nil {
		t.Fatalf("second fetch error: %v", err)
	}

	if hits.Load() != 1 {
		t.Fatalf("expected one upstream fetch, got %d", hits.Load())
	}
	if second.User.Name != "cached-user" {
		t.Fatalf("expected cached profile clone, got %q", second.User.Name)
	}
}

func TestFetchCachedSekaiUserProfileSupportsCancellation(t *testing.T) {
	const userID = "12345678901232"
	oldTTL := harukiConfig.Cfg.Backend.APICacheTTL
	harukiConfig.Cfg.Backend.APICacheTTL = time.Minute
	t.Cleanup(func() {
		harukiConfig.Cfg.Backend.APICacheTTL = oldTTL
		resetSekaiProfileCacheForTest()
	})

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	app := &renderapp.App{
		SekaiAPI: sekaiapi.NewSekaiAPIClient(&harukiConfig.SekaiAPIConfig{BaseURL: server.URL}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := fetchCachedSekaiUserProfile(ctx, app, "jp", userID)
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("profile request did not start")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("fetchCachedSekaiUserProfile() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("profile request did not stop after context cancellation")
	}
}

func TestFetchCachedSekaiUserProfileCanceledCallerDoesNotAbortFollower(t *testing.T) {
	const userID = "12345678901233"
	oldTTL := harukiConfig.Cfg.Backend.APICacheTTL
	harukiConfig.Cfg.Backend.APICacheTTL = time.Minute
	t.Cleanup(func() {
		harukiConfig.Cfg.Backend.APICacheTTL = oldTTL
		resetSekaiProfileCacheForTest()
	})

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		raw, _ := json.Marshal(&sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{UserID: 12345678901233, Name: "shared-user"},
		})
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	app := &renderapp.App{
		SekaiAPI: sekaiapi.NewSekaiAPIClient(&harukiConfig.SekaiAPIConfig{BaseURL: server.URL}),
	}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := fetchCachedSekaiUserProfile(firstCtx, app, "jp", userID)
		firstDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("shared profile request did not start")
	}

	followerDone := make(chan struct {
		resp *sekaiapi.GetAnotherProfileResponse
		err  error
	}, 1)
	go func() {
		resp, err := fetchCachedSekaiUserProfile(context.Background(), app, "jp", userID)
		followerDone <- struct {
			resp *sekaiapi.GetAnotherProfileResponse
			err  error
		}{resp: resp, err: err}
	}()

	cancelFirst()
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first caller error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled caller did not return")
	}

	close(release)
	select {
	case result := <-followerDone:
		if result.err != nil {
			t.Fatalf("follower error: %v", result.err)
		}
		if result.resp == nil || result.resp.User.Name != "shared-user" {
			t.Fatalf("unexpected follower response: %#v", result.resp)
		}
	case <-time.After(time.Second):
		t.Fatal("follower did not receive shared response")
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one shared upstream request, got %d", hits.Load())
	}
}

func TestFetchCachedSekaiUserProfileColdFlightReturnsIndependentResponses(t *testing.T) {
	const userID = "12345678901234"
	oldTTL := harukiConfig.Cfg.Backend.APICacheTTL
	harukiConfig.Cfg.Backend.APICacheTTL = time.Minute
	t.Cleanup(func() {
		harukiConfig.Cfg.Backend.APICacheTTL = oldTTL
		resetSekaiProfileCacheForTest()
	})

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		started <- struct{}{}
		<-release
		raw, _ := json.Marshal(&sekaiapi.GetAnotherProfileResponse{
			User: sekaiapi.AnotherUser{UserID: 12345678901234, Name: "independent-user"},
		})
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	app := &renderapp.App{
		SekaiAPI: sekaiapi.NewSekaiAPIClient(&harukiConfig.SekaiAPIConfig{BaseURL: server.URL}),
	}
	type result struct {
		resp *sekaiapi.GetAnotherProfileResponse
		err  error
	}
	contexts := make([]context.Context, 2)
	traces := make([]*commandtrace.Trace, 2)
	for i := range contexts {
		contexts[i], traces[i] = commandtrace.WithTrace(context.Background())
	}
	results := make(chan result, 2)
	go func() {
		resp, err := fetchCachedSekaiUserProfile(contexts[0], app, "jp", userID)
		results <- result{resp: resp, err: err}
	}()
	<-started
	go func() {
		resp, err := fetchCachedSekaiUserProfile(contexts[1], app, "jp", userID)
		results <- result{resp: resp, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)

	first := <-results
	second := <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("flight errors: first=%v second=%v", first.err, second.err)
	}
	if first.resp == nil || second.resp == nil {
		t.Fatalf("nil flight response: first=%#v second=%#v", first.resp, second.resp)
	}
	if first.resp == second.resp {
		t.Fatal("singleflight waiters received the same mutable response pointer")
	}
	first.resp.User.Name = "mutated"
	if second.resp.User.Name != "independent-user" {
		t.Fatalf("mutation leaked across waiters: %q", second.resp.User.Name)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits = %d, want 1", hits.Load())
	}
	sharedCount := 0
	for index, trace := range traces {
		for _, operation := range []string{"sekai.http", "sekai.decode", "sekai.profile_fetch", "sekai.profile_encode", "sekai.profile_decode"} {
			if count := handlerTraceOperationCount(trace, operation); count == 0 {
				t.Fatalf("trace[%d] missing %s: %+v", index, operation, trace.Snapshot().Operations)
			}
		}
		sharedCount += handlerTraceOperationCount(trace, "sekai.profile_cache_shared")
	}
	if sharedCount != 1 {
		t.Fatalf("shared marker count = %d, want 1", sharedCount)
	}
}

func handlerTraceOperationCount(trace *commandtrace.Trace, name string) int {
	if trace == nil {
		return 0
	}
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name {
			return operation.Count
		}
	}
	return 0
}

func TestSekaiProfileCacheEvictsOldestEntryAtCapacity(t *testing.T) {
	resetSekaiProfileCacheForTest()
	t.Cleanup(resetSekaiProfileCacheForTest)

	now := time.Now()
	for i := 0; i <= sekaiProfileCacheMaxEntries; i++ {
		storeCachedSekaiUserProfile("profile-"+strconv.Itoa(i), []byte(`{"user":{}}`), now.Add(time.Hour), now, time.Hour)
	}

	if _, ok := loadCachedSekaiUserProfileRaw("profile-0", now); ok {
		t.Fatal("oldest cache entry was not evicted")
	}
	if _, ok := loadCachedSekaiUserProfileRaw("profile-"+strconv.Itoa(sekaiProfileCacheMaxEntries), now); !ok {
		t.Fatal("newest cache entry was unexpectedly evicted")
	}
	sekaiProfileCacheMu.RLock()
	entryCount := len(sekaiProfileCache)
	cacheBytes := sekaiProfileCacheBytes
	sekaiProfileCacheMu.RUnlock()
	if entryCount != sekaiProfileCacheMaxEntries {
		t.Fatalf("cache entries = %d, want %d", entryCount, sekaiProfileCacheMaxEntries)
	}
	if cacheBytes <= 0 || cacheBytes > sekaiProfileCacheMaxTotalBytes {
		t.Fatalf("cache bytes = %d, want 1..%d", cacheBytes, sekaiProfileCacheMaxTotalBytes)
	}
}

func TestSekaiProfileCacheEvictsToTotalByteLimit(t *testing.T) {
	resetSekaiProfileCacheForTest()
	t.Cleanup(resetSekaiProfileCacheForTest)

	sekaiProfileCacheMu.Lock()
	sekaiProfileCache = map[string]sekaiProfileCacheEntry{
		"oldest": {raw: []byte("1234"), sequence: 1},
		"middle": {raw: []byte("5678"), sequence: 2},
		"newest": {raw: []byte("9012"), sequence: 3},
	}
	sekaiProfileCacheBytes = 12
	evictSekaiUserProfileCacheLocked(3, 8)
	_, oldestPresent := sekaiProfileCache["oldest"]
	entryCount := len(sekaiProfileCache)
	cacheBytes := sekaiProfileCacheBytes
	sekaiProfileCacheMu.Unlock()

	if oldestPresent {
		t.Fatal("oldest entry was not evicted to satisfy byte limit")
	}
	if entryCount != 2 || cacheBytes != 8 {
		t.Fatalf("cache after byte eviction = %d entries/%d bytes, want 2 entries/8 bytes", entryCount, cacheBytes)
	}
}

func TestSekaiProfileCacheSkipsOversizedItem(t *testing.T) {
	resetSekaiProfileCacheForTest()
	t.Cleanup(resetSekaiProfileCacheForTest)

	now := time.Now()
	storeCachedSekaiUserProfile("oversized", make([]byte, sekaiProfileCacheMaxItemBytes+1), now.Add(time.Hour), now, time.Hour)
	if _, ok := loadCachedSekaiUserProfileRaw("oversized", now); ok {
		t.Fatal("oversized profile was cached")
	}
}

func resetSekaiProfileCacheForTest() {
	sekaiProfileCacheMu.Lock()
	sekaiProfileCache = make(map[string]sekaiProfileCacheEntry)
	sekaiProfileCacheBytes = 0
	sekaiProfileCacheSequence = 0
	sekaiProfileCacheMu.Unlock()
	sekaiProfileCacheNextCleanup.Store(0)
}
