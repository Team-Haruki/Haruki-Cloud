package drawing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/utils/logger"
)

func TestRunSharedRenderFlightDetachesCancellationAndKeepsScopedValues(t *testing.T) {
	parent := displaytime.WithRequestTimeZone(context.Background(), "Asia/Tokyo")
	parent = logger.WithContextAttrs(parent, slog.String("request_id", "shared-1"))
	parent, cancel := context.WithCancel(parent)
	cancel()

	var logs bytes.Buffer
	result := runSharedRenderFlight(parent, func(ctx context.Context) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			t.Fatalf("shared context inherited parent cancellation: %v", err)
		}
		if got := displaytime.RequestTimeZoneFromContext(ctx); got != "Asia/Tokyo" {
			t.Fatalf("request timezone = %q, want Asia/Tokyo", got)
		}
		logger.NewLogger("shared-test", "INFO", &logs).InfoContext(ctx, "shared render")
		return []byte("ok"), nil
	})
	if result.err != nil || string(result.data) != "ok" {
		t.Fatalf("shared render result = %q, %v", result.data, result.err)
	}
	for _, want := range []string{"request_id=shared-1", "shared_work=true"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("shared log %q does not contain %q", logs.String(), want)
		}
	}
}

func TestLocalRenderCacheCanceledLeaderDoesNotAbortFollower(t *testing.T) {
	cache := newLocalRenderCache(time.Minute)
	request := map[string]any{"id": "shared-user"}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	render := func(context.Context) ([]byte, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []byte("shared-image"), nil
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := cache.RenderSharedContext(leaderCtx, "/api/pjsk/profile", request, render)
		leaderDone <- err
	}()
	<-started

	type result struct {
		data []byte
		err  error
	}
	followerDone := make(chan result, 1)
	go func() {
		data, err := cache.RenderSharedContext(context.Background(), "/api/pjsk/profile", request, render)
		followerDone <- result{data: data, err: err}
	}()

	// Keep the shared render blocked long enough for the follower to join the
	// in-flight request before the original caller leaves.
	time.Sleep(20 * time.Millisecond)
	cancelLeader()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v, want context.Canceled", err)
	}
	close(release)

	got := <-followerDone
	if got.err != nil {
		t.Fatalf("follower error: %v", got.err)
	}
	if string(got.data) != "shared-image" {
		t.Fatalf("follower data = %q", got.data)
	}
	if calls.Load() != 1 {
		t.Fatalf("render calls = %d, want 1", calls.Load())
	}
}

