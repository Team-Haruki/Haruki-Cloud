package meta

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"expvar"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/logger"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func testLogger() (*logger.Logger, *lockedBuffer) {
	buffer := &lockedBuffer{}
	return logger.NewLogger("MusicMetaTest", "DEBUG", buffer), buffer
}

func metaRows(tapCount string) string {
	return `[{"music_id":1,"difficulty":"master","tap_count":` + tapCount + `}]`
}

func tapCount(t *testing.T, loader *Loader, region string) int {
	t.Helper()
	view := loader.View(region)
	if view == nil {
		return -1
	}
	entry, ok := view.Find(1, "master")
	if !ok {
		t.Fatalf("%s view has no music 1 master row", region)
	}
	return entry.Int("tap_count")
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// registryRegion extracts the region of a /v1/metas/{region}/music_metas.json path.
func registryRegion(path string) string {
	return strings.TrimSuffix(strings.TrimPrefix(path, "/v1/metas/"), "/music_metas.json")
}

func seedPersisted(memory *storagetest.Memory, tap string) {
	seeded := make(map[string][]byte, len(regionFilenames))
	for _, filename := range regionFilenames {
		seeded[filename] = []byte(metaRows(tap))
	}
	memory.Seed(seeded)
}

func TestLoaderRequestsGzipAndDecodesIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			t.Errorf("Accept-Encoding = %q, want gzip", r.Header.Get("Accept-Encoding"))
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("ETag", `"digest-gzip"`)
		writer := gzip.NewWriter(w)
		_, _ = writer.Write([]byte(metaRows("42")))
		_ = writer.Close()
	}))
	defer server.Close()

	loader := NewLoader(nil, WithSource(SourceRegistry), WithBaseURL(server.URL))
	if err := loader.load(context.Background(), "jp"); err != nil {
		t.Fatalf("load gzip response: %v", err)
	}
	if got := tapCount(t, loader, "jp"); got != 42 {
		t.Fatalf("tap_count = %d, want 42", got)
	}
	if status := loader.Status()["jp"]; status.ETag != `"digest-gzip"` || status.Source != SourceRemote {
		t.Fatalf("status = %+v", status)
	}
}

func TestLoaderStartServesPersistedThenSwapsInRemote(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("ETag", `"`+registryRegion(r.URL.Path)+`-v2"`)
		_, _ = w.Write([]byte(metaRows("2")))
	}))
	defer server.Close()

	memory := storagetest.NewMemory()
	seedPersisted(memory, "1")
	log, output := testLogger()
	loader := NewLoader(log, WithStore(memory), WithSource(SourceRegistry), WithBaseURL(server.URL))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := time.Now()
	loader.Start(ctx, time.Hour)
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("Start blocked for %s with persisted copies present", elapsed)
	}
	for _, region := range Regions() {
		if got := tapCount(t, loader, region); got != 1 {
			t.Fatalf("%s tap_count before the remote answered = %d, want the persisted 1", region, got)
		}
		if status := loader.Status()[region]; status.Source != SourcePersisted {
			t.Fatalf("%s source = %q, want persisted", region, status.Source)
		}
	}

	close(release)
	waitFor(t, "remote swap", func() bool {
		for _, region := range Regions() {
			if loader.Status()[region].Source != SourceRemote || tapCount(t, loader, region) != 2 {
				return false
			}
		}
		return true
	})
	stored, err := memory.Get(context.Background(), "music_metas-cn.json")
	if err != nil || !bytes.Contains(stored, []byte(`"tap_count":2`)) {
		t.Fatalf("remote payload not persisted: %q, %v", stored, err)
	}
	if !strings.Contains(output.String(), "music metadata loaded from persisted copy") ||
		!strings.Contains(output.String(), "music metadata updated") {
		t.Fatalf("source changes were not logged:\n%s", output.String())
	}
}

