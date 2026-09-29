package drawing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
)

func requireDrawingStage(t *testing.T, trace *commandtrace.Trace, name string, count int) {
	t.Helper()
	stat := pipelineOperation(trace, name)
	if stat.Count != count || (count > 0 && stat.Total <= 0) {
		t.Fatalf("%s=%+v, want count %d with elapsed time", name, stat, count)
	}
}

func TestDrawingDecodeStageOnlyForArtifactJSON(t *testing.T) {
	for _, shape := range []drawingResponseShape{shapeRef, shapeBadJSON, shapePNG, shapeDegraded} {
		t.Run(string(shape), func(t *testing.T) {
			server := newArtifactDrawingServer(t)
			server.setShape(shape)
			client := newArtifactTestClient(t, server, ArtifactConfig{Endpoints: []string{"*"}})
			ctx, trace := commandtrace.WithTrace(t.Context())
			image, err := client.WithContext(ctx).GenerateCardBoxImage(&CardBoxRequest{})
			if shape == shapeBadJSON && !errors.Is(err, errDrawingBadArtifactRef) || shape != shapeBadJSON && err != nil {
				t.Fatalf("render error=%v", err)
			}
			if shape == shapeRef && image.Ref() == nil {
				t.Fatal("artifact response lost reference")
			}
			wantDecode := 0
			if shape == shapeRef || shape == shapeBadJSON {
				wantDecode = 1
			}
			requireDrawingStage(t, trace, "drawing.decode", wantDecode)
			requireDrawingStage(t, trace, "image.result_bytes", 0)
		})
	}
}

func TestImageResultBytesStageIncludesErrorsAndCancellation(t *testing.T) {
	ref := testRef(t, "")
	store := storagetest.NewMemory()
	store.Seed(map[string][]byte{ref.CDNPath: []byte("image")})
	fetcher := newArtifactFetcher(store, nil, time.Second)
	for _, tc := range []struct {
		name      string
		result    ImageResult
		canceled  bool
		wantErr   error
		wantFetch int
	}{
		{"bytes", ImageBytes([]byte("image")), false, nil, 0},
		{"artifact", ImageResult{ref: ref, fetcher: fetcher}, false, nil, 1},
		{"unavailable", ImageArtifact(ref), false, ErrArtifactBytesUnavailable, 1},
		{"canceled bytes", ImageBytes([]byte("image")), true, context.Canceled, 0},
		{"canceled artifact", ImageResult{ref: ref, fetcher: fetcher}, true, context.Canceled, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, trace := commandtrace.WithTrace(t.Context())
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			if tc.canceled {
				cancel()
			}
			data, err := tc.result.Bytes(ctx)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Bytes error=%v want=%v", err, tc.wantErr)
			}
			if tc.wantErr == nil && string(data) != "image" {
				t.Fatalf("Bytes data=%q", data)
			}
			requireDrawingStage(t, trace, "image.result_bytes", 1)
			requireDrawingStage(t, trace, "drawing.artifact_fetch", tc.wantFetch)
		})
	}
}

func TestImageResultBytesCanceledWaitHasIndependentTrace(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	objects := storagetest.NewMemory()
	objects.FailGet = func(storage.Key) error { once.Do(func() { close(entered) }); <-release; return storage.ErrNotExist }
	fetcher := newArtifactFetcher(objects, nil, time.Second)
	image := ImageResult{ref: testRef(t, ""), fetcher: fetcher}
	ctx, trace := commandtrace.WithTrace(t.Context())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := image.Bytes(ctx); done <- err }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Bytes error=%v", err)
	}
	requireDrawingStage(t, trace, "image.result_bytes", 1)
	requireDrawingStage(t, trace, "drawing.artifact_fetch", 1)
	requireDrawingStage(t, trace, "drawing.artifact_store", 0)
	close(release)
	_, _ = image.Bytes(t.Context())
	requireDrawingStage(t, trace, "image.result_bytes", 1)
	requireDrawingStage(t, trace, "drawing.artifact_store", 0)
}
