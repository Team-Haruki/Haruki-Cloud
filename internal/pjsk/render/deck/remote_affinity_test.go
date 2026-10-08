package deck

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

type affinityDeckTarget struct {
	server  *httptest.Server
	uploads atomic.Int32
}

func newAffinityDeckTargets(t *testing.T, n int) ([]*affinityDeckTarget, []upstream.TargetConfig) {
	t.Helper()
	targets := make([]*affinityDeckTarget, n)
	configs := make([]upstream.TargetConfig, n)
	for i := range targets {
		target := &affinityDeckTarget{}
		target.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, _ = io.Copy(io.Discard, req.Body)
			_ = req.Body.Close()
			switch req.URL.Path {
			case "/cache_userdata":
				target.uploads.Add(1)
				_, _ = w.Write([]byte(`{"userdata_hash":"hash"}`))
			case "/recommend":
				_, _ = w.Write([]byte(`[]`))
			default:
				http.NotFound(w, req)
			}
		}))
		t.Cleanup(target.server.Close)
		targets[i] = target
		configs[i] = upstream.TargetConfig{Name: fmt.Sprintf("deck-%d", i), BaseURL: target.server.URL, Concurrency: 4}
	}
	return targets, configs
}

func totalUploads(targets []*affinityDeckTarget) int32 {
	var total int32
	for _, target := range targets {
		total += target.uploads.Load()
	}
	return total
}

func TestRemoteRecommendPrefersTargetHoldingUserdata(t *testing.T) {
	targets, configs := newAffinityDeckTargets(t, 3)
	remote := newTestRemoteDeckRecommenderWithTargets(configs, http.DefaultClient)
	remote.logger = logger.NewLogger("DeckAffinityTest", "ERROR", nil)
	request := testRemoteRecommendRequest()
	for _, state := range remote.targetStates {
		state.musicMetaHash = hashPayload(request.MusicMeta)
	}

	ctx, trace := commandtrace.WithTrace(context.Background())
	for range 9 {
		if _, err := remote.RecommendBatchContext(ctx, request); err != nil {
			t.Fatalf("RecommendBatchContext: %v", err)
		}
	}
	if got := totalUploads(targets); got != 1 {
		t.Fatalf("one digest should be uploaded once with affinity, got %d uploads", got)
	}
	if hits, ok := traceOperation(trace.Snapshot(), "deck.userdata_cache_hit"); !ok || hits.Count != 8 {
		t.Fatalf("cache hits = %+v", hits)
	}
	if affinity, ok := traceOperation(trace.Snapshot(), "deck.target_affinity"); !ok || affinity.Count != 9 {
		t.Fatalf("affinity = %+v", affinity)
	}
	if hashes, ok := traceOperation(trace.Snapshot(), "deck.userdata_hash"); !ok || hashes.Count != 9 {
		t.Fatalf("the digest should be computed once per batch, before the lease: %+v", hashes)
	}

	// Distinct digests still spread over the targets.
	for i := range 30 {
		request.UserData = []byte(fmt.Sprintf(`{"user":%d}`, i))
		if _, err := remote.RecommendBatchContext(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	for i, target := range targets {
		if target.uploads.Load() == 0 {
			t.Fatalf("target %d never received a digest", i)
		}
	}
}

func TestRemoteRecommendAffinityFallsBackFromOpenCircuit(t *testing.T) {
	targets, configs := newAffinityDeckTargets(t, 3)
	remote := newTestRemoteDeckRecommenderWithTargets(configs, http.DefaultClient)
	remote.logger = logger.NewLogger("DeckAffinityTest", "ERROR", nil)
	request := testRemoteRecommendRequest()
	for _, state := range remote.targetStates {
		state.musicMetaHash = hashPayload(request.MusicMeta)
	}
	if _, err := remote.RecommendBatchContext(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var preferred *remoteTargetState
	for i, target := range targets {
		if target.uploads.Load() == 1 {
			preferred = remote.targetStates[remoteTargetKey(configs[i])]
		}
	}
	if preferred == nil {
		t.Fatal("no target received the upload")
	}
	// Open the preferred target's circuit: the work moves elsewhere.
	preferred.consecutiveFailures.Store(targetAssignmentSkipFailures)
	preferred.lastFailureAtNanos.Store(remote.timeNow().UnixNano())
	preferred.lastHealthProbeAtNanos.Store(remote.timeNow().UnixNano())

	ctx, trace := commandtrace.WithTrace(context.Background())
	if _, err := remote.RecommendBatchContext(ctx, request); err != nil {
		t.Fatal(err)
	}
	if got := totalUploads(targets); got != 2 {
		t.Fatalf("the fallback target needs its own upload, uploads = %d", got)
	}
	if _, ok := traceOperation(trace.Snapshot(), "deck.target_affinity"); !ok {
		t.Fatal("the next rendezvous target among accepted ones is still an affinity choice")
	}
}

type deckLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *deckLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *deckLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestDeckHTTPRecordsTargetAndSplitTiming(t *testing.T) {
	_, configs := newAffinityDeckTargets(t, 1)
	remote := newTestRemoteDeckRecommenderWithTargets(configs, http.DefaultClient)
	remote.logger = logger.NewLogger("DeckAttributionTest", "ERROR", nil)
	request := testRemoteRecommendRequest()
	for _, state := range remote.targetStates {
		state.musicMetaHash = hashPayload(request.MusicMeta)
	}
	logs := &deckLogBuffer{}
	logger.SetCommandWriter(logs)
	t.Cleanup(func() { logger.SetCommandWriter(nil) })

	ctx, trace := commandtrace.WithTrace(context.Background())
	if _, err := remote.RecommendBatchContext(ctx, request); err != nil {
		t.Fatal(err)
	}
	ops := make(map[string]int)
	for _, op := range trace.Snapshot().Operations {
		ops[op.Name] = op.Count
	}
	// The upload's operations merge into the command trace too.
	if ops["deck.http"] != 2 || ops["deck.http.ttfb"] != 2 || ops["deck.http.body"] != 2 {
		t.Fatalf("operations = %v", ops)
	}
	line := logs.String()
	for _, want := range []string{"op=deck.http", "target=deck-0", "upstream_path=/cache_userdata", "upstream_path=/recommend", "status_code=200"} {
		if !strings.Contains(line, want) {
			t.Fatalf("call records %q lack %q", line, want)
		}
	}
	if remote.targetNameFor("http://unknown") != "" {
		t.Fatal("unknown base URL has no target name")
	}
}
