package drawing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"

	"golang.org/x/sync/singleflight"
)

func testRef(t *testing.T, node string) *ArtifactRef {
	t.Helper()
	ref, err := parseArtifactRef([]byte(testArtifactRefJSON(map[string]string{"node_name": `"` + node + `"`})))
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestImageResultRefBytesStoreHit(t *testing.T) {
	ref := testRef(t, "cn09")
	objects := storagetest.NewMemory()
	objects.Seed(map[string][]byte{ref.CDNPath: []byte("from-store")})
	result := ImageResult{ref: ref, fetcher: newArtifactFetcher(objects, nil, time.Second)}
	if result.Ref() != ref || result.FilePath() != "" {
		t.Fatalf("result = %+v", result)
	}
	data, err := result.Bytes(t.Context())
	if err != nil || string(data) != "from-store" {
		t.Fatalf("bytes = %q, %v", data, err)
	}
	data[0] = 'X'
	again, _ := result.Bytes(t.Context())
	if string(again) != "from-store" {
		t.Fatalf("caller mutation leaked: %q", again)
	}
}

func TestImageResultRefBytesFallsThroughStoreErrorToHosts(t *testing.T) {
	ref := testRef(t, "cn09")
	var hits atomic.Int32
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/"+ref.CDNPath {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("from-host"))
	}))
	defer good.Close()
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	hosts, err := urlhost.New(map[string]string{"cn09": deadURL, "cn01": good.URL}, urlhost.Options{Order: []string{"cn09", "cn01"}})
	if err != nil {
		t.Fatal(err)
	}
	for name, objects := range map[string]storage.Store{
		"store error":     &storagetest.Memory{FailGet: func(storage.Key) error { return errors.New("garage down") }},
		"store not found": storagetest.NewMemory(),
		"no store":        nil,
	} {
		t.Run(name, func(t *testing.T) {
			result := ImageResult{ref: ref, fetcher: newArtifactFetcher(objects, hosts, 2*time.Second)}
			data, err := result.Bytes(t.Context())
			if err != nil || string(data) != "from-host" {
				t.Fatalf("bytes = %q, %v", data, err)
			}
		})
	}
	var marked bool
	for _, status := range hosts.Snapshot() {
		if status.Name == "cn09" && status.Failures > 0 {
			marked = true
		}
	}
	if !marked || hits.Load() == 0 {
		t.Fatalf("dead preferred host not marked: %+v (hits %d)", hosts.Snapshot(), hits.Load())
	}
}

func TestImageResultRefBytesUnavailable(t *testing.T) {
	ref := testRef(t, "cn09")
	missing := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer missing.Close()
	hosts := urlhost.Single(missing.URL)
	cases := map[string]ImageResult{
		"nothing configured": {ref: ref, fetcher: newArtifactFetcher(nil, nil, time.Second)},
		"store miss only":    {ref: ref, fetcher: newArtifactFetcher(storagetest.NewMemory(), nil, time.Second)},
		"both miss":          {ref: ref, fetcher: newArtifactFetcher(storagetest.NewMemory(), hosts, time.Second)},
		"nil fetcher":        {ref: ref},
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := result.Bytes(t.Context()); !errors.Is(err, ErrArtifactBytesUnavailable) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	for _, status := range hosts.Snapshot() {
		if status.Failures != 0 {
			t.Fatalf("a 404 put the host into cooldown: %+v", status)
		}
	}
}

func TestArtifactFetcherLimitsAndCancellation(t *testing.T) {
	ref := testRef(t, "")
	objects := storagetest.NewMemory()
	objects.MaxObjectBytes = drawingMaxResponseBytes + 2
	objects.Seed(map[string][]byte{ref.CDNPath: make([]byte, drawingMaxResponseBytes+1)})
	oversized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, drawingMaxResponseBytes+1))
	}))
	defer oversized.Close()
	fetcher := newArtifactFetcher(objects, urlhost.Single(oversized.URL), time.Second)
	if _, err := fetcher.fetch(t.Context(), ref); !errors.Is(err, ErrArtifactBytesUnavailable) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized err = %v", err)
	}
	var nilFetcher *artifactFetcher
	if _, err := nilFetcher.fetch(t.Context(), ref); !errors.Is(err, ErrArtifactBytesUnavailable) {
		t.Fatalf("nil fetcher err = %v", err)
	}
	if _, err := fetcher.fetch(t.Context(), nil); !errors.Is(err, ErrArtifactBytesUnavailable) {
		t.Fatalf("nil ref err = %v", err)
	}

	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte("late"))
	}))
	defer slow.Close()
	defer close(release)
	slowFetcher := newArtifactFetcher(nil, urlhost.Single(slow.URL), 5*time.Second)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := slowFetcher.fetch(ctx, ref); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled fetch err = %v", err)
	}
	if _, err := (ImageResult{ref: ref, fetcher: slowFetcher}).Bytes(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled Bytes err = %v", err)
	}
}

func TestArtifactFetcherBadBaseAndHostName(t *testing.T) {
	ref := testRef(t, "")
	fetcher := newArtifactFetcher(nil, nil, time.Second)
	if _, transport, err := fetcher.fetchHost(t.Context(), "http://bad host", ref.CDNPath); err == nil || transport {
		t.Fatalf("bad base = %v transport=%v", err, transport)
	}
	if hostNameFor(nil, "http://x") != "" {
		t.Fatal("nil set resolved a host name")
	}
}

func TestWaitForImageFlightKeepsRefResults(t *testing.T) {
	ref := testRef(t, "cn09")
	result := make(chan singleflight.Result, 1)
	result <- singleflight.Result{Val: renderFlightResult{image: ImageResult{ref: ref}}}
	image, err := waitForImageFlight(t.Context(), result, nil, "remote")
	if err != nil || image.Ref() != ref {
		t.Fatalf("image = %+v, %v", image, err)
	}
}
