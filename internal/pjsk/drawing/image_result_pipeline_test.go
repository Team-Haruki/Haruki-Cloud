package drawing

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"haruki-cloud/internal/core/urlhost"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/imagecache"
)

type imagePipelineStore struct {
	storage.Store
	data      []byte
	gets      atomic.Int64
	readBytes atomic.Int64
}

func (s *imagePipelineStore) Get(ctx context.Context, _ storage.Key) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.gets.Add(1)
	s.readBytes.Add(int64(len(s.data)))
	return bytes.Clone(s.data), nil
}

type imagePipelineIndex struct {
	entry   imagecache.RenderIndexEntry
	warm    atomic.Bool
	lookups atomic.Int64
}

func (i *imagePipelineIndex) LookupRender(_ context.Context, key string) (imagecache.RenderIndexEntry, bool, error) {
	i.lookups.Add(1)
	if !i.warm.Load() {
		return imagecache.RenderIndexEntry{}, false, nil
	}
	entry := i.entry
	entry.RequestKey = key
	return entry, true, nil
}

func (*imagePipelineIndex) TouchRender(context.Context, []string) (int64, error) { return 0, nil }
func (*imagePipelineIndex) DeleteExpiredRender(context.Context, []string, time.Time) (int64, error) {
	return 0, nil
}

type imagePipelineTransport func(*http.Request) (*http.Response, error)

