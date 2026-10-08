package drawing

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestDrawingHTTPRecordsTargetNodeAndSplitTiming(t *testing.T) {
	server := newArtifactDrawingServer(t)
	logs := &syncBuffer{}
	logger.SetCommandWriter(logs)
	t.Cleanup(func() { logger.SetCommandWriter(nil) })

	client := NewHarukiDrawingClientWithTargets("", []upstream.TargetConfig{{Name: "render-east", BaseURL: server.URL}})
	ctx, trace := commandtrace.WithTrace(context.Background())
	if _, err := client.WithContext(ctx).postPrepared("/api/pjsk/card/box", map[string]any{"id": 1}); err != nil {
		t.Fatalf("postPrepared: %v", err)
	}
	ops := make(map[string]int)
	for _, op := range trace.Snapshot().Operations {
		ops[op.Name] = op.Count
	}
	if ops["drawing.http"] != 1 || ops["drawing.http.ttfb"] != 1 || ops["drawing.http.body"] != 1 {
		t.Fatalf("operations = %v", ops)
	}
	for name := range ops {
		if strings.Contains(name, "render-east") {
			t.Fatalf("target leaked into op name %q", name)
		}
	}
	line := logs.String()
	for _, want := range []string{"op=drawing.http", "target=render-east", "node=cn01", "upstream_path=/api/pjsk/card/box", "response_bytes=9", "status_code=200"} {
		if !strings.Contains(line, want) {
			t.Fatalf("call record %q lacks %q", line, want)
		}
	}

	legacy := NewHarukiDrawingClient(server.URL)
	if _, err := legacy.WithContext(ctx).postPrepared("/api/pjsk/card/box", map[string]any{"id": 2}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "target="+drawingLegacyTargetName) {
		t.Fatal("the legacy single target should be named")
	}
}