func TestLoaderStartWaitsBoundedWithoutPersistedCopy(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(metaRows("3")))
	}))
	defer server.Close()
	defer close(release)

	log, output := testLogger()
	loader := NewLoader(log, WithSource(SourceRegistry), WithBaseURL(server.URL))
	loader.initialWait = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := time.Now()
	loader.Start(ctx, time.Hour)
	if elapsed := time.Since(started); elapsed < loader.initialWait || elapsed > 2*time.Second {
		t.Fatalf("Start returned after %s, want about %s", elapsed, loader.initialWait)
	}
	if !strings.Contains(output.String(), "still loading in the background") {
		t.Fatalf("bounded wait was not logged:\n%s", output.String())
	}
}

func TestLoaderStartWaitsForFirstPassWithoutPersistedCopy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(metaRows("4")))
	}))
	defer server.Close()

	loader := NewLoader(nil, WithSource(SourceRegistry), WithBaseURL(server.URL))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loader.Start(ctx, time.Hour)
	for _, region := range Regions() {
		if got := tapCount(t, loader, region); got != 4 {
			t.Fatalf("%s tap_count after Start = %d, want 4", region, got)
		}
	}

	cancelled, stop := context.WithCancel(context.Background())
	stop()
	waiting := NewLoader(nil, WithSource(SourceRegistry), WithBaseURL(server.URL))
	waiting.Start(cancelled, time.Hour)
	(*Loader)(nil).Start(ctx, time.Hour)
}

func TestLoaderPersistedValidatorsRevalidateAfterRestart(t *testing.T) {
	var conditional atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := `"` + registryRegion(r.URL.Path) + `-gzip"`
		if r.Header.Get("If-None-Match") == tag {
			conditional.Add(1)
			w.Header().Set("ETag", tag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", tag)
		_, _ = w.Write([]byte(metaRows("5")))
	}))
	defer server.Close()

	memory := storagetest.NewMemory()
	first := NewLoader(nil, WithStore(memory), WithSource(SourceRegistry), WithBaseURL(server.URL))
	if err := first.LoadAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.Get(context.Background(), "music_metas.json"+validatorSuffix); err != nil {
		t.Fatalf("validators not persisted: %v", err)
	}

	log, output := testLogger()
	restarted := NewLoader(log, WithStore(memory), WithSource(SourceRegistry), WithBaseURL(server.URL))
	if missing := restarted.LoadPersistedAll(context.Background()); missing != 0 {
		t.Fatalf("missing persisted regions = %d", missing)
	}
	if status := restarted.Status()["jp"]; status.Source != SourcePersisted || !status.Revalidatable || status.CheckedAt != "" {
		t.Fatalf("persisted status = %+v", status)
	}
	if err := restarted.LoadAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := conditional.Load(); got != int64(len(Regions())) {
		t.Fatalf("conditional 304s = %d, want %d", got, len(Regions()))
	}
	status := restarted.Status()["jp"]
	if status.Source != SourceRemote || status.CheckedAt == "" || status.LoadedAt == "" || tapCount(t, restarted, "jp") != 5 {
		t.Fatalf("revalidated status = %+v", status)
	}
	if !strings.Contains(output.String(), "persisted copy confirmed by remote") {
		t.Fatalf("revalidation was not logged:\n%s", output.String())
	}
}

func TestLoaderIgnoresValidatorsOfOtherBytes(t *testing.T) {
	var conditional atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			conditional.Add(1)
		}
		_, _ = w.Write([]byte(metaRows("6")))
	}))
	defer server.Close()

	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{
		"music_metas.json":                      []byte(metaRows("1")),
		"music_metas.json" + validatorSuffix:    []byte(`{"sha256":"0000","etag":"\"stale\""}`),
		"music_metas-en.json":                   []byte(metaRows("1")),
		"music_metas-en.json" + validatorSuffix: []byte(`{`),
	})
	loader := NewLoader(nil, WithStore(memory), WithSource(SourceRegistry), WithBaseURL(server.URL))
	loader.LoadPersistedAll(context.Background())
	for _, region := range []string{"jp", "en"} {
		if status := loader.Status()[region]; status.Revalidatable || status.ETag != "" {
			t.Fatalf("%s kept validators of other bytes: %+v", region, status)
		}
	}
	if err := loader.load(context.Background(), "jp"); err != nil {
		t.Fatal(err)
	}
	if conditional.Load() != 0 || tapCount(t, loader, "jp") != 6 {
		t.Fatal("a mismatched validator was sent or the full fetch was not installed")
	}
}

