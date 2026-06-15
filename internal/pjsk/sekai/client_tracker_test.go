package sekai

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/config"
)

func TestTrackerClientDeduplicatesConcurrentGETByPath(t *testing.T) {
	var hits atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/event/jp/101/latest-ranking/rank/100" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua != "tracker-test" {
			t.Fatalf("unexpected user agent: %q", ua)
		}
		hits.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rankData":{"userId":"10001","score":1234567,"rank":100,"timestamp":1704067200},"userData":{"userId":"10001","name":"Tester"}}`))
	}))
	defer server.Close()

	client := NewTrackerClient(&config.TrackerConfig{
		BaseURL:   server.URL,
		UserAgent: "tracker-test",
	})

	const callers = 16
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			resp, err := client.GetLatestRankingByRank("jp", 101, 100)
			if err != nil {
				errs <- err
				return
			}
			if resp.RankData.Rank != 100 || resp.UserData.Name != "Tester" {
				t.Errorf("unexpected response: %+v", resp)
			}
		}()
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
}