func (f imagePipelineTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// The fake Drawing and object store do no network IO. Cold responses contain
// a stored ref; warm responses come from the render index for identical input.
func newImagePipelineClient(tb testing.TB, size int, warm bool) (*HarukiDrawingClient, *imagePipelineStore, *imagePipelineIndex, *atomic.Int64) {
	tb.Helper()
	body := testArtifactRefJSON(map[string]string{"expires_at": "null", "size_bytes": strconv.Itoa(size)})
	ref, err := parseArtifactRef([]byte(body))
	if err != nil {
		tb.Fatal(err)
	}
	objects := &imagePipelineStore{Store: storage.Disabled(), data: bytes.Repeat([]byte{1}, size)}
	index := &imagePipelineIndex{entry: imagecache.RenderIndexEntry{
		ContentHash: ref.Hash, APIPath: "api/pjsk/mysekai/shop", UserID: "public", TTLSeconds: 3600,
		Entry: imagecache.ImageEntry{CDNPath: ref.CDNPath, StorageBackend: imagecache.BackendGarage, MediaType: "image/png", SizeBytes: int64(size)},
	}}
	index.warm.Store(warm)
	client := NewHarukiDrawingClient("http://drawing.invalid", WithArtifactConfig(ArtifactConfig{Endpoints: []string{"*"}}))
	client.SetRenderCache(NewRenderCacheClient(RenderCacheConfig{TTL: time.Hour, Index: index, Artifacts: objects}))
	tb.Cleanup(func() { _ = client.Close() })
	renders := new(atomic.Int64)
	client.client.SetTransport(imagePipelineTransport(func(req *http.Request) (*http.Response, error) {
		renders.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	}))
	return client, objects, index, renders
}

func TestMysekaiShopImageKeepsColdAndWarmReferences(t *testing.T) {
	for _, warm := range []bool{false, true} {
		name := "cold"
		if warm {
			name = "warm"
		}
		t.Run(name, func(t *testing.T) {
			client, objects, index, renders := newImagePipelineClient(t, 1<<20, warm)
			ctx, trace := commandtrace.WithTrace(t.Context())
			client = client.WithContext(ctx)
			request := &MysekaiShopRequest{Title: "shop"}
			image, err := client.GenerateMysekaiShopImage(request)
			if err != nil || image.Ref() == nil || len(image.data) != 0 {
				t.Fatalf("image = %+v, %v", image.Ref(), err)
			}
			if objects.gets.Load() != 0 || index.lookups.Load() != 1 {
				t.Fatalf("ImageResult read bytes: gets=%d index=%d", objects.gets.Load(), index.lookups.Load())
			}
			if got := pipelineOperation(trace, "drawing.artifact_fetch"); got.Count != 0 {
				t.Fatalf("reference-only result fetched an artifact: %+v", got)
			}
			data, err := client.GenerateMysekaiShop(request)
			if err != nil || !bytes.Equal(data, objects.data) || objects.gets.Load() != 1 {
				t.Fatalf("legacy bytes: len=%d gets=%d err=%v", len(data), objects.gets.Load(), err)
			}
			wantRenders, wantLookups := int64(1), int64(1)
			if warm {
				wantRenders, wantLookups = 0, 2
			}
			if renders.Load() != wantRenders || index.lookups.Load() != wantLookups {
				t.Fatalf("renders=%d index=%d", renders.Load(), index.lookups.Load())
			}
			for _, operation := range []string{"drawing.artifact_fetch", "drawing.artifact_store"} {
				if got := pipelineOperation(trace, operation); got.Count != 1 || got.Total <= 0 {
					t.Fatalf("%s = %+v", operation, got)
				}
			}
		})
	}
}

func TestCostumeImagePrepareRunsOnlyOnMiss(t *testing.T) {
	client, objects, index, renders := newImagePipelineClient(t, 32, false)
	var prepared atomic.Int32
	prepare := func(ctx context.Context, value any) error {
		if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
			t.Error("prepare did not receive the bounded render context")
		}
		value.(map[string]any)["prepared"] = true
		prepared.Add(1)
		return nil
	}
	cacheRequest := map[string]any{"variant": "one"}
	image, err := client.GenerateCostumeDetailWithContextPrepareImage(cacheRequest, &CostumeDetailRequest{}, prepare)
	if err != nil || image.Ref() == nil || objects.gets.Load() != 0 || prepared.Load() != 1 {
		t.Fatalf("cold prepare: ref=%v calls=%d err=%v", image.Ref(), prepared.Load(), err)
	}
	index.warm.Store(true)
	image, err = client.GenerateCostumeDetailWithContextPrepareImage(cacheRequest, &CostumeDetailRequest{}, prepare)
	if err != nil || image.Ref() == nil || prepared.Load() != 1 || renders.Load() != 1 {
		t.Fatalf("warm prepare: ref=%v calls=%d renders=%d err=%v", image.Ref(), prepared.Load(), renders.Load(), err)
	}
	index.warm.Store(false)
	cacheRequest = map[string]any{"variant": "uncached"}
	want := errors.New("prepare failed")
	_, err = client.GenerateCostumeDetailWithPrepareImage(cacheRequest, &CostumeDetailRequest{}, func(any) error { return want })
	if !errors.Is(err, want) || renders.Load() != 1 {
		t.Fatalf("prepare error=%v renders=%d", err, renders.Load())
	}
}

func TestImageMethodsPreserveSpecialRequestContracts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/pjsk/mysekai/fixture-detail" {
			var body []any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || len(body) != 1 {
				t.Errorf("fixture body=%v err=%v", body, err)
			}
		}
		if req.URL.Path == "/api/pjsk/sk/line" && req.URL.Query().Get("full") != "true" {
			t.Errorf("SK query = %q", req.URL.RawQuery)
		}
		_, _ = w.Write([]byte("png"))
	}))
	defer server.Close()
	client := NewHarukiDrawingClient(server.URL)
	for _, call := range []func() (ImageResult, error){
		func() (ImageResult, error) {
			return client.GenerateMysekaiFixtureDetailImage(&MysekaiFixtureDetailRequest{})
		},
		func() (ImageResult, error) { return client.GenerateSKLineImage(&SklRequest{}, true) },
	} {
		image, err := call()
		if err != nil || string(image.data) != "png" || image.Ref() != nil {
			t.Fatalf("legacy Drawing fallback: image=%+v err=%v", image, err)
		}
	}
}

