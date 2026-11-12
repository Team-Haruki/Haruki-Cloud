package mysekai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newRegistryTestStore(t *testing.T, baseURL, region string, fallback masterdataSource) *registryMasterdataStore {
	t.Helper()
	source := newRegistryMasterdataStore(baseURL, region, fallback)
	store, ok := source.(*registryMasterdataStore)
	if !ok {
		t.Fatalf("registry store was not configured: %T", source)
	}
	t.Cleanup(store.Close)
	return store.WithContext(context.Background()).(*registryMasterdataStore)
}

func expireRegistryTestCache(store *registryMasterdataStore) {
	store.cache.mu.Lock()
	defer store.cache.mu.Unlock()
	for key, entry := range store.cache.entries {
		entry.expiresAt = time.Now().Add(-time.Second)
		store.cache.entries[key] = entry
	}
}

func TestRegistryMasterdataRevalidationAndFailedRefresh(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/registry/v1/master/jp/files/mysekaiConvertItems.json" {
			t.Errorf("unexpected registry path: %s", r.URL.Path)
		}
		switch calls.Add(1) {
		case 1:
			w.Header().Set("ETag", `"first"`)
			fmt.Fprint(w, `[{"id":7,"value":9007199254740993}]`)
		case 2:
			if got := r.Header.Get("If-None-Match"); got != `"first"` {
				t.Errorf("revalidation ETag = %q", got)
			}
			w.WriteHeader(http.StatusNotModified)
		case 3:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.Header().Set("ETag", `"second"`)
			fmt.Fprint(w, `[{"id":8}]`)
		}
	}))
	defer server.Close()
	store := newRegistryTestStore(t, server.URL+"/registry/", "JP", nil)
	got := store.loadMapByID("mysekaiConvertItems.json")
	if intNumber(got[7]["value"], 0) != 9007199254740993 {
		t.Fatalf("integer precision lost: %+v", got)
	}
	clone := store.WithContext(context.Background())
	if len(clone.loadList("mysekaiConvertItems.json")) != 1 || calls.Load() != 1 {
		t.Fatal("request clone did not reuse the fresh cache")
	}
	expireRegistryTestCache(store)
	if got := store.loadMapByID("mysekaiConvertItems.json"); got[7] == nil || calls.Load() != 2 {
		t.Fatalf("304 discarded cached data: %+v", got)
	}
	expireRegistryTestCache(store)
	if got := store.loadList("mysekaiConvertItems.json"); got != nil {
		t.Fatalf("failed refresh served stale data: %+v", got)
	}
	if got := store.loadMapByID("mysekaiConvertItems.json"); got[8] == nil || calls.Load() != 4 {
		t.Fatalf("failed refresh prevented retry: %+v", got)
	}
}

