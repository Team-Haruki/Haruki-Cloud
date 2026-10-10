package drawing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// noStoreDrawingServer answers like Drawing: X-Haruki-Cache-Store: 0 gets
// image bytes echoing that header, a storing directive gets an artifact ref.
type noStoreDrawingServer struct {
	*httptest.Server
	mu      sync.Mutex
	headers map[string]http.Header
	hits    map[string]int
}

func newNoStoreDrawingServer(t *testing.T) *noStoreDrawingServer {
	t.Helper()
	s := &noStoreDrawingServer{headers: map[string]http.Header{}, hits: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.headers[r.URL.Path] = r.Header.Clone()
		s.hits[r.URL.Path]++
		s.mu.Unlock()
		w.Header().Set(headerNode, "cn01")
		if r.Header.Get(headerArtifact) == "1" && r.Header.Get(headerCacheStore) == "1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(testArtifactRefJSON(map[string]string{"node_name": ""})))
			return
		}
		if r.Header.Get(headerCacheStore) == "0" {
			w.Header().Set(headerCacheStore, "0")
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("inline-jpeg"))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *noStoreDrawingServer) seen(path string) (http.Header, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.headers[path], s.hits[path]
}

func newNoStoreTestClient(t *testing.T, server *noStoreDrawingServer, index *fakeRenderIndex, cfg ArtifactConfig) *HarukiDrawingClient {
	t.Helper()
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(cfg))
	cache := NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: index, Artifacts: cfg.Objects})
	client.SetRenderCache(cache)
	t.Cleanup(func() { _ = cache.Close() })
	return client.WithContext(context.Background())
}

func TestNoStorePathSkipsIndexAndReturnsBytes(t *testing.T) {
	server := newNoStoreDrawingServer(t)
	index := &fakeRenderIndex{}
	client := newNoStoreTestClient(t, server, index, ArtifactConfig{
		Endpoints:    []string{"*"},
		NoStorePaths: []string{"api/pjsk/sk", "/api/pjsk/deck/"},
	})

	for round := 1; round <= 2; round++ {
		image, err := client.GenerateSKSpeedImage(&SpeedRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if image.Ref() != nil {
			t.Fatalf("round %d: ref=%v, want bytes", round, image.Ref())
		}
		if data, err := image.Bytes(t.Context()); err != nil || string(data) != "inline-jpeg" {
			t.Fatalf("round %d: bytes = %q, %v", round, data, err)
		}
		headers, hits := server.seen("/api/pjsk/sk/speed")
		ttl := strconv.FormatInt(directiveTTLSeconds(resolveRenderCacheRule("/api/pjsk/sk/speed").TTL, false), 10)
		requireFullDirective(t, harukiHeaders(headers), "0", ttl, "6", "api/pjsk/sk/speed")
		// Nothing is retained in-process: a repeat renders again.
		if hits != round {
			t.Fatalf("round %d: drawing hits = %d", round, hits)
		}
	}
	if lookups, _, _ := index.calls(); lookups != 0 {
		t.Fatalf("no-store path consulted the render index %d times", lookups)
	}

	// The []byte API of a no-store path works unchanged.
	if data, err := client.GenerateDeckRecommendation(&DeckRequest{}); err != nil || string(data) != "inline-jpeg" {
		t.Fatalf("deck bytes = %q, %v", data, err)
	}
	if headers, _ := server.seen("/api/pjsk/deck/recommend"); harukiHeaders(headers)[headerCacheStore] != "0" {
		t.Fatalf("deck headers = %v", harukiHeaders(headers))
	}

	// Every other path keeps the storing directive, the index lookup and refs.
	image, err := client.GenerateCardBoxImage(&CardBoxRequest{})
	if err != nil || image.Ref() == nil {
		t.Fatalf("card box ref=%v err=%v", image.Ref(), err)
	}
	if headers, _ := server.seen("/api/pjsk/card/box"); harukiHeaders(headers)[headerCacheStore] != "1" {
		t.Fatalf("card box headers = %v", harukiHeaders(headers))
	}
	if lookups, _, _ := index.calls(); lookups != 1 {
		t.Fatalf("card box index lookups = %d, want 1", lookups)
	}
}

func TestNoStorePathOutsideAllowListIsUnaffected(t *testing.T) {
	server := newNoStoreDrawingServer(t)
	index := &fakeRenderIndex{}
	client := newNoStoreTestClient(t, server, index, ArtifactConfig{
		Endpoints:    []string{"api/pjsk/card/box"},
		NoStorePaths: []string{"api/pjsk/sk"},
	})
	image, err := client.GenerateSKSpeedImage(&SpeedRequest{})
	if err != nil || image.Ref() != nil {
		t.Fatalf("ref=%v err=%v", image.Ref(), err)
	}
	if headers, _ := server.seen("/api/pjsk/sk/speed"); len(harukiHeaders(headers)) != 0 {
		t.Fatalf("non allow-listed path sent %v", harukiHeaders(headers))
	}
	if lookups, _, _ := index.calls(); lookups != 1 {
		t.Fatalf("index lookups = %d, want today's single lookup", lookups)
	}
}

func TestNoStorePathWithoutRenderCacheSendsNoDirective(t *testing.T) {
	server := newNoStoreDrawingServer(t)
	client := NewHarukiDrawingClient(server.URL, WithArtifactConfig(ArtifactConfig{
		Endpoints: []string{"*"}, NoStorePaths: []string{"api/pjsk/sk"},
	})).WithContext(context.Background())
	image, err := client.GenerateSKSpeedImage(&SpeedRequest{})
	if err != nil || image.Ref() != nil {
		t.Fatalf("ref=%v err=%v", image.Ref(), err)
	}
	if headers, _ := server.seen("/api/pjsk/sk/speed"); len(harukiHeaders(headers)) != 0 {
		t.Fatalf("artifact mode is off without a render cache, sent %v", harukiHeaders(headers))
	}
}

func TestNoStorePathSharesConcurrentRenders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := newIndexClient(t, &fakeRenderIndex{})
		ctx := context.WithValue(t.Context(), artifactModeCtxKey{}, newArtifactSettings(ArtifactConfig{
			Endpoints: []string{"*"}, NoStorePaths: []string{"api/pjsk/card"},
		}))
		release := make(chan struct{})
		var renders atomic.Int32
		render := func(ctx context.Context) ([]byte, error) {
			renders.Add(1)
			if d, ok := directiveFrom(ctx); !ok || d.Store {
				t.Error("no-store render did not carry a Store=false directive")
			}
			<-release
			return []byte("shared"), nil
		}
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				image, err := client.renderRemoteImageFlight(ctx, "/api/pjsk/card/list", "k", testIndexPolicy, render)
				if err != nil || image.Ref() != nil {
					t.Errorf("image=%+v err=%v", image, err)
				}
			})
		}
		synctest.Wait()
		close(release)
		wg.Wait()
		if renders.Load() != 1 {
			t.Fatalf("renders = %d, want one shared flight", renders.Load())
		}
	})
}