func TestUncachedImageMethodsPreserveNoStoreContract(t *testing.T) {
	server := newArtifactDrawingServer(t)
	client := newArtifactTestClient(t, server, ArtifactConfig{Endpoints: []string{"*"}})
	for _, tc := range []struct {
		path string
		call func() (ImageResult, error)
	}{
		{"/api/pjsk/event/detail", func() (ImageResult, error) { return client.GenerateEventDetailImage(&EventDetailRequest{}) }},
		{"/api/pjsk/misc/alias-list", func() (ImageResult, error) { return client.GenerateAliasListImage(&AliasListRequest{}) }},
	} {
		t.Run(tc.path, func(t *testing.T) {
			for _, shape := range []drawingResponseShape{shapePNG, shapeDegraded, shapeRef} {
				server.setShape(shape)
				image, err := tc.call()
				if shape == shapeRef {
					if err == nil || !strings.Contains(err.Error(), "uncached endpoint") {
						t.Fatalf("ref must remain invalid with Cache-Store:0: %v", err)
					}
				} else if err != nil || len(image.data) == 0 || image.Ref() != nil {
					t.Fatalf("uncached fallback: %+v, %v", image, err)
				}
				if got := server.lastHeaders(tc.path).Get(headerCacheStore); got != "0" {
					t.Fatalf("Cache-Store=%q", got)
				}
			}
		})
	}
}

func pipelineOperation(trace *commandtrace.Trace, name string) commandtrace.Stats {
	for _, operation := range trace.Snapshot().Operations {
		if operation.Name == name {
			return operation
		}
	}
	return commandtrace.Stats{}
}

func TestArtifactFetchTraceIncludesFallbackAndFailure(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "fallback", false: "error"}[success], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if !success {
					http.NotFound(w, req)
					return
				}
				_, _ = w.Write([]byte("png"))
			}))
			defer server.Close()
			ctx, trace := commandtrace.WithTrace(t.Context())
			fetcher := newArtifactFetcher(storagetest.NewMemory(), urlhost.Single(server.URL), time.Second)
			_, err := fetcher.fetch(ctx, testRef(t, ""))
			if success && err != nil || !success && !errors.Is(err, ErrArtifactBytesUnavailable) {
				t.Fatalf("fetch error=%v", err)
			}
			for _, op := range []string{"drawing.artifact_fetch", "drawing.artifact_store", "drawing.artifact_store_error", "drawing.artifact_public_fallback", "drawing.artifact_public"} {
				if got := pipelineOperation(trace, op); got.Count < 1 {
					t.Fatalf("%s absent: %+v", op, trace.Snapshot())
				}
			}
			if got := pipelineOperation(trace, "drawing.artifact_fetch_error"); (got.Count == 0) != success {
				t.Fatalf("error stat=%+v success=%v", got, success)
			}
		})
	}
}

func TestArtifactFetchCanceledWaitHasIndependentTrace(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	objects := storagetest.NewMemory()
	objects.FailGet = func(storage.Key) error {
		close(entered)
		<-release
		return storage.ErrNotExist
	}
	fetcher := newArtifactFetcher(objects, nil, time.Second)
	ctx, trace := commandtrace.WithTrace(t.Context())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	ref := testRef(t, "")
	go func() {
		_, err := fetcher.fetch(ctx, ref)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	close(release)
	if got := pipelineOperation(trace, "drawing.artifact_fetch_canceled"); got.Count != 1 {
		t.Fatalf("cancel stat=%+v", got)
	}
	if got := pipelineOperation(trace, "drawing.artifact_fetch"); got.Count != 1 {
		t.Fatalf("wait stat=%+v", got)
	}
	if got := pipelineOperation(trace, "drawing.artifact_store"); got.Count != 0 {
		t.Fatalf("detached work leaked into canceled request: %+v", got)
	}
}