func TestLocalRenderCacheSharedFlightMergesOperationsIntoEveryWaiter(t *testing.T) {
	cache := newLocalRenderCache(time.Minute)
	request := map[string]any{"id": "trace-shared-user"}
	started := make(chan struct{})
	release := make(chan struct{})
	render := func(ctx context.Context) ([]byte, error) {
		commandtrace.RecordOperation(ctx, "drawing.render", 5*time.Millisecond)
		close(started)
		<-release
		return []byte("shared-image"), nil
	}

	leaderCtx, leaderTrace := commandtrace.WithTrace(context.Background())
	followerCtx, followerTrace := commandtrace.WithTrace(context.Background())
	type result struct {
		data []byte
		err  error
	}
	leaderDone := make(chan result, 1)
	followerDone := make(chan result, 1)
	go func() {
		data, err := cache.RenderSharedContext(leaderCtx, "/api/pjsk/profile", request, render)
		leaderDone <- result{data: data, err: err}
	}()
	<-started
	go func() {
		data, err := cache.RenderSharedContext(followerCtx, "/api/pjsk/profile", request, render)
		followerDone <- result{data: data, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)

	leaderResult := <-leaderDone
	followerResult := <-followerDone
	for name, got := range map[string]result{
		"leader":   leaderResult,
		"follower": followerResult,
	} {
		if got.err != nil || string(got.data) != "shared-image" {
			t.Fatalf("%s result = %q, %v", name, got.data, got.err)
		}
	}
	leaderResult.data[0] = 'X'
	if string(followerResult.data) != "shared-image" {
		t.Fatalf("leader mutation leaked to follower: %q", followerResult.data)
	}
	policy, err := buildRenderCachePolicy("/api/pjsk/profile", request)
	if err != nil {
		t.Fatalf("build cache policy: %v", err)
	}
	key, err := buildRenderCacheKey(policy)
	if err != nil {
		t.Fatalf("build cache key: %v", err)
	}
	if cached, ok := cache.get(key); !ok || string(cached) != "shared-image" {
		t.Fatalf("leader mutation leaked to cache: %q, %t", cached, ok)
	}
	for name, trace := range map[string]*commandtrace.Trace{
		"leader":   leaderTrace,
		"follower": followerTrace,
	} {
		if count := drawingTraceOperationCount(trace, "drawing.render"); count != 1 {
			t.Fatalf("%s drawing.render count = %d, operations=%+v", name, count, trace.Snapshot().Operations)
		}
	}
	if count := drawingTraceOperationCount(followerTrace, "drawing.cache_shared"); count != 1 {
		t.Fatalf("follower drawing.cache_shared count = %d, operations=%+v", count, followerTrace.Snapshot().Operations)
	}
}

func TestLocalRenderCacheEnforcesLRUEntryAndByteLimits(t *testing.T) {
	t.Run("entry limit", func(t *testing.T) {
		cache := newLocalRenderCacheWithLimits(time.Minute, 2, 1024)
		cache.set("oldest", []byte("aaa"), time.Minute, true)
		cache.set("recent", []byte("bbb"), time.Minute, false)
		if _, ok := cache.get("oldest"); !ok {
			t.Fatal("oldest entry unexpectedly missing before LRU refresh")
		}
		cache.set("new", []byte("ccc"), time.Minute, false)

		if _, ok := cache.get("recent"); ok {
			t.Fatal("least-recently-used entry was not evicted")
		}
		for _, key := range []string{"oldest", "new"} {
			if _, ok := cache.get(key); !ok {
				t.Fatalf("retained entry %q was evicted", key)
			}
		}
		entries, bytes, lruEntries := localRenderCacheUsage(cache)
		if entries != 2 || bytes != 6 || lruEntries != entries {
			t.Fatalf("usage = entries:%d bytes:%d lru:%d", entries, bytes, lruEntries)
		}
	})

	t.Run("byte limit", func(t *testing.T) {
		cache := newLocalRenderCacheWithLimits(time.Minute, 10, 5)
		cache.set("old", []byte("aaa"), time.Minute, false)
		cache.set("new", []byte("bbb"), time.Minute, false)
		if _, ok := cache.get("old"); ok {
			t.Fatal("byte limit did not evict the least-recently-used entry")
		}
		if got, ok := cache.get("new"); !ok || string(got) != "bbb" {
			t.Fatalf("new entry = %q, %t", got, ok)
		}
		entries, bytes, lruEntries := localRenderCacheUsage(cache)
		if entries != 1 || bytes != 3 || lruEntries != entries {
			t.Fatalf("usage = entries:%d bytes:%d lru:%d", entries, bytes, lruEntries)
		}
	})

	t.Run("oversized value", func(t *testing.T) {
		cache := newLocalRenderCacheWithLimits(time.Minute, 10, 4)
		cache.set("key", []byte("old"), time.Minute, false)
		cache.set("key", []byte("too-large"), time.Minute, false)
		if _, ok := cache.get("key"); ok {
			t.Fatal("oversized replacement left a stale or oversized entry cached")
		}
		entries, bytes, lruEntries := localRenderCacheUsage(cache)
		if entries != 0 || bytes != 0 || lruEntries != 0 {
			t.Fatalf("usage = entries:%d bytes:%d lru:%d", entries, bytes, lruEntries)
		}
	})
}

func TestLocalRenderCacheSweepsUnvisitedExpiredEntries(t *testing.T) {
	cache := newLocalRenderCacheWithLimits(time.Minute, 10, 1024)
	cache.set("expired", []byte("old"), time.Minute, false)
	cache.set("live", []byte("live"), time.Minute, false)

	cache.mu.Lock()
	cache.entries["expired"].expiresAt = time.Now().Add(-time.Second)
	cache.mu.Unlock()
	cache.set("new", []byte("new"), time.Minute, false)

	if _, ok := cache.get("expired"); ok {
		t.Fatal("unvisited expired entry was not swept")
	}
	entries, bytes, lruEntries := localRenderCacheUsage(cache)
	if entries != 2 || bytes != int64(len("live")+len("new")) || lruEntries != entries {
		t.Fatalf("usage = entries:%d bytes:%d lru:%d", entries, bytes, lruEntries)
	}
}

func TestLocalRenderCacheClonesByteOwnership(t *testing.T) {
	cache := newLocalRenderCacheWithLimits(time.Minute, 10, 1024)
	original := []byte("image")
	cache.set("key", original, time.Minute, false)
	original[0] = 'X'

	first, ok := cache.get("key")
	if !ok || string(first) != "image" {
		t.Fatalf("first cached value = %q, %t", first, ok)
	}
	first[0] = 'Y'
	second, ok := cache.get("key")
	if !ok || string(second) != "image" {
		t.Fatalf("second cached value = %q, %t", second, ok)
	}

	var renders atomic.Int32
	firstRender, err := cache.RenderSharedContext(context.Background(), "/api/pjsk/profile", map[string]any{"id": "ownership"}, func(context.Context) ([]byte, error) {
		renders.Add(1)
		return []byte("rendered"), nil
	})
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	firstRender[0] = 'Z'
	secondRender, err := cache.RenderSharedContext(context.Background(), "/api/pjsk/profile", map[string]any{"id": "ownership"}, func(context.Context) ([]byte, error) {
		renders.Add(1)
		return []byte("unexpected"), nil
	})
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if string(secondRender) != "rendered" || renders.Load() != 1 {
		t.Fatalf("second render = %q, render calls = %d", secondRender, renders.Load())
	}
}

func TestLocalRenderCacheConcurrentUsageStaysBounded(t *testing.T) {
	cache := newLocalRenderCacheWithLimits(time.Minute, 16, 512)
	const workers = 32
	const iterations = 100
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for iteration := range iterations {
				key := fmt.Sprintf("%d:%d", worker, iteration)
				cache.set(key, []byte(strings.Repeat("x", 32)), time.Minute, false)
				_, _ = cache.get(key)
			}
		}(worker)
	}
	wg.Wait()

	entries, bytes, lruEntries := localRenderCacheUsage(cache)
	if entries > 16 || bytes > 512 || lruEntries != entries {
		t.Fatalf("usage exceeded bounds: entries:%d bytes:%d lru:%d", entries, bytes, lruEntries)
	}
}

