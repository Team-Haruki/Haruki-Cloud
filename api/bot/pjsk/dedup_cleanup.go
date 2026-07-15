package pjsk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"
)

const (
	requestGuardCleanupQueueCapacity   = 4096
	requestGuardCleanupWorkerCount     = 4
	requestGuardCleanupAttemptTimeout  = 300 * time.Millisecond
	requestGuardCleanupFlushTimeout    = 5 * time.Second
	requestGuardCleanupMaxAttempts     = 3
	requestGuardCleanupRetryDelay      = 25 * time.Millisecond
	requestGuardCleanupFallbackTimeout = 25 * time.Millisecond
)

var (
	requestGuardLogger        = logger.NewLoggerFromGlobal("RequestGuard")
	requestGuardFailureLogger = logger.NewLoggerWithCommandWriter("RequestGuard", "WARN")
)

type requestGuardCleanupJob struct {
	lockKey string
	rateKey string
	owner   string
}

type requestGuardCleanupFunc func(context.Context, requestGuardCleanupJob) (bool, error)

// requestGuardCleanupDispatcher keeps Redis release/retry work out of the
// response path. Jobs contain only derived Redis keys and the random owner
// token; request contexts and request bodies are never retained.
type requestGuardCleanupDispatcher struct {
	complete requestGuardCleanupFunc
	jobs     chan requestGuardCleanupJob
	done     chan struct{}
	worker   context.Context
	cancel   context.CancelFunc

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
}

func newRequestGuardCleanupDispatcher(capacity, workers int, complete requestGuardCleanupFunc) *requestGuardCleanupDispatcher {
	if complete == nil {
		return nil
	}
	if capacity <= 0 {
		capacity = 1
	}
	if workers <= 0 {
		workers = 1
	}
	worker, cancel := context.WithCancel(context.Background())
	dispatcher := &requestGuardCleanupDispatcher{
		complete: complete,
		jobs:     make(chan requestGuardCleanupJob, capacity),
		done:     make(chan struct{}),
		worker:   worker,
		cancel:   cancel,
	}
	var workerGroup sync.WaitGroup
	workerGroup.Add(workers)
	for range workers {
		go func() {
			defer workerGroup.Done()
			dispatcher.runWorker()
		}()
	}
	go func() {
		workerGroup.Wait()
		close(dispatcher.done)
	}()
	return dispatcher
}

func (d *requestGuardCleanupDispatcher) Enqueue(job requestGuardCleanupJob) bool {
	if d == nil || d.complete == nil || job.lockKey == "" || job.rateKey == "" || job.owner == "" {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return false
	}
	select {
	case d.jobs <- job:
		return true
	default:
		return false
	}
}

func (d *requestGuardCleanupDispatcher) runWorker() {
	for {
		if d.worker.Err() != nil {
			return
		}
		select {
		case <-d.worker.Done():
			return
		case job, ok := <-d.jobs:
			if !ok {
				return
			}
			d.process(job)
		}
	}
}

func (d *requestGuardCleanupDispatcher) process(job requestGuardCleanupJob) {
	startedAt := time.Now()
	var lastErr error
	attempts := 0
	for attempt := 1; attempt <= requestGuardCleanupMaxAttempts; attempt++ {
		attempts = attempt
		attemptCtx, cancel := context.WithTimeout(d.worker, requestGuardCleanupAttemptTimeout)
		ownerMatched, err := d.complete(attemptCtx, job)
		cancel()
		if err == nil {
			if !ownerMatched {
				requestGuardLogger.Info("request guard cleanup completed",
					"event", "request_guard_cleanup",
					"outcome", "owner_changed",
					"attempts", attempt,
					"duration_ms", commandtrace.Milliseconds(time.Since(startedAt)),
				)
			} else if attempt > 1 {
				requestGuardLogger.Info("request guard cleanup completed",
					"event", "request_guard_cleanup",
					"outcome", "recovered",
					"attempts", attempt,
					"duration_ms", commandtrace.Milliseconds(time.Since(startedAt)),
				)
			}
			return
		}
		lastErr = err
		if attempt == requestGuardCleanupMaxAttempts || d.worker.Err() != nil {
			break
		}
		delay := time.Duration(attempt) * requestGuardCleanupRetryDelay
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-d.worker.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		if d.worker.Err() != nil {
			break
		}
	}
	requestGuardFailureLogger.Warn("request guard cleanup failed",
		"event", "request_guard_cleanup",
		"outcome", "error",
		"attempts", attempts,
		"duration_ms", commandtrace.Milliseconds(time.Since(startedAt)),
		"error_type", fmt.Sprintf("%T", lastErr),
	)
}

func (d *requestGuardCleanupDispatcher) CloseContext(ctx context.Context) error {
	if d == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	d.closeOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		close(d.jobs)
		d.mu.Unlock()
	})
	select {
	case <-d.done:
		d.cancel()
		return nil
	case <-ctx.Done():
		d.cancel()
		return ctx.Err()
	}
}

func (g *RequestGuard) Close() {
	if g == nil || g.cleanup == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestGuardCleanupFlushTimeout)
	err := g.cleanup.CloseContext(ctx)
	cancel()
	if err != nil {
		requestGuardFailureLogger.Warn("request guard cleanup flush failed",
			"event", "request_guard_cleanup_flush",
			"outcome", "error",
			"queued_jobs", len(g.cleanup.jobs),
			"error_type", fmt.Sprintf("%T", err),
		)
	}
}
