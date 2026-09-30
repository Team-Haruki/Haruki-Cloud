// Package cachepersist coalesces rebuildable cache snapshots into one bounded
// background writer. It never retains the request that scheduled a write.
package cachepersist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/storage"
	"haruki-cloud/utils/logger"
)

type Options struct {
	WriteTimeout   time.Duration
	RetryInterval  time.Duration
	CoalesceWindow time.Duration
}

type Writer struct {
	mu        sync.Mutex
	requested uint64
	persisted uint64
	attempts  uint64
	lastErr   error
	closed    bool
	changed   chan struct{}
	wake      chan struct{}
	done      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	persist   func(context.Context) (uint64, error)
	options   Options
	name      string
}

func New(name string, persist func(context.Context) (uint64, error), opts Options) *Writer {
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = 10 * time.Second
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = 5 * time.Second
	}
	if opts.CoalesceWindow <= 0 {
		opts.CoalesceWindow = 50 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(storage.WithBackgroundIO(context.Background()))
	w := &Writer{ctx: ctx, cancel: cancel, persist: persist, options: opts, name: name, changed: make(chan struct{}), wake: make(chan struct{}, 1), done: make(chan struct{})}
	go w.run()
	return w
}

// Schedule only retains the greatest generation, never a snapshot or caller.
func (w *Writer) Schedule(generation uint64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.closed && generation > w.requested {
		w.requested = generation
	}
	closed := w.closed
	w.mu.Unlock()
	if !closed {
		w.signal()
	}
}

func (w *Writer) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Writer) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	target, before := w.requested, w.attempts
	w.mu.Unlock()
	w.signal()
	for {
		w.mu.Lock()
		complete := w.persisted >= target
		failed := w.attempts > before && w.lastErr != nil
		err, changed := w.lastErr, w.changed
		w.mu.Unlock()
		if complete {
			return nil
		}
		if failed {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.done:
			if err != nil {
				return err
			}
			return errors.New("cache persistence writer is closed")
		case <-changed:
		}
	}
}

// Close flushes accepted generations, then cancels the worker even on error.
// A failed shutdown flush is reported; this cache remains rebuildable upstream.
func (w *Writer) Close(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	err := w.Flush(ctx)
	w.cancel()
	select {
	case <-w.done:
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
	return err
}

func (w *Writer) run() {
	defer close(w.done)
	retry := time.NewTicker(w.options.RetryInterval)
	defer retry.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.wake:
		case <-retry.C:
		}
		timer := time.NewTimer(w.options.CoalesceWindow)
		select {
		case <-w.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		for {
			w.mu.Lock()
			pending := w.requested > w.persisted
			w.mu.Unlock()
			if !pending {
				break
			}
			ctx, cancel := context.WithTimeout(w.ctx, w.options.WriteTimeout)
			generation, err := w.persist(ctx)
			cancel()
			w.mu.Lock()
			w.attempts++
			w.lastErr = err
			if err == nil && generation > w.persisted {
				w.persisted = generation
			}
			close(w.changed)
			w.changed = make(chan struct{})
			w.mu.Unlock()
			if err != nil {
				if w.ctx.Err() == nil {
					logger.WarnContext(w.ctx, "Cache snapshot persistence failed", slog.String("cache", w.name), slog.String("error_type", fmt.Sprintf("%T", err)))
				}
				break
			}
			if w.ctx.Err() != nil {
				return
			}
		}
	}
}

// NamespacedKey isolates each stable instance's mutable object. The original
// key remains a read-only migration fallback, so old cache snapshots still load.
func NamespacedKey(key storage.Key, namespace string) (storage.Key, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || strings.ContainsAny(namespace, "/\\") || namespace == "." || namespace == ".." {
		return "", errors.New("cache persistence requires a stable non-empty instance namespace")
	}
	if _, err := storage.CleanKey(string(key)); err != nil {
		return "", err
	}
	return storage.CleanKey(path.Join("instances", namespace, string(key)))
}