func TestLoaderRefreshRetriesFailedRegions(t *testing.T) {
	var jpRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if registryRegion(r.URL.Path) == "jp" && jpRequests.Add(1) <= 2 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(metaRows("7")))
	}))
	defer server.Close()

	loader := NewLoader(nil, WithSource(SourceRegistry), WithBaseURL(server.URL))
	loader.retryBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	firstPass := false
	loader.refresh(context.Background(), func() { firstPass = true })
	if !firstPass {
		t.Fatal("first pass callback did not run")
	}
	if got := jpRequests.Load(); got != 3 {
		t.Fatalf("jp requests = %d, want 3 (two failures, one success)", got)
	}
	status := loader.Status()["jp"]
	if status.Source != SourceRemote || status.LastErrorType != "" || tapCount(t, loader, "jp") != 7 {
		t.Fatalf("jp status after retries = %+v", status)
	}
}

func TestLoaderRefreshGivesUpAfterRetries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	log, output := testLogger()
	loader := NewLoader(log, WithSource(SourceRegistry), WithBaseURL(server.URL))
	loader.retryBackoff = []time.Duration{time.Millisecond}
	loader.refresh(context.Background(), nil)
	if got := requests.Load(); got != int64(2*len(Regions())) {
		t.Fatalf("requests = %d, want %d", got, 2*len(Regions()))
	}
	status := loader.Status()["kr"]
	if status.Source != "none" || status.LastErrorType == "" || status.LastErrorAt == "" || status.RemoteSource != SourceRegistry {
		t.Fatalf("status after exhausted retries = %+v", status)
	}
	if !strings.Contains(output.String(), "retries exhausted") {
		t.Fatalf("exhausted retries were not logged:\n%s", output.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	slow := NewLoader(nil, WithSource(SourceRegistry), WithBaseURL(server.URL))
	slow.retryBackoff = []time.Duration{time.Hour}
	slow.refresh(ctx, nil)
}

func TestLoaderRefreshCancelledDuringBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	loader := NewLoader(nil, WithSource(SourceRegistry), WithBaseURL(server.URL))
	loader.retryBackoff = []time.Duration{time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loader.refresh(ctx, func() {
			go func() {
				time.Sleep(20 * time.Millisecond)
				cancel()
			}()
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not stop when cancelled during backoff")
	}
}

func TestLoaderDoesNotRetryPersistFailures(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("ETag", `"v"`)
		_, _ = w.Write([]byte(metaRows("8")))
	}))
	defer server.Close()

	memory := storagetest.NewMemory()
	memory.FailPut = func(storage.Key) error { return errors.New("put down") }
	loader := NewLoader(nil, WithStore(memory), WithSource(SourceRegistry), WithBaseURL(server.URL))
	round := loader.loadRegions(context.Background(), []string{"tw"})
	if round.firstErr == nil || len(round.failed) != 0 {
		t.Fatalf("round = %+v, want a persist error that is not retried", round)
	}
	var persistErr *persistError
	if !errors.As(round.firstErr, &persistErr) || !strings.Contains(round.firstErr.Error(), "put down") || errors.Unwrap(persistErr) == nil {
		t.Fatalf("persist error = %v", round.firstErr)
	}
	if requests.Load() != 1 || tapCount(t, loader, "tw") != 8 {
		t.Fatal("persist failure was retried or dropped the fetched payload")
	}

	validatorFails := storagetest.NewMemory()
	validatorFails.FailPut = func(key storage.Key) error {
		if strings.HasSuffix(string(key), validatorSuffix) {
			return errors.New("validator put down")
		}
		return nil
	}
	partial := NewLoader(nil, WithStore(validatorFails), WithSource(SourceRegistry), WithBaseURL(server.URL))
	if err := partial.load(context.Background(), "tw"); err == nil {
		t.Fatal("validator write failure was not reported")
	}
	if _, err := validatorFails.Get(context.Background(), "music_metas-tc.json"); err != nil {
		t.Fatalf("payload was not persisted before the validators: %v", err)
	}
}

func TestLoaderFallbackRecordsRemoteFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	memory := storagetest.NewMemory()
	seedPersisted(memory, "9")
	loader := NewLoader(nil, WithStore(memory), WithSource(SourceRegistry), WithBaseURL(server.URL))
	if err := loader.load(context.Background(), "kr"); err != nil {
		t.Fatalf("fallback load = %v", err)
	}
	status := loader.Status()["kr"]
	if status.Source != SourcePersisted || status.LastErrorType == "" || tapCount(t, loader, "kr") != 9 {
		t.Fatalf("fallback status = %+v", status)
	}
}

func TestLoaderNotModifiedWithoutCachedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()
	if err := NewLoader(nil).loadRemote(context.Background(), "jp", server.URL, nil); err == nil {
		t.Fatal("304 without a cached payload was accepted")
	}
}

func TestLoaderRefreshEveryTicksUntilCancelled(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("ETag", `"v"`)
		_, _ = w.Write([]byte(metaRows("10")))
	}))
	defer server.Close()

	loader := NewLoader(nil, WithSource(SourceRegistry), WithBaseURL(server.URL))
	loader.refreshEvery(context.Background(), 0)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loader.refreshEvery(ctx, 10*time.Millisecond)
		close(done)
	}()
	waitFor(t, "two refresh rounds", func() bool { return requests.Load() >= int64(2*len(Regions())) })
	cancel()
	<-done
}

func TestLoaderPersistedAllReportsUnusableCopies(t *testing.T) {
	memory := storagetest.NewMemory()
	memory.Seed(map[string][]byte{"music_metas.json": []byte("{")})
	log, output := testLogger()
	loader := NewLoader(log, WithStore(memory))
	if missing := loader.LoadPersistedAll(context.Background()); missing != len(Regions()) {
		t.Fatalf("missing = %d, want %d", missing, len(Regions()))
	}
	if !strings.Contains(output.String(), "persisted copy unusable") {
		t.Fatalf("malformed persisted copy was not logged:\n%s", output.String())
	}
	if NewLoader(nil).LoadPersistedAll(context.Background()) != len(Regions()) {
		t.Fatal("loader without a store reported persisted regions")
	}
}

func TestLoaderPublishesStatus(t *testing.T) {
	loader := NewLoader(nil, WithSource(SourceRegistry))
	loader.install("jp", &regionEntry{data: []byte("[]"), digest: digestOf([]byte("[]")), source: SourcePersisted, loadedAt: time.Now()})
	loader.PublishStatus()
	loader.PublishStatus()
	published := expvar.Get(musicMetasExpvarKey)
	if published == nil {
		t.Fatal("music_metas expvar not published")
	}
	if text := published.String(); !strings.Contains(text, `"source":"persisted"`) || !strings.Contains(text, `"remote_source":"registry"`) {
		t.Fatalf("published status = %s", text)
	}
	if (*Loader)(nil).Status() != nil {
		t.Fatal("nil loader reported a status")
	}
	if got := NewLoader(nil).Status()["en"].RemoteSource; got != SourceLegacy {
		t.Fatalf("default remote source = %q", got)
	}
	if shortDigest("abc") != "abc" || formatTime(time.Time{}) != "" {
		t.Fatal("digest/time helpers")
	}
}

func TestLoaderRetriesRegionsServedFromFallback(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(metaRows("11")))
	}))
	defer server.Close()

	// The persisted copy appears only after startup tried to read it, so the
	// first remote failure is answered from the fallback inside load.
	memory := storagetest.NewMemory()
	loader := NewLoader(nil, WithStore(memory), WithSource(SourceRegistry), WithBaseURL(server.URL))
	loader.retryBackoff = []time.Duration{time.Millisecond}
	memory.Seed(map[string][]byte{"music_metas-kr.json": []byte(metaRows("1"))})
	round := loader.loadRegions(context.Background(), []string{"kr"})
	if round.firstErr != nil || len(round.failed) != 1 || loader.Status()["kr"].Source != SourcePersisted {
		t.Fatalf("fallback round = %+v, status = %+v", round, loader.Status()["kr"])
	}
	loader.refresh(context.Background(), nil)
	if status := loader.Status()["kr"]; status.Source != SourceRemote || tapCount(t, loader, "kr") != 11 {
		t.Fatalf("fallback region was not retried: %+v", status)
	}
}