func localRenderCacheUsage(cache *localRenderCache) (entries int, bytes int64, lruEntries int) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.sweepExpiredLocked(time.Now())
	return len(cache.entries), cache.totalBytes, cache.lru.Len()
}

func TestResolveRenderCacheRuleUsesOneDayTTLByDefault(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/profile")
	if rule.TTL != renderCacheTTLOneDay {
		t.Fatalf("expected default ttl %s, got %s", renderCacheTTLOneDay, rule.TTL)
	}
}

func TestResolveRenderCacheRuleUsesHalfDayTTLForSelectedEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"/api/pjsk/deck/recommend",
		"/api/pjsk/music/rewards/basic",
		"/api/pjsk/music/rewards/detail",
		"/api/pjsk/mysekai/map",
		"/api/pjsk/mysekai/resource",
		"/api/pjsk/mysekai/talk-list",
	} {
		rule := resolveRenderCacheRule(endpoint)
		if rule.TTL != renderCacheTTLHalfDay {
			t.Fatalf("%s ttl = %s, want %s", endpoint, rule.TTL, renderCacheTTLHalfDay)
		}
	}
}

func TestResolveRenderCacheRuleDisablesEventDetail(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/event/detail")
	if rule.Enabled {
		t.Fatal("event detail render cache should be disabled")
	}
}

func TestResolveRenderCacheRuleEnablesCharacterBirthday(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/misc/chara-birthday")
	if !rule.Enabled {
		t.Fatal("character birthday render cache should be enabled")
	}
	if rule.TTL != renderCacheTTLOneDay {
		t.Fatalf("character birthday ttl = %s, want %s", rule.TTL, renderCacheTTLOneDay)
	}
}

func TestResolveRenderCacheRuleUsesSevenDayTTLForCustomProfileCard(t *testing.T) {
	rule := resolveRenderCacheRule("/api/pjsk/profile/custom-profile-card")
	if rule.TTL != renderCacheTTLSevenDay {
		t.Fatalf("custom profile card ttl = %s, want %s", rule.TTL, renderCacheTTLSevenDay)
	}
}

func TestResolveRenderCacheRuleUsesCostumeTTLs(t *testing.T) {
	detail := resolveRenderCacheRule("/api/pjsk/costume/detail")
	if detail.TTL != renderCacheTTLSevenDay {
		t.Fatalf("costume detail ttl = %s, want %s", detail.TTL, renderCacheTTLSevenDay)
	}
	list := resolveRenderCacheRule("/api/pjsk/costume/list")
	if list.TTL != renderCacheTTLOneDay {
		t.Fatalf("costume list ttl = %s, want %s", list.TTL, renderCacheTTLOneDay)
	}
}

func TestRenderCacheClientRejectsUntrustedSelfSignedHTTPS(t *testing.T) {
	storageDir := t.TempDir()
	cacheKey := strings.Repeat("a", 64)
	cachePath := filepath.Join(storageDir, "cached.png")
	if err := os.WriteFile(cachePath, []byte("cached-image"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cache" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != cacheKey {
			t.Fatalf("unexpected key: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"key":%q,"file_path":%q}`, cacheKey, cachePath)
	}))
	defer server.Close()

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    server.URL,
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}

	if _, ok := client.lookup(cacheKey, "api/pjsk/profile"); ok {
		t.Fatal("cache lookup accepted an untrusted self-signed HTTPS certificate")
	}
}

func TestRenderCacheClientLimitsCacheAPIResponses(t *testing.T) {
	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    "http://127.0.0.1:1",
		StorageDir: t.TempDir(),
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}
	if got := client.http.ResponseBodyLimit; got != renderCacheAPIResponseMaxBytes {
		t.Fatalf("response body limit = %d, want %d", got, renderCacheAPIResponseMaxBytes)
	}
}

func TestRenderCacheClientRejectsCacheFilesOutsideStorage(t *testing.T) {
	storageDir := t.TempDir()
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.png")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"file_path":%q}`, outsidePath)
	}))
	defer server.Close()

	client := NewRenderCacheClient(RenderCacheConfig{BaseURL: server.URL, StorageDir: storageDir, TTL: time.Minute})
	if _, ok := client.lookup("outside", "api/pjsk/profile"); ok {
		t.Fatal("cache lookup accepted a file outside its storage directory")
	}
}

func TestRenderCacheClientRejectsCacheFileSymlinkEscape(t *testing.T) {
	storageDir := t.TempDir()
	outsidePath := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	linkPath := filepath.Join(storageDir, "linked.png")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    "http://127.0.0.1:1",
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if _, err := client.readCacheFile(linkPath); err == nil {
		t.Fatal("cache read accepted a symlink escaping its storage directory")
	}
}

func TestRenderCacheClientRejectsOversizedCacheFile(t *testing.T) {
	storageDir := t.TempDir()
	cachePath := filepath.Join(storageDir, "oversized.png")
	file, err := os.Create(cachePath)
	if err != nil {
		t.Fatalf("create cache file: %v", err)
	}
	if err := file.Truncate(drawingMaxResponseBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("truncate cache file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close cache file: %v", err)
	}

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    "http://127.0.0.1:1",
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if _, err := client.readCacheFile(cachePath); err == nil {
		t.Fatal("cache read accepted an oversized file")
	}
}

