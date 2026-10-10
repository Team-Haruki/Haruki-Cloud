package drawing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/imagecache"
)

var (
	storeRefHash = strings.Repeat("c", 64)
	storeRefPath = "pjsk/" + storeRefHash + "-ABCDEFGHIJKLMNOPQRSTUVWXYZ.jpg"
)

type storeRefMode int

const (
	storeRefServe        storeRefMode = iota // a store-ref Drawing
	storeRefIgnore                           // a Drawing that predates the mode
	storeRefDegrade                          // upload failed: degraded bytes
	storeRefBadPath                          // a ref Cloud could not have written
	storeRefIndexedPath                      // a ref claiming an index row
	storeRefOtherBucket                      // a ref in a bucket Cloud does not serve
	storeRefOtherBackend                     // a ref on a backend Cloud does not collect
)

// storeRefDrawingServer answers X-Haruki-Artifact-Mode: store-ref like a
// Drawing in the given mode, and Cache-Store: 0 without it with bytes.
type storeRefDrawingServer struct {
	*httptest.Server
	mode    storeRefMode
	mu      sync.Mutex
	headers map[string]http.Header
	hits    map[string]int
}

func newStoreRefDrawingServer(t *testing.T, mode storeRefMode) *storeRefDrawingServer {
	t.Helper()
	s := &storeRefDrawingServer{mode: mode, headers: map[string]http.Header{}, hits: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.headers[r.URL.Path] = r.Header.Clone()
		s.hits[r.URL.Path]++
		s.mu.Unlock()
		w.Header().Set(headerNode, "render-1")
		storeRef := r.Header.Get(headerArtifactMode) == artifactModeStoreRef && r.Header.Get(headerCacheStore) == "0"
		if r.Header.Get(headerArtifact) == "1" && r.Header.Get(headerCacheStore) == "1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(testArtifactRefJSON(nil)))
			return
		}
		if r.Header.Get(headerCacheStore) == "0" {
			w.Header().Set(headerCacheStore, "0")
		}
		if storeRef && s.mode != storeRefIgnore {
			if s.mode == storeRefDegrade {
				w.Header().Set(headerArtifactDegraded, "1")
			} else {
				path := storeRefPath
				indexed := "false"
				bucket, backend := `"image-cache"`, `"garage"`
				if s.mode == storeRefOtherBucket {
					bucket = `"other-bucket"`
				}
				if s.mode == storeRefOtherBackend {
					backend = `"legacy_disk"`
				}
				if s.mode == storeRefBadPath {
					path = "pjsk/" + strings.Repeat("d", 64) + "-ABCDEFGHIJKLMNOPQRSTUVWXYZ.jpg"
				}
				if s.mode == storeRefIndexedPath {
					indexed = "true"
				}
				w.Header().Set(headerArtifact, "1")
				w.Header().Set(headerArtifactMode, artifactModeStoreRef)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(testArtifactRefJSON(map[string]string{
					"hash": `"` + storeRefHash + `"`, "cdn_path": `"` + path + `"`, "object_key": `"` + path + `"`,
					"media_type": `"image/jpeg"`, "size_bytes": `270000`, "index_written": indexed, "node_name": `"gw-1"`,
					"expires_at": `null`, "bucket": bucket, "storage_backend": backend,
				})))
				return
			}
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("bytes-jpeg"))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *storeRefDrawingServer) seen(path string) (http.Header, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.headers[path], s.hits[path]
}

type fakeStoreRefIndexer struct {
	mu       sync.Mutex
	objects  []imagecache.AdoptedObject
	location *imagecache.AdoptedLocation
	err      error
	ctxErr   error
}

func (f *fakeStoreRefIndexer) AdoptObject(ctx context.Context, obj imagecache.AdoptedObject, _ time.Time) (imagecache.AdoptedLocation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects = append(f.objects, obj)
	f.ctxErr = ctx.Err()
	if f.err != nil {
		return imagecache.AdoptedLocation{}, f.err
	}
	if f.location != nil {
		return *f.location, nil
	}
	return imagecache.AdoptedLocation{CDNPath: obj.CDNPath, WriterNode: obj.WriterNode}, nil
}

