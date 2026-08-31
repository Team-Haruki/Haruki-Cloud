package sekai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
)

func TestTrackerClientWithNilConfigIsNotConfigured(t *testing.T) {
	client := NewTrackerClient(nil)
	if _, err := client.GetEventStatus("jp", 1); !errors.Is(err, ErrClientNotConfigured) {
		t.Fatalf("GetEventStatus() error = %v, want ErrClientNotConfigured", err)
	}
}

func TestTrackerClientDeduplicatesConcurrentCloudV2GETByPath(t *testing.T) {
	var hits atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/cloud/events/jp/101/leaderboards/total/sk/query" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua != "tracker-test" {
			t.Fatalf("unexpected user agent: %q", ua)
		}
		hits.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{"server":"jp","eventId":101,"scope":"total","fetchedAt":1},"ranks":[{"rank":100,"userId":"10001","name":"Tester","score":1234567,"timestamp":1704067200}]}`))
	}))
	defer server.Close()

	client := NewTrackerClient(&config.TrackerConfig{BaseURL: server.URL, UserAgent: "tracker-test"})

	const callers = 16
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	contexts := make([]context.Context, callers)
	traces := make([]*commandtrace.Trace, callers)
	for i := range callers {
		contexts[i], traces[i] = commandtrace.WithTrace(context.Background())
	}
	wg.Add(callers)
	for index := range callers {
		go executeTrackerQuery(t, client, contexts[index], errs, &wg)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("tracker test server was not hit")
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("tracker request failed: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected one upstream GET, got %d", got)
	}
	sharedCount := assertTrackerTraceOperations(t, traces)
	if sharedCount != callers-1 {
		t.Fatalf("tracker.shared count = %d, want %d", sharedCount, callers-1)
	}
}

func executeTrackerQuery(t *testing.T, client *TrackerClient, ctx context.Context, errs chan<- error, wg *sync.WaitGroup) {
	t.Helper()
	defer wg.Done()
	resp, err := client.WithContext(ctx).GetCloudSKQuery("jp", 101, nil, []int{100}, nil, false, false, 3600)
	if err != nil {
		errs <- err
		return
	}
	if len(resp.Ranks) != 1 || resp.Ranks[0].Rank != 100 || resp.Ranks[0].Name != "Tester" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func assertTrackerTraceOperations(t *testing.T, traces []*commandtrace.Trace) int {
	t.Helper()
	sharedCount := 0
	for index, trace := range traces {
		for _, operation := range []string{"tracker.wait", "tracker.http", "tracker.decode"} {
			if count := trackerTraceOperationCount(trace, operation); count == 0 {
				t.Fatalf("trace[%d] missing %s: %+v", index, operation, trace.Snapshot().Operations)
			}
		}
		sharedCount += trackerTraceOperationCount(trace, "tracker.shared")
	}
	return sharedCount
}

func trackerTraceOperationCount(trace *commandtrace.Trace, name string) int {
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

func TestTrackerClientCloudV2Paths(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/sk/query"):
			_, _ = w.Write([]byte(`{"meta":{"server":"cn","eventId":170,"scope":"world-bloom/19","characterId":19,"fetchedAt":1},"ranks":[{"rank":1,"userId":"10001","name":"Tester","score":123,"timestamp":1704067200,"characterId":19}],"previous":{"rank":2,"score":100,"timestamp":1704067200},"next":{"rank":3,"score":90,"timestamp":1704067200}}`))
		case strings.Contains(r.URL.Path, "/sk/check-room"):
			_, _ = w.Write([]byte(`{"meta":{"server":"cn","eventId":170,"scope":"world-bloom/19","characterId":19,"fetchedAt":1},"rank":{"rank":1,"userId":"10001","name":"Tester","score":123,"timestamp":1704067200,"characterId":19}}`))
		case strings.Contains(r.URL.Path, "/sk/line"):
			_, _ = w.Write([]byte(`{"meta":{"server":"cn","eventId":170,"scope":"world-bloom/19","characterId":19,"fetchedAt":1},"ranks":[{"rank":100,"score":456,"timestamp":1704067200,"characterId":19}]}`))
		case strings.Contains(r.URL.Path, "/sk/speed"):
			_, _ = w.Write([]byte(`{"meta":{"server":"cn","eventId":170,"scope":"world-bloom/19","characterId":19,"fetchedAt":1},"speeds":[{"rank":100,"score":456,"timestamp":1704067200,"speed":789,"characterId":19}],"intervalSeconds":3600,"unitSeconds":3600}`))
		case strings.Contains(r.URL.Path, "/sk/trace"):
			_, _ = w.Write([]byte(`{"meta":{"server":"cn","eventId":170,"scope":"world-bloom/19","characterId":19,"fetchedAt":1},"subject":{"subjectType":"rank","subject":"1","resolvedUserId":"10001","resolvedRank":1},"rankData":[{"rank":1,"userId":"10001","name":"Tester","score":123,"timestamp":1704067200,"characterId":19}]}`))
		case strings.Contains(r.URL.Path, "/sk/status"):
			_, _ = w.Write([]byte(`{"timestamp":1704067200,"status":1,"statusDesc":"正常","timeAgo":0}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	characterID := 19
	userID := int64(10001)
	client := NewTrackerClient(&config.TrackerConfig{BaseURL: server.URL})
	if _, err := client.GetCloudSKQuery("cn", 170, &characterID, []int{1, 2}, &userID, true, true, 3600); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if _, err := client.GetCloudSKCheckRoom("cn", 170, &characterID, []int{1}, nil, false, 3600); err != nil {
		t.Fatalf("check-room failed: %v", err)
	}
	if _, err := client.GetCloudSKLine("cn", 170, &characterID, []int{100}, nil, false, 3600); err != nil {
		t.Fatalf("line failed: %v", err)
	}
	if _, err := client.GetCloudSKSpeed("cn", 170, &characterID, []int{100}, 3600, 3600, false); err != nil {
		t.Fatalf("speed failed: %v", err)
	}
	if _, err := client.GetCloudSKTrace("cn", 170, &characterID, "rank", "1", 5000); err != nil {
		t.Fatalf("trace failed: %v", err)
	}
	if _, err := client.GetEventStatus("cn", 170); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	wantPrefixes := []string{
		"/api/v2/cloud/events/cn/170/leaderboards/world-bloom/19/sk/query?",
		"/api/v2/cloud/events/cn/170/leaderboards/world-bloom/19/sk/check-room?",
		"/api/v2/cloud/events/cn/170/leaderboards/world-bloom/19/sk/line?",
		"/api/v2/cloud/events/cn/170/leaderboards/world-bloom/19/sk/speed?",
		"/api/v2/cloud/events/cn/170/leaderboards/world-bloom/19/sk/trace?",
		"/api/v2/cloud/events/cn/170/leaderboards/total/sk/status",
	}
	for i, want := range wantPrefixes {
		if i >= len(paths) || !strings.HasPrefix(paths[i], want) {
			t.Fatalf("path[%d] = %q, want prefix %q", i, paths[i], want)
		}
	}
}

