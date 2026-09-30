package s3

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type attemptMetricKey struct{}
type attemptMetric struct {
	op string
	ep *endpointState
}

func (c *client) record(ctx context.Context, ep *endpointState, op, event string, elapsed time.Duration, value int64) {
	ordinal := 0
	if ep != nil {
		ordinal = ep.ordinal
	}
	c.cfg.Runtime.Record(ctx, c.cfg.Slot, ordinal, op, event, elapsed, value)
}

func (c *client) sendObserved(ctx context.Context, ep *endpointState, req *http.Request) (*http.Response, error) {
	metric, _ := ctx.Value(attemptMetricKey{}).(attemptMetric)
	op := metric.op
	if op == "" {
		op = "request"
	}
	started := time.Now()
	var dnsStarted, tlsStarted atomic.Int64
	var connects sync.Map
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { dnsStarted.Store(time.Now().UnixNano()) },
		DNSDone: func(httptrace.DNSDoneInfo) {
			if start := dnsStarted.Load(); start != 0 {
				c.record(ctx, ep, op, "dns", time.Since(time.Unix(0, start)), 0)
			}
		},
		ConnectStart: func(network, addr string) { connects.Store(network+"\x00"+addr, time.Now()) },
		ConnectDone: func(network, addr string, _ error) {
			if start, ok := connects.LoadAndDelete(network + "\x00" + addr); ok {
				c.record(ctx, ep, op, "connect", time.Since(start.(time.Time)), 0)
			}
		},
		TLSHandshakeStart: func() { tlsStarted.Store(time.Now().UnixNano()) },
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			if start := tlsStarted.Load(); start != 0 {
				c.record(ctx, ep, op, "tls", time.Since(time.Unix(0, start)), 0)
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			event := "connection_new"
			if info.Reused {
				event = "connection_reused"
			}
			c.record(ctx, ep, op, event, 0, 0)
		},
		GotFirstResponseByte: func() { c.record(ctx, ep, op, "ttfb", time.Since(started), 0) },
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	if req.Body != nil && req.Body != http.NoBody {
		req.Body = &observedBody{ReadCloser: req.Body, closed: func(count int64) { c.record(ctx, ep, op, "bytes_sent", 0, count) }}
		if getBody := req.GetBody; getBody != nil {
			req.GetBody = func() (io.ReadCloser, error) {
				body, err := getBody()
				if err != nil {
					return nil, err
				}
				return &observedBody{ReadCloser: body, closed: func(count int64) { c.record(ctx, ep, op, "bytes_sent", 0, count) }}, nil
			}
		}
	}
	resp, err := c.httpClient.Do(req)
	c.record(ctx, ep, op, "headers", time.Since(started), 0)
	if err != nil {
		c.record(ctx, ep, op, "transport_error", 0, 0)
		return nil, err
	}
	status := "status_other"
	switch resp.StatusCode / 100 {
	case 2:
		status = "status_2xx"
	case 3:
		status = "status_3xx"
	case 4:
		status = "status_4xx"
	case 5:
		status = "status_5xx"
	}
	c.record(ctx, ep, op, status, 0, 0)
	bodyStarted := time.Now()
	resp.Body = &observedBody{ReadCloser: resp.Body, closed: func(count int64) {
		c.record(ctx, ep, op, "body", time.Since(bodyStarted), 0)
		c.record(ctx, ep, op, "bytes_received", 0, count)
	}}
	return resp, nil
}

type observedBody struct {
	io.ReadCloser
	bytes  atomic.Int64
	once   sync.Once
	closed func(int64)
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.bytes.Add(int64(n))
	return n, err
}

// WriteTo preserves the zero-copy fast path of byte readers while counting
// the actual payload accepted by the transport.
func (b *observedBody) WriteTo(w io.Writer) (int64, error) {
	n, err := io.Copy(w, b.ReadCloser)
	b.bytes.Add(n)
	return n, err
}

func (b *observedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.closed(b.bytes.Load()) })
	return err
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if until, err := http.ParseTime(value); err == nil && until.After(now) {
		return until.Sub(now)
	}
	return 0
}
