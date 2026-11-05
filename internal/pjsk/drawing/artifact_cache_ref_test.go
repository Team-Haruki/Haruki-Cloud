package drawing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/imagecache"
)

type cacheRefMode int

const (
	cacheRefServe         cacheRefMode = iota // a cache-ref Drawing
	cacheRefIgnore                            // a Drawing that predates the mode: it indexes itself
	cacheRefDegrade                           // upload failed: degraded bytes
	cacheRefPlaceholder                       // missing assets: store0 bytes, mode not used
	cacheRefOtherBucket                       // a ref in a bucket Cloud does not serve
	cacheRefOtherKey                          // a ref for another request key
	cacheRefMissingAssets                     // a ref claiming placeholders
	cacheRefNoStore                           // a ref answered with Cache-Store: 0
)

// cacheRefDrawingServer answers Cache-Store: 1 renders like a Drawing in the
// given mode: X-Haruki-Artifact-Mode: cache-ref gets the cache-ref answer,
// anything else today's index-written ref.
type cacheRefDrawingServer struct {
	*httptest.Server
	mode    cacheRefMode
	mu      sync.Mutex
	headers []http.Header
}

func newCacheRefDrawingServer(t *testing.T, mode cacheRefMode) *cacheRefDrawingServer {
	t.Helper()
	s := &cacheRefDrawingServer{mode: mode}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.headers = append(s.headers, r.Header.Clone())
		s.mu.Unlock()
		w.Header().Set(headerNode, "render-1")
		if r.Header.Get(headerCacheStore) == "0" {
			w.Header().Set(headerCacheStore, "0")
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("bytes-jpeg"))
			return
		}
		if r.Header.Get(headerArtifactMode) != artifactModeCacheRef || s.mode == cacheRefIgnore {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(testArtifactRefJSON(nil)))
			return
		}
		switch s.mode {
		case cacheRefDegrade:
			w.Header().Set(headerArtifactDegraded, "1")
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("bytes-jpeg"))
			return
		case cacheRefPlaceholder:
			w.Header().Set(headerCacheStore, "0")
			w.Header().Set(headerRenderMissingAssets, "2")
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("placeholder-jpeg"))
			return
		case cacheRefNoStore:
			w.Header().Set(headerCacheStore, "0")
		}
		key, bucket, missing := r.Header.Get(headerCacheKey), `"image-cache"`, "0"
		switch s.mode {
		case cacheRefOtherBucket:
			bucket = `"other-bucket"`
		case cacheRefOtherKey:
			key = strings.Repeat("9", 64)
		case cacheRefMissingAssets:
			missing = "1"
		}
		w.Header().Set(headerArtifact, "1")
		w.Header().Set(headerArtifactMode, artifactModeCacheRef)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testArtifactRefJSON(map[string]string{
			"hash": `"` + storeRefHash + `"`, "cdn_path": `"` + storeRefPath + `"`, "object_key": `"` + storeRefPath + `"`,
			"media_type": `"image/jpeg"`, "size_bytes": `270000`, "index_written": "false", "node_name": `"gw-1"`,
			"bucket": bucket, "cache_key": `"` + key + `"`, "missing_assets": missing,
			"ttl_seconds": r.Header.Get(headerCacheTTL), "expires_at": `null`,
		})))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *cacheRefDrawingServer) requests() []http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]http.Header(nil), s.headers...)
}

type cacheRefCall struct {
	object imagecache.AdoptedObject
	render imagecache.AdoptedRender
}

type fakeCacheRefIndexer struct {
	mu       sync.Mutex
	calls    []cacheRefCall
	location *imagecache.AdoptedLocation
	err      error
	ctxErr   error
}

func (f *fakeCacheRefIndexer) AdoptRender(ctx context.Context, obj imagecache.AdoptedObject, render imagecache.AdoptedRender, _ time.Time) (imagecache.AdoptedLocation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, cacheRefCall{object: obj, render: render})
	f.ctxErr = ctx.Err()
	if f.err != nil {
		return imagecache.AdoptedLocation{}, f.err
	}
	if f.location != nil {
		return *f.location, nil
	}
	return imagecache.AdoptedLocation{CDNPath: obj.CDNPath, WriterNode: obj.WriterNode}, nil
}

func (f *fakeCacheRefIndexer) recorded() []cacheRefCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]cacheRefCall(nil), f.calls...)
}

func cacheRefConfig(indexer CacheRefIndexer, paths ...string) ArtifactConfig {
	return ArtifactConfig{
		Endpoints:       []string{"*"},
		NoStorePaths:    []string{"api/pjsk/sk", "api/pjsk/deck"},
		StoreRefBucket:  "image-cache",
		CacheRefPaths:   paths,
		CacheRefIndexer: indexer,
	}
}