func TestAPIPathPrefixListMatchesWholeSegments(t *testing.T) {
	list := newAPIPathPrefixList([]string{"api/pjsk/sk", "/api/pjsk/deck/", " ", "../x", "api/pjsk/mysekai/map"})
	for path, want := range map[string]bool{
		"api/pjsk/sk":               true,
		"api/pjsk/sk/line":          true,
		"api/pjsk/deck/recommend":   true,
		"api/pjsk/mysekai/map":      true,
		"api/pjsk/skill":            false,
		"api/pjsk/mysekai/map-x":    false,
		"api/pjsk/mysekai/resource": false,
		"api/pjsk":                  false,
		"":                          false,
	} {
		if got := list.has(path); got != want {
			t.Errorf("has(%q) = %v, want %v", path, got, want)
		}
	}
	if len(list) != 3 {
		t.Fatalf("list = %v, want the three valid prefixes", list)
	}
	var settings *artifactSettings
	if settings.skipsStore("api/pjsk/sk") {
		t.Fatal("nil settings skip the store")
	}
	settings = newArtifactSettings(ArtifactConfig{Endpoints: []string{"*"}, NoStorePaths: []string{"api/pjsk/sk"}})
	if !settings.skipsStore("api/pjsk/sk/line") || settings.skipsStore("api/pjsk/card/box") {
		t.Fatal("skipsStore mismatch")
	}
	settings = newArtifactSettings(ArtifactConfig{Endpoints: []string{"api/pjsk/card/box"}, NoStorePaths: []string{"api/pjsk/sk"}})
	if settings.skipsStore("api/pjsk/sk/line") {
		t.Fatal("no-store path outside the allow-list skips the store")
	}
}