func TestRegistryMasterdataRejectsInvalidResponsesAndRetries(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "object", body: `{}`},
		{name: "null", body: `null`},
		{name: "trailing data", body: `[] {}`},
		{name: "oversized", body: strings.Repeat(" ", mysekaiRegistryMaxSize) + `[]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					fmt.Fprint(w, test.body)
					return
				}
				fmt.Fprint(w, `[]`)
			}))
			defer server.Close()
			store := newRegistryTestStore(t, server.URL, "cn", nil)
			if got := store.loadList("mysekaiConvertItems.json"); got != nil {
				t.Fatalf("invalid response accepted: %+v", got)
			}
			if got := store.loadList("mysekaiConvertItems.json"); got == nil || len(got) != 0 || calls.Load() != 2 {
				t.Fatalf("empty array retry failed: %+v (%d calls)", got, calls.Load())
			}
		})
	}
}

func TestRegistryMasterdataCanceledCallerDoesNotCancelSharedFetch(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		fmt.Fprint(w, `[{"id":1}]`)
	}))
	defer server.Close()
	defer unblock()
	store := newRegistryTestStore(t, server.URL, "jp", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	canceledResult := make(chan []map[string]any, 1)
	go func() { canceledResult <- store.WithContext(ctx).loadList("mysekaiConvertItems.json") }()
	<-started
	sharedResult := make(chan []map[string]any, 1)
	go func() { sharedResult <- store.loadList("mysekaiConvertItems.json") }()
	cancel()
	select {
	case result := <-canceledResult:
		if result != nil {
			t.Fatalf("canceled caller got data: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled caller did not return promptly")
	}
	unblock()
	select {
	case result := <-sharedResult:
		if len(result) != 1 || calls.Load() != 1 {
			t.Fatalf("shared request failed: %+v (%d calls)", result, calls.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("shared caller did not complete")
	}
}

func TestRegistryMasterdataResetDoesNotRetainInFlightResponse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			close(started)
			<-release
		}
		fmt.Fprintf(w, `[{"id":%d}]`, call)
	}))
	defer server.Close()
	defer unblock()
	store := newRegistryTestStore(t, server.URL, "jp", nil)
	first := make(chan []map[string]any, 1)
	go func() { first <- store.loadList("mysekaiConvertItems.json") }()
	<-started
	store.resetCache()
	unblock()
	<-first
	if got := store.loadMapByID("mysekaiConvertItems.json"); got[2] == nil || calls.Load() != 2 {
		t.Fatalf("in-flight response survived reset: %+v (%d calls)", got, calls.Load())
	}
}

func TestRegistryMasterdataAllowlistFallbackAndReset(t *testing.T) {
	root := t.TempDir()
	fixturePath := filepath.Join(root, "mysekaiFixtures.json")
	if err := os.WriteFile(fixturePath, []byte(`[{"id":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	fallback := newLocalMasterdataStore(root)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/v1/master/en/files/") {
			t.Errorf("wrong region path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `[{"id":9}]`)
	}))
	defer server.Close()
	store := newRegistryTestStore(t, server.URL, "en", fallback)
	if got := store.loadMapByID("mysekaiFixtures.json"); got[1] == nil || calls.Load() != 0 {
		t.Fatalf("DB/local table did not use fallback: %+v", got)
	}
	for _, filename := range []string{
		"mysekaiConvertItems.json", "mysekaiConvertItemTabs.json",
		"mysekaiConvertItemObtainMysekaiBlueprints.json", "mysekaiConvertItemObtainMysekaiBlueprintChoices.json",
		"mysekaiConvertItemObtainMysekaiMusicRecords.json", "materials.json", "practiceTickets.json", "skillPracticeTickets.json",
	} {
		if got := store.loadMapByID(filename); got[9] == nil {
			t.Fatalf("registry table %s missing: %+v", filename, got)
		}
	}
	if calls.Load() != 8 {
		t.Fatalf("unexpected registry calls: %d", calls.Load())
	}
	if err := os.WriteFile(fixturePath, []byte(`[{"id":2}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	store.resetCache()
	if got := store.loadMapByID("mysekaiFixtures.json"); got[2] == nil {
		t.Fatalf("fallback cache not reset: %+v", got)
	}
	if got := store.loadList("mysekaiConvertItems.json"); len(got) != 1 || calls.Load() != 9 {
		t.Fatal("registry cache not reset")
	}
	for _, test := range []struct{ baseURL, region string }{
		{"", "jp"}, {"file:///tmp/master", "jp"}, {"https://example.test?bad=1", "jp"}, {"https://example.test", "../jp"},
	} {
		if got := newRegistryMasterdataStore(test.baseURL, test.region, fallback); got != fallback {
			t.Fatalf("invalid config replaced fallback: %+v", test)
		}
	}
	var nilStore *registryMasterdataStore
	nilStore.Close()
	if nilStore.Configured() {
		t.Fatal("nil store reported configured")
	}
}

func TestRegistryMasterdataLayeredSourceResetsAfterIngest(t *testing.T) {
	root := t.TempDir()
	for filename, data := range map[string]string{
		"musics.json":          `[{"id":1,"title":"Local Song"}]`,
		"mysekaiFixtures.json": `[{"id":2,"name":"Local Fixture"}]`,
	} {
		if err := os.WriteFile(filepath.Join(root, filename), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var revision atomic.Int32
	revision.Store(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/master/jp/files/mysekaiConvertItems.json" {
			t.Errorf("DB/local table was requested from registry: %s", r.URL.Path)
		}
		fmt.Fprintf(w, `[{"id":%d}]`, revision.Load())
	}))
	defer server.Close()
	database := newTestDBMasterdataStore(t)
	layered := newLayeredMasterdataSource(database, newLocalMasterdataStore(root))
	store := newRegistryTestStore(t, server.URL, "jp", layered)
	controller := &Controller{resolver: &masterdataResolver{cache: map[string]masterdataSource{"jp": store}}}
	if got := stringValue(store.loadMapByID("musics.json")[1]["title"]); got != "Test Song" {
		t.Fatalf("database priority was lost: %q", got)
	}
	if got := store.loadMapByID("mysekaiFixtures.json"); got[2] == nil {
		t.Fatalf("local fallback was lost: %+v", got)
	}
	if got := store.loadMapByID("mysekaiConvertItems.json"); got[1] == nil {
		t.Fatalf("registry table was not served: %+v", got)
	}
	if _, err := database.db.Exec(`UPDATE musics SET title = 'Updated Song' WHERE game_id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "mysekaiFixtures.json"), []byte(`[{"id":3,"name":"New Fixture"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	revision.Store(2)
	controller.ResetMasterdataCache()
	if got := stringValue(store.loadMapByID("musics.json")[1]["title"]); got != "Updated Song" {
		t.Fatalf("ingest reset did not reach the database: %q", got)
	}
	if got := store.loadMapByID("mysekaiFixtures.json"); got[3] == nil || got[2] != nil {
		t.Fatalf("ingest reset did not reach local fallback: %+v", got)
	}
	if got := store.loadMapByID("mysekaiConvertItems.json"); got[2] == nil || got[1] != nil {
		t.Fatalf("ingest reset did not reach registry: %+v", got)
	}
}