func TestRenderCacheClientRejectsSymlinkedStoreDirectory(t *testing.T) {
	storageDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(storageDir, "api")); err != nil {
		t.Fatalf("create directory symlink: %v", err)
	}

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    "http://127.0.0.1:1",
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	target := client.defaultFilePath("api/pjsk/profile", "public", "key")
	if _, err := client.prepareCacheTarget(target); err == nil {
		t.Fatal("cache store accepted a symlinked directory component")
	}
}

func TestRenderCachePathComponentsAreNormalized(t *testing.T) {
	if got := normalizeRenderCacheAPIPath("api/pjsk/../profile"); got != "" {
		t.Fatalf("unsafe API path normalized to %q", got)
	}
	unsafeUserID := "../../outside"
	got := normalizeRenderCacheUserID(unsafeUserID)
	if got == unsafeUserID || !strings.HasPrefix(got, "user-") || strings.ContainsAny(got, `/\\`) {
		t.Fatalf("unsafe user ID normalized to %q", got)
	}
}

func TestRenderCacheClientRemoteMissUsesSingleflight(t *testing.T) {
	storageDir := t.TempDir()
	var renderCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cache":
			http.Error(w, `{"error":"miss"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/cache":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    server.URL,
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}

	endpoint := "/api/pjsk/sk/query"
	request := map[string]any{
		"region": "jp",
		"ranks":  []any{1, 2, 3},
	}
	render := func() ([]byte, error) {
		atomic.AddInt32(&renderCalls, 1)
		time.Sleep(50 * time.Millisecond)
		return []byte("rendered-image"), nil
	}

	const callers = 8
	var wg sync.WaitGroup
	results := make([][]byte, callers)
	errs := make([]error, callers)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = client.Render(endpoint, request, render)
		}(i)
	}
	wg.Wait()
	client.waitForPendingStores()

	if got := atomic.LoadInt32(&renderCalls); got != 1 {
		t.Fatalf("render called %d times, want 1", got)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d returned error: %v", i, err)
		}
		if string(results[i]) != "rendered-image" {
			t.Fatalf("call %d returned %q, want %q", i, string(results[i]), "rendered-image")
		}
	}
	results[0][0] = 'X'
	if string(results[1]) != "rendered-image" {
		t.Fatalf("one waiter mutation leaked to another: %q", results[1])
	}
}

func TestRenderCacheClientSharedFlightMergesOperationsIntoEveryWaiter(t *testing.T) {
	storageDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cache":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/cache":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewRenderCacheClient(RenderCacheConfig{BaseURL: server.URL, StorageDir: storageDir, TTL: time.Minute})
	request := map[string]any{"region": "jp", "ranks": []any{1, 2, 3}}
	started := make(chan struct{})
	release := make(chan struct{})
	render := func(ctx context.Context) ([]byte, error) {
		commandtrace.RecordOperation(ctx, "drawing.render", 5*time.Millisecond)
		close(started)
		<-release
		return []byte("rendered-image"), nil
	}

	leaderCtx, leaderTrace := commandtrace.WithTrace(context.Background())
	followerCtx, followerTrace := commandtrace.WithTrace(context.Background())
	type result struct {
		data []byte
		err  error
	}
	leaderDone := make(chan result, 1)
	followerDone := make(chan result, 1)
	go func() {
		data, err := client.RenderSharedContext(leaderCtx, "/api/pjsk/sk/query", request, render)
		leaderDone <- result{data: data, err: err}
	}()
	<-started
	go func() {
		data, err := client.RenderSharedContext(followerCtx, "/api/pjsk/sk/query", request, render)
		followerDone <- result{data: data, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)

	for name, completed := range map[string]<-chan result{
		"leader":   leaderDone,
		"follower": followerDone,
	} {
		got := <-completed
		if got.err != nil || string(got.data) != "rendered-image" {
			t.Fatalf("%s result = %q, %v", name, got.data, got.err)
		}
	}
	client.waitForPendingStores()
	for name, trace := range map[string]*commandtrace.Trace{
		"leader":   leaderTrace,
		"follower": followerTrace,
	} {
		for _, operation := range []string{
			"drawing.cache_lookup",
			"drawing.cache_lookup_http",
			"drawing.render",
		} {
			if count := drawingTraceOperationCount(trace, operation); count != 1 {
				t.Fatalf("%s %s count = %d, operations=%+v", name, operation, count, trace.Snapshot().Operations)
			}
		}
		// The store runs write-behind after the flight returns, so store-side
		// operations must no longer appear on any waiter's critical path.
		for _, operation := range []string{
			"drawing.cache_store",
			"drawing.cache_hash",
			"drawing.cache_write",
			"drawing.cache_store_http",
		} {
			if count := drawingTraceOperationCount(trace, operation); count != 0 {
				t.Fatalf("%s %s count = %d, want 0 (write-behind), operations=%+v", name, operation, count, trace.Snapshot().Operations)
			}
		}
	}
	if count := drawingTraceOperationCount(followerTrace, "drawing.cache_shared"); count != 1 {
		t.Fatalf("follower drawing.cache_shared count = %d, operations=%+v", count, followerTrace.Snapshot().Operations)
	}
}

func drawingTraceOperationCount(trace *commandtrace.Trace, name string) int {
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

func TestRenderCacheClientStoresRenderedImageUnderRequestKeyDir(t *testing.T) {
	storageDir := t.TempDir()
	var registeredPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cache":
			http.Error(w, `{"error":"miss"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/cache":
			registeredPath = r.FormValue("file_path")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    server.URL,
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}

	image := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}
	data, err := client.Render("/api/pjsk/profile", map[string]any{"id": "123"}, func() ([]byte, error) {
		return image, nil
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(data) != string(image) {
		t.Fatalf("unexpected image bytes")
	}
	client.waitForPendingStores()
	if !strings.HasPrefix(registeredPath, filepath.Join(storageDir, "api", "pjsk", "profile", "public")+string(os.PathSeparator)) {
		t.Fatalf("registered path %q should keep request-scoped directory", registeredPath)
	}
	if filepath.Ext(registeredPath) != ".jpg" {
		t.Fatalf("registered path ext = %q, want .jpg", filepath.Ext(registeredPath))
	}
	if _, err := os.Stat(registeredPath); err != nil {
		t.Fatalf("expected shared image file to exist: %v", err)
	}
}

func TestRenderCacheClientKeepsExistingContentFileWhenRegisterFails(t *testing.T) {
	storageDir := t.TempDir()
	image := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}

	client := NewRenderCacheClient(RenderCacheConfig{
		BaseURL:    "http://127.0.0.1:1",
		StorageDir: storageDir,
		TTL:        time.Minute,
	})
	if client == nil {
		t.Fatal("expected render cache client")
	}
	_, targetPath := client.contentFilePath("api/pjsk/profile", "public", strings.Repeat("c", 64), image)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(targetPath, image, 0o644); err != nil {
		t.Fatalf("write existing content file: %v", err)
	}

	err := client.store(strings.Repeat("c", 64), "api/pjsk/profile", "public", image, time.Minute, false)
	if err == nil {
		t.Fatal("expected register failure")
	}
	if _, statErr := os.Stat(targetPath); statErr != nil {
		t.Fatalf("existing content file should remain after register failure: %v", statErr)
	}
}

func TestBuildRenderCachePolicyAliasListIsInfiniteAndIgnoresDT(t *testing.T) {
	reqA := map[string]any{
		"title":        "角色别名",
		"entity_label": "角色ID",
		"entity_id":    5,
		"entity_name":  "花里みのり",
		"aliases":      []any{"花里", "花里みのり", "minori"},
		"dt":           int64(1713852000000), // 2024-04-23 12:00:00 UTC
	}
	reqB := map[string]any{
		"title":        "角色别名",
		"entity_label": "角色ID",
		"entity_id":    5,
		"entity_name":  "花里みのり",
		"aliases":      []any{"花里", "花里みのり", "minori"},
		"dt":           int64(1713852660000), // +11m
	}

	policyA, err := buildRenderCachePolicy("/api/pjsk/misc/alias-list", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/misc/alias-list", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("alias-list key should ignore dt when aliases are unchanged: %s != %s", keyA, keyB)
	}
	if !policyA.Infinite {
		t.Fatalf("expected alias-list cache policy to be infinite")
	}
	if policyA.TTL != 0 {
		t.Fatalf("expected infinite alias-list cache ttl to be 0, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyCharacterBirthdayIgnoresDT(t *testing.T) {
	reqA := map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
		"dt": int64(1781251200000),
	}
	reqB := map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
		"dt": int64(1781254800000),
	}

	policyA, err := buildRenderCachePolicy("/api/pjsk/misc/chara-birthday", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/misc/chara-birthday", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("birthday key should ignore dt when request content is unchanged: %s != %s", keyA, keyB)
	}
}

func TestBuildRenderCachePolicyCharacterBirthdayExpiresAtNextDayBoundary(t *testing.T) {
	now := time.Date(2026, time.June, 12, 22, 30, 0, 0, time.FixedZone("CST", 8*3600))
	policy, err := buildRenderCachePolicy("/api/pjsk/misc/chara-birthday", map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
		"timezone": "Asia/Shanghai",
		"dt":       now.UnixMilli(),
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}

	if policy.TTL != 90*time.Minute {
		t.Fatalf("birthday ttl = %s, want %s", policy.TTL, 90*time.Minute)
	}
}

func TestBuildRenderCachePolicyCharacterBirthdayVariesByCharacterAndDay(t *testing.T) {
	base := map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
	}
	dayChanged := map[string]any{
		"cid":                 6,
		"month":               6,
		"day":                 12,
		"region_name":         "JP",
		"days_until_birthday": 1,
		"color_code":          "#33AAEE",
		"sd_image_path":       "sd/6.png",
		"title_image_path":    "title/6.png",
		"card_image_path":     "card/6.png",
		"cards": []any{
			map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 6, "month": 6, "day": 12, "icon_path": "icon/6.png"},
		},
	}
	characterChanged := map[string]any{
		"cid":                 7,
		"month":               6,
		"day":                 24,
		"region_name":         "JP",
		"days_until_birthday": 0,
		"color_code":          "#EE8833",
		"sd_image_path":       "sd/7.png",
		"title_image_path":    "title/7.png",
		"card_image_path":     "card/7.png",
		"cards": []any{
			map[string]any{"id": 1002, "thumbnail_path": "thumb/1002.png"},
		},
		"all_characters": []any{
			map[string]any{"cid": 7, "month": 6, "day": 24, "icon_path": "icon/7.png"},
		},
	}

	baseKey := mustBuildRenderCacheKey(t, "/api/pjsk/misc/chara-birthday", base)
	dayChangedKey := mustBuildRenderCacheKey(t, "/api/pjsk/misc/chara-birthday", dayChanged)
	characterChangedKey := mustBuildRenderCacheKey(t, "/api/pjsk/misc/chara-birthday", characterChanged)

	if baseKey == dayChangedKey {
		t.Fatal("birthday key should change when days_until_birthday changes")
	}
	if baseKey == characterChangedKey {
		t.Fatal("birthday key should change when character changes")
	}
}