func (f *fakeStoreRefIndexer) adopted() []imagecache.AdoptedObject {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]imagecache.AdoptedObject(nil), f.objects...)
}

func storeRefTestClient(t *testing.T, server *storeRefDrawingServer, index *fakeRenderIndex, cfg ArtifactConfig) *HarukiDrawingClient {
	t.Helper()
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(cfg))
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: index})
	client.SetRenderCache(cache)
	t.Cleanup(func() { _ = cache.Close() })
	return client
}

func traceOps(trace *commandtrace.Trace) map[string]int {
	ops := map[string]int{}
	for _, op := range trace.Snapshot().Operations {
		ops[op.Name] += op.Count
	}
	return ops
}

func storeRefConfig(indexer StoreRefIndexer, paths ...string) ArtifactConfig {
	return ArtifactConfig{
		Endpoints:       []string{"*"},
		NoStorePaths:    []string{"api/pjsk/sk", "api/pjsk/deck"},
		StoreRefPaths:   paths,
		StoreRefIndexer: indexer,
		StoreRefBucket:  "image-cache",
	}
}

func TestStoreRefPathRecordsTheRowAndReturnsTheRef(t *testing.T) {
	server := newStoreRefDrawingServer(t, storeRefServe)
	index := &fakeRenderIndex{}
	indexer := &fakeStoreRefIndexer{}
	client := storeRefTestClient(t, server, index, storeRefConfig(indexer, "api/pjsk/sk"))

	for round := 1; round <= 2; round++ {
		ctx, trace := commandtrace.WithNewTrace(t.Context())
		image, err := client.WithContext(ctx).GenerateSKSpeedImage(&SpeedRequest{})
		if err != nil {
			t.Fatal(err)
		}
		ref := image.Ref()
		if ref == nil || ref.CDNPath != storeRefPath || ref.NodeName != "gw-1" || ref.IndexWritten {
			t.Fatalf("round %d: ref = %+v", round, ref)
		}
		headers, hits := server.seen("/api/pjsk/sk/speed")
		got := harukiHeaders(headers)
		if got[http.CanonicalHeaderKey(headerArtifactMode)] != artifactModeStoreRef || got[headerCacheStore] != "0" {
			t.Fatalf("round %d: headers = %v", round, got)
		}
		// A store-ref is never a render cache entry: every round renders.
		if hits != round {
			t.Fatalf("round %d: drawing hits = %d", round, hits)
		}
		ops := traceOps(trace)
		if ops["drawing.store_ref"] != 1 || ops["drawing.http"] != 1 {
			t.Fatalf("round %d: trace ops = %v", round, ops)
		}
	}
	adopted := indexer.adopted()
	want := imagecache.AdoptedObject{Hash: storeRefHash, Group: "pjsk", CDNPath: storeRefPath, MediaType: "image/jpeg", SizeBytes: 270000, WriterNode: "gw-1"}
	if len(adopted) != 2 || adopted[0] != want {
		t.Fatalf("adopted = %+v", adopted)
	}
	if lookups, _, _ := index.calls(); lookups != 0 {
		t.Fatalf("store-ref path consulted the render index %d times", lookups)
	}
}

