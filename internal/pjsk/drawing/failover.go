package drawing

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http/httptrace"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/observability/upstreamcall"

	"github.com/go-resty/resty/v2"
)

// drawingNodeCooldown is how long a Drawing node that could not take a
// connection is skipped by later renders (it is still used when every node is
// cooling down).
const drawingNodeCooldown = 30 * time.Second

// nodeHealth remembers Drawing nodes that recently failed at the connection
// level. It is shared by every clone of a client.
type nodeHealth struct {
	mu       sync.Mutex
	until    map[string]time.Time
	cooldown time.Duration
	now      func() time.Time
}

func newNodeHealth(cooldown time.Duration) *nodeHealth {
	return &nodeHealth{until: make(map[string]time.Time), cooldown: cooldown, now: time.Now}
}

func (h *nodeHealth) markDown(name string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.until[name] = h.now().Add(h.cooldown)
	h.mu.Unlock()
}

func (h *nodeHealth) markUp(name string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.until, name)
	h.mu.Unlock()
}

func (h *nodeHealth) healthy(name string) bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	until, ok := h.until[name]
	if !ok {
		return true
	}
	if h.now().Before(until) {
		return false
	}
	delete(h.until, name)
	return true
}

// acquireTarget leases a node that is not in exclude, preferring nodes that
// are not cooling down. A first attempt (empty exclude) falls back to any
// node, so cooldowns alone never fail a render.
func (c *HarukiDrawingClient) acquireTarget(ctx context.Context, exclude []string) (*upstream.Lease, error) {
	notExcluded := func(target upstream.TargetConfig) bool {
		return !slices.Contains(exclude, target.Name)
	}
	lease, err := c.pool.AcquireFunc(ctx, func(target upstream.TargetConfig) bool {
		return notExcluded(target) && c.health.healthy(target.Name)
	})
	if !errors.Is(err, upstream.ErrNoAvailableTargets) {
		return lease, err
	}
	return c.pool.AcquireFunc(ctx, notExcluded)
}

// connTrace records how far one HTTP attempt got, to tell whether the node
// can have received the request.
type connTrace struct {
	gotConn       atomic.Bool
	writeFailed   atomic.Bool
	gotFirstBytes atomic.Bool
}

func withConnTrace(ctx context.Context) (context.Context, *connTrace) {
	trace := &connTrace{}
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(httptrace.GotConnInfo) { trace.gotConn.Store(true) },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err != nil {
				trace.writeFailed.Store(true)
			}
		},
		GotFirstResponseByte: func() { trace.gotFirstBytes.Store(true) },
	}), trace
}

// failoverSafe reports that a failed attempt cannot have been processed by
// the node, so the same request may go to another node: no connection was
// ever obtained (dial error, connection refused, DNS or TLS handshake
// failure), or the connection broke while the request was still being
// written and nothing came back (Drawing reads the whole JSON body before
// rendering). A timeout after a connection, a fully written request and any
// HTTP response are never safe.
func failoverSafe(err error, trace *connTrace) bool {
	if err == nil || trace == nil || trace.gotFirstBytes.Load() {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if !trace.gotConn.Load() {
		return true
	}
	if !trace.writeFailed.Load() {
		return false
	}
	var netErr net.Error
	return !errors.As(err, &netErr) || !netErr.Timeout()
}

// drawingAttempt is the attempt whose response postPreparedOnce handles. Its
// lease, when set, is released by the caller after the response is used.
type drawingAttempt struct {
	lease   *upstream.Lease
	resp    *resty.Response
	elapsed time.Duration
}

// sendWithFailover posts the encoded render to one node. When that node
// fails in a way that proves it never processed the request (failoverSafe),
// the node is skipped for drawingNodeCooldown and the same request is sent
// once more to another node, while the request context is still live. Store
// and store-ref directives stay correct: the first node wrote nothing, and
// artifact keys are content hashes, so a node can only ever publish the same
// object under the same key.
func (c *HarukiDrawingClient) sendWithFailover(endpoint string, encodedBody []byte, directive *renderDirective) (drawingAttempt, error) {
	requestCtx := c.requestCtx
	pooled := c.pool != nil && c.pool.Enabled()
	var tried []string
	for {
		targetBaseURL, targetName := c.baseURL, drawingLegacyTargetName
		var lease *upstream.Lease
		if pooled {
			var err error
			finishUpstreamQueue := commandtrace.MeasureOperation(requestCtx, "drawing.upstream_queue")
			lease, err = c.acquireTarget(requestCtx, tried)
			finishUpstreamQueue()
			if err != nil {
				return drawingAttempt{}, upstreamerr.Tag(upstreamerr.ServiceRender, upstreamerr.KindUnavailable, "", fmt.Errorf("drawing upstream is unavailable: %w", err))
			}
			targetBaseURL, targetName = lease.Target.BaseURL, lease.Target.Name
		}
		if strings.TrimSpace(targetBaseURL) == "" {
			lease.Release()
			return drawingAttempt{}, upstreamerr.Tag(upstreamerr.ServiceRender, upstreamerr.KindNotConfigured, "drawing client base_url is empty", nil)
		}

		tPost := time.Now()
		finishHTTP := commandtrace.MeasureOperation(requestCtx, "drawing.http")
		httpCtx, timing := upstreamcall.Start(requestCtx)
		httpCtx, trace := withConnTrace(httpCtx)
		resp, err := c.sendPrepared(httpCtx, targetBaseURL, endpoint, encodedBody, directive)
		finishHTTP()
		elapsed := time.Since(tPost)
		recordDrawingCall(requestCtx, timing, targetName, endpoint, len(encodedBody), resp, err)
		if err == nil {
			if pooled {
				c.health.markUp(targetName)
			}
			return drawingAttempt{lease: lease, resp: resp, elapsed: elapsed}, nil
		}
		lease.Release()

		safe := pooled && failoverSafe(err, trace)
		if safe {
			c.health.markDown(targetName)
		}
		tried = append(tried, targetName)
		retry := safe && len(tried) == 1 && c.pool.Size() > 1 && contextLive(requestCtx)
		c.logger.WarnContext(requestCtx, "drawing request failed",
			"upstream", "drawing",
			"upstream_path", endpoint,
			"target", targetName,
			"duration_ms", commandtrace.Milliseconds(elapsed),
			"error_type", fmt.Sprintf("%T", err),
			"connection_failure", safe,
			"failover", retry,
		)
		if retry {
			commandtrace.RecordOperation(requestCtx, "drawing.failover", 0)
			continue
		}
		return drawingAttempt{}, upstreamerr.Transport(upstreamerr.ServiceRender, "drawing request failed: "+err.Error(), err)
	}
}

func contextLive(ctx context.Context) bool {
	return ctx == nil || ctx.Err() == nil
}
