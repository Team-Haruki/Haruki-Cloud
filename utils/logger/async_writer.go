package logger

import (
	"context"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// AsyncWriter keeps terminal/file I/O off request goroutines with a bounded
// queue. Constructors choose whether saturation drops after a bounded wait or
// synchronously falls back to an independent sink.
type AsyncWriter struct {
	destination io.Writer
	queue       chan []byte
	done        chan struct{}
	enqueueWait time.Duration
	reporter    io.Writer
	overflow    io.Writer
	report      chan struct{}
	overflowMu  sync.Mutex

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once

	errorMu    sync.Mutex
	firstErr   error
	dropped    atomic.Uint64
	overflowed atomic.Uint64
	writeErr   atomic.Uint64
}

func NewAsyncWriter(destination io.Writer, capacity int) *AsyncWriter {
	return newAsyncWriter(destination, capacity, 0, nil, nil)
}

// NewPriorityAsyncWriter creates a dedicated bounded sink for records that
// should not contend with ordinary application logs. When enqueueWait is
// positive, a saturated queue waits for at most that duration; zero selects a
// fully non-blocking enqueue. A drop reports cumulative health counters through
// reporter outside the destination queue. Memory use remains bounded even when
// both the destination and reporter are blocked.
func NewPriorityAsyncWriter(destination io.Writer, capacity int, enqueueWait time.Duration, reporter io.Writer) *AsyncWriter {
	if enqueueWait < 0 {
		enqueueWait = 0
	}
	if reporter == nil {
		reporter = os.Stderr
	}
	return newAsyncWriter(destination, capacity, enqueueWait, reporter, nil)
}

// NewReliableAsyncWriter keeps the bounded asynchronous fast path while
// synchronously writing a saturated record to overflow. This preserves every
// accepted record without allowing the in-memory queue to grow without bound.
// The fallback is expected to be an independent terminal or durable sink.
func NewReliableAsyncWriter(destination io.Writer, capacity int, overflow io.Writer) *AsyncWriter {
	if overflow == nil {
		overflow = os.Stderr
	}
	return newAsyncWriter(destination, capacity, 0, overflow, overflow)
}

func newAsyncWriter(destination io.Writer, capacity int, enqueueWait time.Duration, reporter, overflow io.Writer) *AsyncWriter {
	if destination == nil {
		destination = io.Discard
	}
	if capacity <= 0 {
		capacity = 1
	}
	writer := &AsyncWriter{
		destination: destination,
		queue:       make(chan []byte, capacity),
		done:        make(chan struct{}),
		enqueueWait: enqueueWait,
		reporter:    reporter,
		overflow:    overflow,
	}
	if reporter != nil {
		writer.report = make(chan struct{}, 1)
		go writer.runWithReporter()
	} else {
		go writer.run()
	}
	return writer
}

func (w *AsyncWriter) Write(payload []byte) (int, error) {
	if w == nil {
		return 0, io.ErrClosedPipe
	}
	w.mu.RLock()
	if w.closed {
		w.mu.RUnlock()
		return 0, io.ErrClosedPipe
	}
	record := append([]byte(nil), payload...)
	if w.enqueueWait > 0 {
		timer := time.NewTimer(w.enqueueWait)
		defer timer.Stop()
		select {
		case w.queue <- record:
			w.mu.RUnlock()
			return len(payload), nil
		case <-timer.C:
			w.mu.RUnlock()
			w.dropped.Add(1)
			w.signalReport()
			return len(payload), nil
		}
	}
	select {
	case w.queue <- record:
		w.mu.RUnlock()
		return len(payload), nil
	default:
		w.mu.RUnlock()
		if w.overflow != nil {
			w.overflowMu.Lock()
			written, err := w.overflow.Write(record)
			w.overflowMu.Unlock()
			if err == nil && written != len(record) {
				err = io.ErrShortWrite
			}
			if err == nil {
				w.overflowed.Add(1)
				w.signalReport()
				return len(payload), nil
			}
			w.recordWriteError(err)
		}
		w.dropped.Add(1)
		w.signalReport()
	}
	return len(payload), nil
}

func (w *AsyncWriter) Dropped() uint64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

func (w *AsyncWriter) Overflowed() uint64 {
	if w == nil {
		return 0
	}
	return w.overflowed.Load()
}

func (w *AsyncWriter) run() {
	defer close(w.done)
	w.drain()
}

func (w *AsyncWriter) runWithReporter() {
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		w.drain()
	}()

	reportedDropped := uint64(0)
	reportedOverflowed := uint64(0)
	reportedWriteErr := uint64(0)
	for {
		select {
		case <-w.report:
			w.writeHealthReport(&reportedDropped, &reportedOverflowed, &reportedWriteErr)
		case <-workerDone:
			// Do not close report: a reliable writer releases the lifecycle lock
			// before its independent overflow write, so a concurrent Close may
			// finish first. Leaving this bounded channel open avoids a send-on-
			// closed-channel panic without retaining any goroutine.
			w.writeHealthReport(&reportedDropped, &reportedOverflowed, &reportedWriteErr)
			close(w.done)
			return
		}
	}
}

func (w *AsyncWriter) drain() {
	for record := range w.queue {
		written, err := w.destination.Write(record)
		if err == nil && written != len(record) {
			err = io.ErrShortWrite
		}
		if err != nil {
			w.recordWriteError(err)
		}
	}
}

func (w *AsyncWriter) recordWriteError(err error) {
	if err == nil {
		return
	}
	w.writeErr.Add(1)
	w.errorMu.Lock()
	if w.firstErr == nil {
		w.firstErr = err
	}
	w.errorMu.Unlock()
	w.signalReport()
}

func (w *AsyncWriter) signalReport() {
	if w == nil || w.report == nil {
		return
	}
	select {
	case w.report <- struct{}{}:
	default:
	}
}

func (w *AsyncWriter) writeHealthReport(reportedDropped, reportedOverflowed, reportedWriteErr *uint64) {
	dropped := w.dropped.Load()
	overflowed := w.overflowed.Load()
	writeErr := w.writeErr.Load()
	if dropped == *reportedDropped && overflowed == *reportedOverflowed && writeErr == *reportedWriteErr {
		return
	}
	_ = WriteEmergency(w.reporter, "AsyncLogSink", "async log sink degraded",
		"event", "async_log_sink_health",
		"dropped_records", dropped,
		"overflow_records", overflowed,
		"write_errors", writeErr,
	)
	*reportedDropped = dropped
	*reportedOverflowed = overflowed
	*reportedWriteErr = writeErr

	// If counters changed while the reporter was blocked, schedule a follow-up
	// without creating another goroutine or retaining individual failures.
	if w.dropped.Load() != dropped || w.overflowed.Load() != overflowed || w.writeErr.Load() != writeErr {
		w.signalReport()
	}
}

func (w *AsyncWriter) CloseContext(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.queue)
		w.mu.Unlock()
	})
	select {
	case <-w.done:
		w.errorMu.Lock()
		err := w.firstErr
		w.errorMu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *AsyncWriter) Close() error {
	return w.CloseContext(context.Background())
}
