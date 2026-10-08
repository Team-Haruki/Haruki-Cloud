package upstreamcall

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureCallLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	logger.SetCommandWriter(buf)
	t.Cleanup(func() { logger.SetCommandWriter(nil) })
	return buf
}

func TestTimingSplitsHeadersFromBody(t *testing.T) {
	const bodyDelay = 80 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(NodeHeader, "node-a")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(bodyDelay)
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer server.Close()

	logs := captureCallLog(t)
	ctx, trace := commandtrace.WithTrace(context.Background())
	httpCtx, timing := Start(ctx)
	req, err := http.NewRequestWithContext(httpCtx, http.MethodPost, server.URL+"/api/render", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now()
	ttfb, bodyRead := timing.Split(end)
	if bodyRead < bodyDelay/2 || ttfb >= bodyDelay {
		t.Fatalf("ttfb=%v body=%v, want the %v delay in the body read", ttfb, bodyRead, bodyDelay)
	}

	Record(ctx, timing, end, Call{
		Op: "drawing.http", Target: "drawing-1", Path: "/api/render", StatusCode: resp.StatusCode,
		RequestBytes: 2, ResponseBytes: len(body), Node: resp.Header.Get(NodeHeader),
	})
	snapshot := trace.Snapshot()
	names := make(map[string]commandtrace.Stats)
	for _, op := range snapshot.Operations {
		names[op.Name] = op
	}
	if names["drawing.http.ttfb"].Count != 1 || names["drawing.http.body"].Count != 1 {
		t.Fatalf("trace operations = %+v", snapshot.Operations)
	}
	line := logs.String()
	for _, want := range []string{`msg="upstream http call"`, "op=drawing.http", "target=drawing-1", "node=node-a", "response_bytes=11", "status_code=200", "ttfb_ms=", "body_ms="} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q lacks %q", line, want)
		}
	}
	if strings.Contains(line, "drawing.http.drawing-1") {
		t.Fatal("the target must be a field, not part of the op name")
	}
}

func TestTimingWithoutResponse(t *testing.T) {
	logs := captureCallLog(t)
	var none context.Context // nil is accepted
	_, timing := Start(none)
	end := time.Now().Add(10 * time.Millisecond)
	ttfb, body := timing.Split(end)
	if body != 0 || ttfb < 10*time.Millisecond {
		t.Fatalf("no first byte: ttfb=%v body=%v", ttfb, body)
	}
	var nilTiming *Timing
	if a, b := nilTiming.Split(end); a != 0 || b != 0 {
		t.Fatal("nil timing splits to zero")
	}
	Record(none, nil, end, Call{Op: "deck.http", Target: "deck-1", Err: errors.New("dial failed")})
	if line := logs.String(); !strings.Contains(line, "error_type=") || strings.Contains(line, "node=") {
		t.Fatalf("error record = %q", line)
	}
}