func TestResolveRenderCacheRuleUsesInfiniteTTLForStaticEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"/api/pjsk/card/detail",
		"/api/pjsk/card/list",
		"/api/pjsk/help/render",
		"/api/pjsk/mysekai/fixture-list",
		"/api/pjsk/mysekai/fixture-detail",
	} {
		rule := resolveRenderCacheRule(endpoint)
		if !rule.Infinite {
			t.Fatalf("%s should use infinite ttl", endpoint)
		}
		if rule.TTL != 0 {
			t.Fatalf("%s ttl = %s, want 0 for infinite ttl", endpoint, rule.TTL)
		}
	}
}

func TestBuildRenderCachePolicyEventListUsesNextPhaseBoundaryTTL(t *testing.T) {
	now := int64(1774118400000)
	endAt := now + int64((2*time.Hour)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"dt": now,
		"event_info": []any{
			map[string]any{
				"id":       101,
				"start_at": now - int64((time.Hour)/time.Millisecond),
				"end_at":   endAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	want := 2 * time.Hour
	if policy.TTL != want {
		t.Fatalf("event list ttl = %v, want %v", policy.TTL, want)
	}
	if policy.Infinite {
		t.Fatal("event list should no longer use infinite ttl")
	}
}

func TestBuildRenderCachePolicyEventListExpiresAtNextEventStart(t *testing.T) {
	now := int64(1774118400000)
	nextStartAt := now + int64((45*time.Minute)/time.Millisecond)
	nextEndAt := nextStartAt + int64((7*24*time.Hour)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"dt": now,
		"event_info": []any{
			map[string]any{
				"id":       101,
				"start_at": now - int64((7*24*time.Hour)/time.Millisecond),
				"end_at":   now - int64((time.Hour)/time.Millisecond),
			},
			map[string]any{
				"id":       102,
				"start_at": nextStartAt,
				"end_at":   nextEndAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	want := 45 * time.Minute
	if policy.TTL != want {
		t.Fatalf("event list ttl = %v, want %v", policy.TTL, want)
	}
}

func TestBuildRenderCachePolicyVLiveUsesDynamicWindowTTL(t *testing.T) {
	now := int64(1774118400000)
	endAt := now + int64((90*time.Minute)/time.Millisecond)
	policy, err := buildRenderCachePolicy("/api/pjsk/vlive/list", map[string]any{
		"dt": now,
		"lives": []any{
			map[string]any{
				"id":     1,
				"end_at": endAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	want := 90*time.Minute + renderCacheWindowTTLBuffer
	if policy.TTL != want {
		t.Fatalf("vlive ttl = %v, want %v", policy.TTL, want)
	}
}

func TestBuildRenderCachePolicyEventDetailIsDisabled(t *testing.T) {
	now := int64(1774118400000)
	endAt := now - int64((time.Hour)/time.Millisecond)
	_, err := buildRenderCachePolicy("/api/pjsk/event/detail", map[string]any{
		"dt": now,
		"event_info": map[string]any{
			"id":     101,
			"end_at": endAt,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "render cache disabled") {
		t.Fatalf("expected disabled render cache error, got %v", err)
	}
}

func TestBuildRenderCachePolicyMarksCardListAsInfinite(t *testing.T) {
	policy, err := buildRenderCachePolicy("/api/pjsk/card/list", CardListRequest{
		Region: "JP",
		Cards: []CardBasic{
			{CardID: 1001},
		},
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	if !policy.Infinite {
		t.Fatalf("expected card list cache policy to be infinite")
	}
	if policy.TTL != 0 {
		t.Fatalf("expected infinite cache policy ttl to be 0, got %s", policy.TTL)
	}
}

func TestBuildRenderCachePolicyIgnoresEventRecordUserUpdateTime(t *testing.T) {
	reqA := EventRecordRequest{
		EventInfo: []EventHistory{{ID: 1, EventName: "Event"}},
		UserInfo: DetailedProfileCardRequest{
			ID:              "123",
			Region:          "JP",
			Nickname:        "Tester",
			Source:          "suite",
			UpdateTime:      1,
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
	}
	reqB := reqA
	reqB.UserInfo.UpdateTime = 2

	policyA, err := buildRenderCachePolicy("/api/pjsk/event/record", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/event/record", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("event record key should ignore user update time: %s != %s", keyA, keyB)
	}
	if policyA.APIPath != policyB.APIPath {
		t.Fatalf("event record api_path should stay stable: %s != %s", policyA.APIPath, policyB.APIPath)
	}
}

func TestBuildRenderCachePolicyIgnoresUnusedProfileUpdateTime(t *testing.T) {
	reqA := ProfileRequest{
		Profile: BasicProfile{
			ID:              "123",
			Region:          "JP",
			Nickname:        "Tester",
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
		UpdateTime: new(int64(1)),
	}
	reqB := reqA
	reqB.UpdateTime = new(int64(2))

	policyA, err := buildRenderCachePolicy("/api/pjsk/profile", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/profile", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("profile key should ignore unused update_time: %s != %s", keyA, keyB)
	}
	if policyA.APIPath != policyB.APIPath {
		t.Fatalf("profile api_path should stay stable: %s != %s", policyA.APIPath, policyB.APIPath)
	}
}

func TestBuildRenderCachePolicyBucketsSKWinRateUpdatedAtBy10Seconds(t *testing.T) {
	reqA := WinRateRequest{
		UpdatedAt:        1774118400000,
		EventStartAt:     10,
		EventAggregateAt: 1774118404000,
		TeamInfo: []TeamInfo{
			{TeamID: 1, TeamName: "A", WinRate: 0.5},
			{TeamID: 2, TeamName: "B", WinRate: 0.5},
		},
	}
	reqB := reqA
	reqB.UpdatedAt = 1774118409000
	reqB.EventAggregateAt = 1774118409000
	reqC := reqA
	reqC.UpdatedAt = 1774118411000
	reqC.EventAggregateAt = 1774118411000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/winrate", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("winrate key should bucket updated_at/event_aggregate_at within 10s: %s != %s", keyA, keyB)
	}
	if keyA == keyC {
		t.Fatalf("winrate key should change after 10s bucket boundary")
	}
	if policyA.TTL != 10*time.Second {
		t.Fatalf("expected 10s ttl for winrate cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyMusicListUsesRenderFlagsAndPublicFallback(t *testing.T) {
	req := MusicListRequest{
		UserResults: map[int]any{1: "ap"},
		MusicList: []map[string]any{
			{"id": 1, "difficulty": 32},
		},
		RequiredDifficulties: "master",
		Profile: &DetailedProfileCardRequest{
			ID:              "service",
			Region:          "JP",
			Nickname:        "Lunabot",
			Source:          "lunabot-service",
			UpdateTime:      1,
			IsHideUID:       true,
			LeaderImagePath: "leader.png",
		},
	}

	showPolicy, err := buildRenderCachePolicy("/api/pjsk/music/list?show_id=true&show_leak=false", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy show: %v", err)
	}
	hidePolicy, err := buildRenderCachePolicy("/api/pjsk/music/list?show_id=false&show_leak=false", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy hide: %v", err)
	}

	if showPolicy.UserID != renderCachePublic {
		t.Fatalf("expected public fallback user_id, got %s", showPolicy.UserID)
	}
	if showPolicy.APIPath != "api/pjsk/music/list" {
		t.Fatalf("unexpected api_path: %s", showPolicy.APIPath)
	}
	if showPolicy.APIPath != hidePolicy.APIPath {
		t.Fatalf("expected stable api_path across render flags")
	}

	keyShow, err := buildRenderCacheKey(showPolicy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey show: %v", err)
	}
	keyHide, err := buildRenderCacheKey(hidePolicy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey hide: %v", err)
	}
	if keyShow == keyHide {
		t.Fatalf("expected different keys for different render flags")
	}
}

func TestBuildRenderCachePolicySKQueryIgnoresTopLevelEventIDForUserID(t *testing.T) {
	req := SKRequest{
		ID:     1,
		Region: "JP",
		Name:   "Event",
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester"},
		},
	}

	policy, err := buildRenderCachePolicy("/api/pjsk/sk/query", req)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}

	if policy.UserID != renderCachePublic {
		t.Fatalf("expected public user_id, got %s", policy.UserID)
	}
	if policy.APIPath != "api/pjsk/sk/query" {
		t.Fatalf("unexpected api_path: %s", policy.APIPath)
	}
}

func TestBuildRenderCachePolicySKQueryOnlyChangesWhenPulledInfoChanges(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "JP",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqC := reqA
	reqC.AggregateAt = 1774118420000
	reqC.Ranks[0].Time = 1774118420000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("sk query key should stay stable when pulled info is unchanged: %s != %s", keyA, keyB)
	}
	if keyA == keyC {
		t.Fatalf("sk query key should change after pulled info changes")
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyTWSKQueryDoesNotBucketByTime(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "TW",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqB.AggregateAt = 1774118429000
	reqB.Ranks[0].Time = 1774118429000
	reqC := reqA
	reqC.AggregateAt = 1774118404000
	reqC.Ranks[0].Time = 1774118405000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA == keyB {
		t.Fatalf("tw sk query key should change when tracker timestamps differ")
	}
	if keyA != keyC {
		t.Fatalf("tw sk query key should remain identical for identical content: %s != %s", keyA, keyC)
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for tw sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyENSKQueryDoesNotBucketByTime(t *testing.T) {
	reqA := SKRequest{
		ID:          1,
		Region:      "EN",
		Name:        "Event",
		AggregateAt: 1774118404000,
		Ranks: []RankInfo{
			{Rank: 100, Name: "Tester", Time: 1774118405000},
		},
	}
	reqB := reqA
	reqB.AggregateAt = 1774118459000
	reqB.Ranks[0].Time = 1774118459000
	reqC := reqA
	reqC.AggregateAt = 1774118404000
	reqC.Ranks[0].Time = 1774118405000

	policyA, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqA)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqB)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}
	policyC, err := buildRenderCachePolicy("/api/pjsk/sk/query", reqC)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqC: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}
	keyC, err := buildRenderCacheKey(policyC)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqC: %v", err)
	}

	if keyA == keyB {
		t.Fatalf("en sk query key should change when tracker timestamps differ")
	}
	if keyA != keyC {
		t.Fatalf("en sk query key should remain identical for identical content: %s != %s", keyA, keyC)
	}
	if policyA.TTL != renderCacheTTLHalfDay {
		t.Fatalf("expected half-day ttl for en sk query cache, got %v", policyA.TTL)
	}
}

func TestBuildRenderCachePolicyIgnoresRootDT(t *testing.T) {
	policyA, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"region": "JP",
		"dt":     1774118400000,
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqA: %v", err)
	}
	policyB, err := buildRenderCachePolicy("/api/pjsk/event/list", map[string]any{
		"region": "JP",
		"dt":     1774118700000,
	})
	if err != nil {
		t.Fatalf("buildRenderCachePolicy reqB: %v", err)
	}

	keyA, err := buildRenderCacheKey(policyA)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqA: %v", err)
	}
	keyB, err := buildRenderCacheKey(policyB)
	if err != nil {
		t.Fatalf("buildRenderCacheKey reqB: %v", err)
	}

	if keyA != keyB {
		t.Fatalf("expected dt to be ignored by cache key: %s != %s", keyA, keyB)
	}
}

func TestBuildRenderCachePolicyDoesNotMutatePreparedRenderPayload(t *testing.T) {
	payload := map[string]any{
		"dt":         float64(1774118400000),
		"model_name": "challenge-v1",
		"cost_times": []any{float64(5), float64(10)},
		"profile": map[string]any{
			"update_time": float64(1774118400000),
		},
	}

	policy, err := buildRenderCachePolicy(
		"/api/pjsk/deck/recommend",
		preparedRenderCachePayload{payload: payload},
	)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}

	if _, ok := payload["dt"]; !ok {
		t.Fatal("prepared render payload lost dt")
	}
	if got := payload["model_name"]; got != "challenge-v1" {
		t.Fatalf("prepared render payload model_name = %v", got)
	}
	if got := len(payload["cost_times"].([]any)); got != 2 {
		t.Fatalf("prepared render payload cost_times length = %d", got)
	}
	if _, ok := payload["profile"].(map[string]any)["update_time"]; !ok {
		t.Fatal("prepared render payload lost profile.update_time")
	}

	cachePayload := policy.Params.(map[string]any)
	for _, key := range []string{"dt", "model_name", "cost_times"} {
		if _, ok := cachePayload[key]; ok {
			t.Fatalf("cache payload retained ignored field %s", key)
		}
	}
	if _, ok := cachePayload["profile"].(map[string]any)["update_time"]; ok {
		t.Fatal("cache payload retained ignored profile.update_time")
	}
}

func mustBuildRenderCacheKey(t *testing.T, endpoint string, request any) string {
	t.Helper()
	policy, err := buildRenderCachePolicy(endpoint, request)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy: %v", err)
	}
	key, err := buildRenderCacheKey(policy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey: %v", err)
	}
	return key
}

func TestBuildRenderCachePolicyCardBoxIgnoresUserInfoUpdateTime(t *testing.T) {
	buildRequest := func(updateTime int64) map[string]any {
		return map[string]any{
			"cards": []any{
				map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png", "level": 50},
			},
			"show_box": true,
			"user_info": map[string]any{
				"id":          "123456",
				"source":      "snapshot",
				"nickname":    "player",
				"update_time": updateTime,
			},
		}
	}

	keyA := mustBuildRenderCacheKeyForTest(t, "/api/pjsk/card/box", buildRequest(1781251200000))
	keyB := mustBuildRenderCacheKeyForTest(t, "/api/pjsk/card/box", buildRequest(1781254800000))
	if keyA != keyB {
		t.Fatalf("card box key should ignore user_info.update_time: %s != %s", keyA, keyB)
	}

	changed := buildRequest(1781251200000)
	changed["cards"] = []any{
		map[string]any{"id": 2002, "thumbnail_path": "thumb/2002.png", "level": 1},
	}
	keyC := mustBuildRenderCacheKeyForTest(t, "/api/pjsk/card/box", changed)
	if keyA == keyC {
		t.Fatalf("card box key must still vary with box contents")
	}
}

func TestBuildRenderCachePolicyCardListIgnoresUserInfoUpdateTime(t *testing.T) {
	buildRequest := func(updateTime int64) map[string]any {
		return map[string]any{
			"cards": []any{
				map[string]any{"id": 1001, "thumbnail_path": "thumb/1001.png"},
			},
			"user_info": map[string]any{
				"id":          "123456",
				"source":      "snapshot",
				"nickname":    "player",
				"update_time": updateTime,
			},
		}
	}

	keyA := mustBuildRenderCacheKeyForTest(t, "/api/pjsk/card/list", buildRequest(1781251200000))
	keyB := mustBuildRenderCacheKeyForTest(t, "/api/pjsk/card/list", buildRequest(1781254800000))
	if keyA != keyB {
		t.Fatalf("card list key should ignore user_info.update_time: %s != %s", keyA, keyB)
	}
}

func mustBuildRenderCacheKeyForTest(t *testing.T, endpoint string, payload map[string]any) string {
	t.Helper()
	policy, err := buildRenderCachePolicy(endpoint, payload)
	if err != nil {
		t.Fatalf("buildRenderCachePolicy %s: %v", endpoint, err)
	}
	key, err := buildRenderCacheKey(policy)
	if err != nil {
		t.Fatalf("buildRenderCacheKey %s: %v", endpoint, err)
	}
	return key
}
