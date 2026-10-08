// Package upstreamcall attributes one upstream HTTP call (Drawing, deck) to
// its target and splits its time into waiting for the response headers and
// reading the body.
//
// The command trace keeps one fixed pair of extra operations per call kind
// (<op>.ttfb, <op>.body), never one per target, so operation_stats does not
// grow with the node count. The per-target detail goes to one structured
// "upstream http call" record per call on the command summary sink.
package upstreamcall

import (
	"context"
	"fmt"
	"net/http/httptrace"
	"sync/atomic"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

// NodeHeader is the response header a Drawing or deck node names itself in.
const NodeHeader = "X-Haruki-Node"

var callLogger = logger.NewLoggerWithCommandWriter("Upstream", "INFO")

// Timing records when one call started and when its first response byte
// (the status line) arrived.
type Timing struct {
	start     time.Time
	firstByte atomic.Int64
}

// Start returns ctx with an httptrace hook that notes the first response
// byte. Hooks already on ctx keep running. Use the returned context for the
// request; with retries or resends the last first byte wins.
func Start(ctx context.Context) (context.Context, *Timing) {
	if ctx == nil {
		ctx = context.Background()
	}
	timing := &Timing{start: time.Now()}
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			timing.firstByte.Store(time.Now().UnixNano())
		},
	}
	return httptrace.WithClientTrace(ctx, trace), timing
}

// Split divides start..end into time to the response headers and body read.
// Without a first byte (transport error) everything counts as ttfb.
func (t *Timing) Split(end time.Time) (ttfb, body time.Duration) {
	if t == nil {
		return 0, 0
	}
	total := end.Sub(t.start)
	first := t.firstByte.Load()
	if first == 0 {
		return total, 0
	}
	ttfb = time.Unix(0, first).Sub(t.start)
	ttfb = min(max(ttfb, 0), total)
	return ttfb, total - ttfb
}

// Call describes one finished upstream HTTP call.
type Call struct {
	// Op is the trace operation that wraps the call ("drawing.http").
	Op            string
	Target        string
	Node          string
	Path          string
	StatusCode    int
	RequestBytes  int
	ResponseBytes int
	Err           error
}

// Record adds <op>.ttfb and <op>.body to the command trace and writes the
// call's attribution record.
func Record(ctx context.Context, timing *Timing, end time.Time, call Call) {
	if ctx == nil {
		ctx = context.Background()
	}
	ttfb, body := timing.Split(end)
	var total time.Duration
	if timing != nil {
		total = end.Sub(timing.start)
	}
	commandtrace.RecordOperation(ctx, call.Op+".ttfb", ttfb)
	commandtrace.RecordOperation(ctx, call.Op+".body", body)
	attrs := []any{
		"event", "upstream_http",
		"op", call.Op,
		"target", call.Target,
		"upstream_path", call.Path,
		"status_code", call.StatusCode,
		"request_bytes", call.RequestBytes,
		"response_bytes", call.ResponseBytes,
		"ttfb_ms", commandtrace.Milliseconds(ttfb),
		"body_ms", commandtrace.Milliseconds(body),
		"duration_ms", commandtrace.Milliseconds(total),
	}
	if call.Node != "" {
		attrs = append(attrs, "node", call.Node)
	}
	if call.Err != nil {
		attrs = append(attrs, "error_type", fmt.Sprintf("%T", call.Err))
	}
	callLogger.InfoContext(ctx, "upstream http call", attrs...)
}
