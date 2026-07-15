package misc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
)

func TestRenderRecordsPayloadBuildOnValidationError(t *testing.T) {
	ctx, trace := commandtrace.WithTrace(context.Background())
	controller := NewController(drawing.NewHarukiDrawingClient("http://unused.invalid")).WithContext(ctx)

	if _, err := controller.RenderCharaBirthday(drawing.CharaBirthdayRequest{}); err == nil {
		t.Fatal("RenderCharaBirthday() error = nil, want validation error")
	}

	operations := trace.Snapshot().Operations
	if len(operations) != 1 {
		t.Fatalf("operation count = %d, want 1: %#v", len(operations), operations)
	}
	if operations[0].Name != "payload.build" || operations[0].Count != 1 {
		t.Fatalf("payload build stats = %#v, want one payload.build operation", operations[0])
	}
}

func TestPayloadBuildTimerStopsBeforeDrawingHTTP(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseResponse
		_, _ = w.Write([]byte("image"))
	}))
	defer server.Close()

	ctx, trace := commandtrace.WithTrace(context.Background())
	controller := NewController(drawing.NewHarukiDrawingClient(server.URL)).WithContext(ctx)
	renderResult := make(chan error, 1)
	go func() {
		_, err := controller.RenderCharaBirthday(drawing.CharaBirthdayRequest{
			Cid:   1,
			Month: 1,
			Day:   1,
			Cards: []drawing.CharaBirthdayCard{{}},
		})
		renderResult <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		close(releaseResponse)
		t.Fatal("drawing request did not start")
	}

	operations := trace.Snapshot().Operations
	var payloadBuildCount int
	for _, operation := range operations {
		if operation.Name == "payload.build" {
			payloadBuildCount = operation.Count
			break
		}
	}
	if payloadBuildCount != 1 {
		close(releaseResponse)
		t.Fatalf("operations while drawing is blocked = %#v, want completed payload.build", operations)
	}

	close(releaseResponse)
	if err := <-renderResult; err != nil {
		t.Fatalf("RenderCharaBirthday() error = %v", err)
	}
}