func TestTrackerClientMaps404ToRankingNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewTrackerClient(&config.TrackerConfig{BaseURL: server.URL})
	_, err := client.GetCloudSKQuery("jp", 101, nil, []int{100}, nil, false, false, 3600)
	if !errors.Is(err, ErrRankingNotFound) {
		t.Fatalf("error = %v, want ErrRankingNotFound", err)
	}
}

func TestTrackerClientDoesNotCacheServerErrors(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit := hits.Add(1)
		if hit <= 5 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{"server":"jp","eventId":101,"scope":"total","fetchedAt":1},"ranks":[{"rank":100,"userId":"10001","name":"Tester","score":1234567,"timestamp":1704067200}]}`))
	}))
	defer server.Close()

	client := NewTrackerClient(&config.TrackerConfig{BaseURL: server.URL, Timeout: 2 * time.Second})
	client.http.SetRetryWaitTime(time.Millisecond)
	if _, err := client.GetCloudSKQuery("jp", 101, nil, []int{100}, nil, false, false, 3600); err == nil {
		t.Fatal("expected first request to fail")
	}
	if _, err := client.GetCloudSKQuery("jp", 101, nil, []int{100}, nil, false, false, 3600); err != nil {
		t.Fatalf("second request should refetch and succeed: %v", err)
	}
	if got := hits.Load(); got <= 5 {
		t.Fatalf("expected second request to refetch after 5xx, got %d hits", got)
	}
}