func TestStoreRefDuplicateServesTheRecordedRow(t *testing.T) {
	server := newStoreRefDrawingServer(t, storeRefServe)
	existing := "pjsk/" + storeRefHash + "-ZYXWVUTSRQPONMLKJIHGFEDCBA.jpg"
	indexer := &fakeStoreRefIndexer{location: &imagecache.AdoptedLocation{CDNPath: existing, Duplicate: true}}
	client := storeRefTestClient(t, server, &fakeRenderIndex{}, storeRefConfig(indexer, "api/pjsk/sk"))

	image, err := client.WithContext(t.Context()).GenerateSKSpeedImage(&SpeedRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if ref := image.Ref(); ref == nil || ref.CDNPath != existing || ref.ObjectKey != existing || ref.NodeName != "" {
		t.Fatalf("ref = %+v, want the recorded row's location", ref)
	}
}

func TestStoreRefIndexFailureStillDeliversTheRef(t *testing.T) {
	server := newStoreRefDrawingServer(t, storeRefServe)
	indexer := &fakeStoreRefIndexer{err: errors.New("pg down")}
	client := storeRefTestClient(t, server, &fakeRenderIndex{}, storeRefConfig(indexer, "api/pjsk/sk"))

	ctx, trace := commandtrace.WithNewTrace(t.Context())
	image, err := client.WithContext(ctx).GenerateSKSpeedImage(&SpeedRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if ref := image.Ref(); ref == nil || ref.CDNPath != storeRefPath || ref.NodeName != "gw-1" {
		t.Fatalf("ref = %+v", ref)
	}
	if ops := traceOps(trace); ops["image.ref_index_error"] != 1 {
		t.Fatalf("trace ops = %v", ops)
	}
}

func TestStoreRefIndexOutlivesACancelledCaller(t *testing.T) {
	server := newStoreRefDrawingServer(t, storeRefServe)
	indexer := &fakeStoreRefIndexer{}
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(storeRefConfig(indexer, "api/pjsk/sk")))
	ctx, cancel := context.WithCancel(t.Context())
	d := newRenderDirective(strings.Repeat("e", 64), renderCachePolicy{APIPath: "api/pjsk/sk/speed", UserID: "public"}, time.Minute, false)
	d.StoreRef = true
	resp, err := client.client.R().SetHeader(headerArtifactMode, artifactModeStoreRef).SetHeader(headerCacheStore, "0").
		SetHeader(headerArtifact, "1").Post(server.URL + "/api/pjsk/sk/speed")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	active := client.WithContext(ctx)
	if _, err := active.successBody("/api/pjsk/sk/speed", d, resp); err != nil {
		t.Fatal(err)
	}
	if indexer.ctxErr != nil {
		t.Fatalf("index write saw the caller's cancellation: %v", indexer.ctxErr)
	}
	if d.outcome.Ref == nil || !d.outcome.StoreRef {
		t.Fatalf("outcome = %+v", d.outcome)
	}
}

func TestStoreRefWithoutIndexerUsesTheRefAsIs(t *testing.T) {
	server := newStoreRefDrawingServer(t, storeRefServe)
	client := storeRefTestClient(t, server, &fakeRenderIndex{}, storeRefConfig(nil, "api/pjsk/sk"))
	image, err := client.WithContext(t.Context()).GenerateSKSpeedImage(&SpeedRequest{})
	if err != nil || image.Ref() == nil || image.Ref().CDNPath != storeRefPath {
		t.Fatalf("ref = %+v, err = %v", image.Ref(), err)
	}
}

func TestStoreRefFallsBackToBytes(t *testing.T) {
	for name, mode := range map[string]storeRefMode{"old drawing": storeRefIgnore, "degraded": storeRefDegrade} {
		t.Run(name, func(t *testing.T) {
			server := newStoreRefDrawingServer(t, mode)
			indexer := &fakeStoreRefIndexer{}
			client := storeRefTestClient(t, server, &fakeRenderIndex{}, storeRefConfig(indexer, "api/pjsk/sk"))
			ctx, trace := commandtrace.WithNewTrace(t.Context())
			image, err := client.WithContext(ctx).GenerateSKSpeedImage(&SpeedRequest{})
			if err != nil || image.Ref() != nil {
				t.Fatalf("ref = %+v, err = %v", image.Ref(), err)
			}
			if data, err := image.Bytes(t.Context()); err != nil || string(data) != "bytes-jpeg" {
				t.Fatalf("bytes = %q, %v", data, err)
			}
			if len(indexer.adopted()) != 0 {
				t.Fatal("fallback bytes reached the indexer")
			}
			want := map[storeRefMode]string{storeRefIgnore: "drawing.store_ref_unsupported", storeRefDegrade: "drawing.store_ref_degraded"}[mode]
			if ops := traceOps(trace); ops[want] != 1 {
				t.Fatalf("trace ops = %v, want %s", ops, want)
			}
		})
	}
}

func TestStoreRefRejectedRefIsRenderedAgainWithoutTheMode(t *testing.T) {
	for name, mode := range map[string]storeRefMode{
		"other bucket":  storeRefOtherBucket,
		"other backend": storeRefOtherBackend,
		"foreign hash":  storeRefBadPath,
		"index row":     storeRefIndexedPath,
	} {
		t.Run(name, func(t *testing.T) {
			server := newStoreRefDrawingServer(t, mode)
			indexer := &fakeStoreRefIndexer{}
			client := storeRefTestClient(t, server, &fakeRenderIndex{}, storeRefConfig(indexer, "api/pjsk/sk"))
			ctx, trace := commandtrace.WithNewTrace(t.Context())
			image, err := client.WithContext(ctx).GenerateSKSpeedImage(&SpeedRequest{})
			if err != nil || image.Ref() != nil {
				t.Fatalf("ref = %+v, err = %v, want the bytes of a second render", image.Ref(), err)
			}
			if data, err := image.Bytes(t.Context()); err != nil || string(data) != "bytes-jpeg" {
				t.Fatalf("bytes = %q, %v", data, err)
			}
			headers, hits := server.seen("/api/pjsk/sk/speed")
			got := harukiHeaders(headers)
			if hits != 2 || got[http.CanonicalHeaderKey(headerArtifactMode)] != "" || got[headerCacheStore] != "0" {
				t.Fatalf("hits = %d, retry headers = %v", hits, got)
			}
			if len(indexer.adopted()) != 0 {
				t.Fatal("rejected store-ref reached the indexer")
			}
			if client.StoreRefRejectedCount() != 1 {
				t.Fatalf("rejected count = %d", client.StoreRefRejectedCount())
			}
			if ops := traceOps(trace); ops["drawing.store_ref_rejected"] != 1 || ops["drawing.http"] != 2 {
				t.Fatalf("trace ops = %v", ops)
			}
		})
	}
}

func TestStoreRefMalformedBodyIsARenderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerArtifact, "1")
		w.Header().Set(headerArtifactMode, artifactModeStoreRef)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"artifact_ref"}`))
	}))
	t.Cleanup(server.Close)
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(storeRefConfig(nil, "api/pjsk/sk")))
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: &fakeRenderIndex{}})
	client.SetRenderCache(cache)
	t.Cleanup(func() { _ = cache.Close() })
	if _, err := client.WithContext(t.Context()).GenerateSKSpeedImage(&SpeedRequest{}); !errors.Is(err, errDrawingBadArtifactRef) {
		t.Fatalf("err = %v, want a bad artifact ref", err)
	}
}

func TestStoreRefOnlyForListedCacheStoreZeroPaths(t *testing.T) {
	server := newStoreRefDrawingServer(t, storeRefServe)
	indexer := &fakeStoreRefIndexer{}
	// card/box stores (Cache-Store: 1), deck is no-store but not listed.
	client := storeRefTestClient(t, server, &fakeRenderIndex{}, storeRefConfig(indexer, "api/pjsk/sk", "api/pjsk/card"))

	image, err := client.WithContext(t.Context()).GenerateDeckRecommendationImage(&DeckRequest{})
	if err != nil || image.Ref() != nil {
		t.Fatalf("deck ref = %+v, err = %v", image.Ref(), err)
	}
	if headers, _ := server.seen("/api/pjsk/deck/recommend"); harukiHeaders(headers)[http.CanonicalHeaderKey(headerArtifactMode)] != "" {
		t.Fatalf("unlisted path sent the mode: %v", harukiHeaders(headers))
	}
	image, err = client.WithContext(t.Context()).GenerateCardBoxImage(&CardBoxRequest{})
	if err != nil || image.Ref() == nil || !image.Ref().IndexWritten {
		t.Fatalf("card box ref = %+v, err = %v", image.Ref(), err)
	}
	headers, _ := server.seen("/api/pjsk/card/box")
	if got := harukiHeaders(headers); got[http.CanonicalHeaderKey(headerArtifactMode)] != "" || got[headerCacheStore] != "1" {
		t.Fatalf("storing path sent the mode: %v", got)
	}
	if len(indexer.adopted()) != 0 {
		t.Fatalf("adopted = %+v", indexer.adopted())
	}
}

func TestStoreRefOnUncachedEndpoints(t *testing.T) {
	server := newStoreRefDrawingServer(t, storeRefServe)
	indexer := &fakeStoreRefIndexer{}
	client := storeRefTestClient(t, server, &fakeRenderIndex{}, storeRefConfig(indexer, "api/pjsk/event/detail")).WithContext(t.Context())

	image, err := client.GenerateEventDetailImage(&EventDetailRequest{})
	if err != nil || image.Ref() == nil || image.Ref().CDNPath != storeRefPath {
		t.Fatalf("event detail ref = %+v, err = %v", image.Ref(), err)
	}
	// The []byte API never asks for a ref.
	if data, err := client.postUncached("/api/pjsk/event/detail", &EventDetailRequest{}); err != nil || string(data) != "bytes-jpeg" {
		t.Fatalf("[]byte API = %q, %v", data, err)
	}
	headers, hits := server.seen("/api/pjsk/event/detail")
	if hits != 2 || harukiHeaders(headers)[http.CanonicalHeaderKey(headerArtifactMode)] != "" {
		t.Fatalf("[]byte API sent %v", harukiHeaders(headers))
	}
	// An unlisted uncached endpoint keeps returning bytes.
	image, err = client.GenerateAliasListImage(&AliasListRequest{})
	if err != nil || image.Ref() != nil {
		t.Fatalf("alias list ref = %+v, err = %v", image.Ref(), err)
	}
	if len(indexer.adopted()) != 1 {
		t.Fatalf("adopted = %+v", indexer.adopted())
	}
}

func TestStoreRefDirectiveHeader(t *testing.T) {
	policy := renderCachePolicy{APIPath: "api/pjsk/sk/line", UserID: "public"}
	for _, tc := range []struct {
		store, storeRef bool
		want            string
	}{{false, true, artifactModeStoreRef}, {false, false, ""}, {true, true, ""}} {
		d := newRenderDirective(strings.Repeat("f", 64), policy, time.Minute, tc.store)
		d.StoreRef = tc.storeRef
		request := NewHarukiDrawingClient("http://drawing.invalid").client.R()
		d.apply(request)
		if got := request.Header.Get(headerArtifactMode); got != tc.want {
			t.Fatalf("store=%v storeRef=%v: mode = %q, want %q", tc.store, tc.storeRef, got, tc.want)
		}
	}
	var settings *artifactSettings
	if settings.storeRefFor("api/pjsk/sk") {
		t.Fatal("nil settings enable store-ref")
	}
	settings = newArtifactSettings(ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}, StoreRefPaths: []string{"api/pjsk/sk"}, StoreRefBucket: "image-cache"})
	if settings.storeRefFor("api/pjsk/sk/line") {
		t.Fatal("store-ref outside the allow-list")
	}
	settings = newArtifactSettings(ArtifactConfig{Endpoints: []string{"*"}, StoreRefPaths: []string{"api/pjsk/sk"}})
	if settings.storeRefFor("api/pjsk/sk/line") {
		t.Fatal("store-ref without a known image cache bucket")
	}
	var client *HarukiDrawingClient
	if client.StoreRefRejectedCount() != 0 {
		t.Fatal("nil client counts rejections")
	}
}
