package drawing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	defaultDrawingSKMaxConcurrency = 8
	defaultDrawingAcquireTimeout   = 250 * time.Millisecond
)

type LimiterConfig struct {
	SKMaxConcurrency int
	SKAcquireTimeout time.Duration
	MaxConcurrency   int
}

type drawingLimiter struct {
	sk             chan struct{}
	global         chan struct{}
	acquireTimeout time.Duration
}

type drawingPermit struct {
	sk     chan struct{}
	global chan struct{}
}

func WithLimiter(cfg LimiterConfig) ClientOption {
	return func(_ *resty.Client, client *HarukiDrawingClient) {
		if client == nil {
			return
		}
		client.limiter = newDrawingLimiter(cfg)
	}
}

func newDrawingLimiter(cfg LimiterConfig) *drawingLimiter {
	timeout := cfg.SKAcquireTimeout
	if timeout <= 0 {
		timeout = defaultDrawingAcquireTimeout
	}
	skLimit := cfg.SKMaxConcurrency
	if skLimit <= 0 {
		skLimit = defaultDrawingSKMaxConcurrency
	}
	if cfg.MaxConcurrency > 0 && cfg.SKMaxConcurrency <= 0 {
		skLimit = cfg.MaxConcurrency
	}
	limiter := &drawingLimiter{acquireTimeout: timeout}
	if skLimit > 0 {
		limiter.sk = make(chan struct{}, skLimit)
	}
	if cfg.MaxConcurrency > 0 {
		limiter.global = make(chan struct{}, cfg.MaxConcurrency)
	}
	return limiter
}

func (c *HarukiDrawingClient) acquireRenderPermit(endpoint string) (drawingPermit, error) {
	if c == nil || c.limiter == nil {
		return drawingPermit{}, nil
	}
	return c.limiter.acquire(c.requestCtx, endpoint)
}

func (l *drawingLimiter) acquire(ctx context.Context, endpoint string) (drawingPermit, error) {
	if l == nil {
		return drawingPermit{}, nil
	}
	permit := drawingPermit{}
	if l.global != nil {
		if err := acquireDrawingSlot(ctx, l.global, l.acquireTimeout); err != nil {
			return drawingPermit{}, err
		}
		permit.global = l.global
	}
	if strings.HasPrefix(strings.TrimSpace(endpoint), "/api/pjsk/sk/") && l.sk != nil {
		if err := acquireDrawingSlot(ctx, l.sk, l.acquireTimeout); err != nil {
			permit.release()
			return drawingPermit{}, err
		}
		permit.sk = l.sk
	}
	return permit, nil
}

func acquireDrawingSlot(ctx context.Context, ch chan struct{}, timeout time.Duration) error {
	if ch == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		select {
		case ch <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return fmt.Errorf("drawing render concurrency limit reached")
	}
}

func (p drawingPermit) release() {
	if p.sk != nil {
		<-p.sk
	}
	if p.global != nil {
		<-p.global
	}
}
