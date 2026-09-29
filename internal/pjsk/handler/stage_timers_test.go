package handler

import (
	"context"
	"errors"
	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/utils/imagecache"
	"testing"
)

func handlerStageOperationCount(snapshot commandtrace.Snapshot, name string) int {
	for _, stat := range snapshot.Operations {
		if stat.Name == name {
			return stat.Count
		}
	}
	return 0
}

func TestImageResultStageTimersDistinguishURLAndBytes(t *testing.T) {
	ctx, trace := commandtrace.WithTrace(t.Context())
	app := &renderapp.App{ImageHosts: urlhost.Single("https://ic.example")}
	result := drawing.ImageArtifact(&drawing.ArtifactRef{CDNPath: "pjsk/test.png"})
	if _, err := renderedImageMessage(ctx, result, app); err != nil {
		t.Fatal(err)
	}
	snapshot := trace.Snapshot()
	if handlerStageOperationCount(snapshot, "image.result_url") != 1 || handlerStageOperationCount(snapshot, "image.result_bytes") != 0 || handlerStageOperationCount(snapshot, "image.store") != 0 {
		t.Fatalf("URL pipeline: %+v", snapshot)
	}
	ctx, trace = commandtrace.WithTrace(t.Context())
	app.ImageCache = imagecache.New("https://ic.example", t.TempDir())
	t.Cleanup(func() { _ = app.ImageCache.Close() })
	if _, err := renderedImageMessage(ctx, drawing.ImageBytes([]byte("image")), app); err != nil {
		t.Fatal(err)
	}
	snapshot = trace.Snapshot()
	if handlerStageOperationCount(snapshot, "image.result_url") != 0 || handlerStageOperationCount(snapshot, "image.result_bytes") != 1 || handlerStageOperationCount(snapshot, "image.store") != 1 {
		t.Fatalf("byte pipeline: %+v", snapshot)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	ctx, trace = commandtrace.WithTrace(canceled)
	if _, err := renderedImageMessage(ctx, result, app); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	if handlerStageOperationCount(trace.Snapshot(), "image.result_url") != 0 {
		t.Fatal("canceled request should not build a URL")
	}
}

func TestPrepareExecutionRuntimeStageTimers(t *testing.T) {
	ctx, trace := commandtrace.WithTrace(t.Context())
	resolved := &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "123", Region: "jp", RegionExplicit: true}
	runtime, short, err := PrepareExecutionRuntime(ctx, resolved, &renderapp.App{})
	if err != nil || short != nil || runtime == nil {
		t.Fatalf("runtime=%+v short=%v err=%v", runtime, short, err)
	}
	for _, name := range []string{"runtime.ban_check", "runtime.region_resolve", "runtime.timezone_resolve"} {
		if got := handlerStageOperationCount(trace.Snapshot(), name); got != 1 {
			t.Errorf("%s count = %d", name, got)
		}
	}
	if len(trace.Snapshot().Phases) != 0 {
		t.Fatal("runtime helpers must not add nested exclusive phases")
	}
}