func cacheRefTestClient(t *testing.T, server *cacheRefDrawingServer, index *fakeRenderIndex, cfg ArtifactConfig) *HarukiDrawingClient {
	t.Helper()
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(cfg))
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: index})
	client.SetRenderCache(cache)
	t.Cleanup(func() { _ = cache.Close() })
	return client
}

func TestCacheRefRecordsBothRowsAndServesThePendingRef(t *testing.T) {
	server := newCacheRefDrawingServer(t, cacheRefServe)
	index := &fakeRenderIndex{}
	indexer := &fakeCacheRefIndexer{}
	client := cacheRefTestClient(t, server, index, cacheRefConfig(indexer, "*"))

	ctx, trace := commandtrace.WithNewTrace(t.Context())
	image, err := client.WithContext(ctx).GenerateCardBoxImage(&CardBoxRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ref := image.Ref()
	if ref == nil || ref.CDNPath != storeRefPath || ref.NodeName != "gw-1" || !ref.IndexWritten {
		t.Fatalf("ref = %+v", ref)
	}
	requests := server.requests()
	if len(requests) != 1 {
		t.Fatalf("drawing requests = %d", len(requests))
	}
	sent := harukiHeaders(requests[0])
	if sent[http.CanonicalHeaderKey(headerArtifactMode)] != artifactModeCacheRef || sent[headerCacheStore] != "1" {
		t.Fatalf("headers = %v", sent)
	}
	calls := indexer.recorded()
	if len(calls) != 1 {
		t.Fatalf("indexer calls = %+v", calls)
	}
	wantObject := imagecache.AdoptedObject{Hash: storeRefHash, Group: "pjsk", CDNPath: storeRefPath, MediaType: "image/jpeg", SizeBytes: 270000, WriterNode: "gw-1"}
	ttl, _ := strconv.ParseInt(requests[0].Get(headerCacheTTL), 10, 64)
	version, _ := strconv.Atoi(requests[0].Get(headerCacheKeyVersion))
	wantRender := imagecache.AdoptedRender{
		RequestKey: requests[0].Get(headerCacheKey), APIPath: requests[0].Get(headerAPIPath), UserID: requests[0].Get(headerUserID),
		GroupName: "pjsk", KeyVersion: version, TTLSeconds: ttl,
	}
	if calls[0].object != wantObject || calls[0].render != wantRender || version != renderCacheKeyVersion || ttl <= 0 {
		t.Fatalf("recorded %+v, want %+v / %+v", calls[0], wantObject, wantRender)
	}
	ops := traceOps(trace)
	if ops["drawing.cache_ref"] != 1 || ops["drawing.http"] != 1 || ops["drawing.cache_ref_rejected"] != 0 {
		t.Fatalf("trace ops = %v", ops)
	}

	// Like an index-written ref, the cache-ref is kept briefly for the next caller.
	again, err := client.WithContext(t.Context()).GenerateCardBoxImage(&CardBoxRequest{})
	if err != nil || again.Ref() == nil || again.Ref().CDNPath != storeRefPath {
		t.Fatalf("second ref = %+v, err = %v", again.Ref(), err)
	}
	if len(server.requests()) != 1 || len(indexer.recorded()) != 1 {
		t.Fatal("the second render was not served from the pending ref")
	}
}

func TestCacheRefDuplicateServesTheRecordedRow(t *testing.T) {
	server := newCacheRefDrawingServer(t, cacheRefServe)
	existing := "pjsk/api/pjsk/card/box/" + storeRefHash + "-0123456789abcdef.jpg"
	indexer := &fakeCacheRefIndexer{location: &imagecache.AdoptedLocation{CDNPath: existing, Duplicate: true}}
	client := cacheRefTestClient(t, server, &fakeRenderIndex{}, cacheRefConfig(indexer, "*"))

	image, err := client.WithContext(t.Context()).GenerateCardBoxImage(&CardBoxRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if ref := image.Ref(); ref == nil || ref.CDNPath != existing || ref.ObjectKey != existing || ref.NodeName != "" {
		t.Fatalf("ref = %+v, want the recorded row's location", ref)
	}
}

func TestCacheRefIndexFailureStillDeliversTheRef(t *testing.T) {
	server := newCacheRefDrawingServer(t, cacheRefServe)
	indexer := &fakeCacheRefIndexer{err: errors.New("pg down")}
	client := cacheRefTestClient(t, server, &fakeRenderIndex{}, cacheRefConfig(indexer, "*"))

	ctx, trace := commandtrace.WithNewTrace(t.Context())
	image, err := client.WithContext(ctx).GenerateCardBoxImage(&CardBoxRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if ref := image.Ref(); ref == nil || ref.CDNPath != storeRefPath || ref.IndexWritten {
		t.Fatalf("ref = %+v", ref)
	}
	if ops := traceOps(trace); ops["image.render_index_error"] != 1 {
		t.Fatalf("trace ops = %v", ops)
	}
}

func TestCacheRefIndexOutlivesACancelledCaller(t *testing.T) {
	server := newCacheRefDrawingServer(t, cacheRefServe)
	indexer := &fakeCacheRefIndexer{}
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(cacheRefConfig(indexer, "*")))
	ctx, cancel := context.WithCancel(t.Context())
	d := newRenderDirective(strings.Repeat("e", 64), renderCachePolicy{APIPath: "api/pjsk/card/box", UserID: "public"}, time.Minute, true)
	d.CacheRef = true
	resp, err := client.client.R().SetHeader(headerArtifactMode, artifactModeCacheRef).SetHeader(headerCacheStore, "1").
		SetHeader(headerArtifact, "1").SetHeader(headerCacheKey, d.Key).SetHeader(headerCacheTTL, "60").
		Post(server.URL + "/api/pjsk/card/box")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := client.WithContext(ctx).successBody("/api/pjsk/card/box", d, resp); err != nil {
		t.Fatal(err)
	}
	if indexer.ctxErr != nil {
		t.Fatalf("index write saw the caller's cancellation: %v", indexer.ctxErr)
	}
	if d.outcome.Ref == nil || !d.outcome.CacheRef || d.outcome.StoreRef {
		t.Fatalf("outcome = %+v", d.outcome)
	}
}

func TestCacheRefFallbacksKeepTodaysPaths(t *testing.T) {
	for name, tc := range map[string]struct {
		mode      cacheRefMode
		wantOp    string
		wantRef   bool
		wantBytes string
	}{
		"old drawing indexes itself": {mode: cacheRefIgnore, wantOp: "drawing.cache_ref_unsupported", wantRef: true},
		"degraded upload":            {mode: cacheRefDegrade, wantOp: "drawing.cache_ref_degraded", wantBytes: "bytes-jpeg"},
		"missing assets store0":      {mode: cacheRefPlaceholder, wantOp: "drawing.placeholder_render", wantBytes: "placeholder-jpeg"},
	} {
		t.Run(name, func(t *testing.T) {
			server := newCacheRefDrawingServer(t, tc.mode)
			indexer := &fakeCacheRefIndexer{}
			client := cacheRefTestClient(t, server, &fakeRenderIndex{}, cacheRefConfig(indexer, "*"))
			ctx, trace := commandtrace.WithNewTrace(t.Context())
			image, err := client.WithContext(ctx).GenerateCardBoxImage(&CardBoxRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantRef {
				if ref := image.Ref(); ref == nil || !ref.IndexWritten || !strings.HasPrefix(ref.CDNPath, "pjsk/api/") {
					t.Fatalf("ref = %+v", ref)
				}
			} else if data, err := image.Bytes(t.Context()); image.Ref() != nil || err != nil || string(data) != tc.wantBytes {
				t.Fatalf("ref = %+v, bytes = %q, %v", image.Ref(), data, err)
			}
			if len(indexer.recorded()) != 0 {
				t.Fatal("a fallback reached the cache-ref indexer")
			}
			if ops := traceOps(trace); ops[tc.wantOp] != 1 || ops["drawing.http"] != 1 {
				t.Fatalf("trace ops = %v, want one %s", ops, tc.wantOp)
			}
		})
	}
}

func TestCacheRefPlaceholderIsNeverKept(t *testing.T) {
	server := newCacheRefDrawingServer(t, cacheRefPlaceholder)
	client := cacheRefTestClient(t, server, &fakeRenderIndex{}, cacheRefConfig(&fakeCacheRefIndexer{}, "*"))
	for range 2 {
		if _, err := client.WithContext(t.Context()).GenerateCardBoxImage(&CardBoxRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(server.requests()); got != 2 {
		t.Fatalf("drawing requests = %d, want every placeholder render to go back to Drawing", got)
	}
}

func TestCacheRefRejectedRefIsRenderedAgainWithoutTheMode(t *testing.T) {
	for name, mode := range map[string]cacheRefMode{
		"other bucket":   cacheRefOtherBucket,
		"other key":      cacheRefOtherKey,
		"missing assets": cacheRefMissingAssets,
		"cache-store 0":  cacheRefNoStore,
	} {
		t.Run(name, func(t *testing.T) {
			server := newCacheRefDrawingServer(t, mode)
			indexer := &fakeCacheRefIndexer{}
			client := cacheRefTestClient(t, server, &fakeRenderIndex{}, cacheRefConfig(indexer, "*"))
			ctx, trace := commandtrace.WithNewTrace(t.Context())
			image, err := client.WithContext(ctx).GenerateCardBoxImage(&CardBoxRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if ref := image.Ref(); ref == nil || !ref.IndexWritten || !strings.HasPrefix(ref.CDNPath, "pjsk/api/") {
				t.Fatalf("ref = %+v, want Drawing's own index-written ref", ref)
			}
			requests := server.requests()
			if len(requests) != 2 || harukiHeaders(requests[1])[http.CanonicalHeaderKey(headerArtifactMode)] != "" ||
				harukiHeaders(requests[1])[headerCacheStore] != "1" {
				t.Fatalf("requests = %d, retry headers = %v", len(requests), harukiHeaders(requests[len(requests)-1]))
			}
			if len(indexer.recorded()) != 0 {
				t.Fatal("a rejected cache-ref reached the indexer")
			}
			if ops := traceOps(trace); ops["drawing.cache_ref_rejected"] != 1 || ops["drawing.http"] != 2 {
				t.Fatalf("trace ops = %v", ops)
			}
		})
	}
}

func TestCacheRefOnlyForListedStoredPaths(t *testing.T) {
	server := newCacheRefDrawingServer(t, cacheRefServe)
	indexer := &fakeCacheRefIndexer{}
	// deck is a no-store path even though it is listed; card is listed; music is not.
	client := cacheRefTestClient(t, server, &fakeRenderIndex{}, cacheRefConfig(indexer, "api/pjsk/card", "api/pjsk/deck"))

	if _, err := client.WithContext(t.Context()).GenerateDeckRecommendationImage(&DeckRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.WithContext(t.Context()).GenerateMusicDetailImage(&MusicDetailRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.WithContext(t.Context()).GenerateCardBoxImage(&CardBoxRequest{}); err != nil {
		t.Fatal(err)
	}
	var modes []string
	for _, headers := range server.requests() {
		modes = append(modes, headers.Get(headerArtifactMode))
	}
	if strings.Join(modes, ",") != ",,"+artifactModeCacheRef {
		t.Fatalf("modes sent = %q", modes)
	}
	if len(indexer.recorded()) != 1 {
		t.Fatalf("indexer calls = %+v", indexer.recorded())
	}
}

func TestCacheRefDirectiveHeaderAndSettings(t *testing.T) {
	policy := renderCachePolicy{APIPath: "api/pjsk/card/box", UserID: "public"}
	for _, tc := range []struct {
		store, storeRef, cacheRef bool
		want                      string
	}{
		{true, false, true, artifactModeCacheRef},
		{false, false, true, ""},
		{true, false, false, ""},
		{false, true, false, artifactModeStoreRef},
	} {
		d := newRenderDirective(strings.Repeat("f", 64), policy, time.Minute, tc.store)
		d.StoreRef, d.CacheRef = tc.storeRef, tc.cacheRef
		request := NewHarukiDrawingClient("http://drawing.invalid").client.R()
		d.apply(request)
		if got := request.Header.Get(headerArtifactMode); got != tc.want {
			t.Fatalf("%+v: mode = %q, want %q", tc, got, tc.want)
		}
	}
	var settings *artifactSettings
	if settings.cacheRefFor("api/pjsk/card/box") {
		t.Fatal("nil settings enable cache-ref")
	}
	var typedNil *imagecache.PGStore
	for name, cfg := range map[string]ArtifactConfig{
		"no indexer":       {Endpoints: []string{"*"}, CacheRefPaths: []string{"*"}, StoreRefBucket: "image-cache"},
		"typed nil":        {Endpoints: []string{"*"}, CacheRefPaths: []string{"*"}, StoreRefBucket: "image-cache", CacheRefIndexer: typedNil},
		"no bucket":        {Endpoints: []string{"*"}, CacheRefPaths: []string{"*"}, CacheRefIndexer: &fakeCacheRefIndexer{}},
		"outside endpoint": {Endpoints: []string{"api/pjsk/music/detail"}, CacheRefPaths: []string{"*"}, StoreRefBucket: "image-cache", CacheRefIndexer: &fakeCacheRefIndexer{}},
		"no paths":         {Endpoints: []string{"*"}, StoreRefBucket: "image-cache", CacheRefIndexer: &fakeCacheRefIndexer{}},
	} {
		if newArtifactSettings(cfg).cacheRefFor("api/pjsk/card/box") {
			t.Fatalf("%s: cache-ref enabled", name)
		}
	}
}
